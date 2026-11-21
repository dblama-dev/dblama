package postgres

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// prepareTableExport validates the portable FK boundary and includes local
// parents transitively. Rows and constraints may form cycles; table creation
// itself deliberately has no foreign keys, so it needs no topological order.
func (d *PostgresDriver) prepareTableExport(ctx context.Context, tables []string) ([]string, map[string]driver.TableDef, error) {
	infos, err := d.ListTables(ctx, d.database)
	if err != nil {
		return nil, nil, err
	}
	catalog := map[string]bool{}
	for _, info := range infos {
		catalog[info.Name] = info.IsView
	}
	definitions := map[string]driver.TableDef{}
	pending := slices.Clone(tables)
	schema := d.schemaFor(ctx)
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
		def, err := d.TableDef(ctx, d.database, name)
		if err != nil {
			return nil, nil, err
		}
		for i := range def.ForeignKeys {
			fk := &def.ForeignKeys[i]
			if fk.RefSchema != "" && fk.RefSchema != schema {
				return nil, nil, fmt.Errorf("table %s has unsupported external schema foreign key %s to %s.%s", name, fk.Name, fk.RefSchema, fk.RefTable)
			}
			fk.RefSchema = ""
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
func portableTableCreate(table string, def driver.TableDef) (string, error) {
	def.ForeignKeys = nil
	statements, err := buildCreateTableQualified(sqlgen.QuoteIdentPg(table), def)
	if err != nil {
		return "", err
	}
	return strings.Join(statements, ";\n"), nil
}
func writeForeignKeysSQL(out io.Writer, tables []string, defs map[string]driver.TableDef, req driver.ExportRequest) error {
	for _, table := range tables {
		for _, fk := range defs[table].ForeignKeys {
			fk.RefSchema = ""
			fragment, err := foreignKeyFragmentPg(fk)
			if err != nil {
				return err
			}
			ddl := "ALTER TABLE " + sqlgen.QuoteIdentPg(table) + " ADD " + fragment
			statement := ddl + ";\n"
			if req.IfNotExists {
				tag := "$dbm_fk$"
				for i := 0; strings.Contains(ddl, tag); i++ {
					tag = fmt.Sprintf("$dbm_fk_%d$", i)
				}
				condition := "SELECT 1 FROM pg_catalog.pg_constraint WHERE conrelid=pg_catalog.to_regclass(" + sqlQuoteStringPg(sqlgen.QuoteIdentPg(table)) + ") AND conname=" + sqlQuoteStringPg(fk.Name)
				statement = "DO " + tag + " BEGIN IF NOT EXISTS(" + condition + ") THEN EXECUTE " + sqlQuoteStringPg(ddl) + "; END IF; END " + tag + ";\n"
			}
			if err := writeExport(out, statement); err != nil {
				return err
			}
		}
	}
	return nil
}

// orderDataTables loads parents before children while existing constraints stay
// active. A cycle (including a self-reference) needs row-level planning or a
// transaction with deferrable constraints; neither is promised by data mode,
// so reject it before producing bytes and direct callers to the complete dump.
func orderDataTables(tables []string, definitions map[string]driver.TableDef) ([]string, error) {
	state := make(map[string]uint8, len(tables))
	ordered := make([]string, 0, len(tables))
	var stack []string
	var visit func(string) error
	visit = func(table string) error {
		if state[table] == 2 {
			return nil
		}
		if state[table] == 1 {
			return fmt.Errorf("data-only export cannot order foreign key cycle %s; use structure and data mode", strings.Join(append(slices.Clone(stack), table), " -> "))
		}
		def, ok := definitions[table]
		if !ok {
			return fmt.Errorf("data-only export has missing foreign key dependency %q", table)
		}
		state[table] = 1
		stack = append(stack, table)
		parents := make([]string, 0, len(def.ForeignKeys))
		for _, fk := range def.ForeignKeys {
			parents = append(parents, fk.RefTable)
		}
		slices.Sort(parents)
		for _, parent := range parents {
			if err := visit(parent); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[table] = 2
		ordered = append(ordered, table)
		return nil
	}
	for _, table := range tables {
		if err := visit(table); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}
