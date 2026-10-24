//go:build integration

package mysql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// rowStrings renders a Result's rows as comparable strings: one string
// per row, cells joined with ",". Keeps the assertions independent of
// the exact Go types database/sql delivers (int64 vs string).
func rowStrings(res driver.Result) []string {
	out := make([]string, 0, len(res.Rows))
	for _, row := range res.Rows {
		cells := make([]string, len(row))
		for i, v := range row {
			cells[i] = fmt.Sprint(v)
		}
		out = append(out, strings.Join(cells, ","))
	}
	return out
}

func TestExecSQL_MultiSelect(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			results, err := drv.ExecSQL(context.Background(), cfg.Database, "SELECT 1; SELECT 2", "q-multi", driver.ExecOpts{})
			if err != nil {
				t.Fatalf("ExecSQL() error: %v", err)
			}
			if len(results) != 2 {
				t.Fatalf("ExecSQL() returned %d results, want 2", len(results))
			}
			for i, want := range []string{"1", "2"} {
				got := rowStrings(results[i])
				if len(got) != 1 || got[0] != want {
					t.Errorf("result %d rows = %v, want [%s]", i, got, want)
				}
				if len(results[i].Columns) != 1 {
					t.Errorf("result %d has %d columns, want 1", i, len(results[i].Columns))
				}
				if results[i].Columns[0].Name == "" {
					t.Errorf("result %d column name is empty", i)
				}
				if results[i].Seconds < 0 {
					t.Errorf("result %d Seconds = %v, want >= 0", i, results[i].Seconds)
				}
			}
		})
	}
}

func TestExecSQL_MixedStatements(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE t_exec (id INT PRIMARY KEY AUTO_INCREMENT, v VARCHAR(20))")
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE t_exec") })

			query := "INSERT INTO t_exec (v) VALUES ('a'); SELECT v FROM t_exec ORDER BY id"
			results, err := drv.ExecSQL(context.Background(), cfg.Database, query, "q-mixed", driver.ExecOpts{})
			if err != nil {
				t.Fatalf("ExecSQL() error: %v", err)
			}
			if len(results) != 2 {
				t.Fatalf("ExecSQL() returned %d results, want 2", len(results))
			}
			if results[0].AffectedRows != 1 {
				t.Errorf("INSERT AffectedRows = %d, want 1", results[0].AffectedRows)
			}
			if results[0].LastInsertID != 1 {
				t.Errorf("INSERT LastInsertID = %d, want 1", results[0].LastInsertID)
			}
			if got := rowStrings(results[1]); len(got) != 1 || got[0] != "a" {
				t.Errorf("SELECT rows = %v, want [a]", got)
			}
		})
	}
}

func TestExecSQL_MidError(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			results, err := drv.ExecSQL(context.Background(), cfg.Database, "SELECT 1; BADSYNTAX; SELECT 2", "q-err", driver.ExecOpts{})
			if err == nil {
				t.Fatal("ExecSQL() error: expected error, got nil")
			}
			if !strings.Contains(err.Error(), "syntax") {
				t.Errorf("ExecSQL() error %q does not mention the syntax problem", err)
			}
			// The result of the statement before the error must survive.
			if len(results) != 1 {
				t.Fatalf("ExecSQL() returned %d results with the error, want 1", len(results))
			}
			if got := rowStrings(results[0]); len(got) != 1 || got[0] != "1" {
				t.Errorf("first result rows = %v, want [1]", got)
			}

			// Review note (Task 1): a batch that aborts mid-error must
			// have unregistered itself too.
			if n := registryLen(drv.(*MysqlDriver)); n != 0 {
				t.Errorf("registry holds %d entries after a mid-error batch, want 0", n)
			}
		})
	}
}

// registryLen reads the whitebox size of the driver's running-query
// registry (the test lives in the same package).
func registryLen(d *MysqlDriver) int {
	d.queries.mu.Lock()
	defer d.queries.mu.Unlock()
	return len(d.queries.conns)
}

