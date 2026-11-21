package mysql

import (
	"context"
	"database/sql"
	dbdriver "database/sql/driver"
	"fmt"
	"strings"
	"time"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/sqlgen"
)

var _ driver.DatabaseManager = (*MysqlDriver)(nil)
var _ driver.TableTransfer = (*MysqlDriver)(nil)
var _ driver.MetadataProvider = (*MysqlDriver)(nil)

func databaseName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("database name: %w", sqlgen.ErrEmptyIdentifier)
	}
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("database name contains NUL")
	}
	return nil
}

// CreateDatabase creates a database with a collation validated by the server.
func (d *MysqlDriver) CreateDatabase(ctx context.Context, name, collation string) error {
	if err := databaseName(name); err != nil {
		return err
	}
	clause := ""
	if collation != "" {
		// COLLATE also selects its character set. Let the server resolve names:
		// MariaDB exposes UCA collations in SCHEMATA under charset-qualified
		// aliases that are absent from information_schema.COLLATIONS.
		clause = " COLLATE " + sqlgen.QuoteIdent(collation)
	}
	_, err := d.db.ExecContext(ctx, "CREATE DATABASE "+sqlgen.QuoteIdent(name)+clause)
	return err
}

// DropDatabase drops a database without silently ignoring missing names.
func (d *MysqlDriver) DropDatabase(ctx context.Context, name string) error {
	if name != "" && name == d.loginDB {
		return fmt.Errorf("reconnect without a default database before dropping the session database")
	}
	if err := databaseName(name); err != nil {
		return err
	}
	_, err := d.db.ExecContext(ctx, "DROP DATABASE "+sqlgen.QuoteIdent(name))
	return err
}

// RenameDatabase copies first and drops only after every table has copied.
// Unsupported schema objects are refused before mutation, since LIKE cannot
// preserve them. Source WRITE locks keep concurrent writes from being lost.
func (d *MysqlDriver) RenameDatabase(ctx context.Context, source, target string) error {
	if source != "" && source == d.loginDB {
		return fmt.Errorf("reconnect without a default database before renaming the session database")
	}
	if err := validateTransfer(source, nil, target); err != nil {
		return err
	}
	var collation string
	if err := d.db.QueryRowContext(ctx, "SELECT DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?", source).Scan(&collation); err != nil {
		return fmt.Errorf("source database: %w", err)
	}
	var count int
	// Objects other than base tables must never disappear during rename.
	for _, q := range []string{
		"SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_TYPE <> 'BASE TABLE'",
		"SELECT COUNT(*) FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA = ?",
		"SELECT COUNT(*) FROM information_schema.EVENTS WHERE EVENT_SCHEMA = ?",
		"SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA = ?",
		"SELECT COUNT(*) FROM information_schema.KEY_COLUMN_USAGE WHERE REFERENCED_TABLE_SCHEMA = ?",
	} {
		if err := d.db.QueryRowContext(ctx, q, source).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("rename refused: database contains views, routines, events, triggers or foreign-key references; use an SQL export/import preserving these objects")
		}
	}
	tables, err := d.ListTables(ctx, source)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(tables))
	for _, t := range tables {
		names = append(names, t.Name)
	}
	if err := d.validateCopy(ctx, source, names, target); err != nil {
		return err
	}
	if err := d.CreateDatabase(ctx, target, collation); err != nil {
		return err
	}
	// A failed copy retains the source. The newly created (empty) database
	// may remain when cleanup is denied; an existing target is never reused.
	return d.copyTables(ctx, source, names, target, true)
}

func validateTransfer(source string, tables []string, target string) error {
	if err := databaseName(source); err != nil {
		return err
	}
	if err := databaseName(target); err != nil {
		return err
	}
	if source == target {
		return fmt.Errorf("source and target databases must differ")
	}
	seen := map[string]bool{}
	for _, name := range tables {
		if err := databaseName(name); err != nil {
			return err
		}
		if seen[name] {
			return fmt.Errorf("duplicate table %s", name)
		}
		seen[name] = true
	}
	return nil
}

