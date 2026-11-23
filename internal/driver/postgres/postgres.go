// Package postgres implements the dblama driver for PostgreSQL on
// jackc/pgx's database/sql adapter (stdlib): the same pooling and
// scanning shape as the MySQL driver, speaking PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/sqlgen"
)

// maxOpenConns caps the connection pool per driver instance (spec §4.1).
const maxOpenConns = 4

// defaultSchema is the schema a fresh driver instance reads and writes
// until the API layer selects another one through SchemaAware.SetSchema.
const defaultSchema = "public"

var errSessionClosed = errors.New("postgres session is closed")

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

	// database is the actual database this driver's pool is bound to.
	// loginDB is the session database protected from DROP and RENAME.
	database string
	loginDB  string
	session  *databaseSession

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

	// serialPK caches per table ("schema.table") the single-column
	// server-generated primary key column ("" when there is none) — the
	// RETURNING target of InsertRow, detected once per table (Ruling 2).
	// serialMu guards it; the save paths drop entries their ALTERs may
	// have invalidated.
	serialMu sync.Mutex
	serialPK map[string]string
}

// databaseSession holds the original connection settings and the running
// console queries shared between the root driver and request scopes.
type databaseSession struct {
	config  *pgx.ConnConfig
	root    *sql.DB
	console consoleRegistry
	slots   chan struct{} // one slot per active alternate database pool
	done    chan struct{} // closed during session teardown to wake waiters
	mu      sync.Mutex
	scopes  map[*sql.DB]struct{} // nil after the session closes
}

var _ driver.Driver = (*PostgresDriver)(nil)
var _ driver.DatabaseConnector = (*PostgresDriver)(nil)

// Connect opens a pooled connection to a PostgreSQL server, verifies it
// with a ping, and reads the server version. Connection errors never
// contain the DSN or the password.
func Connect(ctx context.Context, cfg driver.ConnConfig) (driver.Driver, error) {
	if cfg.Port == "" {
		cfg.Port = "5432"
	}
	if strings.ContainsRune(cfg.Database, 0) {
		return nil, fmt.Errorf("database name contains NUL")
	}
	parsed, err := parseConnectionConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection to %s:%s: %w", cfg.Host, cfg.Port, err)
	}

	db, err := openDatabase(ctx, parsed)
	if cfg.Database == "" && canUseMaintenanceDatabase(err) {
		parsed = parsed.Copy()
		parsed.Database = "postgres"
		db, err = openDatabase(ctx, parsed)
	}
	if err != nil {
		return nil, fmt.Errorf("connect to %s:%s as %s: %w", cfg.Host, cfg.Port, cfg.User, err)
	}

	var versionRaw, database string
	if err := db.QueryRowContext(ctx, "SELECT pg_catalog.version(), pg_catalog.current_database()").Scan(&versionRaw, &database); err != nil {
		_ = db.Close() // best-effort; the query error is the real failure
		return nil, fmt.Errorf("read server information on %s:%s: %w", cfg.Host, cfg.Port, err)
	}

	return &PostgresDriver{
		db:       db,
		info:     driver.DriverInfo{Name: "PostgreSQL", Flavor: "postgres", Version: parseVersion(versionRaw), User: cfg.User, Host: cfg.Host, Port: cfg.Port},
		caps:     postgresCaps(),
		database: database,
		loginDB:  database,
		session: &databaseSession{
			config: parsed,
			root:   db,
			slots:  make(chan struct{}, maxOpenConns),
			done:   make(chan struct{}),
			scopes: make(map[*sql.DB]struct{}),
		},
		schema:   defaultSchema,
		serialPK: make(map[string]string),
	}, nil
}

func openDatabase(ctx context.Context, cfg *pgx.ConnConfig) (*sql.DB, error) {
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(maxOpenConns)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func canUseMaintenanceDatabase(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case "3D000", "42501", "55000": // missing, CONNECT denied, or connections disabled
		return true
	}
	return false
}

// ForDatabase binds a driver to the named database for one request. The
// caller releases its pool after the request, including unsuccessful operations.
func (d *PostgresDriver) ForDatabase(ctx context.Context, name string) (driver.Driver, func(), error) {
	if name == "" {
		return nil, nil, fmt.Errorf("database name: %w", sqlgen.ErrEmptyIdentifier)
	}
	if strings.ContainsRune(name, 0) {
		return nil, nil, fmt.Errorf("database name contains NUL")
	}
	s := d.session
	if name == d.database {
		select {
		case <-s.done:
			return nil, nil, errSessionClosed
		default:
			return d, func() {}, nil
		}
	}
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-s.done:
		return nil, nil, errSessionClosed
	case s.slots <- struct{}{}:
	}
	s.mu.Lock()
	if s.scopes == nil {
		s.mu.Unlock()
		<-s.slots
		return nil, nil, errSessionClosed
	}

	cfg := s.config.Copy()
	cfg.Database = name
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(maxOpenConns)
	s.scopes[db] = struct{}{}
	s.mu.Unlock()

	bound := &PostgresDriver{
		db:       db,
		info:     d.info,
		caps:     d.caps,
		database: name,
		loginDB:  d.loginDB,
		session:  s,
		schema:   d.CurrentSchema(),
		serialPK: make(map[string]string),
	}
	release := func() { _ = bound.Close() }
	var current string
	if err := db.QueryRowContext(ctx, "SELECT pg_catalog.current_database()").Scan(&current); err != nil {
		release()
		return nil, nil, fmt.Errorf("connect to database %q: %w", name, err)
	}
	if current != name {
		release()
		return nil, nil, fmt.Errorf("requested database %q but server selected %q", name, current)
	}
	return bound, release, nil
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

// Close releases this driver's pool. Closing the session's root driver
// also closes any outstanding request pools and prevents new scopes.
func (d *PostgresDriver) Close() error {
	s := d.session
	if d.db != s.root {
		err := d.db.Close()
		s.mu.Lock()
		_, tracked := s.scopes[d.db]
		delete(s.scopes, d.db)
		s.mu.Unlock()
		if tracked {
			<-s.slots
		}
		return err
	}
	s.mu.Lock()
	scopes := s.scopes
	if scopes != nil {
		s.scopes = nil
		close(s.done)
	}
	s.mu.Unlock()

	err := d.db.Close()
	for db := range scopes {
		err = errors.Join(err, db.Close())
	}
	return err
}

// parseVersion extracts "17.2" from the full version() banner. A banner
// without the expected shape falls back to the trimmed raw string rather
// than failing the connection.
func parseVersion(banner string) string {
	if m := versionPattern.FindStringSubmatch(banner); len(m) == 2 {
		return m[1]
	}
	return strings.TrimSpace(banner)
}

// postgresCaps advertises the operations implemented by this driver.
func postgresCaps() driver.Caps {
	return driver.Caps{
		Views:      true,
		Triggers:   true,
		Routines:   true,
		Procedures: true,
		Monitor:    true,
		Roles:      true,
		Comments:   true,
		Upsert:     true,
		Sequences:  true,
		Schemas:    true,
		// Events, Partitions, Users, MoveCopy and DescendingIndexes stay
		// false.
	}
}

// PostgreSQL pools are bound to a single database.
func (d *PostgresDriver) checkDatabase(db string) error {
	if db != "" && db != d.database {
		return fmt.Errorf("database %q does not match the connection database %q", db, d.database)
	}
	return nil
}
