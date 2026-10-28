package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// The structure-reading queries take the current schema as $1 (SetSchema
// swaps it) and the table name as $2 — parameters do not case-fold, so
// "MixedCase" names bind exactly.

// defTableQuery reads the table comment and pins existence: relkind r
// (ordinary table), p (partitioned) or v (view) — no row means the table
// does not exist.
const defTableQuery = `
SELECT COALESCE(obj_description(c.oid, 'pg_class'), '')
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind IN ('r', 'p', 'v')`

// defColumnsQuery reads the columns of one table in ordinal order.
// data_type is the SQL-standard name ("character varying", "timestamp
// with time zone", "ARRAY", "USER-DEFINED"), udt_name the PostgreSQL
// type name ("varchar", "timestamptz", "_text", an enum's own name).
// column_default holds the raw default — nextval(...) for serials,
// gen_random_uuid(), now(), 'x'::character varying for string literals.
// A generated column has is_generated = 'ALWAYS' with its expression in
// generation_expression and no column_default.
const defColumnsQuery = `
SELECT column_name, data_type, udt_name,
       character_maximum_length, numeric_precision, numeric_scale,
       is_nullable, column_default, collation_name,
       is_identity, is_generated, generation_expression,
       COALESCE(col_description(format('%I.%I', table_schema, table_name)::regclass::oid, ordinal_position), '')
FROM information_schema.columns
WHERE table_schema = $1 AND table_name = $2
ORDER BY ordinal_position`

// defIndexesQuery reads the indexes of one table. Every index appears —
// the PRIMARY ones auto-created by constraints included (indisprimary
// distinguishes them). pg_get_indexdef renders the server's own CREATE
// INDEX text, whose column list the driver parses (see
// indexColumnsFromDef).
const defIndexesQuery = `
SELECT i.relname, ix.indisunique, ix.indisprimary, pg_get_indexdef(ix.indexrelid), COALESCE(con.conname, '')
FROM pg_index ix
JOIN pg_class t ON t.oid = ix.indrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
JOIN pg_class i ON i.oid = ix.indexrelid
LEFT JOIN pg_constraint con ON con.conindid = ix.indexrelid AND con.contype IN ('p','u')
WHERE n.nspname = $1 AND t.relname = $2
ORDER BY i.relname`

// defForeignKeysQuery reads the foreign keys of one table: conkey lists
// the local column attnums and confkey the referenced ones in key order,
// so unnest WITH ORDINALITY pairs them positionally — information_schema
// alone cannot (constraint_column_usage exposes no ordering). confupdtype
// / confdeltype are the one-letter referential actions, cast to text for
// scanning.
const defForeignKeysQuery = `
SELECT con.conname, loc.attname, ref.attname,
       refcl.relname, refns.nspname,
       con.confupdtype::text, con.confdeltype::text
FROM pg_constraint con
JOIN pg_class t ON t.oid = con.conrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
JOIN LATERAL unnest(con.conkey) WITH ORDINALITY AS lk(attnum, ord) ON true
JOIN LATERAL unnest(con.confkey) WITH ORDINALITY AS rk(attnum, ord) ON rk.ord = lk.ord
JOIN pg_attribute loc ON loc.attrelid = t.oid AND loc.attnum = lk.attnum
JOIN pg_class refcl ON refcl.oid = con.confrelid
JOIN pg_namespace refns ON refns.oid = refcl.relnamespace
JOIN pg_attribute ref ON ref.attrelid = con.confrelid AND ref.attnum = rk.attnum
WHERE con.contype = 'f' AND n.nspname = $1 AND t.relname = $2
ORDER BY con.conname, lk.ord`

