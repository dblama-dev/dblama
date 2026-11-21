//go:build integration

package mysql

import (
	"context"
	"github.com/dblama-dev/dblama/internal/driver"
	"testing"
)

func TestForeignKeysLifecycle(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			d := connectOK(t, cfg).(*MysqlDriver)
			ctx := context.Background()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, sql := range []string{"CREATE DATABASE fk_other", "CREATE TABLE fk_other.parent (tenant INT, id INT, PRIMARY KEY(tenant,id))", "CREATE TABLE child (tenant INT, parent INT)"} {
				_, err := d.db.ExecContext(ctx, sql)
				must(err)
			}
			fk := driver.ForeignKey{Name: "parent fk", Columns: []string{"tenant", "parent"}, RefSchema: "fk_other", RefTable: "parent", RefColumns: []string{"tenant", "id"}, OnDelete: "CASCADE", OnUpdate: "RESTRICT"}
			must(d.SaveForeignKeys(ctx, cfg.Database, "child", []driver.ForeignKey{fk}))
			def, err := d.TableDef(ctx, cfg.Database, "child")
			must(err)
			if len(def.ForeignKeys) != 1 || len(def.ForeignKeys[0].Columns) != 2 || def.ForeignKeys[0].RefSchema != "fk_other" || def.ForeignKeys[0].RefTable != "parent" {
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
				t.Fatalf("failed ALTER lost old FK %#v", def.ForeignKeys)
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
		})
	}
}
