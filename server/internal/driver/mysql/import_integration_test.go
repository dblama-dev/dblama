//go:build integration

package mysql

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// importScript runs one ImportSQL call with a nil progress callback and
// fails the test on error.
func importScript(t *testing.T, drv driver.Driver, db, script string) {
	t.Helper()
	if err := drv.ImportSQL(context.Background(), db, strings.NewReader(script), nil); err != nil {
		t.Fatalf("ImportSQL(%s) error: %v", db, err)
	}
}

// rawQuery runs one query on a raw connection and returns its rows.
func rawQuery(t *testing.T, cfg driver.ConnConfig, query string) *sql.Rows {
	t.Helper()
	db, err := sql.Open("mysql", DSN(cfg, cfg.Password))
	if err != nil {
		t.Fatalf("open raw connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	t.Cleanup(func() { _ = rows.Close() })
	return rows
}

// TestImportSQL_Basic pins the Export↔Import contract: a dump produced
// by Export imports into a fresh database and reproduces both the table
// structure (TableDef, modulo AUTO_INCREMENT) and the data.
func TestImportSQL_Basic(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			execDDL(t, cfg, "CREATE TABLE imp_basic_t (id INT PRIMARY KEY, name VARCHAR(100), created DATETIME)")
			execDDL(t, cfg, "INSERT INTO imp_basic_t VALUES (1, 'первый', '2026-01-02 10:20:30')")
			execDDL(t, cfg, "INSERT INTO imp_basic_t VALUES (2, NULL, NULL)")
			execDDL(t, cfg, "INSERT INTO imp_basic_t VALUES (3, 'quote''and,comma', '2026-12-31 23:59:59')")
			t.Cleanup(func() {
				execDDL(t, cfg, "DROP TABLE IF EXISTS imp_basic_t")
				execDDL(t, cfg, "DROP DATABASE IF EXISTS imp_basic_target")
			})

			var dump bytes.Buffer
			if err := drv.Export(ctx, driver.ExportRequest{
				Format: "sql",
				Tables: []string{"imp_basic_t"},
			}, &dump); err != nil {
				t.Fatalf("Export(sql) error: %v", err)
			}

			execDDL(t, cfg, "CREATE DATABASE imp_basic_target")
			importScript(t, drv, "imp_basic_target", dump.String())

			srcDef, err := drv.TableDef(ctx, cfg.Database, "imp_basic_t")
			if err != nil {
				t.Fatalf("TableDef source error: %v", err)
			}
			tgtDef, err := drv.TableDef(ctx, "imp_basic_target", "imp_basic_t")
			if err != nil {
				t.Fatalf("TableDef target error: %v", err)
			}
			srcDef.Options.AutoIncrement, tgtDef.Options.AutoIncrement = 0, 0
			if !reflect.DeepEqual(srcDef, tgtDef) {
				t.Errorf("TableDef mismatch:\nsource: %+v\ntarget: %+v", srcDef, tgtDef)
			}

			srcPage, err := drv.SelectRows(ctx, cfg.Database, "imp_basic_t", sqlgen.SelectQuery{Limit: 1000})
			if err != nil {
				t.Fatalf("SelectRows source error: %v", err)
			}
			tgtPage, err := drv.SelectRows(ctx, "imp_basic_target", "imp_basic_t", sqlgen.SelectQuery{Limit: 1000})
			if err != nil {
				t.Fatalf("SelectRows target error: %v", err)
			}
			sortDumpRows(srcPage.Rows)
			sortDumpRows(tgtPage.Rows)
			if !reflect.DeepEqual(srcPage.Rows, tgtPage.Rows) {
				t.Errorf("data mismatch:\nsource: %v\ntarget: %v", srcPage.Rows, tgtPage.Rows)
			}
		})
	}
}

// TestImportSQL_DelimiterBlock feeds a hand-written dump whose trigger
// is wrapped in DELIMITER $$ ... $$ with internal semicolons: the
// trigger must be created whole (not chopped at the internal ';') and
// the following INSERTs must fire it.
func TestImportSQL_DelimiterBlock(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			script := "CREATE TABLE imp_trg_t (id INT PRIMARY KEY, log VARCHAR(100));\n" +
				"DELIMITER $$\n" +
				"CREATE TRIGGER imp_trg_bi BEFORE INSERT ON imp_trg_t\n" +
				"FOR EACH ROW\n" +
				"BEGIN\n" +
				"  SET NEW.log = CONCAT('id=', CAST(NEW.id AS CHAR));\n" +
				"END$$\n" +
				"DELIMITER ;\n" +
				"INSERT INTO imp_trg_t (id) VALUES (1);\n" +
				"INSERT INTO imp_trg_t (id) VALUES (2);\n"
			t.Cleanup(func() {
				execDDL(t, cfg, "DROP TABLE IF EXISTS imp_trg_t")
				execDDL(t, cfg, "DROP DATABASE IF EXISTS imp_trg_target")
			})

			execDDL(t, cfg, "CREATE DATABASE imp_trg_target")
			importScript(t, drv, "imp_trg_target", script)

			// The trigger exists and its body is intact.
			rows := rawQuery(t, cfg, "SHOW TRIGGERS FROM "+sqlgen.QuoteIdent("imp_trg_target"))
			var found bool
			var stmt string
			for rows.Next() {
				var name, event, table, timing string
				if err := rows.Scan(&name, &event, &table, &stmt, &timing,
					new(sql.NullString), new(sql.NullString), new(sql.NullString),
					new(sql.NullString), new(sql.NullString), new(sql.NullString)); err != nil {
					t.Fatalf("scan trigger row: %v", err)
				}
				if name == "imp_trg_bi" {
					found = true
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("iterate triggers: %v", err)
			}
			if !found {
				t.Fatal("trigger imp_trg_bi not created")
			}
			if !strings.Contains(stmt, "BEGIN") || !strings.Contains(stmt, "CONCAT") {
				t.Errorf("trigger body truncated: %q", stmt)
			}

			// The inserts applied and fired the trigger.
			logRows := rawQuery(t, cfg, "SELECT id, log FROM imp_trg_target.imp_trg_t ORDER BY id")
			var got []string
			var id int
			var lg sql.NullString
			for logRows.Next() {
				if err := logRows.Scan(&id, &lg); err != nil {
					t.Fatalf("scan row: %v", err)
				}
				got = append(got, lg.String)
			}
			if err := logRows.Err(); err != nil {
				t.Fatalf("iterate rows: %v", err)
			}
			want := []string{"id=1", "id=2"}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("rows = %v, want %v", got, want)
			}
		})
	}
}

