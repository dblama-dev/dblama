package mysql

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
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
