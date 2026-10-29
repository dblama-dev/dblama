//go:build integration

package mysql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

func TestTransferModesAndMapping(t *testing.T) {
	for _, image := range testImages {
		t.Run(image.image, func(t *testing.T) {
			cfg := startDB(t, image.image)
			d := connectOK(t, cfg).(*MysqlDriver)
			ctx := context.Background()
			execDDL(t, cfg, "CREATE DATABASE transfer_selected")
			execDDL(t, cfg, "CREATE TABLE transfer_selected.transfer_modes(id BIGINT PRIMARY KEY, name TEXT)")
			execDDL(t, cfg, "INSERT INTO transfer_selected.transfer_modes VALUES (1,'selected')")
			execDDL(t, cfg, "CREATE TABLE transfer_selected.transfer_mapped(id BIGINT PRIMARY KEY, name TEXT, note VARCHAR(20) DEFAULT 'default', bin VARBINARY(16))")
			execDDL(t, cfg, "CREATE VIEW transfer_selected.transfer_view AS SELECT id,name FROM transfer_selected.transfer_modes")
			for _, mode := range []string{"", "all", "structure", "data", "bad"} {
				var req driver.ExportRequest
				json.Unmarshal([]byte(fmt.Sprintf(`{"format":"sql","mode":%q,"tables":["transfer_modes","transfer_view"],"dropIfExists":true,"includeViews":true}`, mode)), &req)
				var out bytes.Buffer
				err := d.Export(ctx, "transfer_selected", req, &out)
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

			var current string
			if err := d.db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&current); err != nil || current != cfg.Database {
				t.Fatalf("database scope changed to %q: %v", current, err)
			}
			noLoginCfg := cfg
			noLoginCfg.Database = ""
			noLogin := connectOK(t, noLoginCfg)
			var noLoginOut bytes.Buffer
			if err := noLogin.Export(ctx, "transfer_selected", driver.ExportRequest{Format: "csv", Tables: []string{"transfer_modes"}}, &noLoginOut); err != nil || !strings.Contains(noLoginOut.String(), "selected") {
				t.Fatalf("export without login db: %s %v", noLoginOut.String(), err)
			}
			var opts driver.CSVImportOpts
			json.Unmarshal([]byte(`{"columns":["name","","id"],"skipHeader":true}`), &opts)
			var done int64
			err := d.ImportCSV(ctx, "transfer_selected", "transfer_mapped", strings.NewReader("name,ignored,id\n\"Привет,\nмир\",ignored,9007199254740993\n"), opts, func(n, total int64) { done = n })
			if err != nil {
				t.Fatal(err)
			}
			page, err := d.SelectRows(ctx, "transfer_selected", "transfer_mapped", driver.SelectQuery{Limit: 10})
			if err != nil || len(page.Rows) != 1 || fmt.Sprint(page.Rows[0][0]) != "9007199254740993" || page.Rows[0][1] != "Привет,\nмир" || page.Rows[0][2] != "default" || done != 1 {
				t.Fatalf("page %+v progress %d error %v", page, done, err)
			}
			for _, mapping := range []string{`["id","id"]`, `["missing"]`, `["",""]`} {
				json.Unmarshal([]byte(`{"columns":`+mapping+`}`), &opts)
				if err := d.ImportCSV(ctx, "transfer_selected", "transfer_mapped", strings.NewReader("1,2\n"), opts, nil); err == nil {
					t.Fatalf("accepted mapping %s", mapping)
				}
			}

			// Explicit mapping preserves empty strings, NULL, omitted DEFAULT and exact large integers.
			opts = driver.CSVImportOpts{Columns: []string{"name", "id", "bin"}}
			if err := d.ImportCSV(ctx, "transfer_selected", "transfer_mapped", strings.NewReader(",9007199254740994,0x00FF\n\\N,9007199254740995,0x\n"), opts, nil); err != nil {
				t.Fatal(err)
			}
			values, err := d.SelectRows(ctx, "transfer_selected", "transfer_mapped", driver.SelectQuery{Limit: 10, Sorts: []sqlgen.Sort{{Column: "id"}}})
			if err != nil || len(values.Rows) != 3 || values.Rows[1][1] != "" || values.Rows[2][1] != nil || values.Rows[1][3] != "0x00ff" || values.Rows[2][3] != "0x" {
				t.Fatalf("mapped empty/null: %+v %v", values, err)
			}
			execDDL(t, cfg, "CREATE TABLE transfer_selected.transfer_partial(id INT PRIMARY KEY, name TEXT)")
			execDDL(t, cfg, "CREATE TABLE transfer_selected.transfer_cancel(id INT PRIMARY KEY, name TEXT)")
			var source strings.Builder
			for i := 1; i <= 100; i++ {
				fmt.Fprintf(&source, "%d,value\n", i)
			}
			source.WriteString("malformed\n")
			done = 0
			var total int64
			err = d.ImportCSV(ctx, "transfer_selected", "transfer_partial", strings.NewReader(source.String()), driver.CSVImportOpts{}, func(n, m int64) { done = n; total = m })
			if err == nil || done != 100 || total != 0 {
				t.Fatalf("partial error=%v progress=%d/%d", err, done, total)
			}
			partial, err := d.SelectRows(ctx, "transfer_selected", "transfer_partial", driver.SelectQuery{Limit: 200})
			if err != nil || len(partial.Rows) != 100 {
				t.Fatalf("partial rows=%d err=%v", len(partial.Rows), err)
			}
			source.Reset()
			for i := 1; i <= 200; i++ {
				fmt.Fprintf(&source, "%d,value\n", i)
			}
			cancelCtx, cancel := context.WithCancel(ctx)
			err = d.ImportCSV(cancelCtx, "transfer_selected", "transfer_cancel", strings.NewReader(source.String()), driver.CSVImportOpts{}, func(n, m int64) {
				if n == 100 {
					cancel()
				}
			})
			cancel()
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation: %v", err)
			}
			cancelled, err := d.SelectRows(ctx, "transfer_selected", "transfer_cancel", driver.SelectQuery{Limit: 300})
			if err != nil || len(cancelled.Rows) != 100 {
				t.Fatalf("cancelled rows=%d err=%v", len(cancelled.Rows), err)
			}
			infoJSON, _ := json.Marshal(d.Info())
			if !strings.Contains(string(infoJSON), `"port":"`+cfg.Port+`"`) {
				t.Fatalf("missing connection port: %s", infoJSON)
			}

		})
	}
}

