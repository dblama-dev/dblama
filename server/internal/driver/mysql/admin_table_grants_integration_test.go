//go:build integration

package mysql

import (
	"context"
	"slices"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
)

func TestCompletedTableGrantRemainsEditable(t *testing.T) {
	for _, image := range testImages {
		t.Run(image.image, func(t *testing.T) {
			cfg := startDB(t, image.image)
			d := connectOK(t, cfg).(*MysqlDriver)
			ctx := context.Background()
			a := driver.UserAccount{User: "table_reader", Host: "%"}
			if err := d.CreateUser(ctx, a, "table-password"); err != nil {
				t.Fatal(err)
			}
			scope := driver.GrantScope{Level: "table", Database: cfg.Database, Table: "items_1"}
			if _, err := d.db.ExecContext(ctx, "CREATE TABLE items_1 (id INT)"); err != nil {
				t.Fatal(err)
			}
			if _, err := d.db.ExecContext(ctx, "GRANT ALL PRIVILEGES ON items_1 TO ?@? WITH GRANT OPTION", a.User, a.Host); err != nil {
				t.Fatal(err)
			}
			rows, err := d.db.QueryContext(ctx, "SELECT PRIVILEGE_TYPE FROM information_schema.TABLE_PRIVILEGES WHERE GRANTEE = ? AND TABLE_SCHEMA = ? AND TABLE_NAME = ?", "'table_reader'@'%'", scope.Database, scope.Table)
			if err != nil {
				t.Fatal(err)
			}
			var complete []string
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err != nil {
					t.Fatal(err)
				}
				complete = append(complete, name)
			}
			if err := rows.Close(); err != nil {
				t.Fatal(err)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			slices.Sort(complete)
			if !slices.Contains(complete, "SELECT") {
				t.Fatalf("server table privilege set missing SELECT: %v", complete)
			}
			change := driver.GrantChange{Scope: scope, Privileges: []string{"SELECT"}}
			if err := d.RevokePrivileges(ctx, a, change); err != nil {
				t.Fatal(err)
			}
			// Adding the final missing privilege makes SHOW GRANTS compress
			// the explicit list into ALL PRIVILEGES.
			if err := d.GrantPrivileges(ctx, a, change); err != nil {
				t.Fatal(err)
			}
			verify := func(want []string) {
				t.Helper()
				grants, err := d.UserGrants(ctx, a)
				if err != nil {
					t.Fatal(err)
				}
				for _, g := range grants {
					if g.Scope != scope {
						continue
					}
					slices.Sort(g.Privileges)
					if !g.Editable || !g.GrantOption || !slices.Equal(g.Privileges, want) {
						t.Fatalf("table grant = %#v, want editable %v with grant option", g, want)
					}
					return
				}
				t.Fatalf("missing grant on %v: %#v", scope, grants)
			}
			verify(complete)
			if err := d.RevokePrivileges(ctx, a, change); err != nil {
				t.Fatal(err)
			}
			verify(slices.DeleteFunc(complete, func(name string) bool { return name == "SELECT" }))
		})
	}
}
