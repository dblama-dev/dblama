package postgres

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// The browsing queries qualify the current schema (SetSchema swaps it)
// with the sqlgen Pg builders: double-quoted identifiers and $1..$N
// placeholders, parameters never case-folded. The db argument of every
// method below is the login database — validated non-empty to keep the
// driver contract uniform, it selects nothing on PostgreSQL.

// dataColumnsQuery reads the browsing metadata of one table's columns in
// ordinal order: the raw information_schema type fields (mapped through
// pgColumnType to the driver's base type labels) and nullability.
const dataColumnsQuery = `
SELECT column_name, data_type, udt_name,
       character_maximum_length, numeric_precision, numeric_scale,
       is_nullable = 'YES'
FROM information_schema.columns
WHERE table_schema = $1 AND table_name = $2
ORDER BY ordinal_position`

// serialPKQuery reports the one column of a single-column primary key
// whose value the server generates: a serial (a nextval(...) default in
// pg_attrdef — the form CREATE TABLE serial produces) or an identity
// column (pg_attribute.attidentity). No row means the primary key is
// composite, manually assigned or absent, so an insert generates no id
// to RETURN. Ruling 2 of the phase 4 plan.
const serialPKQuery = `
SELECT a.attname
FROM pg_index ix
JOIN pg_class t ON t.oid = ix.indrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ix.indkey[0]
LEFT JOIN pg_attrdef ad ON ad.adrelid = t.oid AND ad.adnum = a.attnum
WHERE n.nspname = $1 AND t.relname = $2
  AND ix.indisprimary AND ix.indnkeyatts = 1
  AND (a.attidentity IN ('a', 'd') OR pg_get_expr(ad.adbin, ad.adrelid) LIKE 'nextval(%')
LIMIT 1`

// SelectRows returns one page of browsed rows of table in the current
// schema (SetSchema selects it): the columns with the driver's type
// labels, the rows as JSON-compatible values and the total number of
// rows matching the filters (COUNT(*) without sort, limit or offset).
// The db argument is validated non-empty and selects nothing. An empty
// db or table name fails before any query is issued.
func (d *PostgresDriver) SelectRows(ctx context.Context, db, table string, q sqlgen.SelectQuery) (driver.Page, error) {
	if err := d.checkDatabase(db); err != nil {
		return driver.Page{}, err
	}
	if db == "" {
		return driver.Page{}, fmt.Errorf("select rows: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return driver.Page{}, fmt.Errorf("select rows: %w: table name", sqlgen.ErrEmptyIdentifier)
	}
	schema := d.schemaFor(ctx)

	columns, err := d.selectColumns(ctx, schema, table, q.Columns)
	if err != nil {
		return driver.Page{}, err
	}

	total, err := d.countRows(ctx, schema, table, q.Filters)
	if err != nil {
		return driver.Page{}, err
	}

	selectQuery, args, err := sqlgen.BuildSelectQualifiedPg(schema, table, q)
	if err != nil {
		return driver.Page{}, fmt.Errorf("select rows of %s.%s: %w", schema, table, err)
	}
	rows, err := d.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return driver.Page{}, fmt.Errorf("select rows of %s.%s: %w", schema, table, err)
	}
	defer func() { _ = rows.Close() }()

	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}

	page := driver.Page{Columns: columns, TotalRows: total}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return driver.Page{}, fmt.Errorf("scan row of %s.%s: %w", schema, table, err)
		}
		row := make([]any, len(values))
		for i, v := range values {
			row[i] = convertValue(v, columns[i].Type)
		}
		page.Rows = append(page.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return driver.Page{}, fmt.Errorf("iterate rows of %s.%s: %w", schema, table, err)
	}
	return page, nil
}

