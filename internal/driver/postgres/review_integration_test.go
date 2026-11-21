//go:build integration

package postgres

import (
	"bytes"
	"context"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/sqlgen"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReview_DumpAndPool(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	pg := drv.(*PostgresDriver) //nolint:forcetypeassert // test helper
	pg.db.SetMaxOpenConns(1)
	execDataDDL(t, drv, `CREATE SCHEMA source; CREATE SCHEMA target; CREATE TABLE source.items (id integer PRIMARY KEY, note text); CREATE INDEX note_idx ON source.items(note); COMMENT ON TABLE source.items IS 'example'; INSERT INTO source.items VALUES (1,'keep');`)
	src := driver.WithSchema(context.Background(), "source")
	dst := driver.WithSchema(context.Background(), "target")
	t.Run("dump", func(t *testing.T) {
		var out bytes.Buffer
		if err := pg.Export(src, cfg.Database, driver.ExportRequest{Format: "sql", Tables: []string{"items"}, DropIfExists: true}, &out); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), `DROP TABLE IF EXISTS "source".`) {
			t.Error("dump drops source table")
		}
		// Remove the dangerous drop only to independently exercise multi-DDL syntax.
		script := strings.ReplaceAll(out.String(), `DROP TABLE IF EXISTS "source"."items";`, "")
		if err := pg.ImportSQL(dst, cfg.Database, strings.NewReader(script), nil); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("console pool", func(t *testing.T) {
		if _, err := pg.ExecSQL(src, cfg.Database, "SET application_name='leaked'; SELECT 1", "review", driver.ExecOpts{}); err != nil {
			t.Fatal(err)
		}
		var name string
		if err := pg.db.QueryRow("SHOW application_name").Scan(&name); err != nil {
			t.Fatal(err)
		}
		if name == "leaked" {
			t.Fatal("console leaked pooled state")
		}
	})
	t.Run("import pool", func(t *testing.T) {
		if err := pg.ImportSQL(src, cfg.Database, strings.NewReader("SET application_name='import-leak';"), nil); err != nil {
			t.Fatal(err)
		}
		var name string
		if err := pg.db.QueryRow("SHOW application_name").Scan(&name); err != nil {
			t.Fatal(err)
		}
		if name == "import-leak" {
			t.Fatal("import leaked pooled state")
		}
	})
}
func TestReview_TypedWrites(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	pg := drv.(*PostgresDriver) //nolint:forcetypeassert // test helper
	ctx := context.Background()
	execDataDDL(t, drv, `CREATE TABLE typed(id serial PRIMARY KEY, ts timestamptz, tags text[], bin bytea); INSERT INTO typed(ts) VALUES ('2026-01-01T00:00:00.123456Z');`)
	t.Run("precision", func(t *testing.T) {
		page, err := pg.SelectRows(ctx, cfg.Database, "typed", sqlgen.SelectQuery{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(page.Rows[0][1].(string), ".123456") {
			t.Fatalf("precision lost: %v", page.Rows[0][1])
		}
	})
	t.Run("null serial", func(t *testing.T) {
		if err := pg.InsertRow(ctx, cfg.Database, "typed", driver.RowValues{"id": nil, "ts": nil}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("array", func(t *testing.T) {
		if err := pg.InsertRow(ctx, cfg.Database, "typed", driver.RowValues{"tags": `["a",null,"NULL"]`, "bin": "0x00ff"}); err != nil {
			t.Fatal(err)
		}
		if err := pg.UpdateRow(ctx, cfg.Database, "typed", driver.PK{"id": int64(1)}, driver.RowValues{"tags": `["b"]`}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("keyset", func(t *testing.T) {
		execDataDDL(t, drv, `CREATE TABLE timed(id timestamptz PRIMARY KEY); INSERT INTO timed SELECT '2026-01-01'::timestamptz+n*interval '1 microsecond' FROM generate_series(1,101) n`)
		limited, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		var out bytes.Buffer
		if err := pg.Export(limited, cfg.Database, driver.ExportRequest{Format: "sql", Tables: []string{"timed"}}, &out); err != nil {
			t.Fatal(err)
		}
	})
}

func TestReview_RestoreAndStructure(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	pg := drv.(*PostgresDriver) //nolint:forcetypeassert // test helper
	ctx := context.Background()
	execDataDDL(t, drv, `CREATE SCHEMA copied; CREATE TABLE serials(id serial PRIMARY KEY, val text); INSERT INTO serials(val) VALUES ('one'),('two'); CREATE TABLE csv_src(id integer, val text, payload jsonb, tags text[]); CREATE TABLE csv_dst(LIKE csv_src); INSERT INTO csv_src VALUES (1,NULL,'{"ok":true}',ARRAY['NULL',NULL]),(2,'','{}',ARRAY['a']),(3,'\N','{}',ARRAY[]::text[]); CREATE SCHEMA "FK Schema"; CREATE TABLE "FK Schema"."parent.name"(id integer PRIMARY KEY); CREATE TABLE public."parent.name"(id integer PRIMARY KEY);`)
	t.Run("wrong database", func(t *testing.T) {
		if _, err := pg.ListTables(ctx, "another_database"); err == nil {
			t.Fatal("wrong database silently accepted")
		}
	})
	t.Run("serial restore", func(t *testing.T) {
		var out bytes.Buffer
		if err := pg.Export(ctx, cfg.Database, driver.ExportRequest{Format: "sql", Tables: []string{"serials"}}, &out); err != nil {
			t.Fatal(err)
		}
		dst := driver.WithSchema(ctx, "copied")
		if err := pg.ImportSQL(dst, cfg.Database, &out, nil); err != nil {
			t.Fatal(err)
		}
		if err := pg.InsertRow(dst, cfg.Database, "serials", driver.RowValues{"val": "three"}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("csv distinct null and empty and arrays", func(t *testing.T) {
		var out bytes.Buffer
		if err := pg.Export(ctx, cfg.Database, driver.ExportRequest{Format: "csv", Tables: []string{"csv_src"}}, &out); err != nil {
			t.Fatal(err)
		}
		if err := pg.ImportCSV(ctx, cfg.Database, "csv_dst", &out, driver.CSVImportOpts{SkipHeader: true}, nil); err != nil {
			t.Fatal(err)
		}
		a, err := pg.SelectRows(ctx, cfg.Database, "csv_src", sqlgen.SelectQuery{Sorts: []sqlgen.Sort{{Column: "id"}}})
		if err != nil {
			t.Fatal(err)
		}
		b, err := pg.SelectRows(ctx, cfg.Database, "csv_dst", sqlgen.SelectQuery{Sorts: []sqlgen.Sort{{Column: "id"}}})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(a.Rows, b.Rows) {
			t.Fatalf("restored %v want %v", b.Rows, a.Rows)
		}
	})
	t.Run("foreign key in selected schema with dotted name", func(t *testing.T) {
		scoped := driver.WithSchema(ctx, "FK Schema")
		if err := pg.SaveTable(scoped, cfg.Database, "", driver.TableDef{NewName: "child", Columns: []driver.ColumnDef{{Name: "parent", Type: "integer"}}, ForeignKeys: []driver.ForeignKey{{Name: "fk", Columns: []string{"parent"}, RefTable: "parent.name", RefColumns: []string{"id"}}}}); err != nil {
			t.Fatal(err)
		}
		var schema string
		if err := pg.db.QueryRow(`SELECT n.nspname FROM pg_constraint c JOIN pg_class p ON p.oid=c.confrelid JOIN pg_namespace n ON n.oid=p.relnamespace WHERE c.conname='fk'`).Scan(&schema); err != nil {
			t.Fatal(err)
		}
		if schema != "FK Schema" {
			t.Fatalf("referenced wrong schema %s", schema)
		}
	})
	t.Run("unique constraint drop and case sensitive indexes", func(t *testing.T) {
		execDataDDL(t, drv, `CREATE TABLE indexes(id integer CONSTRAINT uniq UNIQUE)`)
		if err := pg.SaveIndexes(ctx, cfg.Database, "indexes", nil); err != nil {
			t.Fatal(err)
		}
		indexes := []driver.Index{{Name: "Foo", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "id"}}}, {Name: "foo", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "id"}}}}
		if err := pg.SaveIndexes(ctx, cfg.Database, "indexes", indexes); err != nil {
			t.Fatal(err)
		}
	})
}
