package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
	"github.com/jackc/pgx/v5"
)

var _ driver.DatabaseManager = (*PostgresDriver)(nil)
var _ driver.MetadataProvider = (*PostgresDriver)(nil)

func validDatabase(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("database name: %w", sqlgen.ErrEmptyIdentifier)
	}
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("database name contains NUL")
	}
	return nil
}

// adminConnection creates a short-lived maintenance connection with the
// session's original connection settings and credentials.
func (d *PostgresDriver) adminConnection(ctx context.Context, target string) (*pgx.Conn, error) {
	s := d.session
	s.mu.Lock()
	if s.scopes == nil {
		s.mu.Unlock()
		return nil, errSessionClosed
	}
	cfg := s.config.Copy()
	s.mu.Unlock()
	cfg.Database = "postgres"
	if target == "postgres" {
		cfg.Database = "template1"
	}
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("administrative connection failed; reconnect to a maintenance database")
	}
	return admin, nil
}

// CreateDatabase creates a database with a verified server collation.
func (d *PostgresDriver) CreateDatabase(ctx context.Context, name, collation string) error {
	if err := validDatabase(name); err != nil {
		return err
	}
	clause := ""
	if collation != "" {
		var found bool
		if err := d.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_collation WHERE collcollate = $1 AND collprovider = 'c')", collation).Scan(&found); err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("unknown database locale %q", collation)
		}
		// CREATE DATABASE parameters do not support bind placeholders; a verified
		// server locale is encoded as a SQL literal, identifiers use sqlgen.
		literal := "'" + strings.ReplaceAll(collation, "'", "''") + "'"
		clause = " TEMPLATE template0 LC_COLLATE " + literal + " LC_CTYPE " + literal
	}
	admin, err := d.adminConnection(ctx, name)
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close(context.Background()) }()
	_, err = admin.Exec(ctx, "CREATE DATABASE "+sqlgen.QuoteIdentPg(name)+clause)
	return err
}

// DropDatabase drops a database without silently ignoring missing names.
func (d *PostgresDriver) DropDatabase(ctx context.Context, name string) error {
	if err := validDatabase(name); err != nil {
		return err
	}
	if name == d.loginDB {
		return fmt.Errorf("reconnect to another database before dropping the session database")
	}
	admin, err := d.adminConnection(ctx, name)
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close(context.Background()) }()
	_, err = admin.Exec(ctx, "DROP DATABASE "+sqlgen.QuoteIdentPg(name))
	return err
}

// RenameDatabase renames a database through a separate administrative connection.
func (d *PostgresDriver) RenameDatabase(ctx context.Context, source, target string) error {
	if err := validDatabase(source); err != nil {
		return err
	}
	if err := validDatabase(target); err != nil {
		return err
	}
	if source == target {
		return fmt.Errorf("source and target databases must differ")
	}
	if source == d.loginDB {
		return fmt.Errorf("reconnect to another database before renaming the session database")
	}
	admin, err := d.adminConnection(ctx, source)
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close(context.Background()) }()
	_, err = admin.Exec(ctx, "ALTER DATABASE "+sqlgen.QuoteIdentPg(source)+" RENAME TO "+sqlgen.QuoteIdentPg(target))
	return err
}

// Metadata returns column types, database collations and browsing operators.
func (d *PostgresDriver) Metadata(ctx context.Context) (driver.Metadata, error) {
	meta := driver.Metadata{ColumnTypes: []driver.TypeInfo{}, Collations: []driver.Collation{}, Operators: []driver.Operator{}}
	rows, err := d.db.QueryContext(ctx, "SELECT typname, typcategory::text FROM pg_type t JOIN pg_namespace n ON n.oid=t.typnamespace WHERE n.nspname='pg_catalog' AND typtype IN ('b','e') AND typname NOT LIKE '\\_%' AND typcategory NOT IN ('A','P') ORDER BY typname")
	if err != nil {
		return meta, err
	}
	for rows.Next() {
		var name, category string
		if err := rows.Scan(&name, &category); err != nil {
			_ = rows.Close()
			return meta, err
		}
		meta.ColumnTypes = append(meta.ColumnTypes, driver.TypeInfo{Name: name, Category: category, HasLength: name == "varchar" || name == "bpchar" || name == "numeric"})
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return meta, err
	}
	rows, err = d.db.QueryContext(ctx, "SELECT DISTINCT collcollate FROM pg_collation WHERE collprovider='c' AND collcollate IS NOT NULL ORDER BY collcollate")
	if err != nil {
		return meta, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c driver.Collation
		c.Database = true
		if err := rows.Scan(&c.Name); err != nil {
			return meta, err
		}
		meta.Collations = append(meta.Collations, c)
	}
	for _, op := range sqlgen.AllowedOperators() {
		arity := 1
		if op == "IS NULL" || op == "IS NOT NULL" {
			arity = 0
		}
		meta.Operators = append(meta.Operators, driver.Operator{Value: op, Label: op, Arity: arity})
	}
	return meta, rows.Err()
}
