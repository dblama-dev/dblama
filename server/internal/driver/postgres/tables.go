package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// The table-reading queries of this file take the current schema as the
// $1 parameter (SetSchema swaps it — never a SET search_path on the
// shared pool, spec §13.5) and qualify nothing else: parameters do not
// case-fold, so a "MixedCase" table name binds exactly.

// listTablesQuery returns the tables and views of one schema ordered by
// name. information_schema.tables carries name and kind (BASE TABLE /
// VIEW), pg_class — joined on the schema-qualified relation — the row
// estimate and the total on-disk size, obj_description the comment.
// reltuples is -1 for a relation PostgreSQL has not yet sampled
// (PG 14+); GREATEST folds that unknown to 0, matching what an empty
// table reports. A view has no storage: its size is 0.
const listTablesQuery = `
SELECT t.table_name,
       GREATEST(c.reltuples::bigint, 0),
       pg_total_relation_size(c.oid),
       obj_description(c.oid, 'pg_class'),
       t.table_type = 'VIEW'
FROM information_schema.tables t
JOIN pg_class c ON c.relname = t.table_name
JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = t.table_schema
WHERE t.table_schema = $1 AND t.table_type IN ('BASE TABLE', 'VIEW')
ORDER BY t.table_name`

// primaryKeyQuery returns the primary key column names of one table in
// key order: pg_index.indkey lists the member attnums in key order, and
// unnest WITH ORDINALITY keeps that order through the join.
const primaryKeyQuery = `
SELECT a.attname
FROM pg_index ix
JOIN pg_class t ON t.oid = ix.indrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
JOIN LATERAL unnest(ix.indkey::int2[]) WITH ORDINALITY AS k(attnum, ord) ON true
JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = k.attnum
WHERE n.nspname = $1 AND t.relname = $2 AND ix.indisprimary
ORDER BY k.ord`

// SetSchema makes schema the current schema of this driver instance; the
// table and structure methods qualify it in their SQL from the next call
// on. An empty schema name is rejected and leaves the current selection
// unchanged (the API layer passes "public" for an absent ?schema=, never
// "").
func (d *PostgresDriver) SetSchema(schema string) error {
	if schema == "" {
		return fmt.Errorf("set schema: %w: schema name", sqlgen.ErrEmptyIdentifier)
	}
	d.schemaMu.Lock()
	defer d.schemaMu.Unlock()
	d.schema = schema
	return nil
}

// CurrentSchema returns the current schema ("public" until the first
// SetSchema).
func (d *PostgresDriver) CurrentSchema() string {
	d.schemaMu.RLock()
	defer d.schemaMu.RUnlock()
	return d.schema
}

// Compile-time proof of the optional driver extension (§4.1).
var _ driver.SchemaAware = (*PostgresDriver)(nil)

// ListTables returns the tables and views of the current schema of the
// connected database, ordered by name. The pool is bound to one
// database, so db selects nothing on PostgreSQL — it is validated
// non-empty to keep the driver contract uniform, and the schema
// (SetSchema) picks the object set.
func (d *PostgresDriver) ListTables(ctx context.Context, db string) ([]driver.TableInfo, error) {
	if err := d.checkDatabase(db); err != nil {
		return nil, err
	}
	if db == "" {
		return nil, fmt.Errorf("list tables: %w: database name", sqlgen.ErrEmptyIdentifier)
	}

	rows, err := d.db.QueryContext(ctx, listTablesQuery, d.schemaFor(ctx))
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tables []driver.TableInfo
	for rows.Next() {
		var (
			t       driver.TableInfo
			comment sql.NullString
		)
		if err := rows.Scan(&t.Name, &t.Rows, &t.SizeBytes, &comment, &t.IsView); err != nil {
			return nil, fmt.Errorf("scan table row: %w", err)
		}
		t.Comment = comment.String // PostgreSQL has no engines; Engine stays ""
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tables: %w", err)
	}
	return tables, nil
}

// PrimaryKey returns the primary key column names of table in key order;
// a table without a primary key (or a missing table) yields an empty
// slice, not an error. Empty db or table names fail before any query.
func (d *PostgresDriver) PrimaryKey(ctx context.Context, db, table string) ([]string, error) {
	if err := d.checkDatabase(db); err != nil {
		return nil, err
	}
	if db == "" {
		return nil, fmt.Errorf("read primary key: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return nil, fmt.Errorf("read primary key: %w: table name", sqlgen.ErrEmptyIdentifier)
	}

	rows, err := d.db.QueryContext(ctx, primaryKeyQuery, d.schemaFor(ctx), table)
	if err != nil {
		return nil, fmt.Errorf("query primary key of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var pk []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan primary key column of %s: %w", table, err)
		}
		pk = append(pk, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate primary key of %s: %w", table, err)
	}
	return pk, nil
}

// schemaFor never mutates the session default while handling a request.
func (d *PostgresDriver) schemaFor(ctx context.Context) string {
	if schema := driver.SchemaFromContext(ctx); schema != "" {
		return schema
	}
	return d.CurrentSchema()
}
