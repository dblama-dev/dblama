//go:build integration

package postgres

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/sqlgen"
)

func TestRequestSchemas_ParallelAndSequences(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	pg := drv.(*PostgresDriver) //nolint:forcetypeassert // helper connects PostgreSQL
	for _, name := range []string{"Odd Schema", "second"} {
		execDataDDL(t, drv, "CREATE SCHEMA "+sqlgen.QuoteIdentPg(name))
		ctx := driver.WithSchema(context.Background(), name)
		err := pg.SaveTable(ctx, cfg.Database, "", driver.TableDef{NewName: "MixedCase", Columns: []driver.ColumnDef{{Name: "value", Type: "text", Nullable: true}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := pg.InsertRow(ctx, cfg.Database, "MixedCase", driver.RowValues{"value": name}); err != nil {
			t.Fatal(err)
		}
		if err := pg.CreateSequence(ctx, "Odd Sequence", 3); err != nil {
			t.Fatal(err)
		}
		restart, increment := int64(12), int64(2)
		if err := pg.AlterSequence(ctx, "Odd Sequence", &restart, &increment); err != nil {
			t.Fatal(err)
		}
		seqs, err := pg.ListSequences(ctx, name)
		if err != nil || len(seqs) != 1 || seqs[0].Increment != 2 {
			t.Fatalf("sequences %v: %v", seqs, err)
		}
	}
	var wg sync.WaitGroup
	for _, name := range []string{"Odd Schema", "second"} {
		wg.Go(func() {
			ctx := driver.WithSchema(context.Background(), name)
			for i := 0; i < 10; i++ {
				page, err := pg.SelectRows(ctx, cfg.Database, "MixedCase", sqlgen.SelectQuery{Limit: 10})
				if err != nil || len(page.Rows) != 1 || fmt.Sprint(page.Rows[0][0]) != name {
					t.Errorf("schema %s: page=%v err=%v", name, page, err)
					return
				}
				result, err := pg.ExecSQL(ctx, cfg.Database, `SELECT value FROM "MixedCase"`, name+fmt.Sprint(i), driver.ExecOpts{})
				if err != nil || len(result) != 1 || len(result[0].Rows) != 1 || fmt.Sprint(result[0].Rows[0][0]) != name {
					t.Errorf("console schema %s: %v %v", name, result, err)
					return
				}
			}
		})
	}
	wg.Wait()
	if pg.CurrentSchema() != "public" {
		t.Fatal("request changed default schema")
	}
	for _, name := range []string{"Odd Schema", "second"} {
		if err := pg.DropSequence(driver.WithSchema(context.Background(), name), "Odd Sequence"); err != nil {
			t.Fatal(err)
		}
	}
}
