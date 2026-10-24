package mysql

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// dataColumnsQuery reads the browsing metadata of one table's columns in
// ordinal order: the base type label (DATA_TYPE, e.g. "int", "varchar")
// and nullability. DATA_TYPE carries the base type without the length or
// the unsigned suffix, matching the driver's type labels elsewhere.
const dataColumnsQuery = `
SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE = 'YES'
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
ORDER BY ORDINAL_POSITION`

// primaryKeyQuery reads the primary key column names in key order. No
// rows means the table has no primary key.
const primaryKeyQuery = `
SELECT COLUMN_NAME
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND INDEX_NAME = 'PRIMARY'
ORDER BY SEQ_IN_INDEX`

// SelectRows returns one page of browsed rows of table in database db:
// the columns with their driver type labels, the rows as JSON-compatible
// values and the total number of rows matching the filters (COUNT(*)
// without sort, limit or offset). The db argument qualifies the queried
// schema regardless of the DSN's default database. An empty db or table
// name fails before any query is issued.
func (d *MysqlDriver) SelectRows(ctx context.Context, db, table string, q sqlgen.SelectQuery) (driver.Page, error) {
	if db == "" {
		return driver.Page{}, fmt.Errorf("select rows: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return driver.Page{}, fmt.Errorf("select rows: %w: table name", sqlgen.ErrEmptyIdentifier)
	}

	columns, err := d.selectColumns(ctx, db, table, q.Columns)
	if err != nil {
		return driver.Page{}, err
	}

	total, err := d.countRows(ctx, db, table, q.Filters)
	if err != nil {
		return driver.Page{}, err
	}

	selectQuery, args, err := sqlgen.BuildSelectQualified(db, table, q)
	if err != nil {
		return driver.Page{}, fmt.Errorf("select rows of %s.%s: %w", db, table, err)
	}
	rows, err := d.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return driver.Page{}, fmt.Errorf("select rows of %s.%s: %w", db, table, err)
	}
	defer func() { _ = rows.Close() }()

	// Binary columns (per the server's own column type of this result)
	// scan to []byte that is rendered as hex; text columns that also
	// scan to []byte are rendered as strings. If the column types cannot
	// be read, everything is treated as text.
	binary := binaryColumns(rows)
	if binary == nil {
		binary = make([]bool, len(columns))
	}

	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}

	page := driver.Page{Columns: columns, TotalRows: total}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return driver.Page{}, fmt.Errorf("scan row of %s.%s: %w", db, table, err)
		}
		row := make([]any, len(values))
		for i, v := range values {
			row[i] = convertValue(v, binary[i])
		}
		page.Rows = append(page.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return driver.Page{}, fmt.Errorf("iterate rows of %s.%s: %w", db, table, err)
	}
	return page, nil
}

// PrimaryKey returns the primary key column names of table in database
// db in key order; a table without a primary key yields an empty slice.
// An empty db or table name fails before any query is issued.
func (d *MysqlDriver) PrimaryKey(ctx context.Context, db, table string) ([]string, error) {
	if db == "" {
		return nil, fmt.Errorf("read primary key: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return nil, fmt.Errorf("read primary key: %w: table name", sqlgen.ErrEmptyIdentifier)
	}

	rows, err := d.db.QueryContext(ctx, primaryKeyQuery, db, table)
	if err != nil {
		return nil, fmt.Errorf("query primary key of %s.%s: %w", db, table, err)
	}
	defer func() { _ = rows.Close() }()

	var pk []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan primary key column of %s.%s: %w", db, table, err)
		}
		pk = append(pk, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate primary key of %s.%s: %w", db, table, err)
	}
	return pk, nil
}

