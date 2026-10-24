package mysql

import (
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// DSN builds a go-sql-driver connection string for cfg. The password is
// a separate argument so the DSN can be logged with a redacted password:
// DSN(cfg, "****").
//
// The DSN is assembled through mysql.Config rather than fmt.Sprintf so
// that special characters in the user, password or database (/, @, ?,
// parentheses, ...) are escaped instead of breaking the DSN grammar.
func DSN(cfg driver.ConnConfig, pass string) string {
	c := mysql.Config{
		User:   cfg.User,
		Passwd: pass,
		Net:    "tcp",
		Addr:   cfg.Host + ":" + cfg.Port,
		DBName: cfg.Database,
		Params: map[string]string{
			"parseTime": "true",
			"charset":   "utf8mb4",
			// The SQL console sends multi-statement batches; additive for
			// every other caller (single statements are unaffected).
			"multiStatements":   "true",
			"interpolateParams": "true",
		},
		Timeout:      5 * time.Second,
		WriteTimeout: 30 * time.Second,
		ReadTimeout:  30 * time.Second,
		// The zero value disables native-password auth and FormatDSN
		// pins allowNativePasswords=false, which breaks mysql_native_
		// password servers (e.g. MariaDB root). Keep it enabled, as the
		// go-sql-driver default does.
		AllowNativePasswords: true,
	}
	return c.FormatDSN()
}
