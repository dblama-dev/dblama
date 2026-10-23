package sqlgen

import (
	"fmt"
	"sort"
	"strings"
)

// BuildInsert builds a single-row INSERT statement with one ? placeholder
// per column. The argsPlaceholder return value is unused in v1 (the query
// already contains the placeholders) and is always empty.
func BuildInsert(table string, cols []string) (query string, argsPlaceholder string, err error) {
	if table == "" {
		return "", "", fmt.Errorf("%w: table", ErrEmptyIdentifier)
	}
	if len(cols) == 0 {
		return "", "", fmt.Errorf("%w: insert needs at least one column", ErrEmptyIdentifier)
	}
	quoted := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, c := range cols {
		if c == "" {
			return "", "", fmt.Errorf("%w: insert column", ErrEmptyIdentifier)
		}
		quoted[i] = QuoteIdent(c)
		placeholders[i] = "?"
	}

	query = "INSERT INTO " + QuoteIdent(table) +
		" (" + strings.Join(quoted, ",") + ") VALUES (" + strings.Join(placeholders, ",") + ")"
	return query, "", nil
}

// BuildUpdate builds an UPDATE statement. SET terms are ordered by column
// name and WHERE terms by key, so the output is deterministic. Only the
// where values are returned as args (in sorted-key order); the caller
// supplies the SET values. An empty where produces a statement without a
// WHERE clause.
func BuildUpdate(table string, cols []string, where map[string]any) (query string, args []any, err error) {
	if table == "" {
		return "", nil, fmt.Errorf("%w: table", ErrEmptyIdentifier)
	}
	if len(cols) == 0 {
		return "", nil, fmt.Errorf("%w: update needs at least one column", ErrEmptyIdentifier)
	}

	sorted := make([]string, len(cols))
	copy(sorted, cols)
	sort.Strings(sorted)
	sets := make([]string, len(sorted))
	for i, c := range sorted {
		if c == "" {
			return "", nil, fmt.Errorf("%w: update column", ErrEmptyIdentifier)
		}
		sets[i] = QuoteIdent(c) + "=?"
	}
	query = "UPDATE " + QuoteIdent(table) + " SET " + strings.Join(sets, ",")

	whereKeys := make([]string, 0, len(where))
	for k := range where {
		whereKeys = append(whereKeys, k)
	}
	sort.Strings(whereKeys)
	if len(whereKeys) > 0 {
		conds := make([]string, len(whereKeys))
		args = make([]any, 0, len(whereKeys))
		for i, k := range whereKeys {
			if k == "" {
				return "", nil, fmt.Errorf("%w: where column", ErrEmptyIdentifier)
			}
			conds[i] = QuoteIdent(k) + "=?"
			args = append(args, where[k])
		}
		query += " WHERE " + strings.Join(conds, " AND ")
	}

	return query, args, nil
}

// BuildDelete builds a DELETE statement. WHERE terms are ordered by key, so
// the output is deterministic. An empty where produces a statement without
// a WHERE clause.
func BuildDelete(table string, where map[string]any) (query string, args []any, err error) {
	if table == "" {
		return "", nil, fmt.Errorf("%w: table", ErrEmptyIdentifier)
	}

	query = "DELETE FROM " + QuoteIdent(table)

	whereKeys := make([]string, 0, len(where))
	for k := range where {
		whereKeys = append(whereKeys, k)
	}
	sort.Strings(whereKeys)
	if len(whereKeys) > 0 {
		conds := make([]string, len(whereKeys))
		args = make([]any, 0, len(whereKeys))
		for i, k := range whereKeys {
			if k == "" {
				return "", nil, fmt.Errorf("%w: where column", ErrEmptyIdentifier)
			}
			conds[i] = QuoteIdent(k) + "=?"
			args = append(args, where[k])
		}
		query += " WHERE " + strings.Join(conds, " AND ")
	}

	return query, args, nil
}
