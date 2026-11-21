//go:build integration

package mysql

import (
	"context"
	"net"
	"strings"
	"testing"

	sqldriver "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/dblama-dev/dblama/internal/driver"
)

// testRootPassword is the root password set on every test container.
const testRootPassword = "dblama-test"

// testImages is the integration matrix from the phase 1 plan: image →
// expected Info().Flavor.
var testImages = []struct {
	image  string
	flavor string
}{
	{"mysql:8.4", FlavorMySQL},
	{"mariadb:11.4", FlavorMariaDB},
}

// startDB boots image as a testcontainer and returns the root
// ConnConfig derived from the container's connection string. It skips
// the calling test in -short mode.
func startDB(t *testing.T, image string) driver.ConnConfig {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	ctx := context.Background()
	ctr, err := tcmysql.Run(ctx, image,
		tcmysql.WithUsername("root"),
		tcmysql.WithPassword(testRootPassword), // → env MYSQL_ROOT_PASSWORD
		// MySQL and MariaDB both log "port: 3306" only when the networked
		// server is up; the module default waits for a MySQL-only message.
		testcontainers.WithWaitStrategy(wait.ForLog("port: 3306")),
	)
	if err != nil {
		t.Fatalf("start %s container: %v", image, err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	dsn, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("container connection string: %v", err)
	}
	parsed, err := sqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse container connection string: %v", err)
	}
	host, port, err := net.SplitHostPort(parsed.Addr)
	if err != nil {
		t.Fatalf("split container address: %v", err)
	}
	return driver.ConnConfig{
		Host:     host,
		Port:     port,
		User:     parsed.User,
		Password: parsed.Passwd,
		Database: parsed.DBName,
	}
}

func connectOK(t *testing.T, cfg driver.ConnConfig) driver.Driver {
	t.Helper()
	drv, err := Connect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	t.Cleanup(func() { _ = drv.Close() })
	return drv
}

func TestConnect_OK(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			info := drv.Info()
			if info.Flavor != tc.flavor {
				t.Errorf("Info().Flavor = %q, want %q", info.Flavor, tc.flavor)
			}
		})
	}
}

func TestConnect_WrongPassword(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			cfg.Password = "definitely-wrong"

			drv, err := Connect(context.Background(), cfg)
			if err == nil {
				_ = drv.Close()
				t.Fatal("Connect() with wrong password: expected error, got nil")
			}
			// Anti-regression: error text (which may embed the DSN) must
			// not leak the passwords — neither the wrong one passed to
			// Connect nor the container's real root password.
			if strings.Contains(err.Error(), "definitely-wrong") {
				t.Errorf("Connect() error %q leaks the wrong password", err)
			}
			if strings.Contains(err.Error(), testRootPassword) {
				t.Errorf("Connect() error %q leaks the root password", err)
			}
		})
	}
}

func TestListDatabases(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			dbs, err := drv.ListDatabases(context.Background())
			if err != nil {
				t.Fatalf("ListDatabases() error: %v", err)
			}

			byName := make(map[string]driver.DatabaseInfo, len(dbs))
			names := make([]string, 0, len(dbs))
			for _, db := range dbs {
				if db.SizeBytes < 0 {
					t.Errorf("database %q SizeBytes = %d, want >= 0", db.Name, db.SizeBytes)
				}
				byName[db.Name] = db
				names = append(names, db.Name)
			}
			for _, want := range []string{"information_schema", "mysql", "performance_schema"} {
				if _, ok := byName[want]; !ok {
					t.Errorf("ListDatabases() missing %q, got %v", want, names)
				}
			}
			if c := byName["information_schema"].Collation; c == "" {
				t.Error("information_schema Collation is empty, want non-empty")
			}
		})
	}
}

func TestClose(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			if err := drv.Close(); err != nil {
				t.Fatalf("first Close() error: %v", err)
			}
			if err := drv.Close(); err != nil {
				t.Fatalf("second Close() error: %v", err)
			}
		})
	}
}
