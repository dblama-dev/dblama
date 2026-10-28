package dump

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// View holds a portable SELECT body and the local relations it depends on.
type View struct {
	Name, Query     string
	Dependencies    []string
	SecurityInvoker bool
	SecurityBarrier bool
	CheckOption     string
}

// OrderViews places every selected view after the views it references.
func OrderViews(views []View) ([]View, error) {
	byName := map[string]View{}
	for _, v := range views {
		byName[v.Name] = v
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	slices.Sort(names)
	state := map[string]int{}
	ordered := []View{}
	var visit func(string) error
	visit = func(name string) error {
		if state[name] == 2 {
			return nil
		}
		if state[name] == 1 {
			return fmt.Errorf("cyclic view dependency at %s", name)
		}
		state[name] = 1
		v := byName[name]
		for _, dep := range v.Dependencies {
			if _, ok := byName[dep]; ok {
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		state[name] = 2
		ordered = append(ordered, v)
		return nil
	}
	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

// ResolveViewSelection expands selected views to the tables/views needed by a
// fresh restore. The returned base tables precede dependency-ordered views.
func ResolveViewSelection(catalog map[string]bool, selected []string, views []View) ([]string, []View, error) {
	byName := map[string]View{}
	for _, v := range views {
		byName[v.Name] = v
	}
	return PrepareViews(catalog, selected, func(name string) (View, error) {
		v, ok := byName[name]
		if !ok {
			return View{}, fmt.Errorf("view definition %q unavailable", name)
		}
		return v, nil
	})
}

// PrepareViews reads only the views within the selected dependency closure.
func PrepareViews(catalog map[string]bool, selected []string, load func(string) (View, error)) ([]string, []View, error) {
	if len(selected) == 0 {
		for name := range catalog {
			selected = append(selected, name)
		}
	}
	tables := map[string]bool{}
	chosen := map[string]View{}
	visiting := map[string]bool{}
	var add func(string) error
	add = func(name string) error {
		isView, exists := catalog[name]
		if !exists {
			return fmt.Errorf("export object %q not found", name)
		}
		if !isView {
			tables[name] = true
			return nil
		}
		if _, ok := chosen[name]; ok {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("cyclic view dependency at %s", name)
		}
		visiting[name] = true
		v, err := load(name)
		if err != nil {
			return err
		}
		for _, dep := range v.Dependencies {
			if err := add(dep); err != nil {
				return err
			}
		}
		visiting[name] = false
		chosen[name] = v
		return nil
	}
	for _, name := range selected {
		if err := add(name); err != nil {
			return nil, nil, err
		}
	}
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	slices.Sort(names)
	chosenViews := make([]View, 0, len(chosen))
	for _, v := range chosen {
		chosenViews = append(chosenViews, v)
	}
	ordered, err := OrderViews(chosenViews)
	return names, ordered, err
}

type viewToken struct {
	start, end int
	text       string
	ident      bool
}

// RewriteViewQuery removes only relation qualifiers belonging to the source.
// SQL strings, comments, dollar strings and alias-qualified columns retain their
// bytes. Qualified external relations and local/external routines fail closed.
func RewriteViewQuery(query, source string, postgres bool) (string, []string, error) {
	tokens, err := viewTokens(query, postgres)
	if err != nil {
		return "", nil, err
	}
	type parseContext struct{ from, expect, selectQuery bool }
	contexts := []parseContext{{}}
	remove := map[int]int{}
	dependencies := map[string]bool{}
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		current := &contexts[len(contexts)-1]
		if tok.text == "(" {
			current.expect = false
			contexts = append(contexts, parseContext{})
			continue
		}
		if tok.text == ")" {
			if len(contexts) > 1 {
				contexts = contexts[:len(contexts)-1]
			}
			continue
		}
		upper := strings.ToUpper(tok.text)
		keyword := tok.ident && query[tok.start] != '`' && query[tok.start] != '"'
		if tok.ident && current.expect && (!keyword || upper != "LATERAL" && upper != "ONLY") {
			current.expect = false
			name := tok.text
			if i+2 < len(tokens) && tokens[i+1].text == "." && tokens[i+2].ident {
				if name != source {
					return "", nil, fmt.Errorf("view has unsupported external schema dependency %q", name)
				}
				name = tokens[i+2].text
				remove[tok.start] = tokens[i+2].start
				i += 2
			}
			if i+1 < len(tokens) && tokens[i+1].text == "(" {
				return "", nil, fmt.Errorf("view table-function dependency %q is not supported", name)
			}
			dependencies[name] = true
			continue
		}
		if keyword {
			switch upper {
			case "SELECT":
				current.selectQuery = true
			case "FROM":
				if current.selectQuery {
					current.from = true
					current.expect = true
				}
			case "JOIN":
				if current.selectQuery {
					current.expect = true
				}
			case "WHERE", "GROUP", "ORDER", "HAVING", "LIMIT", "OFFSET", "UNION", "EXCEPT", "INTERSECT", "WINDOW":
				current.from = false
				current.expect = false
			}
		} else if tok.text == "," && current.from {
			current.expect = true
		}
	}
	for i := 0; i+2 < len(tokens); i++ {
		if !tokens[i].ident || tokens[i+1].text != "." || !tokens[i+2].ident {
			continue
		}
		if i+3 < len(tokens) && tokens[i+3].text == "(" && tokens[i].text != "pg_catalog" {
			return "", nil, fmt.Errorf("view routine dependency %s.%s is not included in the dump", tokens[i].text, tokens[i+2].text)
		}
		// MySQL canonical SELECT expressions use database.table.column, while
		// alias.column is only two parts and must not be changed.
		if tokens[i].text == source && dependencies[tokens[i+2].text] && i+4 < len(tokens) && tokens[i+3].text == "." && tokens[i+4].ident {
			remove[tokens[i].start] = tokens[i+2].start
		}
	}
	var out strings.Builder
	for i := 0; i < len(query); {
		if end, ok := remove[i]; ok {
			i = end
			continue
		}
		out.WriteByte(query[i])
		i++
	}
	deps := make([]string, 0, len(dependencies))
	for name := range dependencies {
		deps = append(deps, name)
	}
	slices.Sort(deps)
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(out.String()), ";")), deps, nil
}

func viewTokens(query string, postgres bool) ([]viewToken, error) {
	tokens := []viewToken{}
	for i := 0; i < len(query); {
		start := i
		c := query[i]
		if unicode.IsSpace(rune(c)) {
			i++
			continue
		}
		if c == '-' && i+1 < len(query) && query[i+1] == '-' || (!postgres && c == '#') {
			for i < len(query) && query[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(query) && query[i+1] == '*' {
			end := strings.Index(query[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("unterminated SQL comment")
			}
			i += end + 4
			continue
		}
		if postgres && c == '$' {
			end := i + 1
			for end < len(query) && (query[end] == '_' || query[end] >= 'a' && query[end] <= 'z' || query[end] >= 'A' && query[end] <= 'Z' || query[end] >= '0' && query[end] <= '9') {
				end++
			}
			if end < len(query) && query[end] == '$' {
				tag := query[i : end+1]
				closing := strings.Index(query[end+1:], tag)
				if closing < 0 {
					return nil, fmt.Errorf("unterminated dollar string")
				}
				i = end + 1 + closing + len(tag)
				continue
			}
		}
		if c == '\'' || (!postgres && c == '"') {
			escape := !postgres || (start > 0 && (query[start-1] == 'e' || query[start-1] == 'E'))
			i++
			closed := false
			for i < len(query) {
				if escape && query[i] == '\\' {
					i += 2
					continue
				}
				if query[i] == c {
					if i+1 < len(query) && query[i+1] == c {
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated SQL string")
			}
			continue
		}
		if c == '`' || (postgres && c == '"') {
			i++
			var value strings.Builder
			closed := false
			for i < len(query) {
				if query[i] == c {
					if i+1 < len(query) && query[i+1] == c {
						value.WriteByte(c)
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				value.WriteByte(query[i])
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated SQL identifier")
			}
			tokens = append(tokens, viewToken{start, i, value.String(), true})
			continue
		}
		if c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 128 {
			i++
			for i < len(query) && (query[i] == '_' || query[i] == '$' || query[i] >= 'a' && query[i] <= 'z' || query[i] >= 'A' && query[i] <= 'Z' || query[i] >= '0' && query[i] <= '9' || query[i] >= 128) {
				i++
			}
			tokens = append(tokens, viewToken{start, i, query[start:i], true})
			continue
		}
		i++
		tokens = append(tokens, viewToken{start, i, query[start:i], false})
	}
	return tokens, nil
}

// WritablePage removes computed columns from SQL INSERT projections. Filters,
// sorting and paging have already been applied to the original result.
func WritablePage(page driver.Page, generated map[string]bool) (driver.Page, error) {
	if len(generated) == 0 {
		return page, nil
	}
	indices := []int{}
	columns := []driver.ColumnMeta{}
	for i, c := range page.Columns {
		if !generated[c.Name] {
			indices = append(indices, i)
			columns = append(columns, c)
		}
	}
	if len(indices) == 0 {
		return driver.Page{}, fmt.Errorf("SQL export selection has no writable columns")
	}
	result := driver.Page{Columns: columns, Rows: make([][]any, len(page.Rows)), TotalRows: page.TotalRows}
	for i, row := range page.Rows {
		if len(row) != len(page.Columns) {
			return result, fmt.Errorf("export row length does not match columns")
		}
		values := make([]any, 0, len(indices))
		for _, idx := range indices {
			values = append(values, row[idx])
		}
		result.Rows[i] = values
	}
	return result, nil
}