// TestImportSQL_Gzip imports a gzip-wrapped dump: the gzip magic is
// detected and the stream decompressed transparently.
func TestImportSQL_Gzip(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE imp_gz_t (id INT PRIMARY KEY, v VARCHAR(20))")
			execDDL(t, cfg, "INSERT INTO imp_gz_t VALUES (1, 'a'), (2, 'b')")
			t.Cleanup(func() {
				execDDL(t, cfg, "DROP TABLE IF EXISTS imp_gz_t")
				execDDL(t, cfg, "DROP DATABASE IF EXISTS imp_gz_target")
			})

			var plain bytes.Buffer
			if err := drv.Export(context.Background(), driver.ExportRequest{
				Format: "sql",
				Tables: []string{"imp_gz_t"},
			}, &plain); err != nil {
				t.Fatalf("Export(sql) error: %v", err)
			}
			var gzBuf bytes.Buffer
			gw := gzip.NewWriter(&gzBuf)
			if _, err := gw.Write(plain.Bytes()); err != nil {
				t.Fatalf("gzip write: %v", err)
			}
			if err := gw.Close(); err != nil {
				t.Fatalf("gzip close: %v", err)
			}

			execDDL(t, cfg, "CREATE DATABASE imp_gz_target")
			importScript(t, drv, "imp_gz_target", gzBuf.String())

			page, err := drv.SelectRows(context.Background(), "imp_gz_target", "imp_gz_t", sqlgen.SelectQuery{Limit: 100})
			if err != nil {
				t.Fatalf("SelectRows target error: %v", err)
			}
			if len(page.Rows) != 2 {
				t.Errorf("imported %d rows, want 2", len(page.Rows))
			}
		})
	}
}

// TestImportSQL_Progress pins the progress contract: the callback is
// called at least twice, done never decreases and the final call
// reports done == total > 0.
func TestImportSQL_Progress(t *testing.T) {
	cfg := startDB(t, "mysql:8.4")
	drv := connectOK(t, cfg)

	script := "CREATE TABLE imp_prog_t (id INT PRIMARY KEY);\n"
	for i := 0; i < 50; i++ {
		script += "INSERT INTO imp_prog_t (id) VALUES (" + strconv.Itoa(i) + ");\n"
	}
	t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS imp_prog_t") })

	var calls int
	var lastDone, lastTotal int64
	progress := func(done, total int64) {
		calls++
		if done < lastDone {
			t.Errorf("progress not monotonic: done %d after %d", done, lastDone)
		}
		lastDone, lastTotal = done, total
	}
	if err := drv.ImportSQL(context.Background(), cfg.Database, strings.NewReader(script), progress); err != nil {
		t.Fatalf("ImportSQL error: %v", err)
	}
	if calls < 2 {
		t.Errorf("progress called %d times, want >= 2", calls)
	}
	if lastDone <= 0 {
		t.Errorf("final done = %d, want > 0", lastDone)
	}
	if lastDone != lastTotal {
		t.Errorf("final progress = (%d, %d), want done == total", lastDone, lastTotal)
	}
}

// TestImportSQL_MidError stops at the first failing statement: the
// error names the statement index and its excerpt, statements before it
// stay applied and the last progress reflects the failure position.
func TestImportSQL_MidError(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			bad := "INSERT INTO imp_no_such_table VALUES (99, 'boom here')"
			script := "CREATE TABLE imp_err_t (id INT PRIMARY KEY, v VARCHAR(50));\n" +
				"INSERT INTO imp_err_t VALUES (1, 'ok');\n" +
				bad + ";\n" +
				"INSERT INTO imp_err_t VALUES (2, 'never');\n"
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS imp_err_t") })

			var lastDone int64
			progress := func(done, total int64) { lastDone = done }
			err := drv.ImportSQL(context.Background(), cfg.Database, strings.NewReader(script), progress)
			if err == nil {
				t.Fatal("ImportSQL with bad statement: expected error, got nil")
			}
			excerpt := bad
			if len(excerpt) > 100 {
				excerpt = excerpt[:100]
			}
			if !strings.Contains(err.Error(), excerpt) {
				t.Errorf("error %q does not contain the failing statement excerpt %q", err, excerpt)
			}
			if !strings.Contains(err.Error(), "3") {
				t.Errorf("error %q does not mention the statement index 3", err)
			}

			// Statements before the failure are applied, after are not.
			page, err := drv.SelectRows(context.Background(), cfg.Database, "imp_err_t", sqlgen.SelectQuery{Limit: 100})
			if err != nil {
				t.Fatalf("SelectRows error: %v", err)
			}
			if len(page.Rows) != 1 {
				t.Errorf("applied %d rows, want exactly the 1 before the failure", len(page.Rows))
			}

			// Progress reflects the position: past the first statement,
			// short of the whole script.
			if lastDone <= 0 {
				t.Errorf("progress after failure done = %d, want > 0", lastDone)
			}
		})
	}
}
