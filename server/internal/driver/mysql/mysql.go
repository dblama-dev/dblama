// Package mysql implements the db-manager driver for MySQL and MariaDB.
// The two servers share one wire protocol; the flavor is detected after
// connecting via SELECT VERSION().
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"

	_ "github.com/go-sql-driver/mysql" // registers the database/sql driver "mysql"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// maxOpenConns caps the connection pool per driver instance (spec §4.1).
const maxOpenConns = 4

// listDatabasesQuery returns every schema with its total table data and
// index size and its default collation.
const listDatabasesQuery = `
SELECT s.SCHEMA_NAME,
       COALESCE(SUM(t.DATA_LENGTH + t.INDEX_LENGTH), 0),
       s.DEFAULT_COLLATION_NAME
FROM information_schema.SCHEMATA s
LEFT JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = s.SCHEMA_NAME
GROUP BY s.SCHEMA_NAME, s.DEFAULT_COLLATION_NAME
ORDER BY s.SCHEMA_NAME`

// MysqlDriver is a connection-bound MySQL/MariaDB driver instance.
// Outside this package it is consumed only as a driver.Driver.
//
//nolint:revive // name mirrors the package, "MySQL" would collide with the driver name
type MysqlDriver struct {
	db   *sql.DB
	info driver.DriverInfo
	caps driver.Caps

	// lastInsertID holds the auto_increment value of the most recent
	// row write (InsertRow/UpsertRows). The mutex guards it because a
	// driver instance is shared across requests.
	writeMu      sync.Mutex
	lastInsertID int64
}

var _ driver.Driver = (*MysqlDriver)(nil)

// Connect opens a pooled connection to a MySQL or MariaDB server,
// verifies it with a ping, and detects flavor and version. Connection
// errors never contain the DSN or the password.
func Connect(ctx context.Context, cfg driver.ConnConfig) (driver.Driver, error) {
	db, err := sql.Open("mysql", DSN(cfg, cfg.Password))
	if err != nil {
		return nil, fmt.Errorf("open mysql connection to %s:%s: %w", cfg.Host, cfg.Port, err)
	}
	db.SetMaxOpenConns(maxOpenConns)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close() // best-effort; the ping error is the real failure
		return nil, fmt.Errorf("connect to %s:%s as %s: %w", cfg.Host, cfg.Port, cfg.User, err)
	}

	var versionRaw string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&versionRaw); err != nil {
		_ = db.Close() // best-effort; the query error is the real failure
		return nil, fmt.Errorf("read server version on %s:%s: %w", cfg.Host, cfg.Port, err)
	}

	flavor, version := ParseFlavor(versionRaw)
	return &MysqlDriver{
		db:   db,
		info: driver.DriverInfo{Name: serverName(flavor), Flavor: flavor, Version: version, User: cfg.User, Host: cfg.Host},
		caps: capsFor(flavor, version),
	}, nil
}

// Info describes the connected server.
func (d *MysqlDriver) Info() driver.DriverInfo { return d.info }

// Caps declares the features supported by the connected server.
func (d *MysqlDriver) Caps() driver.Caps { return d.caps }

// ListDatabases returns all databases on the server with their sizes in
// bytes and default collations.
func (d *MysqlDriver) ListDatabases(ctx context.Context) ([]driver.DatabaseInfo, error) {
	rows, err := d.db.QueryContext(ctx, listDatabasesQuery)
	if err != nil {
		return nil, fmt.Errorf("query databases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var dbs []driver.DatabaseInfo
	for rows.Next() {
		var (
			name      string
			size      int64
			collation sql.NullString
		)
		if err := rows.Scan(&name, &size, &collation); err != nil {
			return nil, fmt.Errorf("scan database row: %w", err)
		}
		dbs = append(dbs, driver.DatabaseInfo{Name: name, SizeBytes: size, Collation: collation.String})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate databases: %w", err)
	}
	return dbs, nil
}

// Close closes the underlying connection pool. It is safe to call more
// than once: *sql.DB.Close is idempotent.
func (d *MysqlDriver) Close() error { return d.db.Close() }

// serverName returns the human-readable server name for a flavor.
func serverName(flavor string) string {
	if flavor == FlavorMariaDB {
		return "MariaDB"
	}
	return "MySQL"
}

// capsFor fills the caps for the connected server: everything except
// Sequences and Schemas is supported; descending indexes need MySQL 8+.
func capsFor(flavor, version string) driver.Caps {
	majorStr, _, _ := strings.Cut(version, ".")
	major, _ := strconv.Atoi(majorStr)
	return driver.Caps{
		Views:      true,
		Triggers:   true,
		Routines:   true,
		Procedures: true,
		Events:     true,
		Partitions: true,
		Users:      true,
		Monitor:    true,
		MoveCopy:   true,
		Comments:   true,
		Upsert:     true,
		// Sequences and Schemas stay false.
		DescendingIndexes: flavor == FlavorMySQL && major >= 8,
	}
}
