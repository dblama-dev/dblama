package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hitslab/db-manager/server/internal/driver"
)

func adminQuote(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

// PrivilegeCatalog returns the server's complete privilege whitelist. Requests
// use ReadPrivilegeCatalog to propagate their cancellation and report errors.
func (d *MysqlDriver) PrivilegeCatalog() []driver.PrivilegeInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	catalog, _ := d.ReadPrivilegeCatalog(ctx)
	return catalog
}

// ReadPrivilegeCatalog derives valid privilege names and scopes from the server.
// The SQL grammar whitelist and the database catalog must both accept a name.
func (d *MysqlDriver) ReadPrivilegeCatalog(ctx context.Context) ([]driver.PrivilegeInfo, error) {
	rows, err := d.db.QueryContext(ctx, "SHOW PRIVILEGES")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	catalog := []driver.PrivilegeInfo{}
	for rows.Next() {
		var name, scope, comment string
		if err := rows.Scan(&name, &scope, &comment); err != nil {
			return nil, err
		}
		name = strings.ToUpper(strings.TrimSpace(name))
		if name == "USAGE" || name == "GRANT OPTION" || name == "PROXY" {
			continue
		}
		if !privilegeNamePattern.MatchString(name) {
			continue
		}
		catalog = append(catalog, driver.PrivilegeInfo{Name: name, Levels: privilegeLevels(name, scope)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return catalog, nil
}
func privilegeLevels(name, scope string) []string {
	// SHOW PRIVILEGES labels EVENT as "Server Admin", but both engines
	// also grant it on individual databases.
	if name == "EVENT" {
		return []string{"global", "database"}
	}
	scope = strings.ToLower(scope)
	levels := []string{"global"}
	if strings.Contains(scope, "database") || strings.Contains(scope, "table") || strings.Contains(scope, "column") || strings.Contains(scope, "function") || strings.Contains(scope, "procedure") || strings.Contains(scope, "routine") {
		levels = append(levels, "database")
	}
	if strings.Contains(scope, "table") || strings.Contains(scope, "column") {
		levels = append(levels, "table")
	}
	return levels
}

var privilegeNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*(?: [A-Z][A-Z0-9_]*)*$`)

func grantDatabaseName(name string, literal bool) string {
	if !literal {
		name = strings.NewReplacer(`\`, `\\`, `_`, `\_`, `%`, `\%`).Replace(name)
	}
	return adminQuote(name)
}

// DatabasePatternsSupported reads the live server setting. MariaDB and older
// MySQL versions have no partial_revokes variable and support wildcard grants.
func (d *MysqlDriver) DatabasePatternsSupported(ctx context.Context) (bool, error) {
	var name, value string
	err := d.db.QueryRowContext(ctx, "SHOW GLOBAL VARIABLES WHERE Variable_name = 'partial_revokes'").Scan(&name, &value)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return strings.EqualFold(value, "OFF") || value == "0", nil
}
func grantScopeSQL(s driver.GrantScope, literal bool) (string, error) {
	if !s.Valid() {
		return "", fmt.Errorf("invalid privilege scope")
	}
	switch s.Level {
	case "global":
		return "*.*", nil
	case "database":
		return grantDatabaseName(s.Database, literal) + ".*", nil
	case driver.DatabasePatternLevel:
		if literal {
			return "", driver.ErrDatabasePatternsDisabled
		}
		return adminQuote(s.Database) + ".*", nil
	case "table":
		return adminQuote(s.Database) + "." + adminQuote(s.Table), nil
	}
	return "", fmt.Errorf("invalid privilege scope")
}
func privilegesSQL(names []string, s driver.GrantScope, catalog []driver.PrivilegeInfo) (string, error) {
	if len(names) == 0 {
		return "", fmt.Errorf("at least one privilege is required")
	}
	out := []string{}
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.ToUpper(strings.TrimSpace(name))
		valid := false
		for _, p := range catalog {
			if name == p.Name && privilegeNamePattern.MatchString(name) {
				for _, level := range p.Levels {
					if level == s.PrivilegeLevel() {
						valid = true
					}
				}
			}
		}
		if !valid {
			return "", fmt.Errorf("invalid privilege for scope")
		}
		if !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	return strings.Join(out, ", "), nil
}

// The account and trailing server clauses are deliberately not returned: older
// servers may append password hashes, SSL settings and resource limits.
const (
	// SHOW GRANTS follows ANSI_QUOTES and sql_quote_show_create. Doubled
	// identifier quotes represent one literal quote; backslashes in schema
	// names belong to the grant pattern and must survive unquoting.
	grantQuotedIdentifier = "`(?:``|[^`])*`" + `|"(?:""|[^"])*"`
	grantIdentifier       = `(?:[a-zA-Z0-9$_\x{0080}-\x{FFFF}]+|` + grantQuotedIdentifier + `)`
	grantTarget           = `(\*|` + grantIdentifier + `)`
	grantAccountPart      = `(?:` + grantIdentifier + `|'(?:''|[^'])*')`
)

var grantPattern = regexp.MustCompile(`^(GRANT|REVOKE) ((?:[(), ]|` + grantIdentifier + `)+) ON ` +
	grantTarget + `\.` + grantTarget + ` (?:TO|FROM) ` + grantAccountPart + `@` + grantAccountPart + `((?: .*)?)$`)

func unquoteGrantIdentifier(s string) string {
	if s[0] == '`' || s[0] == '"' {
		quote := s[:1]
		return strings.ReplaceAll(s[1:len(s)-1], quote+quote, quote)
	}
	return s
}

func parseGrant(raw string, literal bool) driver.GrantInfo {
	g := driver.GrantInfo{Privileges: []string{}}
	m := grantPattern.FindStringSubmatch(raw)
	if m == nil {
		return g
	}
	validScope := true
	switch {
	case m[3] == "*" && m[4] == "*":
		g.Scope.Level = "global"
	case m[3] != "*" && m[4] == "*":
		db := unquoteGrantIdentifier(m[3])
		if literal {
			g.Scope = driver.GrantScope{Level: "database", Database: db}
		} else {
			g.Scope, validScope = driver.ParseDatabaseGrantName(db)
		}
	case m[3] != "*":
		g.Scope = driver.GrantScope{Level: "table", Database: unquoteGrantIdentifier(m[3]), Table: unquoteGrantIdentifier(m[4])}
	default:
		return g
	}

	// Retain the target of restrictions and column grants so the editor can
	// protect that scope without disclosing unparsed SQL or account clauses.
	if m[1] == "REVOKE" || (g.Scope.Level != "global" && m[5] != "" && m[5] != " WITH GRANT OPTION") {
		return g
	}
	names := strings.Split(m[2], ", ")
	for _, name := range names {
		if !privilegeNamePattern.MatchString(name) {
			return g
		}
	}
	g.Privileges = names
	g.GrantOption = strings.Contains(m[5], "WITH GRANT OPTION")
	g.Editable = validScope && g.Scope.Valid() && m[2] != "ALL PRIVILEGES"
	return g
}
