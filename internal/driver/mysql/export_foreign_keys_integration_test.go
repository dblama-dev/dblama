//go:build integration

package mysql

import (
	"bytes"
	"context"
	"github.com/dblama-dev/dblama/internal/driver"
	"strings"
	"testing"
)

func TestExportPortableForeignKeys(t *testing.T) {
	for _, image := range testImages {
		t.Run(image.image, func(t *testing.T) {
			ctx := context.Background()
			cfg := startDB(t, image.image)
			d := connectOK(t, cfg).(*MysqlDriver)
			exec := func(q string) {
				t.Helper()
				if _, err := d.db.ExecContext(ctx, q); err != nil {
					t.Fatal(err)
				}
			}
			exec("CREATE TABLE z_parent(a INT,b INT,PRIMARY KEY(a,b))")
			exec("CREATE TABLE a_child(id INT PRIMARY KEY,pa INT,pb INT,CONSTRAINT fk_composite FOREIGN KEY(pa,pb) REFERENCES z_parent(a,b))")
			exec("INSERT INTO z_parent VALUES(1,2)")
			exec("INSERT INTO a_child VALUES(9,1,2)")
			exec("CREATE TABLE cycle_left(id INT PRIMARY KEY,other INT)")
			exec("CREATE TABLE cycle_right(id INT PRIMARY KEY,other INT)")
			exec("INSERT INTO cycle_left VALUES(1,2)")
			exec("INSERT INTO cycle_right VALUES(2,1)")
			exec("ALTER TABLE cycle_left ADD CONSTRAINT fk_right FOREIGN KEY(other) REFERENCES cycle_right(id)")
			exec("ALTER TABLE cycle_right ADD CONSTRAINT fk_left FOREIGN KEY(other) REFERENCES cycle_left(id)")
			var output bytes.Buffer
			req := driver.ExportRequest{Format: "sql", DropIfExists: true}
			if err := d.Export(ctx, cfg.Database, req, &output); err != nil {
				t.Fatal(err)
			}
			exec("CREATE DATABASE fk_restore")
			for i := 0; i < 2; i++ {
				if err := d.ImportSQL(ctx, "fk_restore", strings.NewReader(output.String()), nil); err != nil {
					t.Fatalf("restore iteration %d: %v\n%s", i, err, output.String())
				}
			}
			def, err := d.TableDef(ctx, "fk_restore", "a_child")
			if err != nil || len(def.ForeignKeys) != 1 || len(def.ForeignKeys[0].Columns) != 2 {
				t.Fatalf("composite FK lost: %+v %v", def, err)
			}
			// Resolve actual catalog schemas: local FKs may intentionally use empty RefSchema in TableDef.
			var wrong int
			if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA='fk_restore' AND REFERENCED_TABLE_SCHEMA IS NOT NULL AND REFERENCED_TABLE_SCHEMA<>'fk_restore'").Scan(&wrong); err != nil || wrong != 0 {
				t.Fatalf("source references leaked: %d %v", wrong, err)
			}
			req.Tables = []string{"a_child"}
			output.Reset()
			if err := d.Export(ctx, cfg.Database, req, &output); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "cycle_left") {
				t.Fatal("selected export included unrelated tables")
			}
			exec("CREATE DATABASE fk_selected")
			if err := d.ImportSQL(ctx, "fk_selected", strings.NewReader(output.String()), nil); err != nil {
				t.Fatalf("selected closure restore: %v", err)
			}
			exec("CREATE DATABASE external_fk")
			exec("CREATE TABLE external_fk.parent(id INT PRIMARY KEY)")
			exec("CREATE TABLE external_child(id INT,FOREIGN KEY(id) REFERENCES external_fk.parent(id))")
			req.Tables = nil
			output.Reset()
			if err := d.Export(ctx, cfg.Database, req, &output); err == nil || output.Len() != 0 {
				t.Fatalf("external FK export must fail before output: %v bytes=%d", err, output.Len())
			}

			var sourceRows int
			if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM a_child").Scan(&sourceRows); err != nil || sourceRows != 1 {
				t.Fatalf("source changed: %d %v", sourceRows, err)
			}
		})
	}
}
