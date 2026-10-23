package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hitslab/db-manager/server/internal/driver"
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