// validateCopy refuses every object CREATE LIKE would silently omit.
func (d *MysqlDriver) validateCopy(ctx context.Context, source string, tables []string, target string) error {
	for _, table := range tables {
		var typ string
		if err := d.db.QueryRowContext(ctx, "SELECT TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?", source, table).Scan(&typ); err != nil {
			return fmt.Errorf("source table %s: %w", table, err)
		}
		if typ != "BASE TABLE" {
			return fmt.Errorf("cannot copy view %s", table)
		}
		var count int
		if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.TABLE_CONSTRAINTS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND CONSTRAINT_TYPE = 'FOREIGN KEY'", source, table).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("copy refused: %s has foreign keys; CREATE LIKE would omit them", table)
		}
		if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA = ? AND EVENT_OBJECT_TABLE = ?", source, table).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("copy refused: %s has triggers", table)
		}
		if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?", target, table).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("target table %s already exists", table)
		}
	}
	return nil
}

// CopyTables copies table definitions and rows without overwriting existing tables.
func (d *MysqlDriver) CopyTables(ctx context.Context, source string, tables []string, target string) error {
	if err := validateTransfer(source, tables, target); err != nil {
		return err
	}
	if len(tables) == 0 {
		return fmt.Errorf("select at least one table")
	}
	if err := d.validateCopy(ctx, source, tables, target); err != nil {
		return err
	}
	return d.copyTables(ctx, source, tables, target, false)
}

// copyTables never overwrites a destination table. All DDL and data copying use
// one connection; generated columns are recalculated, rather than inserted.
func (d *MysqlDriver) copyTables(ctx context.Context, source string, tables []string, target string, dropSource bool) (err error) {
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	var originalMode string
	if err = conn.QueryRowContext(ctx, "SELECT @@SESSION.sql_mode").Scan(&originalMode); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, "SET SESSION sql_mode = ?", originalMode+",NO_AUTO_VALUE_ON_ZERO"); err != nil {
		return err
	}
	created := []string{}
	sourceRemoved := false
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Unlock before pool return even when cancellation killed the statement.
		_, unlockErr := conn.ExecContext(cleanup, "UNLOCK TABLES")
		if err != nil && !sourceRemoved {
			for _, table := range created {
				_, _ = conn.ExecContext(cleanup, "DROP TABLE "+qualifiedIdent(target, table))
			}
		}
		_, restoreErr := conn.ExecContext(cleanup, "SET SESSION sql_mode = ?", originalMode)
		if unlockErr != nil || restoreErr != nil {
			_ = conn.Raw(func(any) error { return dbdriver.ErrBadConn })
		}

	}()
	columns := map[string][]string{}
	for _, table := range tables {
		rows, e := conn.QueryContext(ctx, "SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COALESCE(GENERATION_EXPRESSION, '') = '' ORDER BY ORDINAL_POSITION", source, table)
		if e != nil {
			return e
		}
		for rows.Next() {
			var name string
			if e = rows.Scan(&name); e != nil {
				_ = rows.Close()
				return e
			}
			columns[table] = append(columns[table], sqlgen.QuoteIdent(name))
		}
		e = rows.Err()
		_ = rows.Close()
		if e != nil {
			return e
		}
		if len(columns[table]) == 0 {
			return fmt.Errorf("no writable columns for %s", table)
		}
		if _, e = conn.ExecContext(ctx, "CREATE TABLE "+qualifiedIdent(target, table)+" LIKE "+qualifiedIdent(source, table)); e != nil {
			return e
		}
		created = append(created, table)
		var next sql.NullInt64
		if e = conn.QueryRowContext(ctx, "SELECT AUTO_INCREMENT FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?", source, table).Scan(&next); e != nil {
			return e
		}
		if next.Valid && next.Int64 > 0 {
			if _, e = conn.ExecContext(ctx, "ALTER TABLE "+qualifiedIdent(target, table)+fmt.Sprintf(" AUTO_INCREMENT = %d", next.Int64)); e != nil {
				return e
			}
		}

	}
	if len(tables) > 0 {
		locks := []string{}
		for _, table := range tables {
			locks = append(locks, qualifiedIdent(source, table)+" WRITE", qualifiedIdent(target, table)+" WRITE")
		}
		if _, err = conn.ExecContext(ctx, "LOCK TABLES "+strings.Join(locks, ", ")); err != nil {
			return err
		}
	}
	for _, table := range tables {
		cols := strings.Join(columns[table], ", ")
		if _, err = conn.ExecContext(ctx, "INSERT INTO "+qualifiedIdent(target, table)+" ("+cols+") SELECT "+cols+" FROM "+qualifiedIdent(source, table)); err != nil {
			return fmt.Errorf("copy %s: %w", table, err)
		}
	}
	if dropSource {
		// DROP DATABASE is illegal while LOCK TABLES is active. Remove all
		// copied source tables in ONE statement under their WRITE locks, then
		// drop the empty schema. Once sources are removed destinations must
		// survive even if final schema cleanup fails.
		sourceRemoved = true // An interrupted DROP may have committed: retain destination.
		if len(tables) > 0 {
			names := make([]string, 0, len(tables))
			for _, table := range tables {
				names = append(names, qualifiedIdent(source, table))
			}
			if _, err = conn.ExecContext(ctx, "DROP TABLE "+strings.Join(names, ", ")); err != nil {
				return fmt.Errorf("data copied to %s; source deletion failed or is uncertain: %w", target, err)
			}
		}
		if _, err = conn.ExecContext(ctx, "UNLOCK TABLES"); err != nil {
			return fmt.Errorf("data copied to %s; source schema cleanup failed: %w", target, err)
		}
		var remaining int
		if err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ?", source).Scan(&remaining); err != nil {
			return err
		}
		if remaining > 0 {
			return fmt.Errorf("data copied to %s; source retained because objects were added concurrently", target)
		}
		if _, err = conn.ExecContext(ctx, "DROP DATABASE "+sqlgen.QuoteIdent(source)); err != nil {
			return fmt.Errorf("data copied to %s; empty source database retained: %w", target, err)
		}
	}
	return nil
}

