package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// errNoSuchTable is the MySQL error number for ER_NO_SUCH_TABLE ("Table
// ... doesn't exist").
const errNoSuchTable = 1146

// ShowCreateTable returns the server's own CREATE TABLE statement of one
// table. The statement addresses the schema db, regardless of the DSN's
// default database. A missing table yields an error containing "not
// found"; empty names fail before any query is issued. Some servers
// (MariaDB) append a fake empty second "Create Table" column to keep
// their client buffers happy; the first non-empty value wins, so that
// artifact is dropped.
func (d *MysqlDriver) ShowCreateTable(ctx context.Context, db, table string) (string, error) {
	if db == "" {
		return "", fmt.Errorf("show create table: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return "", fmt.Errorf("show create table: %w: table name", sqlgen.ErrEmptyIdentifier)
	}

	rows, err := d.db.QueryContext(ctx, "SHOW CREATE TABLE "+qualifiedIdent(db, table))
	if err != nil {
		var myErr *mysqldriver.MySQLError
		if errors.As(err, &myErr) && myErr.Number == errNoSuchTable {
			return "", fmt.Errorf("table %q not found", table)
		}
		return "", fmt.Errorf("show create table %s.%s: %w", db, table, err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", fmt.Errorf("show create table %s.%s: %w", db, table, err)
		}
		return "", fmt.Errorf("table %q not found", table)
	}

	// Column 0 is the table name; the CREATE statement follows —
	// possibly shadowed by a trailing fake empty duplicate column.
	cols, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("show create table %s.%s: %w", db, table, err)
	}
	values := make([]sql.NullString, len(cols)-1)
	ptrs := make([]any, len(cols))
	ptrs[0] = new(sql.NullString) // table name, discarded
	for i := range values {
		ptrs[i+1] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return "", fmt.Errorf("scan create statement of %s.%s: %w", db, table, err)
	}
	for _, v := range values {
		if v.Valid && v.String != "" {
			return v.String, nil
		}
	}
	return "", fmt.Errorf("show create table %s.%s: server returned no create statement", db, table)
}
