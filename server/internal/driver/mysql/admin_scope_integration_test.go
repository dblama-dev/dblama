//go:build integration

package mysql

import (
	"context"
	"github.com/hitslab/db-manager/server/internal/driver"
	"strings"
	"testing"
)

func TestDatabaseGrantScopeIsExact(t *testing.T) {
	for _, image := range testImages {
		t.Run(image.image, func(t *testing.T) {
			cfg := startDB(t, image.image)
			d := connectOK(t, cfg).(*MysqlDriver)
			ctx := context.Background()
			for _, name := range []string{"foo_bar", "fooXbar", "scope%one", "scopeXone"} {
				if _, err := d.db.ExecContext(ctx, "CREATE DATABASE "+adminQuote(name)); err != nil {
					t.Fatal(err)
				}
				if _, err := d.db.ExecContext(ctx, "CREATE TABLE "+adminQuote(name)+".probe (id int)"); err != nil {
					t.Fatal(err)
				}
			}
			modes := []bool{false}
			if image.flavor == FlavorMySQL {
				modes = append(modes, true)
			}
			for _, literal := range modes {
				t.Run(map[bool]string{false: "wildcards", true: "partial_revokes"}[literal], func(t *testing.T) {
					if image.flavor == FlavorMySQL {
						if _, err := d.db.ExecContext(ctx, "SET GLOBAL partial_revokes = ?", literal); err != nil {
							t.Fatal(err)
						}
					}
					for _, pair := range [][2]string{{"foo_bar", "fooXbar"}, {"scope%one", "scopeXone"}} {
						t.Run(pair[0], func(t *testing.T) {
							a := driver.UserAccount{User: "scope_reader", Host: "%"}
							if err := d.CreateUser(ctx, a, "scope-password"); err != nil {
								t.Fatal(err)
							}
							defer func() { _ = d.DropUser(ctx, a) }()
							change := driver.GrantChange{Scope: driver.GrantScope{Level: "database", Database: pair[0]}, Privileges: []string{"SELECT"}}
							if err := d.GrantPrivileges(ctx, a, change); err != nil {
								t.Fatal(err)
							}
							login := cfg
							login.User = a.User
							login.Password = "scope-password"
							login.Database = ""
							userDriver, err := Connect(ctx, login)
							if err != nil {
								t.Fatal(err)
							}
							defer func() { _ = userDriver.Close() }()
							reader := userDriver.(*MysqlDriver)
							var count int
							if err := reader.db.QueryRowContext(ctx, "SELECT count(*) FROM "+adminQuote(pair[0])+".probe").Scan(&count); err != nil {
								t.Fatalf("exact database denied: %v", err)
							}
							if err := reader.db.QueryRowContext(ctx, "SELECT count(*) FROM "+adminQuote(pair[1])+".probe").Scan(&count); err == nil {
								t.Fatal("grant leaked to wildcard-matching database")
							}
							grants, err := d.UserGrants(ctx, a)
							if err != nil {
								t.Fatal(err)
							}
							found := false
							for _, g := range grants {
								if g.Scope.Level == "database" && g.Scope.Database == pair[0] && g.Editable {
									found = true
									change.Scope = g.Scope
								}
							}
							if !found {
								t.Fatalf("grant did not round-trip: %#v", grants)
							}
							if err := d.RevokePrivileges(ctx, a, change); err != nil {
								t.Fatal(err)
							}
							_ = userDriver.Close()
							userDriver, err = Connect(ctx, login)
							if err != nil {
								t.Fatal(err)
							}
							defer func() { _ = userDriver.Close() }()
							if err := userDriver.(*MysqlDriver).db.QueryRowContext(ctx, "SELECT count(*) FROM "+adminQuote(pair[0])+".probe").Scan(&count); err == nil {
								t.Fatal("revoke did not remove exact grant")
							}
						})
					}
				})
			}
		})
	}
}

func TestPrivilegeCatalogUsesServerPrivileges(t *testing.T) {
	for _, image := range testImages {
		t.Run(image.image, func(t *testing.T) {
			d := connectOK(t, startDB(t, image.image)).(*MysqlDriver)
			catalog := d.PrivilegeCatalog()
			found := map[string]bool{}
			for _, p := range catalog {
				found[p.Name] = true
			}
			rows, err := d.db.Query("SHOW PRIVILEGES")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var name, context, comment string
				if err := rows.Scan(&name, &context, &comment); err != nil {
					t.Fatal(err)
				}
				name = strings.ToUpper(name)
				if name == "GRANT OPTION" || name == "USAGE" || name == "PROXY" {
					continue
				}
				if !found[name] {
					t.Errorf("supported privilege missing from catalog: %s (%s)", name, context)
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