// MoveTables uses one atomic native RENAME statement: no partial moves.
func (d *MysqlDriver) MoveTables(ctx context.Context, source string, tables []string, target string) error {
	if err := validateTransfer(source, tables, target); err != nil {
		return err
	}
	if len(tables) == 0 {
		return fmt.Errorf("select at least one table")
	}
	pairs := make([]string, 0, len(tables))
	for _, table := range tables {
		pairs = append(pairs, qualifiedIdent(source, table)+" TO "+qualifiedIdent(target, table))
	}
	_, err := d.db.ExecContext(ctx, "RENAME TABLE "+strings.Join(pairs, ", "))
	return err
}

// Metadata returns column types, database collations and browsing operators.
func (d *MysqlDriver) Metadata(ctx context.Context) (driver.Metadata, error) {
	meta := driver.Metadata{ColumnTypes: []driver.TypeInfo{}, Collations: []driver.Collation{}, Operators: []driver.Operator{}}
	for _, group := range []struct {
		category string
		names    []string
		length   bool
	}{
		{"number", []string{"tinyint", "smallint", "mediumint", "int", "bigint", "decimal", "float", "double"}, true},
		{"text", []string{"char", "varchar", "binary", "varbinary"}, true},
		{"text", []string{"tinytext", "text", "mediumtext", "longtext", "tinyblob", "blob", "mediumblob", "longblob", "enum", "set", "json"}, false},
		{"time", []string{"date", "datetime", "timestamp", "time", "year"}, false},
		{"spatial", []string{"geometry", "point", "linestring", "polygon", "multipoint", "multilinestring", "multipolygon", "geometrycollection"}, false},
	} {
		for _, name := range group.names {
			meta.ColumnTypes = append(meta.ColumnTypes, driver.TypeInfo{Name: name, Category: group.category, HasLength: group.length})
		}
	}
	rows, err := d.db.QueryContext(ctx, "SELECT COLLATION_NAME, CHARACTER_SET_NAME FROM information_schema.COLLATIONS WHERE CHARACTER_SET_NAME IS NOT NULL ORDER BY COLLATION_NAME")
	if err != nil {
		return meta, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c driver.Collation
		c.Database = true
		if err := rows.Scan(&c.Name, &c.Charset); err != nil {
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
