// Package postgres implements the db-manager driver for PostgreSQL on
// jackc/pgx's database/sql adapter (stdlib): the same pooling and
// scanning shape as the MySQL driver, speaking PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"sync"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the database/sql driver "pgx"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// maxOpenConns caps the connection pool per driver instance (spec §4.1).
const maxOpenConns = 4

// defaultSchema is the schema a fresh driver instance reads and writes
// until the API layer selects another one through SchemaAware.SetSchema.
const defaultSchema = "public"

// listDatabasesQuery returns every connectable database except the
// templates, with its total on-disk size and its collation.
const listDatabasesQuery = `
SELECT datname, CASE WHEN has_database_privilege(oid, 'CONNECT') THEN pg_database_size(oid) ELSE 0 END, datcollate
FROM pg_database
WHERE datallowconn AND datname NOT IN ('template0', 'template1')
ORDER BY datname`

// versionPattern extracts the major.minor version from the banner
// SELECT version() returns ("PostgreSQL 17.2 (Debian 17.2-1.pgdg120+1)
// on x86_64-pc-linux-gnu, ...").
var versionPattern = regexp.MustCompile(`PostgreSQL (\d+\.\d+)`)

// PostgresDriver is a connection-bound PostgreSQL driver instance.
// Outside this package it is consumed only as a driver.Driver (and as a
// driver.SchemaAware — HTTP handlers bind schema to request context).
//
//nolint:revive // name mirrors the package, "Driver" would collide with the driver contract type
type PostgresDriver struct {
	db   *sql.DB
	info driver.DriverInfo
	caps driver.Caps

	// loginDB is the database the pool's DSN is bound to (the same
	// one-database-per-pool model as the MySQL driver); the console and
	// import paths (task 5) use it when a dedicated connection must
	// target another database.
	loginDB string

	// schema is the current schema every catalog and data query qualifies
	// inside its SQL text (spec §13.5 — a shared pool must not SET
	// search_path). schemaMu guards it; one driver instance serves one
	// API session. HTTP requests override this default through their context.
	schema   string
	schemaMu sync.RWMutex

	// writeMu guards lastInsertID — the id captured from the RETURNING
	// clause of the most recent InsertRow (0 when that insert generated
	// none), read back through LastInsertID.
	writeMu      sync.Mutex
	lastInsertID int64

	// console tracks running console queries for CancelQuery.
	console consoleRegistry

	// serialPK caches per table ("schema.table") the single-column
	// server-generated primary key column ("" when there is none) — the
	// RETURNING target of InsertRow, detected once per table (Ruling 2).
	// serialMu guards it; the save paths drop entries their ALTERs may
	// have invalidated.
	serialMu sync.Mutex
	serialPK map[string]string
}

var _ driver.Driver = (*PostgresDriver)(nil)

// Connect opens a pooled connection to a PostgreSQL server, verifies it
// with a ping, and reads the server version. Connection errors never
// contain the DSN or the password.
func Connect(ctx context.Context, cfg driver.ConnConfig) (driver.Driver, error) {
	db, err := openDatabase(cfg)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection to %s:%s: %w", cfg.Host, cfg.Port, err)
	}
	db.SetMaxOpenConns(maxOpenConns)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close() // best-effort; the ping error is the real failure
		return nil, fmt.Errorf("connect to %s:%s as %s: %w", cfg.Host, cfg.Port, cfg.User, err)
	}

	var versionRaw string
	if err := db.QueryRowContext(ctx, "SELECT version()").Scan(&versionRaw); err != nil {
		_ = db.Close() // best-effort; the query error is the real failure
		return nil, fmt.Errorf("read server version on %s:%s: %w", cfg.Host, cfg.Port, err)
	}

	return &PostgresDriver{
		db:       db,
		info:     driver.DriverInfo{Name: "PostgreSQL", Flavor: "postgres", Version: parseVersion(versionRaw), User: cfg.User, Host: cfg.Host},
		caps:     postgresCaps(),
		loginDB:  cfg.Database,
		schema:   defaultSchema,
		serialPK: make(map[string]string),
	}, nil
}

// Info describes the connected server.
func (d *PostgresDriver) Info() driver.DriverInfo { return d.info }

// Caps declares the features supported by the connected server.
func (d *PostgresDriver) Caps() driver.Caps { return d.caps }

// ListDatabases returns every connectable database on the server with its
// size in bytes and its collation. The template databases are excluded:
// they exist as templates, not as connection targets.
func (d *PostgresDriver) ListDatabases(ctx context.Context) ([]driver.DatabaseInfo, error) {
	rows, err := d.db.QueryContext(ctx, listDatabasesQuery)
	if err != nil {
		return nil, fmt.Errorf("query databases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var dbs []driver.DatabaseInfo
	for rows.Next() {
		var db driver.DatabaseInfo
		if err := rows.Scan(&db.Name, &db.SizeBytes, &db.Collation); err != nil {
			return nil, fmt.Errorf("scan database row: %w", err)
		}
		dbs = append(dbs, db)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate databases: %w", err)
	}
	return dbs, nil
}

// Close closes the underlying connection pool. It is safe to call more
// than once: *sql.DB.Close is idempotent.
func (d *PostgresDriver) Close() error { return d.db.Close() }

// parseVersion extracts "17.2" from the full version() banner. A banner
// without the expected shape falls back to the trimmed raw string rather
// than failing the connection.
func parseVersion(banner string) string {
	if m := versionPattern.FindStringSubmatch(banner); len(m) == 2 {
		return m[1]
	}
	return strings.TrimSpace(banner)
}

// postgresCaps fills the caps for the connected server: views, triggers,
// routines and procedures, monitoring, comments, upsert (ON CONFLICT),
// sequences and schemas are supported. Events and users are MySQL-only
// concepts; partitions, move/copy database and descending indexes stay
// off in v0.1 (phase 4 plan).
func postgresCaps() driver.Caps {
	return driver.Caps{
		Views:      true,
		Triggers:   true,
		Routines:   true,
		Procedures: true,
		Monitor:    true,
		Comments:   true,
		Upsert:     true,
		Sequences:  true,
		Schemas:    true,
		// Events, Partitions, Users, MoveCopy and DescendingIndexes stay
		// false.
	}
}

// PostgreSQL pools cannot switch databases.
func (d *PostgresDriver) checkDatabase(db string) error {
	if db != "" && db != d.loginDB {
		return fmt.Errorf("database %s is not the login database %s; reconnect to select another database", db, d.loginDB)
	}
	return nil
}
