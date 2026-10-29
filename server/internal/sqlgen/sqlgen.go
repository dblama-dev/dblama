// Package sqlgen builds parameterized SQL from structured queries in a
// selectable dialect: MySQL (backtick-quoted identifiers, ? placeholders)
// through the classic Build* functions, PostgreSQL (double-quoted
// identifiers, $1..$N placeholders) through the Build*Pg functions. Both
// share one set of builders parameterized by a Dialect. Filter operators
// are whitelisted by AllowedOperators. User input must never be
// concatenated into the generated SQL text.
package sqlgen

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
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
	"CONTAINS":    true,
	"NOT LIKE":    true,
	"REGEXP":      true,
	"NOT REGEXP":  true,
	"IN":          true,
	"NOT IN":      true,
	"IS NULL":     true,
	"IS NOT NULL": true,
}

// AllowedOperators returns the whitelist of filter operators accepted by
// the Build* select functions.
func AllowedOperators() []string {
	ops := make([]string, 0, len(allowedOperators))
	for op := range allowedOperators {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	return ops
}

// Dialect selects the identifier quoting and the placeholder style of the
// generated statements. QuoteIdent wraps one identifier so it cannot
// break out of its quotes; Placeholder renders the marker of the n-th
// bound parameter of a statement (1-based, counted across the whole
// statement in the order the placeholders appear in its text).
type Dialect struct {
	QuoteIdent  func(string) string
	Placeholder func(n int) string
}

// MySQLDialect quotes identifiers in backticks (inner backticks doubled)
// and renders every placeholder as "?".
var MySQLDialect = Dialect{
	QuoteIdent:  QuoteIdent,
	Placeholder: func(int) string { return "?" },
}

// PgDialect quotes identifiers in double quotes (inner double quotes
// doubled, case preserved) and renders the n-th placeholder as "$n".
var PgDialect = Dialect{
	QuoteIdent:  QuoteIdentPg,
	Placeholder: func(n int) string { return "$" + strconv.Itoa(n) },
}

// dialectPlaceholder hands out the dialect's placeholder markers in
// statement order — the ordinal runs across the whole statement, so the
// PostgreSQL $1..$N numbering stays correct even when SET and WHERE
// clauses share it.
type dialectPlaceholder struct {
	dialect Dialect
	n       int
}

// next returns the next placeholder marker.
func (p *dialectPlaceholder) next() string {
	p.n++
	return p.dialect.Placeholder(p.n)
}

// QuoteIdent wraps name in MySQL backticks, doubling any inner backtick so
// it cannot break out of the quoted identifier. It does not validate
// emptiness: QuoteIdent("") returns two backticks; callers must reject empty
// identifiers with ErrEmptyIdentifier.
func QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// QuoteIdentPg wraps name in PostgreSQL double quotes, doubling any inner
// double quote so it cannot break out of the quoted identifier. Case is
// preserved: "MixedCase" stays mixed-case where an unquoted identifier
// would fold to lowercase. It does not validate emptiness: QuoteIdentPg("")
// returns `""` and callers must reject empty identifiers with
// ErrEmptyIdentifier.
func QuoteIdentPg(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// BuildSelect builds a parameterized MySQL SELECT statement. Filters are
// joined with AND, values are bound as ? placeholders (IS NULL / IS NOT
// NULL emit no placeholder), IN / NOT IN expand into one placeholder per
// comma-separated item, and LIMIT / OFFSET are always parameterized. The
// output is deterministic for a given query. The table is backtick-quoted
// as a single identifier; to address a table in another schema use
// BuildSelectQualified.
func BuildSelect(table string, q SelectQuery) (query string, args []any, err error) {
	if table == "" {
		return "", nil, fmt.Errorf("%w: table", ErrEmptyIdentifier)
	}
	return buildSelect(MySQLDialect, QuoteIdent(table), q)
}

// BuildSelectPg builds the same statement as BuildSelect in the
// PostgreSQL dialect: double-quoted identifiers and $1..$N placeholders.
func BuildSelectPg(table string, q SelectQuery) (query string, args []any, err error) {
	if table == "" {
		return "", nil, fmt.Errorf("%w: table", ErrEmptyIdentifier)
	}
	return buildSelect(PgDialect, QuoteIdentPg(table), q)
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
	return buildSelect(MySQLDialect, QuoteIdent(db)+"."+QuoteIdent(table), q)
}

// BuildSelectQualifiedPg builds the same statement as
// BuildSelectQualified in the PostgreSQL dialect: each of schema and
// table is double-quoted independently.
func BuildSelectQualifiedPg(schema, table string, q SelectQuery) (query string, args []any, err error) {
	if schema == "" {
		return "", nil, fmt.Errorf("%w: database", ErrEmptyIdentifier)
	}
	if table == "" {
		return "", nil, fmt.Errorf("%w: table", ErrEmptyIdentifier)
	}
	return buildSelect(PgDialect, QuoteIdentPg(schema)+"."+QuoteIdentPg(table), q)
}

// BuildCountQualified builds a parameterized MySQL SELECT COUNT(*) over
// the filters of q against the schema-qualified table — no columns,
// sorts, limit or offset, so the count covers every matching row. Used
// for the TotalRows of a browsed page.
func BuildCountQualified(db, table string, q SelectQuery) (query string, args []any, err error) {
	return buildCountQualified(MySQLDialect, db, table, q)
}

// BuildCountQualifiedPg builds the same count statement as
// BuildCountQualified in the PostgreSQL dialect.
func BuildCountQualifiedPg(schema, table string, q SelectQuery) (query string, args []any, err error) {
	return buildCountQualified(PgDialect, schema, table, q)
}

// buildCountQualified renders the count statement of the dialect over the
// schema-qualified table.
func buildCountQualified(d Dialect, db, table string, q SelectQuery) (query string, args []any, err error) {
	if db == "" {
		return "", nil, fmt.Errorf("%w: database", ErrEmptyIdentifier)
	}
	if table == "" {
		return "", nil, fmt.Errorf("%w: table", ErrEmptyIdentifier)
	}

	ph := &dialectPlaceholder{dialect: d}
	args = make([]any, 0, len(q.Filters))
	sb := strings.Builder{}
	sb.WriteString("SELECT COUNT(*) FROM ")
	sb.WriteString(d.QuoteIdent(db))
	sb.WriteString(".")
	sb.WriteString(d.QuoteIdent(table))
	where, whereArgs, err := buildWhere(d, ph, q.Filters)
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

// buildSelect renders the statement of the dialect over an already-quoted
// FROM target.
func buildSelect(d Dialect, from string, q SelectQuery) (query string, args []any, err error) {
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
			quoted[i] = d.QuoteIdent(c)
		}
		sb.WriteString(strings.Join(quoted, ","))
	}
	sb.WriteString(" FROM ")
	sb.WriteString(from)

	ph := &dialectPlaceholder{dialect: d}
	args = make([]any, 0, len(q.Filters)+2)
	where, whereArgs, ferr := buildWhere(d, ph, q.Filters)
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
				terms[i] = d.QuoteIdent(s.Column) + " DESC"
			} else {
				terms[i] = d.QuoteIdent(s.Column)
			}
		}
		sb.WriteString(strings.Join(terms, ", "))
	}

	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	sb.WriteString(" LIMIT ")
	sb.WriteString(ph.next())
	sb.WriteString(" OFFSET ")
	sb.WriteString(ph.next())
	args = append(args, limit, q.Offset)

	return sb.String(), args, nil
}

