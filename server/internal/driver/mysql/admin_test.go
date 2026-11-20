package mysql

import (
	"github.com/hitslab/db-manager/server/internal/driver"
	"testing"
)

func TestParseGrants(t *testing.T) {
	got := parseGrant("GRANT SELECT, INSERT ON `odd``db`.`items` TO `alice`@`%` WITH GRANT OPTION", false)
	if !got.Editable || got.Scope.Database != "odd`db" || got.Scope.Table != "items" || len(got.Privileges) != 2 || !got.GrantOption {
		t.Fatalf("unexpected grant: %#v", got)
	}
	for _, raw := range []string{"GRANT SELECT (`secret`) ON `app`.`t` TO 'a'@'%'", "GRANT `role`@`%` TO `a`@`%`", "GRANT SELECT ON `a`.`b` TO `a`@`%` IDENTIFIED BY PASSWORD 'secret'"} {
		if g := parseGrant(raw, false); g.Editable {
			t.Errorf("unsupported grant editable: %q", raw)
		}
	}
}

func TestAdminScopeAndPrivilegeValidation(t *testing.T) {
	for _, scope := range []driver.GrantScope{{Level: "global", Database: "x"}, {Level: "table", Database: "x"}, {Level: "column"}} {
		if _, err := grantScopeSQL(scope, false); err == nil {
			t.Errorf("invalid scope accepted: %#v", scope)
		}
	}
	got, err := grantScopeSQL(driver.GrantScope{Level: "table", Database: "a`b", Table: "t"}, false)
	if err != nil || got != "`a``b`.`t`" {
		t.Fatalf("quote: %q %v", got, err)
	}
	if _, err := privilegesSQL([]string{"SELECT; DROP USER root"}, driver.GrantScope{Level: "global"}, []driver.PrivilegeInfo{{Name: "SELECT", Levels: []string{"global"}}}); err == nil {
		t.Fatal("SQL injection accepted")
	}
	if _, err := privilegesSQL([]string{"SUPER"}, driver.GrantScope{Level: "table", Database: "x", Table: "y"}, []driver.PrivilegeInfo{{Name: "SUPER", Levels: []string{"global"}}}); err == nil {
		t.Fatal("global privilege accepted at table level")
	}
}

func TestParseMariaDBGlobalGrantWithAccountClauses(t *testing.T) {
	raw := "GRANT PROCESS ON *.* TO `alice`@`%` IDENTIFIED BY PASSWORD '*0123456789ABCDEF' WITH GRANT OPTION MAX_QUERIES_PER_HOUR 23 MAX_UPDATES_PER_HOUR 7 MAX_CONNECTIONS_PER_HOUR 9 MAX_USER_CONNECTIONS 2"
	got := parseGrant(raw, false)
	if !got.Editable || got.Scope.Level != "global" || !got.GrantOption || len(got.Privileges) != 1 || got.Privileges[0] != "PROCESS" {
		t.Fatalf("global grant not parsed: %#v", got)
	}
}

func TestDatabaseGrantScopeEscapesWildcardCharacters(t *testing.T) {
	got, err := grantScopeSQL(driver.GrantScope{Level: "database", Database: "foo_bar%tail"}, false)
	if err != nil || got != "`foo\\_bar\\%tail`.*" {
		t.Fatalf("wildcard grant scope=%q err=%v", got, err)
	}
	escaped := parseGrant("GRANT SELECT ON `foo\\_bar\\%tail`.* TO `a`@`%`", false)
	if !escaped.Editable || escaped.Scope.Database != "foo_bar%tail" {
		t.Fatalf("escaped grant=%#v", escaped)
	}
	wildcard := parseGrant("GRANT SELECT ON `foo_bar%tail`.* TO `a`@`%`", false)
	if !wildcard.Editable || wildcard.Scope.Level != "databasePattern" || wildcard.Scope.Database != "foo_bar%tail" {
		t.Fatalf("wildcard grant not recognized as a distinct pattern: %#v", wildcard)
	}
}

