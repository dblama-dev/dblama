package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// listTablesQuery returns the tables and views of one schema ordered by
// name. TABLE_ROWS is an estimate for InnoDB; the size is the data plus
// index bytes and is 0 for views, which have no storage.
const listTablesQuery = `
SELECT TABLE_NAME,
       COALESCE(TABLE_ROWS, 0),
       COALESCE(DATA_LENGTH + INDEX_LENGTH, 0),
       COALESCE(ENGINE, ''),
       COALESCE(TABLE_COMMENT, ''),
       TABLE_TYPE = 'VIEW'
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = ?
ORDER BY TABLE_NAME`

// ListTables returns the tables and views of database db, ordered by
// name. An empty database name fails before any query is issued.
func (d *MysqlDriver) ListTables(ctx context.Context, db string) ([]driver.TableInfo, error) {
	if db == "" {
		return nil, fmt.Errorf("list tables: database name is empty")
	}

	rows, err := d.db.QueryContext(ctx, listTablesQuery, db)
	if err != nil {
		return nil, fmt.Errorf("query tables of %s: %w", db, err)
	}
	defer func() { _ = rows.Close() }()

	var tables []driver.TableInfo
	for rows.Next() {
		var (
			name    string
			rowsEst int64
			size    int64
			engine  sql.NullString
			comment sql.NullString
			isView  bool
		)
		if err := rows.Scan(&name, &rowsEst, &size, &engine, &comment, &isView); err != nil {
			return nil, fmt.Errorf("scan table row of %s: %w", db, err)
		}
		tables = append(tables, driver.TableInfo{
			Name:      name,
			Rows:      rowsEst,
			SizeBytes: size,
			Engine:    engine.String,
			Comment:   comment.String,
			IsView:    isView,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tables of %s: %w", db, err)
	}
	return tables, nil
}

// The object-destructive operations run against the connection's
// default schema: the DSN binds each driver instance to one database,
// so the names carry no schema prefix and db is validated and used in
// error messages only.

// TruncateTables empties the named tables in order. Every name is
// validated before the first statement runs; the first error aborts
// the remaining tables.
func (d *MysqlDriver) TruncateTables(ctx context.Context, db string, tables []string) error {
	if db == "" {
		return fmt.Errorf("truncate tables: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	for _, table := range tables {
		if table == "" {
			return fmt.Errorf("truncate tables: %w: table name", sqlgen.ErrEmptyIdentifier)
		}
	}
	for _, table := range tables {
		if _, err := d.db.ExecContext(ctx, "TRUNCATE TABLE "+sqlgen.QuoteIdent(table)); err != nil {
			return fmt.Errorf("truncate %s.%s: %w", db, table, err)
		}
	}
	return nil
}

// DropTables drops the named tables without IF EXISTS: the caller is
// expected to have validated existence, so a missing table is an
// error. Every name is validated before the first statement runs; the
// first error aborts the remaining tables.
func (d *MysqlDriver) DropTables(ctx context.Context, db string, tables []string) error {
	if db == "" {
		return fmt.Errorf("drop tables: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	for _, table := range tables {
		if table == "" {
			return fmt.Errorf("drop tables: %w: table name", sqlgen.ErrEmptyIdentifier)
		}
	}
	for _, table := range tables {
		if _, err := d.db.ExecContext(ctx, "DROP TABLE "+sqlgen.QuoteIdent(table)); err != nil {
			return fmt.Errorf("drop table %s.%s: %w", db, table, err)
		}
	}
	return nil
}

// DropViews drops the named views without IF EXISTS. Every name is
// validated before the first statement runs; the first error aborts
// the remaining views.
func (d *MysqlDriver) DropViews(ctx context.Context, db string, views []string) error {
	if db == "" {
		return fmt.Errorf("drop views: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	for _, view := range views {
		if view == "" {
			return fmt.Errorf("drop views: %w: view name", sqlgen.ErrEmptyIdentifier)
		}
	}
	for _, view := range views {
		if _, err := d.db.ExecContext(ctx, "DROP VIEW "+sqlgen.QuoteIdent(view)); err != nil {
			return fmt.Errorf("drop view %s.%s: %w", db, view, err)
		}
	}
	return nil
}
