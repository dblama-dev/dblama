//go:build integration

package postgres

import (
	"bytes"
	"context"
	"github.com/dblama-dev/dblama/internal/driver"
	"strings"
	"testing"
)

func TestExportPortableForeignKeys(t *testing.T) {
	ctx := context.Background()
	cfg := startDB(t)
	d := connectOK(t, cfg).(*PostgresDriver)
	exec := func(q string) {
		t.Helper()
		if _, err := d.db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE SCHEMA "FK Source"`)
	exec(`CREATE SCHEMA "FK Target"`)
	exec(`CREATE TABLE "FK Source".z_parent(a INT,b INT,PRIMARY KEY(a,b))`)
	exec(`CREATE TABLE "FK Source".a_child(id INT PRIMARY KEY,pa INT,pb INT,CONSTRAINT fk_composite FOREIGN KEY(pa,pb) REFERENCES "FK Source".z_parent(a,b))`)
	exec(`INSERT INTO "FK Source".z_parent VALUES(1,2)`)
	exec(`INSERT INTO "FK Source".a_child VALUES(9,1,2)`)
	exec(`CREATE TABLE "FK Source".cycle_left(id INT PRIMARY KEY,other INT)`)
	exec(`CREATE TABLE "FK Source".cycle_right(id INT PRIMARY KEY,other INT)`)
	exec(`INSERT INTO "FK Source".cycle_left VALUES(1,2)`)
	exec(`INSERT INTO "FK Source".cycle_right VALUES(2,1)`)
	exec(`ALTER TABLE "FK Source".cycle_left ADD CONSTRAINT fk_right FOREIGN KEY(other) REFERENCES "FK Source".cycle_right(id)`)
	exec(`ALTER TABLE "FK Source".cycle_right ADD CONSTRAINT fk_left FOREIGN KEY(other) REFERENCES "FK Source".cycle_left(id)`)
	var output bytes.Buffer
	req := driver.ExportRequest{Format: "sql", DropIfExists: true}
	if err := d.Export(driver.WithSchema(ctx, "FK Source"), cfg.Database, req, &output); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := d.ImportSQL(driver.WithSchema(ctx, "FK Target"), cfg.Database, strings.NewReader(output.String()), nil); err != nil {
			t.Fatalf("restore iteration %d: %v\n%s", i, err, output.String())
		}
	}
	for _, table := range []string{"a_child", "cycle_left", "cycle_right"} {
		def, err := d.TableDef(driver.WithSchema(ctx, "FK Target"), cfg.Database, table)
		if err != nil || len(def.ForeignKeys) != 1 {
			t.Fatalf("FK lost for %s: %+v %v", table, def, err)
		}
		if table == "a_child" && len(def.ForeignKeys[0].Columns) != 2 {
			t.Fatal("composite FK changed")
		}
	}
	var count, wrong int
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(*) FILTER (WHERE rn.nspname<>'FK Target') FROM pg_constraint con JOIN pg_class loc ON loc.oid=con.conrelid JOIN pg_namespace ln ON ln.oid=loc.relnamespace JOIN pg_class ref ON ref.oid=con.confrelid JOIN pg_namespace rn ON rn.oid=ref.relnamespace WHERE con.contype='f' AND ln.nspname='FK Target'`).Scan(&count, &wrong); err != nil || count != 3 || wrong != 0 {
		t.Fatalf("resolved FK schema: %d constraints, %d wrong targets: %v", count, wrong, err)
	}
	// Selected child exports include just its required local parent closure.
	req.Tables = []string{"a_child"}
	output.Reset()
	if err := d.Export(driver.WithSchema(ctx, "FK Source"), cfg.Database, req, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "cycle_left") {
		t.Fatal("selected export included unrelated tables")
	}
	exec(`CREATE SCHEMA "FK Selected"`)
	if err := d.ImportSQL(driver.WithSchema(ctx, "FK Selected"), cfg.Database, strings.NewReader(output.String()), nil); err != nil {
		t.Fatalf("selected closure restore: %v", err)
	}
	// A selected DROP must fail atomically when unrelated target objects depend
	// on it, rather than cascading into those objects.
	exec(`CREATE TABLE "FK Selected".outsider(a INT,b INT,FOREIGN KEY(a,b) REFERENCES "FK Selected".z_parent(a,b))`)

	if err := d.ImportSQL(driver.WithSchema(ctx, "FK Selected"), cfg.Database, strings.NewReader(output.String()), nil); err == nil {
		t.Fatal("DROP cascaded past unselected dependency")
	}
	var retained int
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM "FK Selected".a_child`).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("scoped DROP partially applied: %d %v", retained, err)
	}
	exec(`CREATE SCHEMA external_fk`)
	exec(`CREATE TABLE external_fk.parent(id INT PRIMARY KEY)`)
	exec(`CREATE TABLE "FK Source".external_child(id INT REFERENCES external_fk.parent(id))`)
	req.Tables = nil
	output.Reset()
	if err := d.Export(driver.WithSchema(ctx, "FK Source"), cfg.Database, req, &output); err == nil || output.Len() != 0 {
		t.Fatalf("external FK export must fail before output: %v bytes=%d", err, output.Len())
	}

	var sourceRows int
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM "FK Source".a_child`).Scan(&sourceRows); err != nil || sourceRows != 1 {
		t.Fatalf("source changed: %d %v", sourceRows, err)
	}
}
