package postgres

import (
	"context"
	"fmt"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/sqlgen"
)

// CopyRows duplicates matching source rows and lets the server allocate new IDs.
func (d *PostgresDriver) CopyRows(ctx context.Context, db, table string, keys []driver.PK) error {
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
	where := make([]map[string]any, len(keys))
	for i, key := range keys {
		where[i] = map[string]any(key)
	}
	for i, key := range keys {
		values, err := d.normalizeValues(ctx, d.schemaFor(ctx), table, driver.RowValues(key), false)
		if err != nil {
			return err
		}
		where[i] = map[string]any(values)
	}
	query, args, err := sqlgen.BuildCopyRows(sqlgen.PgDialect, d.schemaFor(ctx), table, columns, where)
	if err != nil {
		return err
	}
	_, err = d.db.ExecContext(ctx, query, args...)
	return err
}
