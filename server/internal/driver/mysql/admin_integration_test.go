//go:build integration

package mysql

import (
	"context"
	"github.com/hitslab/db-manager/server/internal/driver"
	"strings"
	"testing"
)

func TestAdminLifecycleAndMonitor(t *testing.T) {
	for _, image := range testImages {
		t.Run(image.image, func(t *testing.T) {
			cfg := startDB(t, image.image)
			d := connectOK(t, cfg).(*MysqlDriver)
			ctx := context.Background()
			account := driver.UserAccount{User: "admin'probe", Host: "%"}
			password := "safe'password\\tail"
			if err := d.CreateUser(ctx, account, password); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = d.DropUser(ctx, account) })
			optionOnly := driver.GrantChange{Scope: driver.GrantScope{Level: "global"}, GrantOption: true, Privileges: []string{}}
			if err := d.GrantPrivileges(ctx, account, optionOnly); err != nil {
				t.Fatal(err)
			}
			optionGrants, err := d.UserGrants(ctx, account)
			if err != nil {
				t.Fatal(err)
			}
			optionFound := false
			for _, g := range optionGrants {
				if g.Scope.Level == "global" && g.GrantOption {
					optionFound = true
				}
			}
			if !optionFound {
				t.Fatal("grant option-only grant missing")
			}
			if err := d.RevokePrivileges(ctx, account, optionOnly); err != nil {
				t.Fatal(err)
			}
			optionGrants, err = d.UserGrants(ctx, account)
			if err != nil {
				t.Fatal(err)
			}
			for _, g := range optionGrants {
				if g.GrantOption {
					t.Fatal("grant option-only revoke failed")
				}
			}
			anonymous := driver.UserAccount{Host: "anonymous-fixture.invalid"}
			if err := d.CreateUser(ctx, anonymous, "anonymous-password"); err != nil {
				t.Fatal(err)
			}
			if err := d.SetPassword(ctx, anonymous, "new-anonymous-password"); err != nil {
				t.Fatal(err)
			}
			if err := d.DropUser(ctx, anonymous); err != nil {
				t.Fatal(err)
			}

			users, err := d.ListUsers(ctx)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, u := range users {
				if u.User == account.User {
					found = true
				}
			}
			if !found {
				t.Fatal("created user missing")
			}
			limits := driver.UserLimits{QueriesPerHour: 23, UpdatesPerHour: 7, ConnectionsPerHour: 9, UserConnections: 2}
			if err := d.SetUserLimits(ctx, account, limits); err != nil {
				t.Fatal(err)
			}
			users, err = d.ListUsers(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, u := range users {
				if u.User == account.User && u.Limits != limits {
					t.Fatalf("limits=%#v", u.Limits)
				}
			}
			if err := d.SetPassword(ctx, account, "another'password"); err != nil {
				t.Fatal(err)
			}
			req := driver.GrantChange{Scope: driver.GrantScope{Level: "database", Database: cfg.Database}, Privileges: []string{"SELECT", "INSERT"}, GrantOption: true}
			if err := d.GrantPrivileges(ctx, account, req); err != nil {
				t.Fatal(err)
			}
			grants, err := d.UserGrants(ctx, account)
			if err != nil {
				t.Fatal(err)
			}
			found = false
			for _, g := range grants {
				if g.Scope.Database == cfg.Database && g.Editable && g.GrantOption {
					found = true
				}
			}
			if !found {
				t.Fatalf("grant missing: %#v", grants)
			}
			global := driver.GrantChange{Scope: driver.GrantScope{Level: "global"}, Privileges: []string{"PROCESS"}, GrantOption: true}
			if err := d.GrantPrivileges(ctx, account, global); err != nil {
				t.Fatal(err)
			}
			grants, err = d.UserGrants(ctx, account)
			if err != nil {
				t.Fatal(err)
			}
			found = false
			for _, g := range grants {
				if g.Scope.Level == "global" && g.Editable && g.GrantOption {
					for _, p := range g.Privileges {
						if p == "PROCESS" {
							found = true
						}
					}
				}
			}
			if !found {
				t.Fatalf("global PROCESS grant missing after resource limits: %#v", grants)
			}
			login := cfg
			login.User = account.User
			login.Password = "another'password"
			other, err := Connect(ctx, login)
			if err != nil {
				t.Fatalf("changed password cannot authenticate: %v", err)
			}
			_ = other.Close()
			if _, err := d.db.ExecContext(ctx, "CREATE TABLE "+adminQuote(cfg.Database)+".`admin_grants_probe` (id int)"); err != nil {
				t.Fatal(err)
			}
			tableGrant := driver.GrantChange{Scope: driver.GrantScope{Level: "table", Database: cfg.Database, Table: "admin_grants_probe"}, Privileges: []string{"UPDATE"}}
			if err := d.GrantPrivileges(ctx, account, tableGrant); err != nil {
				t.Fatal(err)
			}
			if err := d.RevokePrivileges(ctx, account, tableGrant); err != nil {
				t.Fatal(err)
			}
			req.GrantOption = false
			if err := d.RevokePrivileges(ctx, account, req); err != nil {
				t.Fatal(err)
			}
			renamed := driver.UserAccount{User: "renamed probe", Host: "localhost"}
			if err := d.RenameUser(ctx, account, renamed); err != nil {
				t.Fatal(err)
			}
			account = renamed
			if err := d.GrantPrivileges(ctx, account, driver.GrantChange{Scope: driver.GrantScope{Level: "global"}, Privileges: []string{"SELECT; DROP USER root"}}); err == nil {
				t.Fatal("injection accepted")
			}
			if err := d.SetPassword(ctx, driver.UserAccount{User: "missing", Host: "%"}, "secret-leak-marker"); err == nil || strings.Contains(err.Error(), "secret-leak-marker") {
				t.Fatalf("password error redaction failed: %v", err)
			}
			processes, err := d.Processes(ctx)
			if err != nil || len(processes) == 0 {
				t.Fatalf("processes=%v err=%v", processes, err)
			}
			conn, err := d.db.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			var id int64
			if err := conn.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&id); err != nil {
				t.Fatal(err)
			}
			if err := d.KillProcess(ctx, id); err != nil {
				t.Fatal(err)
			}
			for _, fn := range []func(context.Context) ([]driver.ServerValue, error){d.ServerStatus, d.ServerVariables} {
				v, err := fn(ctx)
				if err != nil || len(v) == 0 {
					t.Fatalf("metrics=%v err=%v", v, err)
				}
			}
		})
	}
}
