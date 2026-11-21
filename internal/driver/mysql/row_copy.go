package mysql

import (
	"context"
	"fmt"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/sqlgen"
)

// CopyRows duplicates matching source rows and lets the server allocate new IDs.
func (d *MysqlDriver) CopyRows(ctx context.Context, db, table string, keys []driver.PK) error {
	if len(keys) == 0 || len(keys) > 1000 {
		return fmt.Errorf("copy requires between 1 and 1000 keys")
	}
	def, err := d.TableDef(ctx, db, table)
	if err != nil {
		return err
	}
	columns := []string{}
	for _, col := range def.Columns {
		if !col.AutoIncrement && col.Generated == "" {
			columns = append(columns, col.Name)
		}
	}
	if len(columns) == 0 {
		return fmt.Errorf("table has no copyable columns; add a default row instead")
	}
	keys, err = d.normalizeOriginalKeys(ctx, db, table, keys)
	if err != nil {
		return err
	}
	where := make([]map[string]any, len(keys))
	for i, key := range keys {
		where[i] = map[string]any(key)
	}

	query, args, err := sqlgen.BuildCopyRows(sqlgen.MySQLDialect, db, table, columns, where)
	if err != nil {
		return err
	}
	query, args, err = bindOriginalJSONKeys(query, args)
	if err != nil {
		return err
	}
	_, err = d.db.ExecContext(ctx, query, args...)
	return err
}
