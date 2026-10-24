package mysql

import (
	"strings"
	"testing"
	"time"

	sqldriver "github.com/go-sql-driver/mysql"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// TestDSNRoundTripsSpecialCharacters guards against credentials that
// break the DSN grammar: user and password containing /, @, ? and
// parentheses must survive a ParseDSN round-trip instead of producing
// "invalid DSN".
func TestDSNRoundTripsSpecialCharacters(t *testing.T) {
	cfg := driver.ConnConfig{
		Host:     "db.example.com",
		Port:     "3307",
		User:     "u@x",
		Password: "p/a?s(s)",
		Database: "my@db",
	}

	dsn := DSN(cfg, cfg.Password)

	parsed, err := sqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN(%q) error: %v", dsn, err)
	}
	if parsed.User != cfg.User {
		t.Errorf("parsed User = %q, want %q", parsed.User, cfg.User)
	}
	if parsed.Passwd != cfg.Password {
		t.Errorf("parsed Passwd = %q, want %q", parsed.Passwd, cfg.Password)
	}
	if parsed.Net != "tcp" {
		t.Errorf("parsed Net = %q, want %q", parsed.Net, "tcp")
	}
	if parsed.Addr != cfg.Host+":"+cfg.Port {
		t.Errorf("parsed Addr = %q, want %q", parsed.Addr, cfg.Host+":"+cfg.Port)
	}
	if parsed.DBName != cfg.Database {
		t.Errorf("parsed DBName = %q, want %q", parsed.DBName, cfg.Database)
	}
	if !parsed.InterpolateParams {
		t.Error("parsed InterpolateParams = false, want true")
	}
	// The SQL console executes multi-statement batches, so the DSN must
	// enable multiStatements (additive for every other caller).
	if !parsed.MultiStatements {
		t.Error("parsed MultiStatements = false, want true")
	}
	for _, param := range []string{"charset=utf8mb4", "parseTime=true"} {
		if !strings.Contains(dsn, param) {
			t.Errorf("DSN %q does not contain %q", dsn, param)
		}
	}
	if parsed.Timeout != 5*time.Second {
		t.Errorf("parsed Timeout = %v, want %v", parsed.Timeout, 5*time.Second)
	}
	if parsed.ReadTimeout != 30*time.Second {
		t.Errorf("parsed ReadTimeout = %v, want %v", parsed.ReadTimeout, 30*time.Second)
	}
	if parsed.WriteTimeout != 30*time.Second {
		t.Errorf("parsed WriteTimeout = %v, want %v", parsed.WriteTimeout, 30*time.Second)
	}
	// Anti-regression: FormatDSN pins allowNativePasswords=false on the
	// zero-value Config, which breaks mysql_native_password servers
	// (e.g. MariaDB root). The DSN must keep native-password auth
	// available.
	if parsed.AllowNativePasswords != true {
		t.Errorf("parsed AllowNativePasswords = %v, want true", parsed.AllowNativePasswords)
	}
	if strings.Contains(dsn, "allowNativePasswords=false") {
		t.Errorf("DSN %q disables native password auth", dsn)
	}
}

// TestDSNRedactable keeps the contract that the password travels as a
// separate argument: DSN(cfg, "****") must yield a valid, redacted DSN.
func TestDSNRedactable(t *testing.T) {
	cfg := driver.ConnConfig{Host: "db.example.com", Port: "3306", User: "root", Database: "app"}

	parsed, err := sqldriver.ParseDSN(DSN(cfg, "****"))
	if err != nil {
		t.Fatalf("ParseDSN error: %v", err)
	}
	if parsed.Passwd != "****" {
		t.Errorf("parsed Passwd = %q, want %q", parsed.Passwd, "****")
	}
}
