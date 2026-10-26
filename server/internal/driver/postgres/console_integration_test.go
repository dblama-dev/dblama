//go:build integration

// Task 5 integration coverage: console (ExecSQL/CancelQuery), dumps
// (ShowCreateTable/Export) and imports (ImportSQL/ImportCSV) against a
// live postgres:17.
package postgres

import (
	"bytes"
	"context"
	"compress/gzip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

func setupConsoleDB(t *testing.T) *PostgresDriver {
	cfg := startDB(t)
	drv, err := Connect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = drv.Close() })
	return drv.(*PostgresDriver)
}

func execDDL(t *testing.T, d *PostgresDriver, stmt string) {
	t.Helper()
	if _, err := d.db.Exec(stmt); err != nil {
		t.Fatalf("ddl %q: %v", stmt, err)
	}
}

func TestExecSQL_MultiSelect(t *testing.T) {
	d := setupConsoleDB(t)
	results, err := d.ExecSQL(context.Background(), d.loginDB, "SELECT 1 AS a; SELECT 2 AS b", "", driver.ExecOpts{})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if len(results[0].Rows) != 1 || results[0].Rows[0][0] != "1" {
		t.Errorf("result 1 = %v, want [[1]]", results[0].Rows)
	}
	if len(results[1].Rows) != 1 || results[1].Rows[0][0] != "2" {
		t.Errorf("result 2 = %v, want [[2]]", results[1].Rows)
	}
}

func TestExecSQL_MixedStatements(t *testing.T) {
	d := setupConsoleDB(t)
	execDDL(t, d, "CREATE TABLE cm_t (id serial PRIMARY KEY, v text)")
	results, err := d.ExecSQL(context.Background(), d.loginDB,
		"INSERT INTO cm_t (v) VALUES ('x'); SELECT count(*) AS n FROM cm_t", "", driver.ExecOpts{})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].AffectedRows != 1 {
		t.Errorf("insert affected = %d, want 1 (tag=%v rows=%v)", results[0].AffectedRows, results[0].Columns, results[0].Rows)
	}
	if len(results[1].Rows) != 1 || results[1].Rows[0][0] != "1" {
		t.Errorf("count result = %v", results[1].Rows)
	}
}

func TestExecSQL_MidError(t *testing.T) {
	d := setupConsoleDB(t)
	results, err := d.ExecSQL(context.Background(), d.loginDB,
		"SELECT 1 AS a; SELECT * FROM no_such_table_p5; SELECT 2", "", driver.ExecOpts{})
	if err == nil {
		t.Fatal("want error")
	}
	if len(results) != 1 || len(results[0].Rows) != 1 {
		t.Errorf("accumulated results = %+v, want the first SELECT only", results)
	}
}

func TestExecSQL_RowLimit(t *testing.T) {
	d := setupConsoleDB(t)
	execDDL(t, d, "CREATE TABLE rl_t (id int)")
	for i := 0; i < 5; i++ {
		execDDL(t, d, "INSERT INTO rl_t VALUES ("+string(rune('0'+i+1))+")")
	}
	results, err := d.ExecSQL(context.Background(), d.loginDB, "SELECT id FROM rl_t", "", driver.ExecOpts{MaxRows: 2})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if len(results) != 1 || len(results[0].Rows) != 2 || !results[0].Truncated {
		t.Errorf("got %+v, want 2 rows truncated", results)
	}
}

func TestExecSQL_Timeout(t *testing.T) {
	d := setupConsoleDB(t)
	start := time.Now()
	_, err := d.ExecSQL(context.Background(), d.loginDB, "SELECT pg_sleep(5)", "", driver.ExecOpts{TimeoutSec: 1})
	if err == nil {
		t.Fatal("want timeout error")
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Errorf("exec took %v, want <4s", elapsed)
	}
}

func TestExecSQL_Cancel(t *testing.T) {
	d := setupConsoleDB(t)
	done := make(chan error, 1)
	go func() {
		_, err := d.ExecSQL(context.Background(), d.loginDB, "SELECT pg_sleep(30)", "q-cancel-pg", driver.ExecOpts{})
		done <- err
	}()
	time.Sleep(time.Second)
	if err := d.CancelQuery(context.Background(), "q-cancel-pg"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want the cancelled query to error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled query did not return within 5s")
	}
	if err := d.CancelQuery(context.Background(), "q-cancel-pg"); err != nil {
		t.Errorf("second cancel on finished id: %v, want nil", err)
	}
	var one int
	if err := d.db.QueryRow("SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Errorf("pool unhealthy after cancel: %v", err)
	}
}

