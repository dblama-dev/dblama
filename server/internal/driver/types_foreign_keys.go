package driver

import (
	"context"
	"fmt"
	"strings"
)

// ForeignKeyManager saves the entire desired constraint list, independently
// from columns and indexes. PostgreSQL changes are transactional; MySQL DDL
// commits each step and attempts restoration if constraint replacement fails.
type ForeignKeyManager interface {
	SaveForeignKeys(context.Context, string, string, []ForeignKey) error
}

// ValidateForeignKeys checks structured FK fields before any DDL. CREATE may
// omit constraint names (the server generates them); editing requires names.
func ValidateForeignKeys(keys []ForeignKey, requireNames bool) error {
	seen := map[string]bool{}
	identifier := func(value string) bool { return strings.TrimSpace(value) != "" && !strings.ContainsRune(value, 0) }
	for _, fk := range keys {
		invalid := func(reason string) error {
			return fmt.Errorf("%w: foreign key %q: %s", ErrObjectValidation, fk.Name, reason)
		}
		if (requireNames && !identifier(fk.Name)) || strings.ContainsRune(fk.Name, 0) {
			return invalid("constraint name is required")
		}
		if fk.Name != "" {
			if seen[fk.Name] {
				return invalid("duplicate constraint name")
			}
			seen[fk.Name] = true
		}
		if !identifier(fk.RefTable) || strings.ContainsRune(fk.RefSchema, 0) {
			return invalid("referenced table/schema is invalid")
		}
		if len(fk.Columns) == 0 || len(fk.Columns) != len(fk.RefColumns) {
			return invalid("column mappings must have equal nonzero lengths")
		}
		for _, cols := range [][]string{fk.Columns, fk.RefColumns} {
			names := map[string]bool{}
			for _, name := range cols {
				if !identifier(name) || names[name] {
					return invalid("column names must be nonempty and unique")
				}
				names[name] = true
			}
		}
		for _, action := range []string{fk.OnDelete, fk.OnUpdate} {
			switch strings.ToUpper(strings.TrimSpace(action)) {
			case "", "CASCADE", "RESTRICT", "NO ACTION", "SET NULL", "SET DEFAULT":
			default:
				return invalid("invalid referential action")
			}
		}
	}
	return nil
}
