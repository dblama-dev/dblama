//go:build integration

package mysql

import (
	"context"
	"fmt"
	"github.com/hitslab/db-manager/server/internal/driver"
	"strings"
	"testing"
)

func TestObjectsLifecycle(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			base := connectOK(t, cfg)
			d := base.(*MysqlDriver)
			ctx := context.Background()
			db := cfg.Database
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			_, err := d.db.ExecContext(ctx, "CREATE TABLE items(id INT PRIMARY KEY, value VARCHAR(100))")
			must(err)
			must(d.AlterView(ctx, db, "sample", driver.ObjectDef{Source: "CREATE OR REPLACE VIEW sample AS SELECT id FROM items"}))
			views, err := d.ListViews(ctx, db)
			must(err)
			if len(views) != 1 || !strings.Contains(views[0].Source, "CREATE") {
				t.Fatalf("views: %#v", views)
			}
			must(d.AlterView(ctx, db, "sample", driver.ObjectDef{Source: views[0].Source}))
			must(d.AlterView(ctx, db, "sample", driver.ObjectDef{Source: "create view sample as select 2 as id, 'OR REPLACE' as note"}))
			var replacedID int
			must(d.db.QueryRowContext(ctx, "SELECT id FROM sample").Scan(&replacedID))
			if replacedID != 2 {
				t.Fatalf("lower-case view update returned id %d", replacedID)
			}
			must(d.DropView(ctx, db, "sample"))
			must(d.CreateTrigger(ctx, db, "on_insert", driver.ObjectDef{Source: "CREATE TRIGGER on_insert BEFORE INSERT ON items FOR EACH ROW SET NEW.value = 'changed'"}))
			triggers, err := d.ListTriggers(ctx, db)
			must(err)
			if len(triggers) != 1 || triggers[0].Table != "items" {
				t.Fatalf("triggers %#v", triggers)
			}
			must(d.DropTrigger(ctx, db, "on_insert", "items"))
			must(d.CreateRoutine(ctx, db, "echo_value", driver.ObjectDef{Kind: "PROCEDURE", Source: "CREATE PROCEDURE echo_value(IN arg TEXT) BEGIN SELECT arg AS value; SELECT 2 AS second; END"}))
			must(d.CreateRoutine(ctx, db, "constant_value", driver.ObjectDef{Kind: "FUNCTION", Source: "CREATE FUNCTION constant_value() RETURNS INT DETERMINISTIC RETURN 7"}))
			routines, err := d.ListRoutines(ctx, db)
			must(err)
			if len(routines) != 2 {
				t.Fatalf("routines %#v", routines)
			}
			argument := "quote'; DROP TABLE items; --"
			results, err := d.CallProcedure(ctx, db, "echo_value", "", []any{argument})
			must(err)
			if len(results) < 2 || len(results[0].Rows) != 1 || fmt.Sprint(results[0].Rows[0][0]) != argument {
				t.Fatalf("call %#v", results)
			}
			must(d.CreateRoutine(ctx, db, "out_values", driver.ObjectDef{Kind: "PROCEDURE", Source: "CREATE PROCEDURE out_values(IN input TEXT, INOUT counter INT, OUT output TEXT) BEGIN SET counter=counter+1; SET output=input; END"}))
			out, err := d.CallProcedure(ctx, db, "out_values", "", []any{"hello", 4, nil})
			must(err)
			last := out[len(out)-1]
			if len(last.Rows) != 1 || fmt.Sprint(last.Rows[0][0]) != "5" || fmt.Sprint(last.Rows[0][1]) != "hello" {
				t.Fatalf("OUT results %#v", out)
			}
			must(d.DropRoutine(ctx, db, "out_values", "PROCEDURE", ""))
			must(d.DropRoutine(ctx, db, "echo_value", "PROCEDURE", ""))
			must(d.DropRoutine(ctx, db, "constant_value", "FUNCTION", ""))
			must(d.CreateEvent(ctx, db, "scheduled", driver.ObjectDef{Source: "CREATE EVENT scheduled ON SCHEDULE EVERY 1 DAY DISABLE DO INSERT INTO items VALUES (9, 'event')"}))
			events, err := d.ListEvents(ctx, db)
			must(err)
			if len(events) != 1 || events[0].Status != "DISABLED" {
				t.Fatalf("events %#v", events)
			}
			must(d.DropEvent(ctx, db, "scheduled"))
			_, err = d.db.ExecContext(ctx, "CREATE TABLE partitioned(id INT) PARTITION BY RANGE(id) (PARTITION p0 VALUES LESS THAN (10))")
			must(err)
			must(d.AddPartition(ctx, db, "partitioned", "PARTITION p1 VALUES LESS THAN (20)"))
			parts, err := d.ListPartitions(ctx, db, "partitioned")
			must(err)
			if len(parts) != 2 {
				t.Fatalf("partitions %#v", parts)
			}
			must(d.TruncatePartition(ctx, db, "partitioned", "p1"))
			must(d.DropPartition(ctx, db, "partitioned", "p1"))
		})
	}
}
