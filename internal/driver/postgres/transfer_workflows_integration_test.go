//go:build integration

package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/sqlgen"
)

func TestTransferModesAndMapping(t *testing.T) {
	cfg := startDB(t)
	d := connectOK(t, cfg).(*PostgresDriver)
	ctx := driver.WithSchema(context.Background(), "Transfer schema")
	execDDL(t, d, `CREATE SCHEMA "Transfer schema"`)
	execDDL(t, d, "CREATE TABLE \"Transfer schema\".transfer_modes(id BIGINT PRIMARY KEY, name TEXT)")
	execDDL(t, d, "INSERT INTO \"Transfer schema\".transfer_modes VALUES (1,'selected')")
	execDDL(t, d, "CREATE TABLE \"Transfer schema\".transfer_mapped(id BIGINT PRIMARY KEY, name TEXT, note VARCHAR(20) DEFAULT 'default', bin bytea)")
	execDDL(t, d, `CREATE VIEW "Transfer schema".transfer_view AS SELECT id,name FROM "Transfer schema".transfer_modes`)
	for _, mode := range []string{"", "all", "structure", "data", "bad"} {
		var req driver.ExportRequest
		json.Unmarshal([]byte(fmt.Sprintf(`{"format":"sql","mode":%q,"tables":["transfer_modes","transfer_view"],"dropIfExists":true,"includeViews":true}`, mode)), &req)
		var out bytes.Buffer
		err := d.Export(ctx, cfg.Database, req, &out)
		if mode == "bad" {
			if err == nil || out.Len() != 0 {
				t.Fatalf("invalid mode: %v %s", err, out.String())
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for token, want := range map[string]bool{"CREATE TABLE": mode != "data", "CREATE VIEW": mode != "data", "DROP VIEW": mode != "data", "DROP TABLE": mode != "data", "INSERT INTO": mode != "structure"} {
			if strings.Contains(out.String(), token) != want {
				t.Fatalf("mode %s token %s: %s", mode, token, out.String())
			}
		}
	}

	var mismatch bytes.Buffer
	if err := d.Export(ctx, "other_database", driver.ExportRequest{Format: "sql"}, &mismatch); err == nil || mismatch.Len() != 0 {
		t.Fatalf("Postgres mismatch: %v %s", err, mismatch.String())
	}
	var opts driver.CSVImportOpts
	json.Unmarshal([]byte(`{"columns":["name","","id"],"skipHeader":true}`), &opts)
	var done int64
	err := d.ImportCSV(ctx, cfg.Database, "transfer_mapped", strings.NewReader("name,ignored,id\n\"Привет,\nмир\",ignored,9007199254740993\n"), opts, func(n, total int64) { done = n })
	if err != nil {
		t.Fatal(err)
	}
	page, err := d.SelectRows(ctx, cfg.Database, "transfer_mapped", driver.SelectQuery{Limit: 10})
	if err != nil || len(page.Rows) != 1 || fmt.Sprint(page.Rows[0][0]) != "9007199254740993" || page.Rows[0][1] != "Привет,\nмир" || page.Rows[0][2] != "default" || done != 1 {
		t.Fatalf("page %+v progress %d error %v", page, done, err)
	}
	for _, mapping := range []string{`["id","id"]`, `["missing"]`, `["",""]`} {
		json.Unmarshal([]byte(`{"columns":`+mapping+`}`), &opts)
		if err := d.ImportCSV(ctx, cfg.Database, "transfer_mapped", strings.NewReader("1,2\n"), opts, nil); err == nil {
			t.Fatalf("accepted mapping %s", mapping)
		}
	}

	// Explicit mapping preserves empty strings, NULL, omitted DEFAULT and exact large integers.
	opts = driver.CSVImportOpts{Columns: []string{"name", "id", "bin"}}
	if err := d.ImportCSV(ctx, cfg.Database, "transfer_mapped", strings.NewReader(",9007199254740994,0x00FF\n\\N,9007199254740995,0x\n"), opts, nil); err != nil {
		t.Fatal(err)
	}
	values, err := d.SelectRows(ctx, cfg.Database, "transfer_mapped", driver.SelectQuery{Limit: 10, Sorts: []sqlgen.Sort{{Column: "id"}}})
	if err != nil || len(values.Rows) != 3 || values.Rows[1][1] != "" || values.Rows[2][1] != nil || values.Rows[1][3] != "0x00ff" || values.Rows[2][3] != "0x" {
		t.Fatalf("mapped empty/null: %+v %v", values, err)
	}
	execDDL(t, d, `CREATE TABLE "Transfer schema".transfer_partial(id INT PRIMARY KEY, name TEXT)`)
	execDDL(t, d, `CREATE TABLE "Transfer schema".transfer_cancel(id INT PRIMARY KEY, name TEXT)`)
	var source strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&source, "%d,value\n", i)
	}
	source.WriteString("malformed\n")
	done = 0
	var total int64
	err = d.ImportCSV(ctx, cfg.Database, "transfer_partial", strings.NewReader(source.String()), driver.CSVImportOpts{}, func(n, m int64) { done = n; total = m })
	if err == nil || done != 100 || total != 0 {
		t.Fatalf("partial error=%v progress=%d/%d", err, done, total)
	}
	partial, err := d.SelectRows(ctx, cfg.Database, "transfer_partial", driver.SelectQuery{Limit: 200})
	if err != nil || len(partial.Rows) != 100 {
		t.Fatalf("partial rows=%d err=%v", len(partial.Rows), err)
	}
	source.Reset()
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&source, "%d,value\n", i)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	err = d.ImportCSV(cancelCtx, cfg.Database, "transfer_cancel", strings.NewReader(source.String()), driver.CSVImportOpts{}, func(n, m int64) {
		if n == 100 {
			cancel()
		}
	})
	cancel()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	cancelled, err := d.SelectRows(ctx, cfg.Database, "transfer_cancel", driver.SelectQuery{Limit: 300})
	if err != nil || len(cancelled.Rows) != 100 {
		t.Fatalf("cancelled rows=%d err=%v", len(cancelled.Rows), err)
	}
	infoJSON, _ := json.Marshal(d.Info())
	if !strings.Contains(string(infoJSON), `"port":"`+cfg.Port+`"`) {
		t.Fatalf("missing connection port: %s", infoJSON)
	}
}