// selectColumns reads the table's column metadata from information_schema
// and returns it in ordinal order — or, when requested is not empty, the
// metadata of exactly the requested columns in the requested order (the
// SELECT built from the same list returns the rows in that order). No
// columns at all means the table does not exist.
func (d *PostgresDriver) selectColumns(ctx context.Context, schema, table string, requested []string) ([]driver.ColumnMeta, error) {
	rows, err := d.db.QueryContext(ctx, dataColumnsQuery, schema, table)
	if err != nil {
		return nil, fmt.Errorf("query column metadata of %s.%s: %w", schema, table, err)
	}
	defer func() { _ = rows.Close() }()

	byName := make(map[string]driver.ColumnMeta)
	order := make([]string, 0, len(requested))
	for rows.Next() {
		var (
			meta     driver.ColumnMeta
			dataType string
			udtName  string
			charLen  sql.NullInt64
			numPrec  sql.NullInt64
			numScale sql.NullInt64
		)
		if err := rows.Scan(&meta.Name, &dataType, &udtName, &charLen, &numPrec, &numScale, &meta.Nullable); err != nil {
			return nil, fmt.Errorf("scan column metadata of %s.%s: %w", schema, table, err)
		}
		meta.Type, _ = pgColumnType(dataType, udtName, charLen, numPrec, numScale)
		byName[meta.Name] = meta
		order = append(order, meta.Name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate column metadata of %s.%s: %w", schema, table, err)
	}
	if len(order) == 0 {
		return nil, fmt.Errorf("table %q not found", table)
	}

	if len(requested) == 0 {
		columns := make([]driver.ColumnMeta, len(order))
		for i, name := range order {
			columns[i] = byName[name]
		}
		return columns, nil
	}

	columns := make([]driver.ColumnMeta, len(requested))
	for i, name := range requested {
		meta, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("column %q not found in table %q", name, table)
		}
		columns[i] = meta
	}
	return columns, nil
}

// countRows runs SELECT COUNT(*) over the same filters as the page query
// — no columns, sorts, limit or offset — for the page's TotalRows.
func (d *PostgresDriver) countRows(ctx context.Context, schema, table string, filters []sqlgen.Filter) (int64, error) {
	countQuery, args, err := sqlgen.BuildCountQualifiedPg(schema, table, sqlgen.SelectQuery{Filters: filters})
	if err != nil {
		return 0, fmt.Errorf("count rows of %s.%s: %w", schema, table, err)
	}
	var total int64
	if err := d.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count rows of %s.%s: %w", schema, table, err)
	}
	return total, nil
}

// convertValue renders one scanned value as a JSON-compatible value.
// pgx's database/sql layer already delivers base types (int64, float64,
// bool, string, []byte, time.Time), so: NULL stays nil, time.Time
// becomes an RFC3339 string, a bytea-typed []byte becomes a "0x"-
// prefixed hex string (spec §4.5) while every other []byte (jsonb, xml)
// becomes its text, and an array column arrives as the PostgreSQL array
// literal string ("{a,NULL,\"b c\"}") and renders as a JSON array
// string. Anything else passes through as delivered.
func convertValue(v any, columnType string) any {
	switch val := v.(type) {
	case nil:
		return nil
	case time.Time:
		return val.Format(time.RFC3339Nano)
	case []byte:
		if columnType == "bytea" {
			return "0x" + hex.EncodeToString(val)
		}
		return string(val)
	case string:
		if strings.HasSuffix(columnType, "[]") {
			return pgArrayToJSON(val, strings.TrimSuffix(columnType, "[]"))
		}
		return val
	case int64, uint64:
		return driver.JSONSafeInteger(val)
	default:
		return v
	}
}

// pgArrayToJSON converts a PostgreSQL array literal — the string form
// array columns scan to through database/sql — into a JSON array string:
// NULL elements render as null, and elements of numeric or boolean
// element types render as JSON numbers and booleans, everything else as
// strings. Nested literals (multi-dimensional arrays) recurse. A literal
// that does not parse passes through unchanged.
func pgArrayToJSON(literal, elemType string) string {
	elements, ok := arrayElements(literal)
	if !ok {
		return literal
	}
	out := make([]any, len(elements))
	for i, el := range elements {
		if !el.quoted && strings.HasPrefix(el.text, "{") {
			out[i] = json.RawMessage(pgArrayToJSON(el.text, elemType))
			continue
		}
		out[i] = arrayElementJSON(el, elemType)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return literal
	}
	return string(b)
}

// arrayElem is one element of a parsed array literal: its unquoted text
// and whether it was double-quoted (a quoted "NULL" is the string,
// an unquoted NULL is SQL NULL).
type arrayElem struct {
	text   string
	quoted bool
}

