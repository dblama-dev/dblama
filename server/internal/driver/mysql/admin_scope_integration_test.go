//go:build integration

package mysql

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	sqldriver "github.com/go-sql-driver/mysql"
	"github.com/hitslab/db-manager/server/internal/driver"
)

func TestDatabasePatternGrants(t *testing.T) {
	for _, image := range testImages {
		t.Run(image.image, func(t *testing.T) {
			cfg := startDB(t, image.image)
			d := connectOK(t, cfg).(*MysqlDriver)
			ctx := context.Background()
			if supported, err := d.DatabasePatternsSupported(ctx); err != nil || !supported {
				t.Fatalf("server should support patterns by default: supported=%v err=%v", supported, err)
			}
			a := driver.UserAccount{User: "group_a", Host: "%"}
			if err := d.CreateUser(ctx, a, "scope-password"); err != nil {
				t.Fatal(err)
			}
			change := driver.GrantChange{
				Scope:      driver.GrantScope{Level: "databasePattern", Database: `group\_a\_%`},
				Privileges: []string{"CREATE", "SELECT"},
			}
			t.Run("new matching databases", func(t *testing.T) {
				if err := d.GrantPrivileges(ctx, a, change); err != nil {
					t.Fatal(err)
				}
				login := cfg
				login.User, login.Password, login.Database = a.User, "scope-password", ""
				member := connectOK(t, login).(*MysqlDriver)
				assertCreate := func(d *MysqlDriver, name string, allowed bool) {
					t.Helper()
					_, err := d.db.ExecContext(ctx, "CREATE DATABASE "+adminQuote(name))
					var denied *sqldriver.MySQLError
					if allowed && err != nil || !allowed && (!errors.As(err, &denied) || denied.Number != 1044) {
						t.Fatalf("CREATE DATABASE %s: allowed=%t err=%v", name, allowed, err)
					}
				}
				assertCreate(member, "group_a_one", true)
				assertCreate(member, "group_a_two", true)
				assertCreate(member, "groupXa_one", false)
				assertCreate(member, "group_aXone", false)
				assertCreate(member, "other", false)
				grants, err := d.UserGrants(ctx, a)
				if err != nil {
					t.Fatal(err)
				}
				if !slices.ContainsFunc(grants, func(g driver.GrantInfo) bool {
					return g.Scope == change.Scope && g.Editable && slices.Contains(g.Privileges, "CREATE") && slices.Contains(g.Privileges, "SELECT")
				}) {
					t.Fatalf("missing editable pattern: %#v", grants)
				}
				if err := d.RevokePrivileges(ctx, a, change); err != nil {
					t.Fatal(err)
				}
				assertCreate(connectOK(t, login).(*MysqlDriver), "group_a_three", false)
			})
			t.Run("phpMyAdmin ALL", func(t *testing.T) {
				// Exercise a grant created externally, including the server's
				// compressed ALL PRIVILEGES representation in SHOW GRANTS.
				if _, err := d.db.ExecContext(ctx, "GRANT ALL PRIVILEGES ON `group\\_a\\_%`.* TO ?@?", a.User, a.Host); err != nil {
					t.Fatal(err)
				}
				rows, err := d.db.QueryContext(ctx, "SELECT PRIVILEGE_TYPE FROM information_schema.SCHEMA_PRIVILEGES WHERE GRANTEE = ? ORDER BY PRIVILEGE_TYPE", "'group_a'@'%'")
				if err != nil {
					t.Fatal(err)
				}
				var want []string
				for rows.Next() {
					var name string
					if err := rows.Scan(&name); err != nil {
						t.Fatal(err)
					}
					want = append(want, name)
				}
				if err := rows.Close(); err != nil {
					t.Fatal(err)
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				if !slices.Contains(want, "SELECT") {
					t.Fatalf("missing schema privilege information: %v", want)
				}
				slices.Sort(want)
				var available []string
				for _, p := range d.PrivilegeCatalog() {
					if slices.Contains(p.Levels, "database") {
						available = append(available, p.Name)
					}
				}
				slices.Sort(available)
				if !slices.Equal(available, want) {
					t.Errorf("catalog database privileges %v, want %v", available, want)
				}
				grants, err := d.UserGrants(ctx, a)
				if err != nil {
					t.Fatal(err)
				}
				for _, g := range grants {
					if g.Scope != change.Scope {
						continue
					}
					slices.Sort(g.Privileges)
					if !g.Editable || g.GrantOption || !slices.Equal(g.Privileges, want) {
						t.Fatalf("ALL pattern readback = %#v, want %v", g, want)
					}
					revoke := driver.GrantChange{Scope: g.Scope, Privileges: g.Privileges}
					if err := d.RevokePrivileges(ctx, a, revoke); err != nil {
						t.Fatal(err)
					}
					return
				}
				t.Fatalf("missing ALL pattern: %#v", grants)
			})
			t.Run("quoted and unquoted SHOW GRANTS", func(t *testing.T) {
				// This test owns the pool so its session modes apply to each
				// read without changing settings for the other cases.
				reader := connectOK(t, cfg).(*MysqlDriver)
				reader.db.SetMaxOpenConns(1)
				if _, err := reader.db.ExecContext(ctx, "CREATE TABLE grant_plain (id INT)"); err != nil {
					t.Fatal(err)
				}
				table := driver.GrantScope{Level: "table", Database: cfg.Database, Table: "grant_plain"}
				for _, scope := range []driver.GrantScope{change.Scope, table} {
					if err := reader.GrantPrivileges(ctx, a, driver.GrantChange{Scope: scope, Privileges: []string{"SELECT"}}); err != nil {
						t.Fatal(err)
					}
				}
				for _, settings := range []string{
					"SET SESSION sql_mode='ANSI_QUOTES', SESSION sql_quote_show_create=1",
					"SET SESSION sql_mode='ANSI_QUOTES', SESSION sql_quote_show_create=0",
					"SET SESSION sql_mode='', SESSION sql_quote_show_create=0",
				} {
					if _, err := reader.db.ExecContext(ctx, settings); err != nil {
						t.Fatal(err)
					}
					grants, err := reader.UserGrants(ctx, a)
					if err != nil {
						t.Fatal(err)
					}
					for _, scope := range []driver.GrantScope{change.Scope, table} {
						if !slices.ContainsFunc(grants, func(g driver.GrantInfo) bool {
							return g.Scope == scope && g.Editable && slices.Contains(g.Privileges, "SELECT")
						}) {
							t.Errorf("missing editable %#v after %s: %#v", scope, settings, grants)
						}
					}
				}
			})
			if image.flavor == FlavorMySQL {
				t.Run("partial revokes", func(t *testing.T) {
					if _, err := d.db.ExecContext(ctx, "SET GLOBAL partial_revokes = ON"); err != nil {
						t.Fatal(err)
					}
					if supported, err := d.DatabasePatternsSupported(ctx); err != nil || supported {
						t.Fatalf("partial revokes mode: supported=%v err=%v", supported, err)
					}
					for _, mutate := range []func(context.Context, driver.UserAccount, driver.GrantChange) error{d.GrantPrivileges, d.RevokePrivileges} {
						if err := mutate(ctx, a, change); err == nil || !strings.Contains(err.Error(), "partial_revokes") {
							t.Errorf("expected mode-specific refusal: %v", err)
						}
					}
				})
			}
		})
	}
}

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
