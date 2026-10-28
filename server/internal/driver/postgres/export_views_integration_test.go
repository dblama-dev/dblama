//go:build integration

package postgres

import (
	"bytes"
	"context"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
	"strings"
	"testing"
)

func TestExportViewsGeneratedAndSelection(t *testing.T) {
	ctx := context.Background()
	cfg := startDB(t)
	d := connectOK(t, cfg).(*PostgresDriver)
	exec := func(q string) {
		t.Helper()
		if _, err := d.db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	source := "Source schema"
	exec(`CREATE SCHEMA "Source schema"`)
	exec(`CREATE SCHEMA "Restore schema"`)
	exec(`CREATE SCHEMA "Selected schema"`)
	exec(`CREATE TABLE "Source schema"."Base table" (id INT PRIMARY KEY,n INT,generated_n INT GENERATED ALWAYS AS(n*2) STORED,note TEXT)`)
	exec(`INSERT INTO "Source schema"."Base table"(id,n,note) VALUES (1,10,'a'),(2,20,'b'),(3,30,'c'),(4,40,'d')`)
	exec(`CREATE VIEW "Source schema".z_view WITH (security_invoker=true,security_barrier=true) AS SELECT id,generated_n,note || 'Source schema.literal' AS note FROM "Source schema"."Base table"`)
	exec(`CREATE VIEW "Source schema".a_view AS SELECT * FROM "Source schema".z_view WHERE id>1`)
	sourceCtx := driver.WithSchema(ctx, source)
	var output bytes.Buffer
	req := driver.ExportRequest{Format: "sql", IncludeViews: true, DropIfExists: true}
	if err := d.Export(sourceCtx, cfg.Database, req, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "CREATE VIEW") {
		t.Fatal("views omitted from export")
	}
	if err := d.ImportSQL(driver.WithSchema(ctx, "Restore schema"), cfg.Database, strings.NewReader(output.String()), nil); err != nil {
		t.Fatalf("restore %v\n%s", err, output.String())
	}
	var n int
	var note string
	if err := d.db.QueryRowContext(ctx, `SELECT generated_n,note FROM "Restore schema".a_view WHERE id=2`).Scan(&n, &note); err != nil || n != 40 || note != "bSource schema.literal" {
		t.Fatalf("view restore: %d %q %v", n, note, err)
	}
	// An invoker view cannot inherit the dump owner's underlying-table grants.
	exec(`CREATE ROLE view_reader LOGIN PASSWORD 'reader'`)
	exec(`GRANT USAGE ON SCHEMA "Source schema","Restore schema" TO view_reader`)
	exec(`GRANT SELECT ON "Source schema".z_view,"Restore schema".z_view TO view_reader`)
	readerCfg := cfg
	readerCfg.User = "view_reader"
	readerCfg.Password = "reader"
	reader := connectOK(t, readerCfg).(*PostgresDriver)
	for _, schema := range []string{"Source schema", "Restore schema"} {
		err := reader.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+sqlgen.QuoteIdentPg(schema)+".z_view").Scan(&n)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "permission denied") {
			t.Fatalf("INVOKER access semantics changed for %s: %v", schema, err)
		}
	}
	var options string
	if err := d.db.QueryRowContext(ctx, `SELECT reloptions::text FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='Restore schema' AND c.relname='z_view'`).Scan(&options); err != nil || !strings.Contains(options, "security_barrier=true") || !strings.Contains(options, "security_invoker=true") {
		t.Fatalf("view security flags lost: %q %v", options, err)
	}

	if err := d.ImportSQL(driver.WithSchema(ctx, "Restore schema"), cfg.Database, strings.NewReader(output.String()), nil); err != nil {
		t.Fatalf("in-place restore: %v", err)
	}

	output.Reset()
	req.Tables = []string{"a_view"}
	if err := d.Export(sourceCtx, cfg.Database, req, &output); err != nil {
		t.Fatal(err)
	}
	if err := d.ImportSQL(driver.WithSchema(ctx, "Selected schema"), cfg.Database, strings.NewReader(output.String()), nil); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	query := &sqlgen.SelectQuery{Filters: []sqlgen.Filter{{Column: "n", Operator: ">", Value: "10"}}, Sorts: []sqlgen.Sort{{Column: "id", Desc: true}}, Limit: 2, Offset: 1}
	if err := d.Export(sourceCtx, cfg.Database, driver.ExportRequest{Format: "sql", Tables: []string{"Base table"}, Query: query}, &output); err != nil {
		t.Fatal(err)
	}
	exec(`CREATE SCHEMA selection_rows`)
	exec(`CREATE TABLE selection_rows."Base table" (LIKE "Source schema"."Base table" INCLUDING ALL)`)
	if err := d.ImportSQL(driver.WithSchema(ctx, "selection_rows"), cfg.Database, strings.NewReader(output.String()), nil); err != nil {
		t.Fatalf("selection restore %v\n%s", err, output.String())
	}
	if err := d.db.QueryRowContext(ctx, `SELECT SUM(id),SUM(generated_n) FROM selection_rows."Base table"`).Scan(&n, &note); err != nil || n != 5 || note != "100" {
		t.Fatalf("selection range: %d %q %v", n, note, err)
	}
	output.Reset()
	if err := d.Export(sourceCtx, cfg.Database, driver.ExportRequest{Format: "csv", Tables: []string{"Base table"}, Query: query}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "3,30,60,c") || !strings.Contains(output.String(), "2,20,40,b") || strings.Contains(output.String(), "4,40") {
		t.Fatalf("CSV selection range %s", output.String())
	}
	exec(`CREATE SCHEMA ifexists_restore`)
	exec(`CREATE VIEW ifexists_restore.z_view AS SELECT 99 AS id,198 AS generated_n,'kept'::text AS note`)
	output.Reset()
	safeReq := req
	safeReq.IfNotExists = true
	safeReq.DropIfExists = false
	if err := d.Export(sourceCtx, cfg.Database, safeReq, &output); err != nil {
		t.Fatal(err)
	}
	if err := d.ImportSQL(driver.WithSchema(ctx, "ifexists_restore"), cfg.Database, strings.NewReader(output.String()), nil); err != nil {
		t.Fatalf("IF NOT EXISTS restore %v\n%s", err, output.String())
	}
	if err := d.db.QueryRowContext(ctx, `SELECT note FROM ifexists_restore.z_view WHERE id=99`).Scan(&note); err != nil || note != "kept" {
		t.Fatalf("IF NOT EXISTS replaced view: %q %v", note, err)
	}

	exec(`CREATE VIEW "Source schema".external_view AS SELECT * FROM "Restore schema"."Base table"`)
	output.Reset()
	if err := d.Export(sourceCtx, cfg.Database, req, &output); err != nil {
		t.Fatal(err)
	}
	req.Tables = nil
	output.Reset()
	if err := d.Export(sourceCtx, cfg.Database, req, &output); err == nil {
		t.Fatal("external-schema dependency silently retained")
	}
	var count int
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM "Source schema"."Base table"`).Scan(&count); err != nil || count != 4 {
		t.Fatalf("source damaged: %d %v", count, err)
	}
}