// arrayElements splits an array literal into its elements, honoring
// quoted elements with their backslash escapes and nested braces. It
// reports false when the literal is not a braced array at all.
func arrayElements(literal string) ([]arrayElem, bool) {
	if len(literal) < 2 || literal[0] != '{' || literal[len(literal)-1] != '}' {
		return nil, false
	}
	var (
		elements   []arrayElem
		cur        strings.Builder
		inQuote    bool // inside a "..." element
		escape     bool // the previous char was a backslash inside quotes
		depth      int  // nested brace depth outside quotes
		started    bool // the current element has seen any content
		elemQuoted bool // the current element was double-quoted
	)
	finish := func() {
		elements = append(elements, arrayElem{text: cur.String(), quoted: elemQuoted})
		cur.Reset()
		started = false
		elemQuoted = false
	}
	for i := 1; i < len(literal)-1; i++ {
		c := literal[i]
		switch {
		case escape: // backslash inside quotes escapes any character
			cur.WriteByte(c)
			escape = false
			started = true
		case inQuote && c == '\\':
			if depth > 0 {
				cur.WriteByte(c)
			}
			escape = true
		case c == '"':
			inQuote = !inQuote
			if depth > 0 {
				cur.WriteByte(c)
			} else {
				elemQuoted = true
			}
			started = true
		case inQuote:
			cur.WriteByte(c)
			started = true
		case c == '{':
			depth++
			cur.WriteByte(c)
			started = true
		case c == '}':
			depth--
			cur.WriteByte(c)
		case c == ',' && depth == 0:
			finish()
		default:
			cur.WriteByte(c)
			started = true
		}
	}
	if inQuote || depth != 0 {
		return nil, false
	}
	if started { // "{}" has no element at all; a trailing "" does
		finish()
	}
	return elements, true
}

// arrayElementJSON renders one parsed array element for pgArrayToJSON.
func arrayElementJSON(el arrayElem, elemType string) any {
	if !el.quoted && strings.EqualFold(el.text, "NULL") {
		return nil
	}
	if !el.quoted {
		if isNumericElemType(elemType) {
			if _, err := strconv.ParseFloat(el.text, 64); err == nil {
				return json.RawMessage(el.text)
			}
		}
		if elemType == "boolean" && (el.text == "true" || el.text == "false") {
			return el.text == "true"
		}
	}
	return el.text
}

// isNumericElemType reports whether the driver's element type label is a
// numeric type whose unquoted array elements render as JSON numbers.
func isNumericElemType(t string) bool {
	switch t {
	case "smallint", "integer", "bigint", "numeric", "real", "double precision":
		return true
	}
	return false
}

