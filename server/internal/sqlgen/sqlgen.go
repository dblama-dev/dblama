// Package sqlgen builds parameterized MySQL SQL from structured queries.
//
// Every identifier is backtick-quoted by QuoteIdent (inner backticks are
// doubled), every value is passed through a `?` placeholder, and filter
// operators are whitelisted by AllowedOperators. User input must never be
// concatenated into the generated SQL text.
package sqlgen

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrEmptyIdentifier is returned by the Build* functions when a table or
// column name is empty. QuoteIdent itself does not validate emptiness;
// callers must reject empty names with this error.
var ErrEmptyIdentifier = errors.New("sqlgen: empty identifier")

// defaultLimit is used when SelectQuery.Limit is <= 0.
const defaultLimit = 50

// Filter is a single WHERE condition. Value is always bound as a parameter.
// For IN and NOT IN, Value is a comma-separated list that expands into one
// placeholder per item. For IS NULL and IS NOT NULL, Value is ignored and
// no placeholder is emitted. For every other operator a non-empty Value is
// required.
type Filter struct {
	Column   string
	Operator string
	Value    string
}

// Sort is a single ORDER BY term.
type Sort struct {
	Column string
	Desc   bool
}

// SelectQuery describes a SELECT statement. Empty Columns selects all
// columns (*). Limit <= 0 falls back to defaultLimit. Offset is passed
// through as given.
type SelectQuery struct {
	Columns []string
	Filters []Filter
	Sorts   []Sort
	Limit   int
	Offset  int
}

// allowedOperators is the whitelist of operators usable in Filter.
var allowedOperators = map[string]bool{
	"=":           true,
	"!=":          true,
	"<":           true,
	">":           true,
	"<=":          true,
	">=":          true,
	"LIKE":        true,
	"NOT LIKE":    true,
	"REGEXP":      true,
	"NOT REGEXP":  true,
	"IN":          true,
	"NOT IN":      true,
	"IS NULL":     true,
	"IS NOT NULL": true,
}

// AllowedOperators returns the whitelist of filter operators accepted by
// BuildSelect.
func AllowedOperators() []string {
	ops := make([]string, 0, len(allowedOperators))
	for op := range allowedOperators {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	return ops
}

// QuoteIdent wraps name in MySQL backticks, doubling any inner backtick so
// it cannot break out of the quoted identifier. It does not validate
// emptiness: QuoteIdent("") returns "``" and callers must reject empty
// identifiers with ErrEmptyIdentifier.
func QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// BuildSelect builds a parameterized SELECT statement. Filters are joined
// with AND, values are bound as ? placeholders (IS NULL / IS NOT NULL emit
// no placeholder), IN / NOT IN expand into one placeholder per comma-
// separated item, and LIMIT / OFFSET are always parameterized. The output
// is deterministic for a given query. The table is backtick-quoted as a
// single identifier; to address a table in another schema use
// BuildSelectQualified.
func BuildSelect(table string, q SelectQuery) (query string, args []any, err error) {
	if table == "" {
		return "", nil, fmt.Errorf("%w: table", ErrEmptyIdentifier)
	}
	return buildSelect(QuoteIdent(table), q)
}

// BuildSelectQualified builds the same statement as BuildSelect but
// addresses the table through a schema-qualified identifier: each of db
// and table is backtick-quoted independently, so the db argument selects
// the schema regardless of the connection's default database.
func BuildSelectQualified(db, table string, q SelectQuery) (query string, args []any, err error) {
	if db == "" {
		return "", nil, fmt.Errorf("%w: database", ErrEmptyIdentifier)
	}
	if table == "" {
		return "", nil, fmt.Errorf("%w: table", ErrEmptyIdentifier)
	}
	return buildSelect(QuoteIdent(db)+"."+QuoteIdent(table), q)
}

// BuildCountQualified builds a parameterized SELECT COUNT(*) over the
// filters of q against the schema-qualified table — no columns, sorts,
// limit or offset, so the count covers every matching row. Used for the
// TotalRows of a browsed page.
func BuildCountQualified(db, table string, q SelectQuery) (query string, args []any, err error) {
	if db == "" {
		return "", nil, fmt.Errorf("%w: database", ErrEmptyIdentifier)
	}
	if table == "" {
		return "", nil, fmt.Errorf("%w: table", ErrEmptyIdentifier)
	}

	args = make([]any, 0, len(q.Filters))
	sb := strings.Builder{}
	sb.WriteString("SELECT COUNT(*) FROM ")
	sb.WriteString(QuoteIdent(db))
	sb.WriteString(".")
	sb.WriteString(QuoteIdent(table))
	where, whereArgs, err := buildWhere(q.Filters)
	if err != nil {
		return "", nil, err
	}
	if where != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(where)
		args = append(args, whereArgs...)
	}
	return sb.String(), args, nil
}