// TableDef reads the full structure of one table of the current schema
// (SetSchema selects it; the pool is bound to the login database, so db
// is validated non-empty and selects nothing — as in ListTables): its
// columns in ordinal order, its indexes, its foreign keys and its table
// options. A missing table yields an error containing "not found".
func (d *PostgresDriver) TableDef(ctx context.Context, db, table string) (driver.TableDef, error) {
	if err := d.checkDatabase(db); err != nil {
		return driver.TableDef{}, err
	}
	if db == "" {
		return driver.TableDef{}, fmt.Errorf("read table definition: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return driver.TableDef{}, fmt.Errorf("read table definition: %w: table name", sqlgen.ErrEmptyIdentifier)
	}
	schema := d.schemaFor(ctx)

	options, err := d.tableOptions(ctx, schema, table)
	if err != nil {
		return driver.TableDef{}, err
	}

	columns, err := d.tableColumns(ctx, schema, table)
	if err != nil {
		return driver.TableDef{}, err
	}
	indexes, err := d.tableIndexes(ctx, schema, table)
	if err != nil {
		return driver.TableDef{}, err
	}
	fks, err := d.tableForeignKeys(ctx, schema, table)
	if err != nil {
		return driver.TableDef{}, err
	}

	return driver.TableDef{
		NewName:     table,
		Columns:     columns,
		Indexes:     indexes,
		ForeignKeys: fks,
		Options:     options,
	}, nil
}

// tableOptions reads the table's comment from its pg_class row. No row
// means the table does not exist. PostgreSQL has no engines and no
// table-level auto-increment counter; the collation pin of the phase 4
// plan leaves Collation empty (a table-level collation is rarely set,
// and the database default lives in ListDatabases).
func (d *PostgresDriver) tableOptions(ctx context.Context, schema, table string) (driver.TableOptions, error) {
	var comment string
	err := d.db.QueryRowContext(ctx, defTableQuery, schema, table).Scan(&comment)
	if errors.Is(err, sql.ErrNoRows) {
		return driver.TableOptions{}, fmt.Errorf("table %q not found", table)
	}
	if err != nil {
		return driver.TableOptions{}, fmt.Errorf("query options of %s: %w", table, err)
	}
	return driver.TableOptions{Comment: comment}, nil
}

// tableColumns reads and maps the columns of one table in ordinal order.
func (d *PostgresDriver) tableColumns(ctx context.Context, schema, table string) ([]driver.ColumnDef, error) {
	rows, err := d.db.QueryContext(ctx, defColumnsQuery, schema, table)
	if err != nil {
		return nil, fmt.Errorf("query columns of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var columns []driver.ColumnDef
	for rows.Next() {
		var (
			name       string
			dataType   string
			udtName    string
			charLen    sql.NullInt64
			numPrec    sql.NullInt64
			numScale   sql.NullInt64
			nullable   string
			defRaw     sql.NullString
			collation  sql.NullString
			isIdentity string
			isGen      string
			genExpr    sql.NullString
			comment    string
		)
		if err := rows.Scan(&name, &dataType, &udtName, &charLen, &numPrec, &numScale,
			&nullable, &defRaw, &collation, &isIdentity, &isGen, &genExpr, &comment); err != nil {
			return nil, fmt.Errorf("scan column row of %s: %w", table, err)
		}
		colType, length := pgColumnType(dataType, udtName, charLen, numPrec, numScale)

		// PostgreSQL generated columns are always STORED (no VIRTUAL);
		// the expression travels in Default as SQL, mirroring the MySQL
		// mapping (the save path renders it back as AS (expr) STORED).
		generated := ""
		if isGen == "ALWAYS" {
			generated = "STORED"
		}
		var def *driver.DefaultValue
		if generated != "" {
			def = &driver.DefaultValue{Value: genExpr.String, IsSQLExpression: true}
		} else {
			def = pgColumnDefault(defRaw)
		}

		columns = append(columns, driver.ColumnDef{
			Name:     name,
			OldName:  name, // чтение = Name; SaveTable различает ADD и MODIFY
			Type:     colType,
			Length:   length,
			Nullable: nullable == "YES",
			Default:  def,
			// serial is a not-null integer with a nextval(...) default;
			// GENERATED AS IDENTITY marks is_identity instead.
			AutoIncrement: isIdentity == "YES" || strings.HasPrefix(defRaw.String, "nextval("),
			Generated:     generated,
			Collation:     collation.String,
			Comment:       comment,
			// Position stays "": PostgreSQL has no FIRST/AFTER.
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate columns of %s: %w", table, err)
	}
	return columns, nil
}

// pgColumnType maps an information_schema row to the base type name and
// length of a ColumnDef. The length is the character count for char/
// varchar, "precision,scale" for numeric and empty for the rest — an
// integer's numeric_precision (32, 64) is internal width, not a length,
// and must not leak into Length.
func pgColumnType(dataType, udtName string, charLen, numPrec, numScale sql.NullInt64) (string, string) {
	switch dataType {
	case "ARRAY":
		// udt_name is "_" + element type ("_text", "_int4").
		return arrayElementType(strings.TrimPrefix(udtName, "_")) + "[]", ""
	case "character varying":
		return "varchar", nullIntLength(charLen)
	case "character":
		return "char", nullIntLength(charLen)
	case "bit", "bit varying":
		return dataType, nullIntLength(charLen)
	case "numeric":
		if !numPrec.Valid {
			return "numeric", "" // unconstrained numeric
		}
		scale := int64(0)
		if numScale.Valid {
			scale = numScale.Int64
		}
		return "numeric", fmt.Sprintf("%d,%d", numPrec.Int64, scale)
	case "timestamp with time zone":
		return "timestamptz", ""
	case "timestamp without time zone":
		return "timestamp", ""
	case "time with time zone":
		return "timetz", ""
	case "time without time zone":
		return "time", ""
	case "USER-DEFINED":
		return udtName, "" // enums/domains/composites keep their own name
	default:
		return dataType, ""
	}
}

// arrayElementType renders the element type of an array column from its
// udt_name: PostgreSQL's internal aliases become the readable names,
// anything unknown passes through verbatim.
func arrayElementType(udtName string) string {
	switch udtName {
	case "int2":
		return "smallint"
	case "int4":
		return "integer"
	case "int8":
		return "bigint"
	case "float4":
		return "real"
	case "float8":
		return "double precision"
	case "bool":
		return "boolean"
	case "bpchar":
		return "char"
	case "timestamptz":
		return "timestamptz"
	case "timetz":
		return "timetz"
	default:
		return udtName // text, varchar, numeric, json, jsonb, uuid, ...
	}
}

// nullIntLength renders a nullable length as a decimal string, "" when
// the column has no length constraint.
func nullIntLength(v sql.NullInt64) string {
	if !v.Valid {
		return ""
	}
	return fmt.Sprintf("%d", v.Int64)
}

// pgColumnDefault converts a raw column_default into a DefaultValue; a
// missing default means nil. PostgreSQL prints every default as an
// expression ('x'::character varying, nextval(...), now(), 0) — a
// leading quoted literal optionally followed by a ::cast is the one
// string-literal form and becomes an unquoted literal; everything else
// is kept verbatim as a SQL expression.
func pgColumnDefault(raw sql.NullString) *driver.DefaultValue {
	if !raw.Valid || raw.String == "" {
		return nil
	}
	if lit, ok := unquoteLiteralDefault(raw.String); ok {
		return &driver.DefaultValue{Value: lit}
	}
	return &driver.DefaultValue{Value: raw.String, IsSQLExpression: true}
}

// unquoteLiteralDefault recognizes the printed form of a string-literal
// default: 'x' or 'x'::type, with ” escaping inside the literal. It
// reports the unquoted literal; any other shape (nextval(...), now(),
// bare numbers) is not a simple quoted string.
func unquoteLiteralDefault(raw string) (string, bool) {
	if len(raw) < 2 || raw[0] != '\'' {
		return "", false
	}
	for i := 1; i < len(raw); i++ {
		if raw[i] != '\'' {
			continue
		}
		if i+1 < len(raw) && raw[i+1] == '\'' { // '' escape inside the literal
			i++
			continue
		}
		rest := raw[i+1:]
		if rest != "" && !strings.HasPrefix(rest, "::") {
			return "", false // quoted literal followed by something else — expression
		}
		return strings.ReplaceAll(raw[1:i], "''", "'"), true
	}
	return "", false // no closing quote
}

// tableIndexes reads the indexes of one table and classifies them:
// indisprimary → PRIMARY, indisunique → UNIQUE, the rest INDEX. The key
// columns are parsed out of the server's own pg_get_indexdef text.
func (d *PostgresDriver) tableIndexes(ctx context.Context, schema, table string) ([]driver.Index, error) {
	rows, err := d.db.QueryContext(ctx, defIndexesQuery, schema, table)
	if err != nil {
		return nil, fmt.Errorf("query indexes of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var indexes []driver.Index
	for rows.Next() {
		var (
			name       string
			unique     bool
			primary    bool
			def        string
			constraint string
		)
		if err := rows.Scan(&name, &unique, &primary, &def, &constraint); err != nil {
			return nil, fmt.Errorf("scan index row of %s: %w", table, err)
		}
		kind := "INDEX"
		switch {
		case primary:
			kind = "PRIMARY"
		case unique:
			kind = "UNIQUE"
		}
		indexes = append(indexes, driver.Index{
			Name:       name,
			Constraint: constraint,
			Type:       kind,
			Columns:    indexColumnsFromDef(def),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexes of %s: %w", table, err)
	}
	return indexes, nil
}

// indexColumnsFromDef extracts the key column list from pg_get_indexdef
// output such as `CREATE UNIQUE INDEX i ON "s"."t" USING btree (a, b DESC,
// lower((x)::text))` — the parenthesized group after USING <method> (the
// first group, so INCLUDE (...) and WHERE ... suffixes stay out). An
// element is a plain identifier possibly quoted and/or followed by a
// modifier (DESC, ASC, NULLS FIRST/LAST, COLLATE ..., an operator class)
// — only the identifier survives — or a parenthesized expression, kept
// verbatim as the element's name (PostgreSQL has no MySQL-style prefix
// sub-parts, so SubPart stays 0).
func indexColumnsFromDef(def string) []driver.IndexColumn {
	using := strings.LastIndex(def, " USING ")
	if using < 0 {
		return nil
	}
	open := strings.IndexByte(def[using:], '(')
	if open < 0 {
		return nil
	}
	open += using
	closing, ok := matchParen(def, open)
	if !ok {
		return nil
	}

	var cols []driver.IndexColumn
	for _, el := range splitIndexElements(def[open+1 : closing]) {
		if el == "" {
			continue
		}
		if name, ok := quotedIdentifier(el); ok {
			cols = append(cols, driver.IndexColumn{Name: name})
			continue
		}
		if el[0] == '(' { // expression element
			cols = append(cols, driver.IndexColumn{Name: el})
			continue
		}
		if i := strings.IndexAny(el, " \t"); i >= 0 { // identifier + modifier
			el = el[:i]
		}
		cols = append(cols, driver.IndexColumn{Name: el})
	}
	return cols
}

// matchParen finds the index of the ')' closing the '(' at open,
// skipping quoted regions ('…' and "…" with their doubling escapes).
func matchParen(s string, open int) (int, bool) {
	depth := 0
	var quote byte
	for i := open; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				if i+1 < len(s) && s[i+1] == quote { // doubled quote escape
					i++
					continue
				}
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// splitIndexElements splits an index column list on its top-level commas,
// keeping quoted regions and nested parentheses intact.
func splitIndexElements(s string) []string {
	var (
		parts []string
		cur   strings.Builder
		depth int
		quote byte
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			cur.WriteByte(c)
			if c == quote {
				if i+1 < len(s) && s[i+1] == quote {
					cur.WriteByte(s[i+1])
					i++
					continue
				}
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
			cur.WriteByte(c)
		case c == '(':
			depth++
			cur.WriteByte(c)
		case c == ')':
			depth--
			cur.WriteByte(c)
		case c == ',' && depth == 0:
			parts = append(parts, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	return append(parts, strings.TrimSpace(cur.String()))
}

// quotedIdentifier parses a "MixedCase" identifier (doubling "") at the
// start of an index element, reporting the unquoted name. False means
// the element does not start with a double quote.
func quotedIdentifier(el string) (string, bool) {
	if el == "" || el[0] != '"' {
		return "", false
	}
	var b strings.Builder
	for i := 1; i < len(el); i++ {
		if el[i] != '"' {
			b.WriteByte(el[i])
			continue
		}
		if i+1 < len(el) && el[i+1] == '"' {
			b.WriteByte('"')
			i++
			continue
		}
		return b.String(), true
	}
	return "", false // unterminated quote — not a quoted identifier
}

// tableForeignKeys reads the foreign keys declared on one table, ordered
// by constraint name and key position.
func (d *PostgresDriver) tableForeignKeys(ctx context.Context, schema, table string) ([]driver.ForeignKey, error) {
	rows, err := d.db.QueryContext(ctx, defForeignKeysQuery, schema, table)
	if err != nil {
		return nil, fmt.Errorf("query foreign keys of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var fks []driver.ForeignKey
	for rows.Next() {
		var (
			name       string
			col        string
			refCol     string
			refTable   string
			refSchema  string
			updateRule string
			deleteRule string
		)
		if err := rows.Scan(&name, &col, &refCol, &refTable, &refSchema, &updateRule, &deleteRule); err != nil {
			return nil, fmt.Errorf("scan foreign key row of %s: %w", table, err)
		}
		if len(fks) == 0 || fks[len(fks)-1].Name != name {
			fks = append(fks, driver.ForeignKey{
				Name:      name,
				RefTable:  refTable,
				RefSchema: referenceSchema(refSchema, schema),
				OnUpdate:  fkRule(updateRule),
				OnDelete:  fkRule(deleteRule),
			})
		}
		last := &fks[len(fks)-1]
		last.Columns = append(last.Columns, col)
		last.RefColumns = append(last.RefColumns, refCol)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate foreign keys of %s: %w", table, err)
	}
	return fks, nil
}

// fkRule maps pg_constraint's one-letter referential action to the
// uppercase rule text of the ForeignKey contract. The letters: a NO
// ACTION, r RESTRICT, c CASCADE, n SET NULL, d SET DEFAULT.
func fkRule(code string) string {
	switch code {
	case "c":
		return "CASCADE"
	case "n":
		return "SET NULL"
	case "d":
		return "SET DEFAULT"
	case "r":
		return "RESTRICT"
	default:
		return "NO ACTION"
	}
}

func referenceSchema(ref, current string) string {
	if ref == current {
		return ""
	}
	return ref
}
