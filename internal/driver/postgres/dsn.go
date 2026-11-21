package postgres

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/dblama-dev/dblama/internal/driver"
)

// DSN builds a pgx keyword/value connection string for cfg. The password
// is a separate argument so the DSN can be logged with a redacted
// password: DSN(cfg, "****").
//
// Every value is emitted as a single-quoted token with backslashes and
// single quotes escaped, the two escapes pgconn's keyword/value parser
// understands inside quotes — credentials with spaces, quotes or
// backslashes cannot break the DSN grammar or terminate the value early.
// sslmode=disable matches the v0.1 deployments dblama targets
// (container/LAN; TLS terminates at the reverse proxy).
// default_query_exec_mode=simple_protocol lets the SQL console run
// multi-statement batches through one Exec call (task 5); it is additive
// for single queries — they execute the same, minus the extended
// protocol's describe round-trip.
// pgx requires client_encoding=UTF8 for simple protocol queries.
func DSN(cfg driver.ConnConfig, pass string) string {
	return strings.Join([]string{
		"host=" + quoteDSNValue(cfg.Host),
		"port=" + quoteDSNValue(cfg.Port),
		"user=" + quoteDSNValue(cfg.User),
		"password=" + quoteDSNValue(pass),
		"dbname=" + quoteDSNValue(cfg.Database),
		"sslmode=disable",
		"default_query_exec_mode=simple_protocol",
		"client_encoding=UTF8",
	}, " ")
}

// quoteDSNValue renders v as a single-quoted keyword/value token: a
// backslash becomes \\ and a single quote becomes \'.
func quoteDSNValue(v string) string {
	escaped := strings.ReplaceAll(v, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return "'" + escaped + "'"
}

// parseConnectionConfig parses credentials before opening a pool and deliberately
// omits the parser's error text: pgx includes the original DSN in that error.
func parseConnectionConfig(cfg driver.ConnConfig) (*pgx.ConnConfig, error) {
	parsed, err := pgx.ParseConfig(DSN(cfg, cfg.Password))
	if err != nil {
		return nil, fmt.Errorf("invalid postgres connection configuration")
	}
	return parsed, nil
}
