package driver

import (
	"context"
	"database/sql"
	"fmt"
)

// TableRelationship identifies the source table of a catalog foreign key.
type TableRelationship struct {
	Table string `json:"table"`
	ForeignKey
}

// RelationshipReader lists foreign keys declared in the requested database or schema.
type RelationshipReader interface {
	ListRelationships(context.Context, string) ([]TableRelationship, error)
}

// ReadTableRelationships groups catalog rows sorted by source table,
// constraint name and column position. Composite keys retain their order.
func ReadTableRelationships(rows *sql.Rows, scope string) ([]TableRelationship, error) {
	var result []TableRelationship
	for rows.Next() {
		var table, name, column, target, refColumn string
		var refScope, onUpdate, onDelete sql.NullString
		if err := rows.Scan(&table, &name, &column, &refScope, &target, &refColumn, &onUpdate, &onDelete); err != nil {
			return nil, fmt.Errorf("read foreign key in %s: %w", scope, err)
		}
		if len(result) == 0 || result[len(result)-1].Table != table || result[len(result)-1].Name != name {
			fk := ForeignKey{Name: name, RefTable: target, OnUpdate: onUpdate.String, OnDelete: onDelete.String}
			if refScope.String != scope {
				fk.RefSchema = refScope.String
			}
			result = append(result, TableRelationship{Table: table, ForeignKey: fk})
		}
		fk := &result[len(result)-1]
		fk.Columns = append(fk.Columns, column)
		fk.RefColumns = append(fk.RefColumns, refColumn)
	}
	return result, rows.Err()
}

// ForeignKeysOfTable returns the constraints of a single-table catalog query.
func ForeignKeysOfTable(relations []TableRelationship) []ForeignKey {
	var keys []ForeignKey
	for _, relation := range relations {
		keys = append(keys, relation.ForeignKey)
	}
	return keys
}