// InsertRow inserts one row into table in the current schema. The values
// are bound as parameters in sorted column order, so the statement is
// deterministic for a given row. When the table's primary key is a
// single server-generated column (serial or identity, detected once and
// cached — Ruling 2), the statement carries RETURNING <col> and the
// generated id is remembered in the driver instance for LastInsertID;
// otherwise no id is generated and LastInsertID reads 0. An empty values
// set inserts server defaults; an empty db or table name fails before SQL.
func (d *PostgresDriver) InsertRow(ctx context.Context, db, table string, vals driver.RowValues) error {
	if err := d.checkDatabase(db); err != nil {
		return err
	}
	if db == "" {
		return fmt.Errorf("insert row: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return fmt.Errorf("insert row: %w: table name", sqlgen.ErrEmptyIdentifier)
	}
	schema := d.schemaFor(ctx)
	vals, err := d.normalizeValues(ctx, schema, table, vals, true)
	if err != nil {
		return err
	}

	cols := sortedKeys(vals)
	args := make([]any, len(cols))
	for i, c := range cols {
		args[i] = vals[c]
	}

	query, _, err := sqlgen.BuildInsertPg(table, cols)
	if len(cols) == 0 {
		query = "INSERT INTO " + sqlgen.QuoteIdentPg(table) + " DEFAULT VALUES"
		err = nil
	}
	if err != nil {
		return fmt.Errorf("insert row into %s.%s: %w", schema, table, err)
	}
	serial, err := d.serialPKColumn(ctx, schema, table)
	if err != nil {
		return fmt.Errorf("insert row into %s.%s: %w", schema, table, err)
	}
	if serial != "" {
		query += " RETURNING " + sqlgen.QuoteIdentPg(serial)
	}
	qualified, err := qualifyWrite("INSERT INTO ", query, schema, table)
	if err != nil {
		return fmt.Errorf("insert row into %s.%s: %w", schema, table, err)
	}

	if serial != "" {
		var id int64
		if err := d.db.QueryRowContext(ctx, qualified, args...).Scan(&id); err != nil {
			return fmt.Errorf("insert row into %s.%s: %w", schema, table, err)
		}
		d.setLastInsertID(id)
		return nil
	}
	if _, err := d.db.ExecContext(ctx, qualified, args...); err != nil {
		return fmt.Errorf("insert row into %s.%s: %w", schema, table, err)
	}
	d.setLastInsertID(0)
	return nil
}

// UpdateRow updates every row of table in the current schema whose
// columns match where exactly. The SET values are bound as parameters in
// sorted column order, the WHERE values in sorted key order — the
// statement is deterministic for a given pair. Every row matching the
// where changes (on a table without a unique key that can be more than
// one). An empty db or table name, an empty vals or an empty where
// fails before any statement: the driver never issues an UPDATE without
// a WHERE clause.
func (d *PostgresDriver) UpdateRow(ctx context.Context, db, table string, where driver.PK, vals driver.RowValues) error {
	if err := d.checkDatabase(db); err != nil {
		return err
	}
	if db == "" {
		return fmt.Errorf("update row: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return fmt.Errorf("update row: %w: table name", sqlgen.ErrEmptyIdentifier)
	}
	if len(vals) == 0 {
		return fmt.Errorf("update row in %s.%s: no values", d.schemaFor(ctx), table)
	}
	if len(where) == 0 {
		return fmt.Errorf("update row in %s.%s: refusing an update without a where clause", d.schemaFor(ctx), table)
	}
	schema := d.schemaFor(ctx)
	vals, err := d.normalizeValues(ctx, schema, table, vals, false)
	if err != nil {
		return err
	}
	key, err := d.normalizeValues(ctx, schema, table, driver.RowValues(where), false)
	if err != nil {
		return err
	}
	where = driver.PK(key)

	cols := sortedKeys(vals)
	setArgs := make([]any, len(cols))
	for i, c := range cols {
		setArgs[i] = vals[c]
	}

	query, whereArgs, err := sqlgen.BuildUpdatePg(table, cols, map[string]any(where))
	if err != nil {
		return fmt.Errorf("update row in %s.%s: %w", schema, table, err)
	}
	args := slices.Concat(setArgs, whereArgs)
	qualified, err := qualifyWrite("UPDATE ", query, schema, table)
	if err != nil {
		return fmt.Errorf("update row in %s.%s: %w", schema, table, err)
	}
	if _, err := d.db.ExecContext(ctx, qualified, args...); err != nil {
		return fmt.Errorf("update row in %s.%s: %w", schema, table, err)
	}
	return nil
}

// DeleteRows deletes one row per key in a single transaction — either
// every row is deleted or none is. An empty db or table name, an empty
// key list or an empty key inside the list fails before any statement:
// a DELETE without a WHERE clause never leaves this driver. Pinned
// choice (over one DELETE with OR-ed key groups): a loop of single-row
// DELETEs inside the transaction — the keys usually arrive as primary
// keys, and per-key statements reuse BuildDeletePg unchanged.
func (d *PostgresDriver) DeleteRows(ctx context.Context, db, table string, where []driver.PK) error {
	if err := d.checkDatabase(db); err != nil {
		return err
	}
	if db == "" {
		return fmt.Errorf("delete rows: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return fmt.Errorf("delete rows: %w: table name", sqlgen.ErrEmptyIdentifier)
	}
	if len(where) == 0 {
		return fmt.Errorf("delete rows from %s.%s: no keys", d.schemaFor(ctx), table)
	}
	for i, pk := range where {
		if len(pk) == 0 {
			return fmt.Errorf("delete rows from %s.%s: key %d is empty", d.schemaFor(ctx), table, i+1)
		}
	}
	schema := d.schemaFor(ctx)
	keys := make([]driver.PK, len(where))
	for i, pk := range where {
		value, err := d.normalizeValues(ctx, schema, table, driver.RowValues(pk), false)
		if err != nil {
			return err
		}
		keys[i] = driver.PK(value)
	}
	where = keys

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete rows from %s.%s: begin transaction: %w", schema, table, err)
	}
	for i, pk := range where {
		query, args, err := sqlgen.BuildDeletePg(table, map[string]any(pk))
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete rows from %s.%s: key %d: %w", schema, table, i+1, err)
		}
		qualified, err := qualifyWrite("DELETE FROM ", query, schema, table)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete rows from %s.%s: key %d: %w", schema, table, i+1, err)
		}
		if _, err := tx.ExecContext(ctx, qualified, args...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete rows from %s.%s: key %d: %w", schema, table, i+1, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete rows from %s.%s: commit: %w", schema, table, err)
	}
	return nil
}

