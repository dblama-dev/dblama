package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/dblama-dev/dblama/internal/driver"
)

// TestDSNRoundTripsSpecialCharacters guards against credentials that
// break the DSN grammar: user, password and database containing spaces,
// quotes and backslashes must survive a ParseConfig round-trip instead of
// producing "invalid connection string" (or silently truncated values).
func TestDSNRoundTripsSpecialCharacters(t *testing.T) {
	cfg := driver.ConnConfig{
		Host:     "db.example.com",
		Port:     "5433",
		User:     `u x's`,
		Password: `p/a?s(s)' \`,
		Database: `my db`,
	}

	connCfg, err := pgx.ParseConfig(DSN(cfg, cfg.Password))
	if err != nil {
		t.Fatalf("ParseConfig error: %v", err)
	}
	if connCfg.Host != cfg.Host {
		t.Errorf("parsed Host = %q, want %q", connCfg.Host, cfg.Host)
	}
	if got := connCfg.Port; int(got) != 5433 {
		t.Errorf("parsed Port = %d, want 5433", got)
	}
	if connCfg.User != cfg.User {
		t.Errorf("parsed User = %q, want %q", connCfg.User, cfg.User)
	}
	if connCfg.Password != cfg.Password {
		t.Errorf("parsed Password = %q, want %q", connCfg.Password, cfg.Password)
	}
	if connCfg.Database != cfg.Database {
		t.Errorf("parsed Database = %q, want %q", connCfg.Database, cfg.Database)
	}
	// sslmode=disable keeps TLS off; default_query_exec_mode must parse
	// into the config (simple protocol for the console's multi-statement
	// batches).
	if connCfg.TLSConfig != nil {
		t.Error("parsed TLSConfig is non-nil, want nil (sslmode=disable)")
	}
	if connCfg.DefaultQueryExecMode != pgx.QueryExecModeSimpleProtocol {
		t.Errorf("parsed DefaultQueryExecMode = %v, want %v",
			connCfg.DefaultQueryExecMode, pgx.QueryExecModeSimpleProtocol)
	}
}

// TestDSNRedactable keeps the contract that the password travels as a
// separate argument: DSN(cfg, "****") must yield a valid, redacted DSN.
func TestDSNRedactable(t *testing.T) {
	cfg := driver.ConnConfig{Host: "db.example.com", Port: "5432", User: "postgres", Database: "app"}

	connCfg, err := pgx.ParseConfig(DSN(cfg, "****"))
	if err != nil {
		t.Fatalf("ParseConfig error: %v", err)
	}
	if connCfg.Password != "****" {
		t.Errorf("parsed Password = %q, want %q", connCfg.Password, "****")
	}
}

func TestDSNRequestsUTF8ClientEncoding(t *testing.T) {
	cfg := driver.ConnConfig{Host: "db.example.com", Port: "5432", User: "postgres", Database: "app"}

	connCfg, err := pgx.ParseConfig(DSN(cfg, ""))
	if err != nil {
		t.Fatalf("ParseConfig error: %v", err)
	}
	if got := connCfg.RuntimeParams["client_encoding"]; got != "UTF8" {
		t.Errorf("startup client_encoding = %q, want UTF8 for simple protocol queries", got)
	}
}

func TestConnectMalformedConfigDoesNotLeakPassword(t *testing.T) {
	const password = "secret-DSN-token"
	_, err := Connect(context.Background(), driver.ConnConfig{Host: "localhost", Port: "not-a-port", User: "postgres", Password: password, Database: "app"})
	if err == nil {
		t.Fatal("malformed port accepted")
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("password leaked in malformed connection error: %v", err)
	}
}
