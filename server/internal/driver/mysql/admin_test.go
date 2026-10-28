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
	if wildcard.Editable {
		t.Fatal("wildcard grant exposed as an exact editable scope")
	}
}

func TestPrivilegeCatalogContextsAndOpaqueAllGrant(t *testing.T) {
	for _, tc := range []struct {
		context string
		want    int
	}{{"Tables", 3}, {"Columns", 3}, {"Databases", 2}, {"Functions,Procedures", 2}, {"Server Admin", 1}} {
		if got := privilegeLevels(tc.context); len(got) != tc.want {
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