// UpsertRows inserts every row of table in the current schema as one
// INSERT ... ON CONFLICT (<primary key columns>) DO UPDATE statement: a
// row whose primary key already exists updates the conflicting row with
// the new values, the rest are inserted. The conflict target is the
// table's primary key — a table without one cannot upsert and fails
// before any statement. All rows must have the same column set — the
// statement is built once from the first row, with one VALUES tuple per
// row. An empty db or table name, an empty row list or a mixed column
// set fails before any statement.
func (d *PostgresDriver) UpsertRows(ctx context.Context, db, table string, rows []driver.RowValues) error {
	if err := d.checkDatabase(db); err != nil {
		return err
	}
	if db == "" {
		return fmt.Errorf("upsert rows: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return fmt.Errorf("upsert rows: %w: table name", sqlgen.ErrEmptyIdentifier)
	}
	if len(rows) == 0 {
		return fmt.Errorf("upsert rows into %s.%s: no rows", d.schemaFor(ctx), table)
	}
	schema := d.schemaFor(ctx)
	normalized := make([]driver.RowValues, len(rows))
	for i, row := range rows {
		value, err := d.normalizeValues(ctx, schema, table, row, false)
		if err != nil {
			return err
		}
		normalized[i] = value
	}
	rows = normalized

	pk, err := d.PrimaryKey(ctx, db, table)
	if err != nil {
		return fmt.Errorf("upsert rows into %s.%s: %w", schema, table, err)
	}
	if len(pk) == 0 {
		return fmt.Errorf("upsert rows into %s.%s: table has no primary key to conflict on", schema, table)
	}

	cols := sortedKeys(rows[0])
	for i, row := range rows[1:] {
		if !sameKeys(row, cols) {
			return fmt.Errorf("upsert rows into %s.%s: row %d has a different column set than row 1", schema, table, i+2)
		}
	}

	args := make([]any, 0, len(rows)*len(cols))
	for _, row := range rows {
		for _, c := range cols {
			args = append(args, row[c])
		}
	}

	query, _, err := sqlgen.BuildInsertPg(table, cols)
	if err != nil {
		return fmt.Errorf("upsert rows into %s.%s: %w", schema, table, err)
	}
	// One VALUES tuple per row: BuildInsertPg renders the first one
	// ($1..$n), the rest are appended here, the numbering continuing
	// tuple by tuple.
	n := len(cols)
	tuple := func(t int) string {
		markers := make([]string, n)
		for j := range markers {
			markers[j] = "$" + strconv.Itoa(t*n+j+1)
		}
		return "(" + strings.Join(markers, ",") + ")"
	}
	for t := 1; t < len(rows); t++ {
		query += ", " + tuple(t)
	}
	conflict := make([]string, len(pk))
	for i, c := range pk {
		conflict[i] = sqlgen.QuoteIdentPg(c)
	}
	sets := make([]string, len(cols))
	for i, c := range cols {
		sets[i] = sqlgen.QuoteIdentPg(c) + "=EXCLUDED." + sqlgen.QuoteIdentPg(c)
	}
	query += " ON CONFLICT (" + strings.Join(conflict, ",") + ") DO UPDATE SET " + strings.Join(sets, ",")

	qualified, err := qualifyWrite("INSERT INTO ", query, schema, table)
	if err != nil {
		return fmt.Errorf("upsert rows into %s.%s: %w", schema, table, err)
	}
	if _, err := d.db.ExecContext(ctx, qualified, args...); err != nil {
		return fmt.Errorf("upsert rows into %s.%s: %w", schema, table, err)
	}
	return nil
}

// LastInsertID returns the id generated by the most recent InsertRow
// executed through this driver instance (0 when that insert generated
// none — no server-generated single-column primary key — or nothing was
// written yet). The value lives in the mutex-protected driver state —
// driver instances are shared across requests, and PostgreSQL has no
// connection-neutral equivalent of MySQL's LAST_INSERT_ID(), so the id
// is captured from the insert's RETURNING clause. Because it is driver
// state, the last insert wins across concurrent requests: callers that
// need their id must read it immediately after their insert. UpsertRows
// does not touch it — one multi-row statement has no single id.
func (d *PostgresDriver) LastInsertID(_ context.Context) (int64, error) {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	return d.lastInsertID, nil
}

// setLastInsertID stores id as the driver's most recent generated id.
func (d *PostgresDriver) setLastInsertID(id int64) {
	d.writeMu.Lock()
	d.lastInsertID = id
	d.writeMu.Unlock()
}

// serialPKColumn returns the table's single server-generated primary
// key column ("" when there is none), detected once per table and cached
// in the driver instance — Ruling 2. The cache is dropped by the save
// paths, whose ALTERs may change the key; a console DROP/ALTER of the
// key (task 5) can leave a stale entry until restart, at worst dropping
// the RETURNING clause's value or failing one insert with an explicit
// column list that omits the dropped column.
func (d *PostgresDriver) serialPKColumn(ctx context.Context, schema, table string) (string, error) {
	key := schema + "." + table
	d.serialMu.Lock()
	col, ok := d.serialPK[key]
	d.serialMu.Unlock()
	if ok {
		return col, nil
	}

	col = ""
	err := d.db.QueryRowContext(ctx, serialPKQuery, schema, table).Scan(&col)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil // no server-generated key — cached as ""
	}
	if err != nil {
		return "", fmt.Errorf("read serial key of %s.%s: %w", schema, table, err)
	}

	d.serialMu.Lock()
	d.serialPK[key] = col
	d.serialMu.Unlock()
	return col, nil
}