func TestTransferStructureThenDataWithForeignKeys(t *testing.T) {
	cfg := startDB(t)
	d := connectOK(t, cfg).(*PostgresDriver)
	ctx := context.Background()
	execDDL(t, d, `CREATE SCHEMA fk_source`)
	execDDL(t, d, `CREATE TABLE fk_source.z_parent(id BIGINT PRIMARY KEY)`)
	execDDL(t, d, `CREATE TABLE fk_source.a_child(id BIGINT PRIMARY KEY, parent_id BIGINT NOT NULL REFERENCES fk_source.z_parent(id))`)
	execDDL(t, d, `INSERT INTO fk_source.z_parent VALUES(9007199254740993)`)
	execDDL(t, d, `INSERT INTO fk_source.a_child VALUES(1,9007199254740993)`)
	sourceCtx := driver.WithSchema(ctx, "fk_source")
	for i, tables := range [][]string{{"a_child", "z_parent"}, {"a_child"}, nil} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			target := fmt.Sprintf("fk_target_%d", i)
			execDDL(t, d, "CREATE SCHEMA "+target)
			var structure, data bytes.Buffer
			if err := d.Export(sourceCtx, cfg.Database, driver.ExportRequest{Format: "sql", Mode: "structure", Tables: tables}, &structure); err != nil {
				t.Fatal(err)
			}
			if err := d.Export(sourceCtx, cfg.Database, driver.ExportRequest{Format: "sql", Mode: "data", Tables: tables}, &data); err != nil {
				t.Fatal(err)
			}
			targetCtx := driver.WithSchema(ctx, target)
			if err := d.ImportSQL(targetCtx, cfg.Database, &structure, nil); err != nil {
				t.Fatal(err)
			}
			if err := d.ImportSQL(targetCtx, cfg.Database, strings.NewReader(data.String()), nil); err != nil {
				t.Fatalf("data SQL %s\nrestore error: %v", data.String(), err)
			}
			var parent string
			if err := d.db.QueryRowContext(ctx, "SELECT parent_id FROM "+target+".a_child").Scan(&parent); err != nil || parent != "9007199254740993" {
				t.Fatalf("parent %q error %v", parent, err)
			}
		})
	}
	t.Run("cycles rejected before output", func(t *testing.T) {
		execDDL(t, d, `CREATE SCHEMA fk_cycle`)
		execDDL(t, d, `CREATE TABLE fk_cycle.a(id INT PRIMARY KEY, b_id INT)`)
		execDDL(t, d, `CREATE TABLE fk_cycle.b(id INT PRIMARY KEY, a_id INT REFERENCES fk_cycle.a(id))`)
		execDDL(t, d, `ALTER TABLE fk_cycle.a ADD FOREIGN KEY(b_id) REFERENCES fk_cycle.b(id)`)
		var data bytes.Buffer
		err := d.Export(driver.WithSchema(ctx, "fk_cycle"), cfg.Database, driver.ExportRequest{Format: "sql", Mode: "data"}, &data)
		if err == nil || data.Len() != 0 || !strings.Contains(err.Error(), "cycle") {
			t.Fatalf("cycle output %q error %v", data.String(), err)
		}
	})
}