// buildWhere renders the WHERE body of the filters — the conditions
// joined with AND, without the "WHERE" keyword itself — and returns its
// bound arguments. Zero filters yield an empty string.
func buildWhere(d Dialect, ph *dialectPlaceholder, filters []Filter) (string, []any, error) {
	if len(filters) == 0 {
		return "", nil, nil
	}
	parts := make([]string, len(filters))
	args := make([]any, 0, len(filters))
	for i, f := range filters {
		part, partArgs, ferr := buildFilter(d, ph, f)
		if ferr != nil {
			return "", nil, ferr
		}
		parts[i] = part
		args = append(args, partArgs...)
	}
	return strings.Join(parts, " AND "), args, nil
}

// buildFilter renders one WHERE term and returns its bound arguments.
func buildFilter(d Dialect, ph *dialectPlaceholder, f Filter) (string, []any, error) {
	if f.Column == "" {
		return "", nil, fmt.Errorf("%w: filter column", ErrEmptyIdentifier)
	}
	if !allowedOperators[f.Operator] {
		return "", nil, fmt.Errorf("sqlgen: operator %q is not allowed", f.Operator)
	}

	switch f.Operator {
	case "CONTAINS":
		if f.Value == "" {
			return "", nil, fmt.Errorf("sqlgen: filter %s on %q requires a non-empty value", f.Operator, f.Column)
		}
		// An explicit non-backslash escape works in every MySQL SQL mode
		// and PostgreSQL. Only the pattern is transformed; it stays bound.
		value := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(f.Value)
		return d.QuoteIdent(f.Column) + " LIKE " + ph.next() + " ESCAPE '!'", []any{"%" + value + "%"}, nil
	case "IS NULL", "IS NOT NULL":
		// No placeholder: the operator itself expresses the condition.
		return d.QuoteIdent(f.Column) + " " + f.Operator, nil, nil
	case "IN", "NOT IN":
		if f.Value == "" {
			return "", nil, fmt.Errorf("sqlgen: filter %s on %q requires a non-empty value", f.Operator, f.Column)
		}
		items := strings.Split(f.Value, ",")
		inArgs := make([]any, len(items))
		markers := make([]string, len(items))
		for i, item := range items {
			inArgs[i] = item
			markers[i] = ph.next()
		}
		return d.QuoteIdent(f.Column) + " " + f.Operator + " (" + strings.Join(markers, ",") + ")",
			inArgs, nil
	default:
		if f.Value == "" {
			return "", nil, fmt.Errorf("sqlgen: filter %s on %q requires a non-empty value", f.Operator, f.Column)
		}
		return d.QuoteIdent(f.Column) + " " + f.Operator + " " + ph.next(), []any{f.Value}, nil
	}
}
