package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/hitslab/db-manager/server/internal/driver"
	"regexp"
	"strings"
	"time"
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
		catalog = append(catalog, driver.PrivilegeInfo{Name: name, Levels: privilegeLevels(scope)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return catalog, nil
}
func privilegeLevels(scope string) []string {
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
func (d *MysqlDriver) grantDatabaseLiteralMode(ctx context.Context) (bool, error) {
	var name, value string
	err := d.db.QueryRowContext(ctx, "SHOW GLOBAL VARIABLES LIKE 'partial_revokes'").Scan(&name, &value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.EqualFold(value, "ON") || value == "1", nil
}
func grantScopeSQL(s driver.GrantScope, literal bool) (string, error) {
	switch s.Level {
	case "global":
		if s.Database == "" && s.Table == "" {
			return "*.*", nil
		}
	case "database":
		if s.Database != "" && s.Table == "" && !strings.ContainsRune(s.Database, 0) {
			return grantDatabaseName(s.Database, literal) + ".*", nil
		}
	case "table":
		if s.Database != "" && s.Table != "" && !strings.ContainsRune(s.Database+s.Table, 0) {
			return adminQuote(s.Database) + "." + adminQuote(s.Table), nil
		}
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
					if level == s.Level {
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
var grantPattern = regexp.MustCompile("^GRANT ([A-Z_, ]+) ON (\\*|`(?:``|[^`])+`)\\.(\\*|`(?:``|[^`])+`) TO (?:`(?:``|[^`])*`|'(?:''|[^'])*')@(?:`(?:``|[^`])*`|'(?:''|[^'])*')(.*)$")

func parseGrant(raw string, literal bool) driver.GrantInfo {
	g := driver.GrantInfo{Privileges: []string{}}
	m := grantPattern.FindStringSubmatch(raw)
	if m == nil {
		return g
	}
	g.Privileges = strings.Split(m[1], ", ")
	g.GrantOption = strings.Contains(m[4], "WITH GRANT OPTION")
	unquote := func(s string) string { return strings.ReplaceAll(s[1:len(s)-1], "``", "`") }
	if m[2] == "*" && m[3] == "*" {
		g.Scope.Level = "global"
	} else if m[2] != "*" {
		g.Scope.Database = unquote(m[2])
		if m[3] == "*" {
			g.Scope.Level = "database"
			var exact bool
			g.Scope.Database, exact = decodeGrantDatabase(g.Scope.Database, literal)
			if !exact {
				return g
			}
		} else {
			g.Scope.Level = "table"
			g.Scope.Table = unquote(m[3])
		}
	} else {
		return g
	}
	g.Privileges = strings.Split(m[1], ", ")
	// MariaDB attaches authentication hashes and resource limits to the
	// global grant. Extract only privilege names and never expose those clauses.
	if g.Scope.Level != "global" && m[4] != "" && m[4] != " WITH GRANT OPTION" {
		return driver.GrantInfo{Privileges: []string{}}
	}
	g.GrantOption = strings.Contains(m[4], "WITH GRANT OPTION")
	if m[1] == "ALL PRIVILEGES" {
		return g
	}
	if m[1] == "USAGE" {
		g.Editable = true
		return g
	}
	g.Editable = true
	for _, name := range g.Privileges {
		if !privilegeNamePattern.MatchString(name) {
			g.Editable = false
		}
	}
	return g
}

// Unescaped wildcard grants describe a pattern, not one exact database. They
// remain visible but cannot be edited as an exact scope through this API.
func decodeGrantDatabase(name string, literal bool) (string, bool) {
	if literal {
		return name, true
	}
	var out strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '_' || c == '%' {
			return name, false
		}
		if c == '\\' {
			i++
			if i >= len(name) || !strings.ContainsRune(`_%\`, rune(name[i])) {
				return name, false
			}
			c = name[i]
		}
		out.WriteByte(c)
	}
	return out.String(), true
}
