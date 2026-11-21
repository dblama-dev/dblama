//go:build integration

package postgres

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresDatabaseScope(t *testing.T) {
	cfg := startDB(t)
	admin := connectOK(t, cfg).(*PostgresDriver)
	ctx := context.Background()
	const target = `pop app 'main\two`
	const second = "other_workspace"
	const role = "workspace_reader"
	const password = `reader'\works`
	execDDL(t, admin, `CREATE ROLE workspace_reader LOGIN PASSWORD E'reader\'\\works'`)
	execDDL(t, admin, "CREATE DATABASE "+sqlgen.QuoteIdentPg(target)+" OWNER "+role)
	execDDL(t, admin, "CREATE DATABASE "+second+" OWNER "+role)

	cfg.User, cfg.Password, cfg.Database = role, password, ""
	root := connectOK(t, cfg).(*PostgresDriver)
	if _, err := root.ListTables(ctx, "postgres"); err != nil {
		t.Fatalf("fallback database must match the connection: %v", err)
	}
	if _, err := root.ListTables(ctx, target); err == nil {
		t.Fatal("unscoped driver accepted an alternate database")
	}
	if err := root.SetSchema("default_schema"); err != nil {
		t.Fatal(err)
	}

	t.Run("route selected database and request schema", func(t *testing.T) {
		bound, release := scopeDatabaseForTest(t, root, target)
		t.Cleanup(release)
		if got := bound.(driver.SchemaAware).CurrentSchema(); got != "default_schema" {
			t.Fatalf("scoped default schema = %q", got)
		}
		if _, err := bound.ExecSQL(ctx, target,
			`CREATE SCHEMA "request schema"; CREATE TABLE "request schema".target_table (database_name text DEFAULT current_database(), role_name text DEFAULT current_user)`,
			"", driver.ExecOpts{}); err != nil {
			t.Fatal(err)
		}
		schemaCtx := driver.WithSchema(ctx, "request schema")
		if err := bound.ImportSQL(schemaCtx, target, strings.NewReader("INSERT INTO target_table DEFAULT VALUES;"), nil); err != nil {
			t.Fatal(err)
		}
		results, err := bound.ExecSQL(schemaCtx, target, "SELECT * FROM target_table", "", driver.ExecOpts{})
		if err != nil || len(results) != 1 || !reflect.DeepEqual(results[0].Rows, [][]any{{target, role}}) {
			t.Fatalf("query on target database = %v, %v", results, err)
		}
		tables, err := bound.ListTables(schemaCtx, target)
		if err != nil || len(tables) != 1 || tables[0].Name != "target_table" {
			t.Fatalf("tables for target schema = %v, %v", tables, err)
		}
		if _, err := bound.ListTables(schemaCtx, second); err == nil {
			t.Fatal("driver bound to target accepted a different database")
		}
		tables, err = root.ListTables(schemaCtx, "postgres")
		if err != nil || len(tables) != 0 || root.CurrentSchema() != "default_schema" {
			t.Fatalf("root database or schema changed: %v, %v, %q", tables, err, root.CurrentSchema())
		}
		release()
		if _, err := bound.ListTables(schemaCtx, target); err == nil {
			t.Fatal("released scoped driver still accepted a query")
		}
	})

	t.Run("reuse root database", func(t *testing.T) {
		bound, release := scopeDatabaseForTest(t, root, "postgres")
		if bound != root {
			t.Fatal("opening the existing database should reuse the root driver")
		}
		release()
		if _, err := root.ListDatabases(ctx); err != nil {
			t.Fatalf("releasing root scope closed the session: %v", err)
		}
	})

	t.Run("explicit target errors", func(t *testing.T) {
		execDDL(t, admin, "CREATE DATABASE private_workspace")
		execDDL(t, admin, "REVOKE CONNECT ON DATABASE private_workspace FROM PUBLIC")
		for _, name := range []string{"missing_workspace", "private_workspace", "postgres\x00other", ""} {
			scoped, release, err := root.ForDatabase(ctx, name)
			if release != nil {
				release()
			}
			if scoped != nil || err == nil || strings.Contains(err.Error(), password) {
				t.Errorf("opening invalid database %q returned %v, %v", name, scoped != nil, err)
			}
		}
	})

	t.Run("cancel concurrent console queries with a full root pool", func(t *testing.T) {
		queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		databases := []string{target, second}
		done := make([]chan error, len(databases))
		for i, name := range databases {
			scoped, release := scopeDatabaseForTest(t, root, name)
			t.Cleanup(release)
			done[i] = make(chan error, 1)
			go func() {
				_, err := scoped.ExecSQL(queryCtx, name, "SELECT pg_sleep(30) /* scope cancellation */", name, driver.ExecOpts{})
				done[i] <- err
			}()
		}
		for {
			var active int
			err := root.db.QueryRowContext(queryCtx, `SELECT count(*) FROM pg_stat_activity
WHERE datname IN ($1, $2) AND query LIKE '%scope cancellation%' AND state = 'active'`, target, second).Scan(&active)
			if err != nil {
				t.Fatalf("wait for two running queries: %v", err)
			}
			if active == 2 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		for range maxOpenConns {
			conn, err := root.db.Conn(queryCtx)
			if err != nil {
				t.Fatalf("reserve root pool connection: %v", err)
			}
			defer func() { _ = conn.Close() }()
		}
		cancelCtx, stopCancel := context.WithTimeout(queryCtx, 2*time.Second)
		defer stopCancel()
		for i, name := range databases {
			if err := root.CancelQuery(cancelCtx, name); err != nil {
				t.Fatalf("cancel %q at root: %v", name, err)
			}
			select {
			case err := <-done[i]:
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || pgErr.Code != "57014" {
					t.Fatalf("console query on %q = %v, want backend cancellation", name, err)
				}
			case <-queryCtx.Done():
				t.Fatalf("console query on %q did not cancel: %v", name, queryCtx.Err())
			}
			if err := root.CancelQuery(cancelCtx, name); err != nil {
				t.Fatalf("cancel %q again after query completed: %v", name, err)
			}
		}
	})

	t.Run("waiting for a scope respects the request context", func(t *testing.T) {
		for range maxOpenConns {
			_, release := scopeDatabaseForTest(t, root, target)
			t.Cleanup(release)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		bound, release, err := root.ForDatabase(waitCtx, second)
		if release != nil {
			release()
		}
		if bound != nil || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("request waiting for an alternate database returned %v, %v", bound != nil, err)
		}
	})

	t.Run("close session releases existing scopes", func(t *testing.T) {
		var bound driver.Driver
		for range maxOpenConns {
			scoped, release := scopeDatabaseForTest(t, root, target)
			t.Cleanup(release)
			bound = scoped
		}
		waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		ready := make(chan struct{})
		waitErr := make(chan error, 1)
		go func() {
			close(ready)
			_, release, err := root.ForDatabase(waitCtx, second)
			if release != nil {
				release()
			}
			waitErr <- err
		}()
		<-ready
		if err := root.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := bound.ListTables(ctx, target); err == nil {
			t.Fatal("active scope accepted a query after session close")
		}
		if err := <-waitErr; !errors.Is(err, errSessionClosed) {
			t.Fatalf("request waiting for an alternate database at session close returned %v", err)
		}
	})

	t.Run("preserve role default when available", func(t *testing.T) {
		execDDL(t, admin, "CREATE DATABASE "+role+" OWNER "+role)
		d := connectOK(t, cfg)
		if _, err := d.ListTables(ctx, role); err != nil {
			t.Fatalf("database chosen by default for the role: %v", err)
		}
		if err := d.(driver.DatabaseManager).DropDatabase(ctx, role); err == nil || !strings.Contains(err.Error(), "session database") {
			t.Fatalf("default database was not protected: %v", err)
		}
	})
}

func scopeDatabaseForTest(t *testing.T, root driver.Driver, name string) (driver.Driver, func()) {
	t.Helper()
	d, release, err := root.(driver.DatabaseConnector).ForDatabase(context.Background(), name)
	if err != nil {
		t.Fatalf("open database scope %q: %v", name, err)
	}
	return d, release
}
