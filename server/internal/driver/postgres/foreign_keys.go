package postgres

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

var _ driver.ForeignKeyManager = (*PostgresDriver)(nil)

func normalizeForeignKeysPg(keys []driver.ForeignKey, schema string) ([]driver.ForeignKey, error) {
	if err := driver.ValidateForeignKeys(keys, true); err != nil {
		return nil, err
	}
	out := make([]driver.ForeignKey, len(keys))
	copy(out, keys)
	for i := range out {
		fk := &out[i]
		if fk.RefSchema == "" {
			fk.RefSchema = schema
		}
		identifiers := []string{fk.Name, fk.RefSchema, fk.RefTable}
		identifiers = append(identifiers, fk.Columns...)
		identifiers = append(identifiers, fk.RefColumns...)
		for _, name := range identifiers {
			if len(name) > 63 {
				return nil, fmt.Errorf("%w: PostgreSQL identifiers must contain at most 63 bytes", driver.ErrObjectValidation)
			}
		}
		for _, action := range []*string{&fk.OnDelete, &fk.OnUpdate} {
			*action = strings.ToUpper(strings.TrimSpace(*action))
			if *action == "" {
				*action = "NO ACTION"
			}
		}
	}
	return out, nil
}

// SaveForeignKeys applies the desired named foreign-key constraints.
func (d *PostgresDriver) SaveForeignKeys(ctx context.Context, db, table string, desired []driver.ForeignKey) error {
	if err := d.checkDatabase(db); err != nil {
		return err
	}
	schema := d.schemaFor(ctx)
	target, err := qualify(schema, table)
	if err != nil {
		return err
	}
	desired, err = normalizeForeignKeysPg(desired, schema)
	if err != nil {
		return err
	}
	def, err := d.TableDef(ctx, db, table)
	if err != nil {
		return err
	}
	current, err := normalizeForeignKeysPg(def.ForeignKeys, schema)
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for _, c := range def.Columns {
		columns[c.Name] = true
	}
	for _, fk := range desired {
		for _, col := range fk.Columns {
			if !columns[col] {
				return fmt.Errorf("%w: unknown local column %s", driver.ErrObjectValidation, col)
			}
		}
	}
	old := map[string]driver.ForeignKey{}
	next := map[string]driver.ForeignKey{}
	for _, fk := range current {
		old[fk.Name] = fk
	}
	for _, fk := range desired {
		next[fk.Name] = fk
	}
	statements := []string{}
	for _, fk := range current {
		if want, ok := next[fk.Name]; !ok || !reflect.DeepEqual(fk, want) {
			statements = append(statements, "ALTER TABLE "+target+" DROP CONSTRAINT "+sqlgen.QuoteIdentPg(fk.Name))
		}
	}
	for _, fk := range desired {
		if existing, ok := old[fk.Name]; ok && reflect.DeepEqual(existing, fk) {
			continue
		}
		fragment, err := foreignKeyFragmentPg(fk)
		if err != nil {
			return err
		}
		statements = append(statements, "ALTER TABLE "+target+" ADD "+fragment)
	}
	return d.execDDL(ctx, statements)
}
