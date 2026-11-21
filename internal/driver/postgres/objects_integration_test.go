//go:build integration

package postgres

import (
	"context"
	"fmt"
	"github.com/dblama-dev/dblama/internal/driver"
	"strings"
	"testing"
	"time"
)

func TestObjectsLifecycle(t *testing.T) {
	cfg := startDB(t)
	base := connectOK(t, cfg)
	d := base.(*PostgresDriver)
	db := cfg.Database
	execDataDDL(t, d, `CREATE SCHEMA "Odd Schema"`)
	ctx := driver.WithSchema(context.Background(), "Odd Schema")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(d.AlterView(ctx, db, "sample", driver.ObjectDef{Source: `CREATE OR REPLACE VIEW sample AS SELECT 1 AS value`}))
	views, err := d.ListViews(ctx, db)
	must(err)
	if len(views) != 1 || !strings.Contains(views[0].Source, "CREATE") {
		t.Fatalf("views %#v", views)
	}
	other, err := d.ListViews(context.Background(), db)
	must(err)
	if len(other) != 0 {
		t.Fatalf("schema leaked %#v", other)
	}
	must(d.AlterView(ctx, db, "sample", driver.ObjectDef{Source: views[0].Source}))
	if err := d.AlterView(ctx, db, "extra", driver.ObjectDef{Source: `CREATE VIEW extra AS SELECT 1; DROP VIEW sample`}); err == nil {
		t.Fatal("accepted appended SQL")
	}
	must(d.DropView(ctx, db, "sample"))
	_, err = d.ExecSQL(ctx, db, `CREATE TABLE items(id int); CREATE FUNCTION trigger_fn() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RETURN NEW; END$$`, "", driver.ExecOpts{})
	must(err)
	d.db.SetMaxOpenConns(1)
	triggerCtx, cancelTrigger := context.WithTimeout(ctx, 2*time.Second)
	defer cancelTrigger()
	must(d.CreateTrigger(triggerCtx, db, "on_insert", driver.ObjectDef{Source: `CREATE TRIGGER on_insert BEFORE INSERT ON items FOR EACH ROW EXECUTE FUNCTION trigger_fn()`}))
	triggers, err := d.ListTriggers(ctx, db)
	must(err)
	if len(triggers) != 1 || triggers[0].Table != "items" {
		t.Fatalf("triggers %#v", triggers)
	}
	must(d.CreateTrigger(ctx, db, "on_insert", driver.ObjectDef{Table: "items", Source: strings.Replace(triggers[0].Source, "BEFORE INSERT", "BEFORE UPDATE", 1)}))
	triggers, err = d.ListTriggers(ctx, db)
	must(err)
	if len(triggers) != 1 || !strings.Contains(triggers[0].Source, "UPDATE") {
		t.Fatalf("trigger update %#v", triggers)
	}
	must(d.DropTrigger(ctx, db, "on_insert", "items"))
	must(d.CreateRoutine(ctx, db, "echo_value", driver.ObjectDef{Kind: "PROCEDURE", Source: `CREATE PROCEDURE echo_value(IN x text, INOUT y text) LANGUAGE plpgsql AS $$BEGIN y:=x; END$$`}))
	routines, err := d.ListRoutines(ctx, db)
	must(err)
	var proc driver.ObjectInfo
	for _, r := range routines {
		if r.Name == "echo_value" {
			proc = r
		}
	}
	if proc.Kind != "PROCEDURE" || proc.Identity == "" {
		t.Fatalf("routine %#v", proc)
	}
	argument := "quote'; DROP TABLE items; --"
	results, err := d.CallProcedure(ctx, db, "echo_value", proc.Identity, []any{argument, nil})
	must(err)
	if len(results) != 1 || len(results[0].Rows) != 1 || fmt.Sprint(results[0].Rows[0][0]) != argument {
		t.Fatalf("call %#v", results)
	}
	if err := d.DropRoutine(ctx, db, "echo_value", "PROCEDURE", proc.Identity+"; DROP TABLE items"); err == nil {
		t.Fatal("accepted forged signature")
	}
	must(d.DropRoutine(ctx, db, "echo_value", "PROCEDURE", proc.Identity))
}
