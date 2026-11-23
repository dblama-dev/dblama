//go:build integration

package postgres

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/dblama-dev/dblama/internal/driver"
)

func TestRoleAdministration(t *testing.T) {
	cfg := startDB(t)
	d := connectOK(t, cfg).(*PostgresDriver)
	ctx := context.Background()
	member, group := `Report "user/1"`, "Readers"
	login, noLogin := true, false
	password := `pa'ss\word`
	change := func(c driver.RoleChange) {
		t.Helper()
		if err := d.ChangeRole(ctx, c); err != nil {
			t.Fatalf("%s: %v", c.Action, err)
		}
	}
	change(driver.RoleChange{Action: "create", Name: group, Login: &noLogin})
	change(driver.RoleChange{Action: "create", Name: member, Login: &login, Password: password})
	cfg.User, cfg.Password = member, password
	restricted := connectOK(t, cfg).(*PostgresDriver)
	if err := restricted.ChangeRole(ctx, driver.RoleChange{Action: "create", Name: "forbidden", Login: &login, Password: "secret-marker"}); err == nil || strings.Contains(err.Error(), "secret-marker") {
		t.Fatal("expected redacted permission error")
	}
	change(driver.RoleChange{Action: "join", Name: member, Group: group})
	roles, err := d.ListRoles(ctx)
	if err != nil || !slices.ContainsFunc(roles, func(r driver.RoleInfo) bool {
		return r.Name == member && r.Login && slices.Contains(r.Memberships, group)
	}) {
		t.Fatalf("missing login role or membership: %v", err)
	}
	if _, err := d.db.ExecContext(ctx, `CREATE SCHEMA "Mixed"; CREATE TABLE "Mixed"."Sales.1"(id int)`); err != nil {
		t.Fatal(err)
	}
	obj := driver.RoleObject{Kind: "table", Schema: "Mixed", Name: "Sales.1"}
	check := func(role string, obj driver.RoleObject, source, privilege string, present bool) {
		t.Helper()
		grants, err := d.RolePrivileges(ctx, role, obj)
		if err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(grants.Grants, func(g driver.ObjectPrivilege) bool {
			return g.Source == source && g.Privilege == privilege && g.Public == (source == "")
		}) != present {
			t.Fatalf("unexpected grants: %+v", grants)
		}
	}
	set := func(role string, obj driver.RoleObject, p string, grant bool) {
		t.Helper()
		if err := d.ChangeRolePrivilege(ctx, role, obj, p, grant); err != nil {
			t.Fatal(err)
		}
	}
	db := driver.RoleObject{Kind: "database"}
	if err := d.ChangeRolePrivilege(ctx, "public", db, "CONNECT", false); err == nil {
		t.Fatal("PUBLIC accepted as a named role")
	}
	check(member, db, "", "CONNECT", true)
	set(group, db, "CREATE", true)
	check(member, db, group, "CREATE", true)
	set(group, db, "CREATE", false)
	schema := driver.RoleObject{Kind: "schema", Schema: "Mixed"}
	set(group, schema, "USAGE", true)
	set(group, obj, "MAINTAIN", true)
	check(member, obj, group, "MAINTAIN", true)
	set(group, obj, "SELECT", true)
	check(member, obj, group, "SELECT", true)
	if err := restricted.ChangeRolePrivilege(ctx, member, obj, "SELECT", true); err == nil {
		t.Fatal("granted without grant option")
	}
	set(member, obj, "SELECT", true)
	check(member, obj, member, "SELECT", true)
	set(member, obj, "SELECT", false)
	check(member, obj, member, "SELECT", false)
	check(member, obj, group, "SELECT", true)
	change(driver.RoleChange{Action: "leave", Name: member, Group: group})
	check(member, obj, group, "SELECT", false)
	change(driver.RoleChange{Action: "password", Name: member, Password: password + "2"})
	cfg.Password = password + "2"
	connectOK(t, cfg)
	change(driver.RoleChange{Action: "login", Name: member, Login: &noLogin})
	if c, err := Connect(ctx, cfg); err == nil {
		_ = c.Close()
		t.Fatal("NOLOGIN role connected")
	}
	change(driver.RoleChange{Action: "drop", Name: member})
}
