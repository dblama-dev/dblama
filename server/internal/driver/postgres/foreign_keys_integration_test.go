//go:build integration

package postgres

import (
	"context"
	"github.com/hitslab/db-manager/server/internal/driver"
	"testing"
)

func TestForeignKeysLifecycle(t *testing.T) {
	cfg := startDB(t)
	d := connectOK(t, cfg).(*PostgresDriver)
	ctx := driver.WithSchema(context.Background(), "Odd Schema")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range []string{`CREATE SCHEMA "Odd Schema"`, `CREATE TABLE public.parent (tenant INT, id INT, PRIMARY KEY(tenant,id))`, `CREATE TABLE "Odd Schema".child (tenant INT, parent INT)`} {
		execDataDDL(t, d, sql)
	}
	fk := driver.ForeignKey{Name: "parent fk", Columns: []string{"tenant", "parent"}, RefSchema: "public", RefTable: "parent", RefColumns: []string{"tenant", "id"}, OnDelete: "CASCADE", OnUpdate: "NO ACTION"}
	must(d.SaveForeignKeys(ctx, cfg.Database, "child", []driver.ForeignKey{fk}))
	def, err := d.TableDef(ctx, cfg.Database, "child")
	must(err)
	if len(def.ForeignKeys) != 1 || len(def.ForeignKeys[0].Columns) != 2 || def.ForeignKeys[0].RefSchema != "public" {
		t.Fatalf("FKs %#v", def.ForeignKeys)
	}
	fk.OnDelete = "SET NULL"
	must(d.SaveForeignKeys(ctx, cfg.Database, "child", []driver.ForeignKey{fk}))
	def, err = d.TableDef(ctx, cfg.Database, "child")
	must(err)
	if def.ForeignKeys[0].OnDelete != "SET NULL" {
		t.Fatalf("FKs %#v", def.ForeignKeys)
	}
	bad := fk
	bad.RefColumns = []string{"missing", "id"}
	if err := d.SaveForeignKeys(ctx, cfg.Database, "child", []driver.ForeignKey{bad}); err == nil {
		t.Fatal("accepted invalid reference")
	}
	def, err = d.TableDef(ctx, cfg.Database, "child")
	must(err)
	if len(def.ForeignKeys) != 1 || def.ForeignKeys[0].OnDelete != "SET NULL" {
		t.Fatalf("failed transaction lost old FK %#v", def.ForeignKeys)
	}
	bad = fk
	bad.OnDelete = "CASCADE; DROP TABLE child"
	if err := d.SaveForeignKeys(ctx, cfg.Database, "child", []driver.ForeignKey{bad}); err == nil {
		t.Fatal("accepted injected action")
	}
	must(d.SaveForeignKeys(ctx, cfg.Database, "child", nil))
	def, err = d.TableDef(ctx, cfg.Database, "child")
	must(err)
	if len(def.ForeignKeys) != 0 {
		t.Fatalf("not dropped %#v", def.ForeignKeys)
	}
}