func TestTransferEscapedCSVRoundTrip(t *testing.T) {
	cfg := startDB(t)
	d := connectOK(t, cfg).(*PostgresDriver)
	ctx := context.Background()
	execDDL(t, d, "CREATE TABLE csv_source(id INT PRIMARY KEY, value TEXT NULL, bin bytea)")
	execDDL(t, d, "CREATE TABLE csv_target(LIKE csv_source INCLUDING ALL)")
	values := []struct {
		text any
		bin  any
	}{{nil, nil}, {"", []byte{}}, {`\N`, []byte{0, 255}}, {`\prefix`, nil}, {`\\prefix`, nil}, {"Привет,\nмир", nil}}
	for i, value := range values {
		if _, err := d.db.ExecContext(ctx, "INSERT INTO csv_source VALUES ($1,$2,$3)", i+1, value.text, value.bin); err != nil {
			t.Fatal(err)
		}
	}
	source, err := d.SelectRows(ctx, cfg.Database, "csv_source", driver.SelectQuery{Limit: 20, Sorts: []sqlgen.Sort{{Column: "id"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range []bool{false, true} {
		t.Run(fmt.Sprint(selection), func(t *testing.T) {
			execDDL(t, d, "TRUNCATE TABLE csv_target")
			var req driver.ExportRequest
			if err := json.Unmarshal([]byte(`{"format":"csv","tables":["csv_source"],"csvEncoding":"escaped"}`), &req); err != nil {
				t.Fatal(err)
			}
			if selection {
				req.Query = &driver.SelectQuery{Sorts: []sqlgen.Sort{{Column: "id"}}}
			}
			var output bytes.Buffer
			if err := d.Export(ctx, cfg.Database, req, &output); err != nil {
				t.Fatal(err)
			}
			if err := d.ImportCSV(ctx, cfg.Database, "csv_target", strings.NewReader(output.String()), driver.CSVImportOpts{Columns: []string{"id", "value", "bin"}, SkipHeader: true}, nil); err != nil {
				t.Fatal(err)
			}
			restored, err := d.SelectRows(ctx, cfg.Database, "csv_target", driver.SelectQuery{Limit: 20, Sorts: []sqlgen.Sort{{Column: "id"}}})
			if err != nil || !reflect.DeepEqual(restored.Rows, source.Rows) {
				t.Fatalf("CSV %q\nsource=%#v\nrestored=%#v err=%v", output.String(), source.Rows, restored.Rows, err)
			}
		})
	}
	t.Run("literal mapped external CSV", func(t *testing.T) {
		execDDL(t, d, "TRUNCATE TABLE csv_target")
		var opts driver.CSVImportOpts
		json.Unmarshal([]byte(`{"columns":["id","value"],"csvEncoding":"literal"}`), &opts)
		if err := d.ImportCSV(ctx, cfg.Database, "csv_target", strings.NewReader("1,\\N\n2,\\\\prefix\n3,\n"), opts, nil); err != nil {
			t.Fatal(err)
		}
		page, err := d.SelectRows(ctx, cfg.Database, "csv_target", driver.SelectQuery{Limit: 10, Sorts: []sqlgen.Sort{{Column: "id"}}})
		if err != nil || len(page.Rows) != 3 || page.Rows[0][1] != `\N` || page.Rows[1][1] != `\\prefix` || page.Rows[2][1] != "" {
			t.Fatalf("literal rows=%#v error=%v", page.Rows, err)
		}
	})
}