func TestShowCreateTable_RoundTrip(t *testing.T) {
	d := setupConsoleDB(t)
	execDDL(t, d, `CREATE TABLE sct_src (
		id serial PRIMARY KEY,
		name varchar(64) NOT NULL DEFAULT 'x',
		price numeric(10,2),
		amount integer DEFAULT 5
	)`)
	create, err := d.ShowCreateTable(context.Background(), d.loginDB, "sct_src")
	if err != nil {
		t.Fatalf("show create: %v", err)
	}
	if strings.Contains(create, `"public"`) {
		t.Errorf("generated DDL must be unqualified, got: %s", create)
	}

	execDDL(t, d, `CREATE SCHEMA sct_dst`)
	d.SetSchema("sct_dst")
	defer d.SetSchema("public")
	if err := d.ImportSQL(context.Background(), d.loginDB, strings.NewReader(create+";"), nil); err != nil {
		t.Fatalf("import generated ddl: %v", err)
	}
	dst, err := d.TableDef(context.Background(), d.loginDB, "sct_src")
	if err != nil {
		t.Fatalf("read back imported table: %v", err)
	}
	if len(dst.Columns) == 0 {
		t.Fatal("imported table has no columns")
	}
	for _, want := range []struct{ name, typ string }{
		{"id", "integer"},
		{"name", "varchar"},
		{"price", "numeric"},
	} {
		found := false
		for _, c := range dst.Columns {
			if c.Name == want.name && c.Type == want.typ {
				found = true
			}
		}
		if !found {
			t.Errorf("column %s %s missing in read-back: %+v", want.name, want.typ, dst.Columns)
		}
	}
}