func TestTransferEscapedCSVRoundTrip(t *testing.T) {
	for _, image := range testImages {
		t.Run(image.image, func(t *testing.T) {
			cfg := startDB(t, image.image)
			d := connectOK(t, cfg).(*MysqlDriver)
			ctx := context.Background()
			execDDL(t, cfg, "CREATE TABLE csv_source(id INT PRIMARY KEY, value TEXT NULL, bin VARBINARY(16))")
			execDDL(t, cfg, "CREATE TABLE csv_target LIKE csv_source")
			values := []struct {
				text any
				bin  any
			}{{nil, nil}, {"", []byte{}}, {`\N`, []byte{0, 255}}, {`\prefix`, nil}, {`\\prefix`, nil}, {"Привет,\nмир", nil}}
			for i, value := range values {
				if _, err := d.db.ExecContext(ctx, "INSERT INTO csv_source VALUES (?,?,?)", i+1, value.text, value.bin); err != nil {
					t.Fatal(err)
				}
			}
			source, err := d.SelectRows(ctx, cfg.Database, "csv_source", driver.SelectQuery{Limit: 20, Sorts: []sqlgen.Sort{{Column: "id"}}})
			if err != nil {
				t.Fatal(err)
			}
			for _, selection := range []bool{false, true} {
				t.Run(fmt.Sprint(selection), func(t *testing.T) {
					execDDL(t, cfg, "TRUNCATE TABLE csv_target")
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
				execDDL(t, cfg, "TRUNCATE TABLE csv_target")
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
		})
	}
}