func TestExecSQL_Cancel(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)
			md := drv.(*MysqlDriver)

			// A long-running statement (≈1e9 row combinations, far more
			// than the <5s the test allows). Pinned deviation from the
			// brief's SELECT SLEEP(30): MySQL's SLEEP returns 1 without an
			// error when interrupted by KILL QUERY, so it cannot pin the
			// "ExecSQL comes back with an interruption error" contract; a
			// heavy join errors with 1317 ("Query execution was
			// interrupted") on both servers.
			execDDL(t, cfg, "CREATE TABLE cancel_probe (id INT PRIMARY KEY)")
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE cancel_probe") })
			execDDL(t, cfg, "INSERT INTO cancel_probe WITH RECURSIVE seq AS (SELECT 1 AS id UNION ALL SELECT id+1 FROM seq WHERE id < 1000) SELECT id FROM seq")

			longQuery := "SELECT SUM(a.id + b.id + c.id) FROM cancel_probe a JOIN cancel_probe b JOIN cancel_probe c"

			start := time.Now()
			errCh := make(chan error, 1)
			go func() {
				_, err := drv.ExecSQL(context.Background(), cfg.Database, longQuery, "q-cancel-test", driver.ExecOpts{})
				errCh <- err
			}()

			// Give the goroutine time to acquire its dedicated connection
			// and start the long query, then cancel it.
			time.Sleep(time.Second)
			if err := md.CancelQuery(context.Background(), "q-cancel-test"); err != nil {
				t.Errorf("CancelQuery(running) error: %v, want nil", err)
			}

			// The running ExecSQL must come back quickly with an
			// interruption error (MySQL: "Query execution was interrupted";
			// MariaDB wording differs slightly — match loosely).
			select {
			case err := <-errCh:
				if err == nil {
					t.Fatal("ExecSQL() error: expected cancellation error, got nil")
				}
				lower := strings.ToLower(err.Error())
				if !strings.Contains(lower, "interrupted") && !strings.Contains(lower, "killed") {
					t.Errorf("ExecSQL() error %q mentions neither interrupted nor killed", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("ExecSQL() did not return within 5s of CancelQuery")
			}
			if elapsed := time.Since(start); elapsed > 6*time.Second {
				t.Errorf("cancelled ExecSQL() returned after %v total, want < 6s", elapsed)
			}

			// The cancelled batch must have unregistered itself.
			if n := registryLen(md); n != 0 {
				t.Errorf("registry holds %d entries after cancellation, want 0", n)
			}

			// Cancelling the now-finished query again is not an error
			// (unknown id = already finished, brief ruling).
			if err := md.CancelQuery(context.Background(), "q-cancel-test"); err != nil {
				t.Errorf("CancelQuery(finished) error: %v, want nil", err)
			}

			// The KILL must leave the pool healthy: a follow-up query works.
			results, err := drv.ExecSQL(context.Background(), cfg.Database, "SELECT 1", "q-after-cancel", driver.ExecOpts{})
			if err != nil {
				t.Fatalf("ExecSQL() after cancellation: %v", err)
			}
			if got := rowStrings(results[0]); len(got) != 1 || got[0] != "1" {
				t.Errorf("rows after cancellation = %v, want [1]", got)
			}
		})
	}
}

func TestExecSQL_RowLimit(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			query := "SELECT 1 AS n UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5"
			results, err := drv.ExecSQL(context.Background(), cfg.Database, query, "q-limit", driver.ExecOpts{MaxRows: 2})
			if err != nil {
				t.Fatalf("ExecSQL() error: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("ExecSQL() returned %d results, want 1", len(results))
			}
			got := rowStrings(results[0])
			if len(got) != 2 {
				t.Fatalf("rows = %v, want 2 rows", got)
			}
			if got[0] != "1" || got[1] != "2" {
				t.Errorf("rows = %v, want [1 2]", got)
			}
			if !results[0].Truncated {
				t.Error("Truncated = false, want true")
			}
			if len(results[0].Columns) != 1 || results[0].Columns[0].Name != "n" {
				t.Errorf("columns = %+v, want one column named n", results[0].Columns)
			}
		})
	}
}

func TestExecSQL_Timeout(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			start := time.Now()
			_, err := drv.ExecSQL(context.Background(), cfg.Database, "SELECT SLEEP(5)", "q-timeout", driver.ExecOpts{TimeoutSec: 1})
			elapsed := time.Since(start)
			if err == nil {
				t.Fatal("ExecSQL() error: expected error, got nil")
			}
			if elapsed > 3*time.Second {
				t.Errorf("ExecSQL() returned after %v, want < 3s", elapsed)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("ExecSQL() error %v is not a context deadline", err)
			}
		})
	}
}

func TestExecSQL_UseOtherDb(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE DATABASE exec_sql_other")
			t.Cleanup(func() { execDDL(t, cfg, "DROP DATABASE exec_sql_other") })
			execDDL(t, cfg, "CREATE TABLE exec_sql_other.t (v INT)")
			execDDL(t, cfg, "INSERT INTO exec_sql_other.t VALUES (7)")

			// The pool is bound to the login database: the driver must
			// switch the dedicated connection to exec_sql_other first.
			results, err := drv.ExecSQL(context.Background(), "exec_sql_other", "SELECT v FROM t", "q-usedb", driver.ExecOpts{})
			if err != nil {
				t.Fatalf("ExecSQL() on another database: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("ExecSQL() returned %d results, want 1", len(results))
			}
			if got := rowStrings(results[0]); len(got) != 1 || got[0] != "7" {
				t.Errorf("rows = %v, want [7]", got)
			}
		})
	}
}

func TestExecSQL_EmptyDb(t *testing.T) {
	cfg := startDB(t, "mysql:8.4")
	drv := connectOK(t, cfg)

	if _, err := drv.ExecSQL(context.Background(), "", "SELECT 1", "q-nodb", driver.ExecOpts{}); err == nil {
		t.Fatal("ExecSQL() with an empty db: expected error, got nil")
	}
}

func TestExecSQL_CancelUnknownQuery(t *testing.T) {
	cfg := startDB(t, "mysql:8.4")
	drv := connectOK(t, cfg)

	// An unknown queryID means the query already finished — not an error
	// (brief ruling).
	if err := drv.CancelQuery(context.Background(), "no-such-query"); err != nil {
		t.Errorf("CancelQuery(unknown) error: %v, want nil", err)
	}
}
