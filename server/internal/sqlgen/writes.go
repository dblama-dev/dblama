package sqlgen

import (
	"fmt"
	"sort"
	"strings"
)

// BuildInsert builds a single-row MySQL INSERT statement with one ?
// placeholder per column. The argsPlaceholder return value is unused in
// v1 (the query already contains the placeholders) and is always empty.
func BuildInsert(table string, cols []string) (query string, argsPlaceholder string, err error) {
	return buildInsert(MySQLDialect, table, cols)
}

// BuildInsertPg builds the same statement as BuildInsert in the
// PostgreSQL dialect: double-quoted identifiers and $1..$N placeholders.
func BuildInsertPg(table string, cols []string) (query string, argsPlaceholder string, err error) {
	return buildInsert(PgDialect, table, cols)
}

// buildInsert renders the single-row INSERT statement of the dialect.
func buildInsert(d Dialect, table string, cols []string) (query string, argsPlaceholder string, err error) {
	if table == "" {
		return "", "", fmt.Errorf("%w: table", ErrEmptyIdentifier)
	}
	if len(cols) == 0 {
		return "", "", fmt.Errorf("%w: insert needs at least one column", ErrEmptyIdentifier)
	}
	quoted := make([]string, len(cols))
	markers := make([]string, len(cols))
	ph := &dialectPlaceholder{dialect: d}
	for i, c := range cols {
		if c == "" {
			return "", "", fmt.Errorf("%w: insert column", ErrEmptyIdentifier)
		}
		quoted[i] = d.QuoteIdent(c)
		markers[i] = ph.next()
	}

	query = "INSERT INTO " + d.QuoteIdent(table) +
		" (" + strings.Join(quoted, ",") + ") VALUES (" + strings.Join(markers, ",") + ")"
	return query, "", nil
}

// BuildUpdate builds a MySQL UPDATE statement. SET terms are ordered by
// column name and WHERE terms by key, so the output is deterministic.
// Only the where values are returned as args (in sorted-key order); the
// caller supplies the SET values. An empty where produces a statement
// without a WHERE clause. The placeholder numbering runs through SET and
// WHERE together ($1.. for SET, continuing for WHERE in the Pg dialect).
func BuildUpdate(table string, cols []string, where map[string]any) (query string, args []any, err error) {
	return buildUpdate(MySQLDialect, table, cols, where)
}

// BuildUpdatePg builds the same statement as BuildUpdate in the
// PostgreSQL dialect.
func BuildUpdatePg(table string, cols []string, where map[string]any) (query string, args []any, err error) {
	return buildUpdate(PgDialect, table, cols, where)
}

// buildUpdate renders the UPDATE statement of the dialect.
func buildUpdate(d Dialect, table string, cols []string, where map[string]any) (query string, args []any, err error) {
	if table == "" {
		return "", nil, fmt.Errorf("%w: table", ErrEmptyIdentifier)
	}
	if len(cols) == 0 {
		return "", nil, fmt.Errorf("%w: update needs at least one column", ErrEmptyIdentifier)
	}

	sorted := make([]string, len(cols))
	copy(sorted, cols)
	sort.Strings(sorted)
	ph := &dialectPlaceholder{dialect: d}
	sets := make([]string, len(sorted))
	for i, c := range sorted {
		if c == "" {
			return "", nil, fmt.Errorf("%w: update column", ErrEmptyIdentifier)
		}
		sets[i] = d.QuoteIdent(c) + "=" + ph.next()
	}
	query = "UPDATE " + d.QuoteIdent(table) + " SET " + strings.Join(sets, ",")

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
			conds[i] = d.QuoteIdent(k) + "=" + ph.next()
			args = append(args, where[k])
		}
		query += " WHERE " + strings.Join(conds, " AND ")
	}

	return query, args, nil
}

// BuildDelete builds a MySQL DELETE statement. WHERE terms are ordered by
// key, so the output is deterministic. An empty where produces a
// statement without a WHERE clause.
func BuildDelete(table string, where map[string]any) (query string, args []any, err error) {
	return buildDelete(MySQLDialect, table, where)
}

// BuildDeletePg builds the same statement as BuildDelete in the
// PostgreSQL dialect.
func BuildDeletePg(table string, where map[string]any) (query string, args []any, err error) {
	return buildDelete(PgDialect, table, where)
}

// buildDelete renders the DELETE statement of the dialect.
func buildDelete(d Dialect, table string, where map[string]any) (query string, args []any, err error) {
	if table == "" {
		return "", nil, fmt.Errorf("%w: table", ErrEmptyIdentifier)
	}

	query = "DELETE FROM " + d.QuoteIdent(table)

	whereKeys := make([]string, 0, len(where))
	for k := range where {
		whereKeys = append(whereKeys, k)
	}
	sort.Strings(whereKeys)
	if len(whereKeys) > 0 {
		ph := &dialectPlaceholder{dialect: d}
		conds := make([]string, len(whereKeys))
		args = make([]any, 0, len(whereKeys))
		for i, k := range whereKeys {
			if k == "" {
				return "", nil, fmt.Errorf("%w: where column", ErrEmptyIdentifier)
			}
			conds[i] = d.QuoteIdent(k) + "=" + ph.next()
			args = append(args, where[k])
		}
		query += " WHERE " + strings.Join(conds, " AND ")
	}

	return query, args, nil
}
