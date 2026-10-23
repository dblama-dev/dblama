package mysql

import (
	"fmt"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// dsnParams are the mandatory connection parameters for every connection
// opened by this driver.
const dsnParams = "parseTime=true&charset=utf8mb4&interpolateParams=true&timeout=5s&writeTimeout=30s&readTimeout=30s"

// DSN builds a go-sql-driver connection string for cfg. The password is
// a separate argument so the DSN can be logged with a redacted password:
// DSN(cfg, "****").
func DSN(cfg driver.ConnConfig, pass string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?%s",
		cfg.User, pass, cfg.Host, cfg.Port, cfg.Database, dsnParams)
}
