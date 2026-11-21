package mysql

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/sqlgen"
)

var _ driver.ForeignKeyManager = (*MysqlDriver)(nil)

func normalizeForeignKeysMysql(keys []driver.ForeignKey, db string) ([]driver.ForeignKey, error) {
	if err := driver.ValidateForeignKeys(keys, true); err != nil {
		return nil, err
	}
	result := make([]driver.ForeignKey, len(keys))
	copy(result, keys)
	for i := range result {
		fk := &result[i]
		if fk.RefSchema == "" {
			if schema, table, ok := strings.Cut(fk.RefTable, "."); ok {
				fk.RefSchema = schema
				fk.RefTable = table
			} else {
				fk.RefSchema = db
			}
		}
		identifiers := []string{fk.Name, fk.RefSchema, fk.RefTable}
		identifiers = append(identifiers, fk.Columns...)
		identifiers = append(identifiers, fk.RefColumns...)
		for _, name := range identifiers {
			if utf8.RuneCountInString(name) > 64 || strings.TrimSpace(name) == "" {
				return nil, fmt.Errorf("%w: MySQL identifiers must contain 1 to 64 characters", driver.ErrObjectValidation)
			}
		}
		for _, action := range []*string{&fk.OnDelete, &fk.OnUpdate} {
			*action = strings.ToUpper(strings.TrimSpace(*action))
			if *action == "" || *action == "NO ACTION" {
				*action = "RESTRICT"
			}
			if *action == "SET DEFAULT" {
				return nil, fmt.Errorf("%w: MySQL foreign keys do not support SET DEFAULT", driver.ErrObjectValidation)
			}
		}
	}
	return result, nil
}

// SaveForeignKeys applies the desired named foreign-key constraints.
func (d *MysqlDriver) SaveForeignKeys(ctx context.Context, db, table string, desired []driver.ForeignKey) error {
	target, err := objectIdent(db, table)
	if err != nil {
		return err
	}
	desired, err = normalizeForeignKeysMysql(desired, db)
	if err != nil {
		return err
	}
	def, err := d.TableDef(ctx, db, table)
	if err != nil {
		return err
	}
	current, err := normalizeForeignKeysMysql(def.ForeignKeys, db)
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

	step := 0
	execute := func(clause string) error {
		step++
		_, err := d.db.ExecContext(ctx, "ALTER TABLE "+target+" "+clause)
		if err != nil {
			return fmt.Errorf("foreign key step %d failed (previous successful DDL steps remain applied): %w", step, err)
		}
		return nil
	}
	for _, fk := range current {
		want, exists := next[fk.Name]
		if exists && reflect.DeepEqual(fk, want) {
			continue
		}
		if err := execute("DROP FOREIGN KEY " + sqlgen.QuoteIdent(fk.Name)); err != nil {
			return err
		}
		if exists {
			if err := execute(mysqlForeignKeyAdd(want)); err != nil {
				if restoreErr := execute(mysqlForeignKeyAdd(fk)); restoreErr != nil {
					return fmt.Errorf("%w; restoring previous constraint also failed: %v", err, restoreErr)
				}
				return fmt.Errorf("%w; previous constraint restored", err)
			}
		}
	}
	for _, fk := range desired {
		if _, exists := old[fk.Name]; exists {
			continue
		}
		if err := execute(mysqlForeignKeyAdd(fk)); err != nil {
			return err
		}
	}
	return nil
}
func mysqlForeignKeyAdd(fk driver.ForeignKey) string {
	local := make([]string, len(fk.Columns))
	refs := make([]string, len(fk.RefColumns))
	for i := range local {
		local[i] = sqlgen.QuoteIdent(fk.Columns[i])
		refs[i] = sqlgen.QuoteIdent(fk.RefColumns[i])
	}
	return "ADD CONSTRAINT " + sqlgen.QuoteIdent(fk.Name) + " FOREIGN KEY (" + strings.Join(local, ", ") + ") REFERENCES " + sqlgen.QuoteIdent(fk.RefSchema) + "." + sqlgen.QuoteIdent(fk.RefTable) + " (" + strings.Join(refs, ", ") + ") ON DELETE " + fk.OnDelete + " ON UPDATE " + fk.OnUpdate
}