// selectColumns reads the table's column metadata from information_schema
// and returns it in ordinal order — or, when requested is not empty, the
// metadata of exactly the requested columns in the requested order (the
// SELECT built from the same list returns the rows in that order). No
// columns at all means the table does not exist.
func (d *MysqlDriver) selectColumns(ctx context.Context, db, table string, requested []string) ([]driver.ColumnMeta, error) {
	rows, err := d.db.QueryContext(ctx, dataColumnsQuery, db, table)
	if err != nil {
		return nil, fmt.Errorf("query column metadata of %s.%s: %w", db, table, err)
	}
	defer func() { _ = rows.Close() }()

	byName := make(map[string]driver.ColumnMeta)
	order := make([]string, 0, len(requested))
	for rows.Next() {
		var meta driver.ColumnMeta
		if err := rows.Scan(&meta.Name, &meta.Type, &meta.Nullable); err != nil {
			return nil, fmt.Errorf("scan column metadata of %s.%s: %w", db, table, err)
		}
		byName[meta.Name] = meta
		order = append(order, meta.Name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate column metadata of %s.%s: %w", db, table, err)
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

// countRows runs SELECT COUNT(*) over the same filters as the page
// query — no columns, sorts, limit or offset — for the page's TotalRows.
func (d *MysqlDriver) countRows(ctx context.Context, db, table string, filters []sqlgen.Filter) (int64, error) {
	countQuery, args, err := sqlgen.BuildCountQualified(db, table, sqlgen.SelectQuery{Filters: filters})
	if err != nil {
		return 0, fmt.Errorf("count rows of %s.%s: %w", db, table, err)
	}
	var total int64
	if err := d.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count rows of %s.%s: %w", db, table, err)
	}
	return total, nil
}

// binaryColumns marks which result columns are binary types (BLOB and
// BINARY family, geometry): their []byte values are raw bytes that must
// be rendered as hex, while text columns also arrive as []byte but are
// rendered as strings. A column whose type cannot be read is treated as
// text.
func binaryColumns(rows *sql.Rows) []bool {
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil
	}
	out := make([]bool, len(types))
	for i, ct := range types {
		out[i] = isBinaryTypeName(ct.DatabaseTypeName())
	}
	return out
}

// isBinaryTypeName reports whether a database type name (as reported by
// rows.ColumnTypes()) is a binary type.
func isBinaryTypeName(name string) bool {
	switch strings.ToUpper(name) {
	case "BINARY", "VARBINARY", "TINYBLOB", "BLOB", "MEDIUMBLOB", "LONGBLOB", "GEOMETRY":
		return true
	}
	return false
}

// convertValue renders one scanned value as a JSON-compatible value:
// NULL stays nil, time.Time becomes an RFC3339 string, binary-typed
// []byte becomes a "0x"-prefixed hex string (spec §4.5), text-typed
// []byte becomes a string, and everything else passes through as
// database/sql delivered it (int64, float64, ...).
func convertValue(v any, binary bool) any {
	switch val := v.(type) {
	case nil:
		return nil
	case time.Time:
		return val.Format(time.RFC3339)
	case []byte:
		if binary {
			return "0x" + hex.EncodeToString(val)
		}
		return string(val)
	default:
		return v
	}
}

// InsertRow inserts one row into table in database db. The values are
// bound as parameters in sorted column order, so the statement is
// deterministic for a given row. An empty db or table name, or an empty
// values set, fails before any statement. The auto_increment id
// generated by the insert (0 when the table has none) is remembered in
// the driver instance and read back with LastInsertID.
func (d *MysqlDriver) InsertRow(ctx context.Context, db, table string, vals driver.RowValues) error {
	if db == "" {
		return fmt.Errorf("insert row: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return fmt.Errorf("insert row: %w: table name", sqlgen.ErrEmptyIdentifier)
	}
	if len(vals) == 0 {
		return fmt.Errorf("insert row into %s.%s: no values", db, table)
	}

	cols := sortedKeys(vals)
	args := make([]any, len(cols))
	for i, c := range cols {
		args[i] = vals[c]
	}

	query, _, err := sqlgen.BuildInsert(table, cols)
	if err != nil {
		return fmt.Errorf("insert row into %s.%s: %w", db, table, err)
	}
	res, err := d.db.ExecContext(ctx, qualifyWrite("INSERT INTO ", query, db, table), args...)
	if err != nil {
		return fmt.Errorf("insert row into %s.%s: %w", db, table, err)
	}
	d.rememberLastInsertID(res)
	return nil
}

// UpdateRow updates every row of table in database db whose columns
// match where exactly. The SET values are bound as parameters in sorted
// column order, the WHERE values in sorted key order — the statement is
// deterministic for a given pair. Every row matching the where changes
// (on a table without a unique key that can be more than one). An empty
// db or table name, an empty vals or an empty where fails before any
// statement: the driver never issues an UPDATE without a WHERE clause.
func (d *MysqlDriver) UpdateRow(ctx context.Context, db, table string, where driver.PK, vals driver.RowValues) error {
	if db == "" {
		return fmt.Errorf("update row: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return fmt.Errorf("update row: %w: table name", sqlgen.ErrEmptyIdentifier)
	}
	if len(vals) == 0 {
		return fmt.Errorf("update row in %s.%s: no values", db, table)
	}
	if len(where) == 0 {
		return fmt.Errorf("update row in %s.%s: refusing an update without a where clause", db, table)
	}

	cols := sortedKeys(vals)
	setArgs := make([]any, len(cols))
	for i, c := range cols {
		setArgs[i] = vals[c]
	}

	query, whereArgs, err := sqlgen.BuildUpdate(table, cols, map[string]any(where))
	if err != nil {
		return fmt.Errorf("update row in %s.%s: %w", db, table, err)
	}
	args := slices.Concat(setArgs, whereArgs)
	if _, err := d.db.ExecContext(ctx, qualifyWrite("UPDATE ", query, db, table), args...); err != nil {
		return fmt.Errorf("update row in %s.%s: %w", db, table, err)
	}
	return nil
}

// DeleteRows deletes one row per key in a single transaction — either
// every row is deleted or none is. An empty db or table name, an empty
// key list or an empty key inside the list fails before any statement:
// a DELETE without a WHERE clause never leaves this driver. Pinned
// choice (over one DELETE with OR-ed key groups): a loop of single-row
// DELETEs inside the transaction — the keys usually arrive as primary
// keys, and per-key statements reuse BuildDelete unchanged.
func (d *MysqlDriver) DeleteRows(ctx context.Context, db, table string, where []driver.PK) error {
	if db == "" {
		return fmt.Errorf("delete rows: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return fmt.Errorf("delete rows: %w: table name", sqlgen.ErrEmptyIdentifier)
	}
	if len(where) == 0 {
		return fmt.Errorf("delete rows from %s.%s: no keys", db, table)
	}
	for i, pk := range where {
		if len(pk) == 0 {
			return fmt.Errorf("delete rows from %s.%s: key %d is empty", db, table, i+1)
		}
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete rows from %s.%s: begin transaction: %w", db, table, err)
	}
	for i, pk := range where {
		query, args, err := sqlgen.BuildDelete(table, map[string]any(pk))
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete rows from %s.%s: key %d: %w", db, table, i+1, err)
		}
		if _, err := tx.ExecContext(ctx, qualifyWrite("DELETE FROM ", query, db, table), args...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete rows from %s.%s: key %d: %w", db, table, i+1, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete rows from %s.%s: commit: %w", db, table, err)
	}
	return nil
}

// UpsertRows inserts every row of table in database db as one
// INSERT ... ON DUPLICATE KEY UPDATE statement: a row whose unique key
// already exists updates the conflicting row with the new values, the
// rest are inserted. All rows must have the same column set — the
// statement is built once from the first row, with one VALUES tuple per
// row. An empty db or table name, an empty row list or a mixed column
// set fails before any statement. The auto_increment id of the
// statement is remembered for LastInsertID.
func (d *MysqlDriver) UpsertRows(ctx context.Context, db, table string, rows []driver.RowValues) error {
	if db == "" {
		return fmt.Errorf("upsert rows: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return fmt.Errorf("upsert rows: %w: table name", sqlgen.ErrEmptyIdentifier)
	}
	if len(rows) == 0 {
		return fmt.Errorf("upsert rows into %s.%s: no rows", db, table)
	}

	cols := sortedKeys(rows[0])
	for i, row := range rows[1:] {
		if !sameKeys(row, cols) {
			return fmt.Errorf("upsert rows into %s.%s: row %d has a different column set than row 1", db, table, i+2)
		}
	}

	args := make([]any, 0, len(rows)*len(cols))
	for _, row := range rows {
		for _, c := range cols {
			args = append(args, row[c])
		}
	}

	query, _, err := sqlgen.BuildInsert(table, cols)
	if err != nil {
		return fmt.Errorf("upsert rows into %s.%s: %w", db, table, err)
	}
	// One VALUES tuple per row: BuildInsert renders the first one, the
	// rest are appended here — one "?" placeholder per column each.
	tuple := "(" + strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",") + ")"
	query += strings.Repeat(", "+tuple, len(rows)-1)
	// VALUES(col) inside ON DUPLICATE KEY UPDATE is the only form both
	// flavors accept (MySQL 8 marks it deprecated but keeps it; the
	// alias-based replacement is MySQL-only).
	sets := make([]string, len(cols))
	for i, c := range cols {
		sets[i] = sqlgen.QuoteIdent(c) + "=VALUES(" + sqlgen.QuoteIdent(c) + ")"
	}
	query += " ON DUPLICATE KEY UPDATE " + strings.Join(sets, ",")

	res, err := d.db.ExecContext(ctx, qualifyWrite("INSERT INTO ", query, db, table), args...)
	if err != nil {
		return fmt.Errorf("upsert rows into %s.%s: %w", db, table, err)
	}
	d.rememberLastInsertID(res)
	return nil
}

// LastInsertID returns the auto_increment id generated by the most
// recent InsertRow or UpsertRows executed through this driver instance
// (0 when that write generated none or nothing was written yet). The
// value lives in the mutex-protected driver state — driver instances
// are shared across requests, so a per-connection LAST_INSERT_ID()
// would not be reliable.
func (d *MysqlDriver) LastInsertID(_ context.Context) (int64, error) {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	return d.lastInsertID, nil
}

// rememberLastInsertID stores res.LastInsertId() as the driver's most
// recent generated id. A driver that cannot report one (never happens
// for MySQL/MariaDB) leaves the previous value in place.
func (d *MysqlDriver) rememberLastInsertID(res sql.Result) {
	id, err := res.LastInsertId()
	if err != nil {
		return
	}
	d.writeMu.Lock()
	d.lastInsertID = id
	d.writeMu.Unlock()
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
// write statement for the schema-qualified `db`.`table` form. The table
// reference always directly follows the statement's verb, so the verb
// prefixes the substitution: a table name that also occurs inside the
// quoted database name cannot be hit.
func qualifyWrite(verb, query, db, table string) string {
	return strings.Replace(query, verb+sqlgen.QuoteIdent(table), verb+qualifiedIdent(db, table), 1)
}