// buildSelect renders the statement over an already-quoted FROM target.
func buildSelect(from string, q SelectQuery) (query string, args []any, err error) {
	for _, c := range q.Columns {
		if c == "" {
			return "", nil, fmt.Errorf("%w: select column", ErrEmptyIdentifier)
		}
	}

	var sb strings.Builder
	sb.WriteString("SELECT ")
	if len(q.Columns) == 0 {
		sb.WriteString("*")
	} else {
		quoted := make([]string, len(q.Columns))
		for i, c := range q.Columns {
			quoted[i] = QuoteIdent(c)
		}
		sb.WriteString(strings.Join(quoted, ","))
	}
	sb.WriteString(" FROM ")
	sb.WriteString(from)

	args = make([]any, 0, len(q.Filters)+2)
	where, whereArgs, ferr := buildWhere(q.Filters)
	if ferr != nil {
		return "", nil, ferr
	}
	if where != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(where)
		args = append(args, whereArgs...)
	}

	if len(q.Sorts) > 0 {
		sb.WriteString(" ORDER BY ")
		terms := make([]string, len(q.Sorts))
		for i, s := range q.Sorts {
			if s.Column == "" {
				return "", nil, fmt.Errorf("%w: order by column", ErrEmptyIdentifier)
			}
			if s.Desc {
				terms[i] = QuoteIdent(s.Column) + " DESC"
			} else {
				terms[i] = QuoteIdent(s.Column)
			}
		}
		sb.WriteString(strings.Join(terms, ", "))
	}

	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	sb.WriteString(" LIMIT ? OFFSET ?")
	args = append(args, limit, q.Offset)

	return sb.String(), args, nil
}

// buildWhere renders the WHERE body of the filters — the conditions
// joined with AND, without the "WHERE" keyword itself — and returns its
// bound arguments. Zero filters yield an empty string.
func buildWhere(filters []Filter) (string, []any, error) {
	if len(filters) == 0 {
		return "", nil, nil
	}
	parts := make([]string, len(filters))
	args := make([]any, 0, len(filters))
	for i, f := range filters {
		part, partArgs, ferr := buildFilter(f)
		if ferr != nil {
			return "", nil, ferr
		}
		parts[i] = part
		args = append(args, partArgs...)
	}
	return strings.Join(parts, " AND "), args, nil
}

// buildFilter renders one WHERE term and returns its bound arguments.
func buildFilter(f Filter) (string, []any, error) {
	if f.Column == "" {
		return "", nil, fmt.Errorf("%w: filter column", ErrEmptyIdentifier)
	}
	if !allowedOperators[f.Operator] {
		return "", nil, fmt.Errorf("sqlgen: operator %q is not allowed", f.Operator)
	}

	switch f.Operator {
	case "IS NULL", "IS NOT NULL":
		// No placeholder: the operator itself expresses the condition.
		return QuoteIdent(f.Column) + " " + f.Operator, nil, nil
	case "IN", "NOT IN":
		if f.Value == "" {
			return "", nil, fmt.Errorf("sqlgen: filter %s on %q requires a non-empty value", f.Operator, f.Column)
		}
		items := strings.Split(f.Value, ",")
		inArgs := make([]any, len(items))
		for i, item := range items {
			inArgs[i] = item
		}
		placeholders := strings.Repeat("?,", len(items))
		return QuoteIdent(f.Column) + " " + f.Operator + " (" + placeholders[:len(placeholders)-1] + ")",
			inArgs, nil
	default:
		if f.Value == "" {
			return "", nil, fmt.Errorf("sqlgen: filter %s on %q requires a non-empty value", f.Operator, f.Column)
		}
		return QuoteIdent(f.Column) + " " + f.Operator + " ?", []any{f.Value}, nil
	}
}
