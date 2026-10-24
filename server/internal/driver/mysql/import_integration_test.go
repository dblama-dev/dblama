//go:build integration

package mysql

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
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

// csvByID reads the imported table back through SelectRows and indexes
// the rows by their first column cast to int64.
func csvByID(t *testing.T, drv driver.Driver, db, table string) map[int64][]any {
	t.Helper()
	page, err := drv.SelectRows(context.Background(), db, table, sqlgen.SelectQuery{Limit: 1000})
	if err != nil {
		t.Fatalf("SelectRows %s.%s error: %v", db, table, err)
	}
	byID := make(map[int64][]any, len(page.Rows))
	for _, row := range page.Rows {
		id, ok := row[0].(int64)
		if !ok {
			t.Fatalf("row %v: first column is %T, want int64", row, row[0])
		}
		byID[id] = row
	}
	return byID
}

// TestImportCSV_Basic pins the delimited import basics: rows land in the
// existing table in ordinal column order, numeric fields are passed as
// strings and coerced server-side, an empty field becomes SQL NULL.
func TestImportCSV_Basic(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE imp_csv_t (id INT PRIMARY KEY, name VARCHAR(50), balance FLOAT)")
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS imp_csv_t") })

			// Default opts: separator ",", no header skip.
			data := "1,alice,100.5\n2,,\n3,bob,-3.25\n"
			if err := drv.ImportCSV(context.Background(), cfg.Database, "imp_csv_t",
				strings.NewReader(data), driver.CSVImportOpts{}, nil); err != nil {
				t.Fatalf("ImportCSV error: %v", err)
			}

			byID := csvByID(t, drv, cfg.Database, "imp_csv_t")
			if len(byID) != 3 {
				t.Fatalf("imported %d rows, want 3", len(byID))
			}
			// Numerics coerced server-side: the CSV text "1" lands in the
			// INT column and scans back as int64. The FLOAT value's Go
			// type differs between images/protocols (float32/float64/
			// string), so it is compared via its text form.
			if row := byID[1]; row[1] != "alice" || fmt.Sprint(row[2]) != "100.5" {
				t.Errorf("row 1 = %v, want [1 alice 100.5]", row)
			}
			// Empty fields → NULL in both a text and a numeric column.
			if row := byID[2]; row[1] != nil || row[2] != nil {
				t.Errorf("row 2 = %v, want name and balance NULL", row)
			}
			if row := byID[3]; row[1] != "bob" || fmt.Sprint(row[2]) != "-3.25" {
				t.Errorf("row 3 = %v, want [3 bob -3.25]", row)
			}
		})
	}
}

// TestImportCSV_Quoted pins the enclosure handling: a configured
// enclosure character shields the separator, doubled enclosures become
// literal enclosure characters and line breaks inside an enclosed field
// do not end the record.
func TestImportCSV_Quoted(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE imp_csv_q (id INT PRIMARY KEY, name VARCHAR(100), note VARCHAR(100))")
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS imp_csv_q") })

			data := "1;\"a;b\";\"say \"\"hi\"\"\"\n" +
				"2;\"multi\nline\";plain\n"
			opts := driver.CSVImportOpts{Separator: ";", Enclosure: `"`}
			if err := drv.ImportCSV(context.Background(), cfg.Database, "imp_csv_q",
				strings.NewReader(data), opts, nil); err != nil {
				t.Fatalf("ImportCSV error: %v", err)
			}

			byID := csvByID(t, drv, cfg.Database, "imp_csv_q")
			if len(byID) != 2 {
				t.Fatalf("imported %d rows, want 2", len(byID))
			}
			if row := byID[1]; row[1] != "a;b" || row[2] != `say "hi"` {
				t.Errorf("row 1 = %v, want [1 a;b say \"hi\"]", row)
			}
			if row := byID[2]; row[1] != "multi\nline" || row[2] != "plain" {
				t.Errorf("row 2 = %v, want name \"multi\\nline\" and note plain", row)
			}
		})
	}
}

