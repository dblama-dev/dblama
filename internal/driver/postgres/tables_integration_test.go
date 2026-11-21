//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/dblama-dev/dblama/internal/driver"
)

// schemaFixture builds the "odd schema" playground every task 2 test reads
// from: quoted schema name with a space, mixed-case table, serial PK,
// defaults of every flavor, a generated column, all three index kinds, a
// cross-schema foreign key and a view. Statements run one by one through
// the driver's own pool.
var schemaFixture = []string{
	`DROP SCHEMA IF EXISTS "odd schema" CASCADE`,
	`CREATE SCHEMA "odd schema"`,
	`CREATE TABLE "odd schema".t2 (id integer PRIMARY KEY)`,
	`CREATE TABLE "odd schema".t1 (
		id serial PRIMARY KEY,
		name varchar(64) NOT NULL DEFAULT 'x',
		price numeric(10,2),
		data jsonb,
		ref uuid DEFAULT gen_random_uuid(),
		created_at timestamptz DEFAULT now(),
		tags text[],
		t2_id integer NOT NULL REFERENCES "odd schema".t2 (id) ON DELETE CASCADE,
		double_id integer GENERATED ALWAYS AS (id * 2) STORED)`,
	`COMMENT ON TABLE "odd schema".t1 IS 't1 comment'`,
	`CREATE UNIQUE INDEX t1_name_uidx ON "odd schema".t1 (name)`,
	`CREATE INDEX t1_price_idx ON "odd schema".t1 (price)`,
	`CREATE INDEX t1_expr_idx ON "odd schema".t1 (lower(name))`,
	`CREATE TABLE "odd schema"."MixedCase" (id integer PRIMARY KEY, note text)`,
	`CREATE TABLE "odd schema".nopk (a integer)`,
	`CREATE VIEW "odd schema".v1 AS SELECT id, name FROM "odd schema".t1`,
	`CREATE TABLE public.t3 (id integer PRIMARY KEY)`,
	`CREATE TABLE "odd schema".xref (id integer PRIMARY KEY, t3_id integer NOT NULL REFERENCES public.t3 (id))`,
}

// schemaFixtureName is the schema every statement above creates.
const schemaFixtureName = "odd schema"

// setupSchemaFixture applies schemaFixture through drv's own pool.
func setupSchemaFixture(t *testing.T, drv driver.Driver) {
	t.Helper()
	pg := drv.(*PostgresDriver) //nolint:forcetypeassert // test helper: connectOK only builds PostgresDriver
	for _, stmt := range schemaFixture {
		if _, err := pg.db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("fixture statement %q: %v", stmt, err)
		}
	}
}

// schemaAware returns drv as the SchemaAware extension the postgres driver
// implements; the assertion itself is the compile-time contract test.
func schemaAware(t *testing.T, drv driver.Driver) driver.SchemaAware {
	t.Helper()
	sa, ok := drv.(driver.SchemaAware)
	if !ok {
		t.Fatal("driver does not implement driver.SchemaAware")
	}
	return sa
}

// useFixtureSchema selects schemaFixtureName on drv, failing the test on
// error.
func useFixtureSchema(t *testing.T, drv driver.Driver) {
	t.Helper()
	if err := schemaAware(t, drv).SetSchema(schemaFixtureName); err != nil {
		t.Fatalf("SetSchema(%q) error: %v", schemaFixtureName, err)
	}
}

func TestSetSchema(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	setupSchemaFixture(t, drv)
	sa := schemaAware(t, drv)

	if got := sa.CurrentSchema(); got != "public" {
		t.Errorf("CurrentSchema() = %q before any SetSchema, want %q", got, "public")
	}

	if err := sa.SetSchema(schemaFixtureName); err != nil {
		t.Fatalf("SetSchema(%q) error: %v", schemaFixtureName, err)
	}
	if got := sa.CurrentSchema(); got != schemaFixtureName {
		t.Errorf("CurrentSchema() = %q after SetSchema, want %q", got, schemaFixtureName)
	}

	if err := sa.SetSchema(""); err == nil {
		t.Error(`SetSchema("") = nil error, want error`)
	}
	// The rejected SetSchema must not have changed the current schema.
	if got := sa.CurrentSchema(); got != schemaFixtureName {
		t.Errorf("CurrentSchema() = %q after rejected SetSchema, want %q", got, schemaFixtureName)
	}
}

