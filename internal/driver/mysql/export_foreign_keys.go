package mysql

import (
	"context"
	"fmt"
	"slices"

	"github.com/dblama-dev/dblama/internal/driver"
)

// Local parent tables are included even when a selected child sorts before
// them; external database dependencies fail before any dump bytes are written.
func (d *MysqlDriver) prepareTableExport(ctx context.Context, db string, tables []string) ([]string, map[string]driver.TableDef, error) {
	infos, err := d.ListTables(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	catalog := map[string]bool{}
	for _, info := range infos {
		catalog[info.Name] = info.IsView
	}
	definitions := map[string]driver.TableDef{}
	pending := slices.Clone(tables)
	for len(pending) > 0 {
		name := pending[0]
		pending = pending[1:]
		if _, ok := definitions[name]; ok {
			continue
		}
		isView, ok := catalog[name]
		if !ok || isView {
			return nil, nil, fmt.Errorf("SQL export requires a base table; %q is missing or a view", name)
		}
		def, err := d.TableDef(ctx, db, name)
		if err != nil {
			return nil, nil, err
		}
		for _, fk := range def.ForeignKeys {
			if fk.RefSchema != "" && fk.RefSchema != db {
				return nil, nil, fmt.Errorf("table %s has unsupported external database foreign key %s to %s.%s", name, fk.Name, fk.RefSchema, fk.RefTable)
			}
			pending = append(pending, fk.RefTable)
		}
		definitions[name] = def
	}
	names := make([]string, 0, len(definitions))
	for name := range definitions {
		names = append(names, name)
	}
	slices.Sort(names)
	return names, definitions, nil
}