// TestImportCSV_HeaderSkip pins SkipHeader: the file's first record is
// discarded and only the data rows land in the table.
func TestImportCSV_HeaderSkip(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE imp_csv_h (id INT PRIMARY KEY, name VARCHAR(20))")
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS imp_csv_h") })

			data := "id,name\n1,ann\n2,joe\n"
			opts := driver.CSVImportOpts{SkipHeader: true}
			if err := drv.ImportCSV(context.Background(), cfg.Database, "imp_csv_h",
				strings.NewReader(data), opts, nil); err != nil {
				t.Fatalf("ImportCSV error: %v", err)
			}

			byID := csvByID(t, drv, cfg.Database, "imp_csv_h")
			if len(byID) != 2 {
				t.Fatalf("imported %d rows, want 2 (header skipped)", len(byID))
			}
			if row := byID[1]; row[1] != "ann" {
				t.Errorf("row 1 = %v, want [1 ann]", row)
			}
			if row := byID[2]; row[1] != "joe" {
				t.Errorf("row 2 = %v, want [2 joe]", row)
			}
		})
	}
}

// TestImportCSV_Batching pins the batch mechanics on the boundaries: 250
// rows stream through the 100-row multi-row INSERT batches (100, 100,
// 50) and all of them arrive. The progress contract is pinned on the
// final call: done == total == the imported row count.
func TestImportCSV_Batching(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE imp_csv_b (id INT PRIMARY KEY, v VARCHAR(20))")
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS imp_csv_b") })

			var sb strings.Builder
			for i := 1; i <= 250; i++ {
				fmt.Fprintf(&sb, "%d,v%d\n", i, i)
			}

			var lastDone, lastTotal int64
			progress := func(done, total int64) { lastDone, lastTotal = done, total }
			if err := drv.ImportCSV(context.Background(), cfg.Database, "imp_csv_b",
				strings.NewReader(sb.String()), driver.CSVImportOpts{}, progress); err != nil {
				t.Fatalf("ImportCSV error: %v", err)
			}

			byID := csvByID(t, drv, cfg.Database, "imp_csv_b")
			if len(byID) != 250 {
				t.Fatalf("imported %d rows, want 250", len(byID))
			}
			for i := int64(1); i <= 250; i++ {
				if row, ok := byID[i]; !ok || row[1] != fmt.Sprintf("v%d", i) {
					t.Fatalf("row %d missing or wrong: %v", i, row)
				}
			}
			if lastDone != 250 || lastTotal != 250 {
				t.Errorf("final progress = (%d, %d), want (250, 250)", lastDone, lastTotal)
			}
		})
	}
}

// TestImportCSV_RowError pins the failure semantics: the error names the
// first row of the failing batch, the failing batch is applied
// atomically (none of its rows land) and the batches before it stay
// applied. The garbage row is the first row of batch 2, so the named
// row is the bad one.
func TestImportCSV_RowError(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE imp_csv_e (id INT PRIMARY KEY)")
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS imp_csv_e") })

			var sb strings.Builder
			for i := 1; i <= 100; i++ { // batch 1: rows 1..100
				fmt.Fprintf(&sb, "%d\n", i)
			}
			sb.WriteString("garbage-101\n") // row 101: opens batch 2, not numeric
			for i := 102; i <= 110; i++ {
				fmt.Fprintf(&sb, "%d\n", i)
			}

			err := drv.ImportCSV(context.Background(), cfg.Database, "imp_csv_e",
				strings.NewReader(sb.String()), driver.CSVImportOpts{}, nil)
			if err == nil {
				t.Fatal("ImportCSV with garbage into INT: expected error, got nil")
			}
			if !strings.Contains(err.Error(), "101") {
				t.Errorf("error %q does not mention the failing batch's first row 101", err)
			}

			// Batch 1 applied in full, batch 2 not at all (statement
			// atomicity).
			byID := csvByID(t, drv, cfg.Database, "imp_csv_e")
			if len(byID) != 100 {
				t.Errorf("applied %d rows, want exactly batch 1's 100", len(byID))
			}
			for i := 101; i <= 110; i++ {
				if _, ok := byID[int64(i)]; ok {
					t.Errorf("row %d applied, want rolled back with its batch", i)
				}
			}
		})
	}
}