func TestListTables(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	setupSchemaFixture(t, drv)
	useFixtureSchema(t, drv)

	tables, err := drv.ListTables(context.Background(), testDatabase)
	if err != nil {
		t.Fatalf("ListTables() error: %v", err)
	}

	names := make([]string, 0, len(tables))
	for _, tbl := range tables {
		names = append(names, tbl.Name)
	}
	want := []string{"MixedCase", "nopk", "t1", "t2", "v1", "xref"}
	if len(names) != len(want) {
		t.Fatalf("ListTables() = %v, want exactly %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("ListTables()[%d] = %q, want %q (full list %v)", i, names[i], want[i], names)
		}
	}

	byName := make(map[string]driver.TableInfo, len(tables))
	for _, tbl := range tables {
		byName[tbl.Name] = tbl
	}

	t1 := byName["t1"]
	if t1.Rows < 0 {
		t.Errorf("t1 Rows = %d, want >= 0", t1.Rows)
	}
	if t1.SizeBytes < 0 {
		t.Errorf("t1 SizeBytes = %d, want >= 0", t1.SizeBytes)
	}
	if t1.Comment != "t1 comment" {
		t.Errorf("t1 Comment = %q, want %q", t1.Comment, "t1 comment")
	}
	if t1.Engine != "" {
		t.Errorf("t1 Engine = %q, want empty (PostgreSQL has no engines)", t1.Engine)
	}
	if t1.IsView {
		t.Error("t1 IsView = true, want false")
	}

	if v1 := byName["v1"]; !v1.IsView {
		t.Error("v1 IsView = false, want true")
	}
	if mc := byName["MixedCase"]; mc.IsView {
		t.Error("MixedCase IsView = true, want false")
	}

	// The pool is bound to the login database; an empty db name is still
	// rejected before any query runs.
	if _, err := drv.ListTables(context.Background(), ""); err == nil {
		t.Error(`ListTables(ctx, "") = nil error, want error`)
	}
}

func TestPrimaryKey(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	setupSchemaFixture(t, drv)
	useFixtureSchema(t, drv)
	ctx := context.Background()

	pk, err := drv.PrimaryKey(ctx, testDatabase, "t1")
	if err != nil {
		t.Fatalf("PrimaryKey(t1) error: %v", err)
	}
	if len(pk) != 1 || pk[0] != "id" {
		t.Errorf("PrimaryKey(t1) = %v, want [id]", pk)
	}

	pk, err = drv.PrimaryKey(ctx, testDatabase, "MixedCase")
	if err != nil {
		t.Fatalf("PrimaryKey(MixedCase) error: %v", err)
	}
	if len(pk) != 1 || pk[0] != "id" {
		t.Errorf("PrimaryKey(MixedCase) = %v, want [id]", pk)
	}

	// A table without a primary key yields an empty slice, not an error.
	pk, err = drv.PrimaryKey(ctx, testDatabase, "nopk")
	if err != nil {
		t.Fatalf("PrimaryKey(nopk) error: %v", err)
	}
	if len(pk) != 0 {
		t.Errorf("PrimaryKey(nopk) = %v, want empty", pk)
	}

	if _, err := drv.PrimaryKey(ctx, "", "t1"); err == nil {
		t.Error(`PrimaryKey(ctx, "", "t1") = nil error, want error`)
	}
	if _, err := drv.PrimaryKey(ctx, testDatabase, ""); err == nil {
		t.Error(`PrimaryKey(ctx, db, "") = nil error, want error`)
	}
}