func TestDatabasePatternGrantRoundTrip(t *testing.T) {
	for _, tc := range []struct{ name, sql string }{
		{`alice\_%`, "`alice\\_%`.*"},
		{`part_\%%`, "`part_\\%%`.*"},
		{"a`_%", "`a``_%`.*"},
		{"%", "`%`.*"},
		{`a\\%`, "`a\\\\%`.*"},
	} {
		scope := driver.GrantScope{Level: "databasePattern", Database: tc.name}
		got, err := grantScopeSQL(scope, false)
		if err != nil || got != tc.sql {
			t.Errorf("pattern %q: SQL = %q, err = %v", tc.name, got, err)
		}
		raw := "GRANT SELECT ON " + tc.sql + " TO 'reader'@'%' WITH GRANT OPTION"
		if g := parseGrant(raw, false); g.Scope != scope || !g.Editable || !g.GrantOption {
			t.Errorf("parse pattern: %#v", g)
		}
		if _, err := grantScopeSQL(scope, true); err == nil {
			t.Errorf("pattern %q allowed with partial_revokes", tc.name)
		}
		if g := parseGrant(raw, true); g.Scope.Level != "database" || g.Scope.Database != tc.name || !g.Editable {
			t.Errorf("parse literal: %#v", g)
		}
	}
	for _, name := range []string{"", "*", "*.*", "plain", `foo\_bar\%`, `%trailing\`, `%bad\q`, "bad\x00%"} {
		if _, err := grantScopeSQL(driver.GrantScope{Level: "databasePattern", Database: name}, false); err == nil {
			t.Errorf("invalid pattern accepted: %q", name)
		}
	}
	for _, raw := range []string{
		"GRANT ALL PRIVILEGES ON `app\\_%`.* TO 'reader'@'%'",
		"REVOKE SELECT ON `app\\_%`.* FROM 'reader'@'%'",
		"GRANT SELECT ON `app%\\q`.* TO 'reader'@'%'",
	} {
		if g := parseGrant(raw, false); g.Editable || g.Scope.Level != "databasePattern" {
			t.Errorf("opaque pattern lost its target or became editable: %#v", g)
		}
	}
	catalog := []driver.PrivilegeInfo{{Name: "SELECT", Levels: []string{"global", "database"}}, {Name: "PROCESS", Levels: []string{"global"}}}
	scope := driver.GrantScope{Level: "databasePattern", Database: "tenant%"}
	if got, err := privilegesSQL([]string{"SELECT"}, scope, catalog); err != nil || got != "SELECT" {
		t.Errorf("database privilege denied: %q, %v", got, err)
	}
	if _, err := privilegesSQL([]string{"PROCESS"}, scope, catalog); err == nil {
		t.Error("global-only privilege accepted for pattern")
	}
}

func TestPrivilegeCatalogContextsAndOpaqueAllGrant(t *testing.T) {
	for _, tc := range []struct {
		context string
		want    int
	}{{"Tables", 3}, {"Columns", 3}, {"Databases", 2}, {"Functions,Procedures", 2}, {"Server Admin", 1}} {
		if got := privilegeLevels("", tc.context); len(got) != tc.want {
			t.Fatalf("context %q levels=%v", tc.context, got)
		}
	}
	if got := parseGrant("GRANT ALL PRIVILEGES ON *.* TO `root`@`localhost`", false); got.Editable {
		t.Fatal("ALL PRIVILEGES cannot safely expand static vs dynamic privileges")
	}
}

func TestAnonymousAccountIsValid(t *testing.T) {
	if err := validAccount(driver.UserAccount{User: "", Host: "localhost"}); err != nil {
		t.Fatal(err)
	}
}

func TestComplexGrantsPreserveTheirReadOnlyTarget(t *testing.T) {
	tests := []struct {
		raw   string
		scope driver.GrantScope
	}{
		{`GRANT SELECT ("col ON fake.scope", "a""b"), UPDATE ON "shop"."items" TO reader@"%"`, driver.GrantScope{Level: "table", Database: "shop", Table: "items"}},
		{`GRANT SELECT (lowercase) ON shop.items TO reader@localhost`, driver.GrantScope{Level: "table", Database: "shop", Table: "items"}},
		{"GRANT SELECT (`days ON field`), UPDATE ON `shop`.`items` TO 'a'@'%'", driver.GrantScope{Level: "table", Database: "shop", Table: "items"}},
		{"REVOKE INSERT ON `shop`.* FROM `a`@`%`", driver.GrantScope{Level: "database", Database: "shop"}},
		{"GRANT SELECT ON `shop`.`items` TO 'a'@'%' IDENTIFIED BY PASSWORD 'private'", driver.GrantScope{Level: "table", Database: "shop", Table: "items"}},
	}
	for _, test := range tests {
		got := parseGrant(test.raw, false)
		if got.Scope != test.scope || got.Editable || len(got.Privileges) != 0 {
			t.Errorf("expected only read-only target %#v, got %#v", test.scope, got)
		}
	}
}

func TestParseShowGrantsIdentifierStyles(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		scope driver.GrantScope
	}{
		{`GRANT SELECT ON "qa\_demo\_%".* TO "qa_demo"@"%"`, driver.GrantScope{Level: "databasePattern", Database: `qa\_demo\_%`}},
		{`GRANT SELECT ON "first""db"."odd""table" TO qa_demo@"%"`, driver.GrantScope{Level: "table", Database: `first"db`, Table: `odd"table`}},
		{`GRANT SELECT ON открытая.books_2026 TO qa_demo@localhost`, driver.GrantScope{Level: "table", Database: "открытая", Table: "books_2026"}},
		{`GRANT SELECT ON plain.* TO "an""account"@"some.host"`, driver.GrantScope{Level: "database", Database: "plain"}},
	} {
		g := parseGrant(tc.raw, false)
		if !g.Editable || g.Scope != tc.scope || len(g.Privileges) != 1 || g.Privileges[0] != "SELECT" {
			t.Errorf("grant %q: got %#v, want %#v", tc.raw, g, tc.scope)
		}
	}
}
