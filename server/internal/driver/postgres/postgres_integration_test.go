//go:build integration

package postgres

import (
	"context"
	"net/url"
	"strings"
	"testing"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// The phase 4 integration matrix pins a single image: postgres:17,
// started as the superuser postgres with the shared test password and
// the default postgres database.
const (
	testImage    = "postgres:17"
	testUser     = "postgres"
	testPassword = "dbmanager-test"
	testDatabase = "postgres"
	testFlavor   = "postgres"
)

// startDB boots postgres:17 as a testcontainer and returns the superuser
// ConnConfig derived from the container's connection string. It skips the
// calling test in -short mode. Connection-string errors never print the
// string itself — it embeds the password.
func startDB(t *testing.T) driver.ConnConfig {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, testImage,
		tcpostgres.WithUsername(testUser),
		tcpostgres.WithPassword(testPassword),
		tcpostgres.WithDatabase(testDatabase),
		// The module does not wait by default; postgres logs "ready to
		// accept connections" once, restarts after initdb and only then
		// serves — connecting in between yields connection resets.
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start %s container: %v", testImage, err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("container connection string: %v", err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse container connection string: %v", err)
	}
	password, _ := u.User.Password()
	return driver.ConnConfig{
		Host:     u.Hostname(),
		Port:     u.Port(),
		User:     u.User.Username(),
		Password: password,
		Database: strings.TrimPrefix(u.Path, "/"),
	}
}

// connectOK connects with cfg and registers Close on cleanup.
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
	cfg := startDB(t)
	drv := connectOK(t, cfg)

	info := drv.Info()
	if info.Flavor != testFlavor {
		t.Errorf("Info().Flavor = %q, want %q", info.Flavor, testFlavor)
	}
	if !strings.HasPrefix(info.Version, "17.") {
		t.Errorf("Info().Version = %q, want major 17", info.Version)
	}
	if info.User != cfg.User {
		t.Errorf("Info().User = %q, want %q", info.User, cfg.User)
	}
	if info.Host != cfg.Host {
		t.Errorf("Info().Host = %q, want %q", info.Host, cfg.Host)
	}
}

func TestConnect_WrongPassword(t *testing.T) {
	cfg := startDB(t)
	cfg.Password = "definitely-wrong"

	drv, err := Connect(context.Background(), cfg)
	if err == nil {
		_ = drv.Close()
		t.Fatal("Connect() with wrong password: expected error, got nil")
	}
	// Anti-regression: the error text must not leak the passwords —
	// neither the wrong one passed to Connect nor the container's real
	// superuser password.
	if strings.Contains(err.Error(), "definitely-wrong") {
		t.Errorf("Connect() error %q leaks the wrong password", err)
	}
	if strings.Contains(err.Error(), testPassword) {
		t.Errorf("Connect() error %q leaks the superuser password", err)
	}
}

func TestListDatabases(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)

	dbs, err := drv.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("ListDatabases() error: %v", err)
	}

	names := make([]string, 0, len(dbs))
	var hasPostgres bool
	for _, db := range dbs {
		if db.SizeBytes < 0 {
			t.Errorf("database %q SizeBytes = %d, want >= 0", db.Name, db.SizeBytes)
		}
		if db.Name == testDatabase {
			hasPostgres = true
		}
		names = append(names, db.Name)
	}
	if !hasPostgres {
		t.Errorf("ListDatabases() missing %q, got %v", testDatabase, names)
	}
	// The template databases are not connectable and stay hidden.
	for _, db := range dbs {
		if db.Name == "template0" || db.Name == "template1" {
			t.Errorf("ListDatabases() contains template %q, want excluded", db.Name)
		}
	}
}

func TestClose(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)

	if err := drv.Close(); err != nil {
		t.Fatalf("first Close() error: %v", err)
	}
	if err := drv.Close(); err != nil {
		t.Fatalf("second Close() error: %v", err)
	}
}
