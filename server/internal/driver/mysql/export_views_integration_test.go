//go:build integration

package mysql

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

func TestExportViewsGeneratedAndSelection(t *testing.T) {
	for _, image := range testImages {
		t.Run(image.image, func(t *testing.T) {
			ctx := context.Background()
			cfg := startDB(t, image.image)
			d := connectOK(t, cfg).(*MysqlDriver)
			source := cfg.Database
			exec := func(q string) {
				t.Helper()
				if _, err := d.db.ExecContext(ctx, q); err != nil {
					t.Fatal(err)
				}
			}
			exec("CREATE TABLE " + qualifiedIdent(source, "zero_ids") + "(id INT PRIMARY KEY AUTO_INCREMENT,n INT)")
			exec("SET SESSION sql_mode='NO_AUTO_VALUE_ON_ZERO'")
			exec("INSERT INTO " + qualifiedIdent(source, "zero_ids") + "(id,n) VALUES (0,5)")
			exec("SET SESSION sql_mode=DEFAULT")

			// Reverse alphabetic view order catches dumps that create users before dependencies.
			exec("CREATE TABLE " + qualifiedIdent(source, "Base table") + " (id INT PRIMARY KEY, n INT, generated_n INT GENERATED ALWAYS AS(n*2) STORED, note VARCHAR(100), ts DATETIME(6) DEFAULT '2026-10-28 10:11:12.123456')")
			exec("INSERT INTO " + qualifiedIdent(source, "Base table") + "(id,n,note) VALUES (1,10,'a'),(2,20,'b'),(3,30,'c'),(4,40,'d')")
			exec("CREATE SQL SECURITY INVOKER VIEW " + qualifiedIdent(source, "z_view") + " AS SELECT id,generated_n,CONCAT(note, '" + source + ".literal') AS note FROM " + qualifiedIdent(source, "Base table"))
			exec("CREATE VIEW " + qualifiedIdent(source, "a_view") + " AS SELECT * FROM " + qualifiedIdent(source, "z_view") + " WHERE id > 1")
			var output bytes.Buffer
			req := driver.ExportRequest{Format: "sql", IncludeViews: true, DropIfExists: true}
			if err := d.Export(ctx, source, req, &output); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "CREATE VIEW") {
				t.Fatal("views omitted from export")
			}
			exec("CREATE DATABASE export_restore")
			if err := d.ImportSQL(ctx, "export_restore", strings.NewReader(output.String()), nil); err != nil {
				t.Fatalf("restore: %v\n%s", err, output.String())
			}
			var n int
			var note string
			if err := d.db.QueryRowContext(ctx, "SELECT generated_n,note FROM export_restore.a_view WHERE id=2").Scan(&n, &note); err != nil || n != 40 || note != "b"+source+".literal" {
				t.Fatalf("view restore: %d %q %v", n, note, err)
			}
			// INVOKER views must continue enforcing the querying user's table grants.
			exec("CREATE USER 'view_reader'@'%' IDENTIFIED BY 'reader'")
			exec("GRANT SELECT ON " + qualifiedIdent(source, "z_view") + " TO 'view_reader'@'%'")
			exec("GRANT SELECT ON export_restore.z_view TO 'view_reader'@'%'")
			readerCfg := cfg
			readerCfg.User = "view_reader"
			readerCfg.Password = "reader"
			readerCfg.Database = "export_restore"
			reader := connectOK(t, readerCfg).(*MysqlDriver)
			for _, db := range []string{source, "export_restore"} {
				err := reader.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+qualifiedIdent(db, "z_view")).Scan(&n)
				if err == nil || (!strings.Contains(strings.ToLower(err.Error()), "denied") && !strings.Contains(strings.ToLower(err.Error()), "lack rights")) {
					t.Fatalf("INVOKER access semantics changed for %s: %v", db, err)
				}
			}

			// Restoring in place must remove dependent views before dropping tables.
			if err := d.ImportSQL(ctx, "export_restore", strings.NewReader(output.String()), nil); err != nil {
				t.Fatalf("in-place restore: %v", err)
			}
			var timestamp string
			if err := d.db.QueryRowContext(ctx, "SELECT DATE_FORMAT(ts,'%Y-%m-%d %H:%i:%s.%f') FROM export_restore.`Base table` WHERE id=1").Scan(&timestamp); err != nil || timestamp != "2026-10-28 10:11:12.123456" {
				t.Fatalf("fractional timestamp lost: %q %v", timestamp, err)
			}

			// A selected view includes the dependency closure necessary for fresh restore.
			output.Reset()
			req.Tables = []string{"a_view"}
			if err := d.Export(ctx, source, req, &output); err != nil {
				t.Fatal(err)
			}
			exec("CREATE DATABASE selected_restore")
			if err := d.ImportSQL(ctx, "selected_restore", strings.NewReader(output.String()), nil); err != nil {
				t.Fatal(err)
			}
			// SQL selection emits writable values only, retaining exact IDs and range.
			output.Reset()
			query := &sqlgen.SelectQuery{Filters: []sqlgen.Filter{{Column: "n", Operator: ">", Value: "10"}}, Sorts: []sqlgen.Sort{{Column: "id", Desc: true}}, Limit: 2, Offset: 1}
			if err := d.Export(ctx, source, driver.ExportRequest{Format: "sql", Tables: []string{"Base table"}, Query: query}, &output); err != nil {
				t.Fatal(err)
			}
			exec("CREATE DATABASE selection_rows")
			exec("CREATE TABLE selection_rows.`Base table` LIKE " + qualifiedIdent(source, "Base table"))
			if err := d.ImportSQL(ctx, "selection_rows", strings.NewReader(output.String()), nil); err != nil {
				t.Fatalf("selection restore %v\n%s", err, output.String())
			}
			if err := d.db.QueryRowContext(ctx, "SELECT SUM(id),SUM(generated_n) FROM selection_rows.`Base table`").Scan(&n, &note); err != nil || n != 5 || note != "100" {
				t.Fatalf("selection range: %d %q %v", n, note, err)
			}
			output.Reset()
			if err := d.Export(ctx, source, driver.ExportRequest{Format: "csv", Tables: []string{"Base table"}, Query: query}, &output); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "3,30,60,c") || !strings.Contains(output.String(), "2,20,40,b") || strings.Contains(output.String(), "4,40") {
				t.Fatalf("CSV selection range: %s", output.String())
			}
			output.Reset()
			zeroQuery := &sqlgen.SelectQuery{Limit: 1}
			if err := d.Export(ctx, source, driver.ExportRequest{Format: "sql", Tables: []string{"zero_ids"}, Query: zeroQuery}, &output); err != nil {
				t.Fatal(err)
			}
			exec("CREATE DATABASE zero_restore")
			exec("CREATE TABLE zero_restore.zero_ids LIKE " + qualifiedIdent(source, "zero_ids"))
			if err := d.ImportSQL(ctx, "zero_restore", strings.NewReader(output.String()), nil); err != nil {
				t.Fatal(err)
			}
			if err := d.db.QueryRowContext(ctx, "SELECT id FROM zero_restore.zero_ids WHERE n=5").Scan(&n); err != nil || n != 0 {
				t.Fatalf("SQL selection altered zero ID: %d %v", n, err)
			}

			exec("CREATE DATABASE ifexists_restore")
			exec("CREATE VIEW ifexists_restore.z_view AS SELECT 99 AS id,198 AS generated_n,'kept' AS note")
			output.Reset()
			safeReq := req
			safeReq.IfNotExists = true
			safeReq.DropIfExists = false
			if err := d.Export(ctx, source, safeReq, &output); err != nil {
				t.Fatal(err)
			}
			if err := d.ImportSQL(ctx, "ifexists_restore", strings.NewReader(output.String()), nil); err != nil {
				t.Fatalf("IF NOT EXISTS restore %v\n%s", err, output.String())
			}
			if err := d.db.QueryRowContext(ctx, "SELECT note FROM ifexists_restore.z_view WHERE id=99").Scan(&note); err != nil || note != "kept" {
				t.Fatalf("IF NOT EXISTS replaced view: %q %v", note, err)
			}

			exec("CREATE DATABASE external_dep")
			exec("CREATE TABLE external_dep.outside(id INT)")
			exec("CREATE VIEW " + qualifiedIdent(source, "external_view") + " AS SELECT * FROM external_dep.outside")
			output.Reset()
			if err := d.Export(ctx, source, req, &output); err != nil {
				t.Fatal(err)
			} // selected closure excludes unrelated external view
			req.Tables = nil
			output.Reset()
			if err := d.Export(ctx, source, req, &output); err == nil {
				t.Fatal("external dependency silently retained")
			}
			var count int
			if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+qualifiedIdent(source, "Base table")).Scan(&count); err != nil || count != 4 {
				t.Fatalf("source damaged: %d %v", count, err)
			}
		})
	}
}