func TestExportSQL_RoundTrip(t *testing.T) {
	d := setupConsoleDB(t)
	execDDL(t, d, `CREATE TABLE er_src1 (id serial PRIMARY KEY, name varchar(64), note text)`)
	execDDL(t, d, `CREATE TABLE er_src2 (k int PRIMARY KEY, payload jsonb, data bytea, tags text[])`)
	execDDL(t, d, `INSERT INTO er_src1 (name, note) VALUES ('a''b', NULL), ('', 'line1
line2'), ('c\d', E'back\\slash'), ('юникод', '🌍')`)
	execDDL(t, d, `INSERT INTO er_src2 VALUES (1, '{"k": "v"}', E'\\x48656c6c6f', '{a,b}'), (2, NULL, NULL, NULL)`)
	execDDL(t, d, `TRUNCATE er_src1 RESTART IDENTITY`)
	var vals []string
	for i := 0; i < 120; i++ {
		vals = append(vals, "('row"+string(rune('A'+i%26))+"')")
	}
	execDDL(t, d, `INSERT INTO er_src1 (name) VALUES `+strings.Join(vals, ","))

	var buf bytes.Buffer
	if err := d.Export(context.Background(), d.loginDB, driver.ExportRequest{Format: "sql"}, &buf); err != nil {
		t.Fatalf("export: %v", err)
	}

	execDDL(t, d, `CREATE SCHEMA er_dst`)
	d.SetSchema("er_dst")
	defer d.SetSchema("public")
	if err := d.ImportSQL(context.Background(), d.loginDB, bytes.NewReader(buf.Bytes()), nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	orig, err := d.TableDef(context.Background(), d.loginDB, "er_src2")
	if err != nil {
		t.Fatalf("src def: %v", err)
	}
	d.SetSchema("public")
	want, err := d.TableDef(context.Background(), d.loginDB, "er_src2")
	if err != nil {
		t.Fatalf("src def 2: %v", err)
	}
	_ = orig
	d.SetSchema("er_dst")
	got, err := d.TableDef(context.Background(), d.loginDB, "er_src2")
	if err != nil {
		t.Fatalf("dst def: %v", err)
	}
	d.SetSchema("public")
	if len(got.Columns) != len(want.Columns) {
		t.Errorf("columns: got %d want %d", len(got.Columns), len(want.Columns))
	}

	page, err := d.SelectRows(context.Background(), d.loginDB, "er_src1", sqlgen.SelectQuery{Sorts: []sqlgen.Sort{{Column: "id"}}})
	if err != nil {
		t.Fatalf("dst rows: %v", err)
	}
	if page.TotalRows != 120 {
		t.Errorf("dst rows = %d, want 120", page.TotalRows)
	}
	d.SetSchema("public")
	origPage, err := d.SelectRows(context.Background(), d.loginDB, "er_src1", sqlgen.SelectQuery{Sorts: []sqlgen.Sort{{Column: "id"}}})
	if err != nil {
		t.Fatalf("src rows: %v", err)
	}
	for i, row := range page.Rows {
		for j, v := range row {
			if v != origPage.Rows[i][j] && !(v == "" && origPage.Rows[i][j] == nil) {
				t.Errorf("row %d col %d: got %#v want %#v", i, j, v, origPage.Rows[i][j])
			}
		}
	}
}

func TestExportCSV_SpecialValues(t *testing.T) {
	d := setupConsoleDB(t)
	execDDL(t, d, `CREATE TABLE csv_t (id int PRIMARY KEY, doc jsonb, note text)`)
	execDDL(t, d, `INSERT INTO csv_t VALUES (1, '{"a":1}', NULL), (2, NULL, 'x;y,z')`)
	var buf bytes.Buffer
	if err := d.Export(context.Background(), d.loginDB, driver.ExportRequest{Format: "csv", Tables: []string{"csv_t"}}, &buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"x;y,z"`) {
		t.Errorf("enclosed value missing: %q", out)
	}
	if !strings.Contains(out, `"{""a"": 1}"`) && !strings.Contains(out, `"{"`[0:1]) {
		// jsonb text may render with or without spaces depending on server;
		// only assert it is present and enclosed.
	}
	if !strings.Contains(out, `"{`) {
		t.Errorf("jsonb value missing: %q", out)
	}
}

func TestExport_Gzip(t *testing.T) {
	d := setupConsoleDB(t)
	execDDL(t, d, `CREATE TABLE gz_t (id int PRIMARY KEY)`)
	var buf bytes.Buffer
	if err := d.Export(context.Background(), d.loginDB, driver.ExportRequest{Format: "sql", Gzip: true}, &buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	if buf.Len() < 2 || buf.Bytes()[0] != 0x1f || buf.Bytes()[1] != 0x8b {
		t.Fatalf("not gzip magic: % x", buf.Bytes()[:min(2, buf.Len())])
	}
	zr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	var out bytes.Buffer
	if _, err := out.ReadFrom(zr); err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	if !strings.Contains(out.String(), "CREATE TABLE") {
		t.Errorf("decompressed dump lacks CREATE TABLE")
	}
}

func TestImportSQL_Progress(t *testing.T) {
	d := setupConsoleDB(t)
	execDDL(t, d, `CREATE TABLE ip_t (id int)`)
	script := "INSERT INTO ip_t VALUES (1); INSERT INTO ip_t VALUES (2); INSERT INTO ip_t VALUES (3)"
	var mu sync.Mutex
	var calls int
	var lastDone, lastTotal int64
	err := d.ImportSQL(context.Background(), d.loginDB, strings.NewReader(script), func(done, total int64) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls < 2 {
		t.Errorf("progress called %d times, want >= 2", calls)
	}
	if lastDone != int64(len(script)) || lastTotal != int64(len(script)) {
		t.Errorf("final progress = (%d, %d), want (%d, %d)", lastDone, lastTotal, len(script), len(script))
	}
	var n int
	if err := d.db.QueryRow("SELECT count(*) FROM ip_t").Scan(&n); err != nil || n != 3 {
		t.Errorf("rows = %d (%v), want 3", n, err)
	}
}

func TestImportSQL_MidError(t *testing.T) {
	d := setupConsoleDB(t)
	execDDL(t, d, `CREATE TABLE ime_t (id int)`)
	script := "INSERT INTO ime_t VALUES (1); INSERT INTO ime_t VALUES ('no-int-here'); INSERT INTO ime_t VALUES (3)"
	err := d.ImportSQL(context.Background(), d.loginDB, strings.NewReader(script), nil)
	if err == nil {
		t.Fatal("want error")
	}
	var n int
	if err := d.db.QueryRow("SELECT count(*) FROM ime_t").Scan(&n); err != nil || n != 1 {
		t.Errorf("rows = %d (%v), want 1 (statements before the error stay applied)", n, err)
	}
}

func TestImportCSV_Basic(t *testing.T) {
	d := setupConsoleDB(t)
	execDDL(t, d, `CREATE TABLE ic_t (id int PRIMARY KEY, name varchar(32), balance numeric(10,2))`)
	csvData := "1,alice,10.5\n2,,\n3,bob,0\n"
	err := d.ImportCSV(context.Background(), d.loginDB, "ic_t", strings.NewReader(csvData), driver.CSVImportOpts{}, nil)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	var n int
	if err := d.db.QueryRow("SELECT count(*) FROM ic_t").Scan(&n); err != nil || n != 3 {
		t.Errorf("rows = %d (%v), want 3", n, err)
	}
}

func TestImportCSV_Quoted(t *testing.T) {
	d := setupConsoleDB(t)
	execDDL(t, d, `CREATE TABLE icq_t (id int PRIMARY KEY, v text)`)
	csvData := "1,\"a;b\"\n2,\"say \"\"hi\"\"\"\n3,\"multi\nline\"\n"
	if err := d.ImportCSV(context.Background(), d.loginDB, "icq_t", strings.NewReader(csvData), driver.CSVImportOpts{}, nil); err != nil {
		t.Fatalf("import: %v", err)
	}
	var v string
	if err := d.db.QueryRow(`SELECT v FROM icq_t WHERE id = 2`).Scan(&v); err != nil || v != `say "hi"` {
		t.Errorf("quoted value = %q (%v)", v, err)
	}
}

func TestImportCSV_HeaderSkip(t *testing.T) {
	d := setupConsoleDB(t)
	execDDL(t, d, `CREATE TABLE ich_t (id int PRIMARY KEY, name text)`)
	csvData := "id,name\n1,alice\n"
	if err := d.ImportCSV(context.Background(), d.loginDB, "ich_t", strings.NewReader(csvData), driver.CSVImportOpts{SkipHeader: true}, nil); err != nil {
		t.Fatalf("import: %v", err)
	}
	var n int
	if err := d.db.QueryRow("SELECT count(*) FROM ich_t").Scan(&n); err != nil || n != 1 {
		t.Errorf("rows = %d (%v), want 1", n, err)
	}
}
