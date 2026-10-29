//go:build integration

package mysql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