// invalidateSerialPK drops the cached serial-key detection of a table —
// the save paths call it after ALTERs that may have changed the key.
func (d *PostgresDriver) invalidateSerialPK(schema, table string) {
	d.serialMu.Lock()
	delete(d.serialPK, schema+"."+table)
	d.serialMu.Unlock()
}

// sortedKeys returns the map's keys in sorted order — the deterministic
// column order of every row-write statement.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sameKeys reports whether the map has exactly the given key set (same
// size, every key present) — the identical-column-set check of
// UpsertRows.
func sameKeys(m map[string]any, keys []string) bool {
	if len(m) != len(keys) {
		return false
	}
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}

// qualifyWrite swaps the bare quoted table reference of a sqlgen-built
// write statement for the schema-qualified "schema"."table" form. The
// table reference always directly follows the statement's verb, so the
// verb prefixes the substitution: a table name that also occurs inside
// the quoted schema name cannot be hit. If the verb-anchored pattern is
// absent (a statement layout change would silently drop the schema
// qualification otherwise) an error is returned instead of the
// unqualified query.
func qualifyWrite(verb, query, schema, table string) (string, error) {
	qualified := strings.Replace(query,
		verb+sqlgen.QuoteIdentPg(table),
		verb+sqlgen.QuoteIdentPg(schema)+"."+sqlgen.QuoteIdentPg(table), 1)
	if qualified == query {
		return "", fmt.Errorf("qualifyWrite: verb not found in %q", query)
	}
	return qualified, nil
}

// normalizeValues reverses the JSON-friendly browse representation before
// binding PostgreSQL parameters; it never mutates caller-owned values.
func (d *PostgresDriver) normalizeValues(ctx context.Context, schema, table string, values driver.RowValues, inserting bool) (driver.RowValues, error) {
	columns, err := d.tableColumns(ctx, schema, table)
	if err != nil {
		return nil, err
	}
	out := make(driver.RowValues, len(values))
	for k, v := range values {
		out[k] = v
	}
	for _, col := range columns {
		value, exists := out[col.Name]
		if !exists {
			continue
		}
		if inserting && col.AutoIncrement && value == nil {
			delete(out, col.Name)
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		if strings.HasSuffix(col.Type, "[]") && strings.HasPrefix(strings.TrimSpace(text), "[") {
			literal, valid := jsonToPgArray(text)
			if !valid {
				return nil, fmt.Errorf("invalid JSON array for %s", col.Name)
			}
			out[col.Name] = literal
		}
		if col.Type == "bytea" && strings.HasPrefix(text, "0x") {
			out[col.Name] = `\x` + text[2:]
		}
	}
	return out, nil
}
