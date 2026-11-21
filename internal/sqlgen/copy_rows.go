package sqlgen

import (
	"fmt"
	"sort"
	"strings"
)

// BuildCopyRows copies original rows in a single INSERT SELECT. Keys are ORed,
// so selecting duplicate non-PK records cannot multiply the inserted rows.
func BuildCopyRows(d Dialect, schema, table string, columns []string, keys []map[string]any) (string, []any, error) {
	if schema == "" || table == "" || len(columns) == 0 || len(keys) == 0 {
		return "", nil, fmt.Errorf("%w: copy requires table, columns and keys", ErrEmptyIdentifier)
	}
	target := d.QuoteIdent(schema) + "." + d.QuoteIdent(table)
	cols := make([]string, len(columns))
	for i, c := range columns {
		if c == "" {
			return "", nil, ErrEmptyIdentifier
		}
		cols[i] = d.QuoteIdent(c)
	}
	args := []any{}
	groups := make([]string, len(keys))
	for i, key := range keys {
		if len(key) == 0 {
			return "", nil, fmt.Errorf("copy refuses an empty key")
		}
		names := make([]string, 0, len(key))
		for name := range key {
			names = append(names, name)
		}
		sort.Strings(names)
		conditions := make([]string, 0, len(names))
		for _, name := range names {
			if name == "" {
				return "", nil, ErrEmptyIdentifier
			}
			term := d.QuoteIdent(name)
			if key[name] == nil {
				term += " IS NULL"
			} else {
				args = append(args, key[name])
				term += "=" + d.Placeholder(len(args))
			}
			conditions = append(conditions, term)
		}
		groups[i] = "(" + strings.Join(conditions, " AND ") + ")"
	}
	list := strings.Join(cols, ",")
	return "INSERT INTO " + target + " (" + list + ") SELECT " + list + " FROM " + target + " WHERE " + strings.Join(groups, " OR "), args, nil
}
