//go:build integration

package postgres

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// saveSchemaName is the fresh schema every save test works in — a name
// with a space, so every statement must carry double-quoted
// schema-qualified identifiers.
const saveSchemaName = "save schema"

// useSaveSchema drops and recreates the save schema and selects it as
// the driver's current schema.
func useSaveSchema(t *testing.T, drv driver.Driver) {
	t.Helper()
	pg := drv.(*PostgresDriver) //nolint:forcetypeassert // test helper: connectOK only builds PostgresDriver
	ctx := context.Background()
	for _, stmt := range []string{
		`DROP SCHEMA IF EXISTS "save schema" CASCADE`,
		`CREATE SCHEMA "save schema"`,
	} {
		if _, err := pg.db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("save schema statement %q: %v", stmt, err)
		}
	}
	if err := schemaAware(t, drv).SetSchema(saveSchemaName); err != nil {
		t.Fatalf("SetSchema(%q) error: %v", saveSchemaName, err)
	}
}

// execSaveDDL runs one fixture statement through the driver's own pool.
func execSaveDDL(t *testing.T, drv driver.Driver, stmt string) {
	t.Helper()
	pg := drv.(*PostgresDriver) //nolint:forcetypeassert // test helper: connectOK only builds PostgresDriver
	if _, err := pg.db.ExecContext(context.Background(), stmt); err != nil {
		t.Fatalf("fixture statement %q: %v", stmt, err)
	}
}

// TestSaveTable_CreateAndReadBack creates a table from a desired state
// whose id carries BOTH serial markers exactly as the read path reports
// them (AutoIncrement plus the nextval default) and verifies the
// read-back definition: the serial must come back with a single
// nextval default — the save path renders serial without a second
// DEFAULT clause.
func TestSaveTable_CreateAndReadBack(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useSaveSchema(t, drv)
	ctx := context.Background()

	desired := driver.TableDef{
		NewName: "t3_create",
		Columns: []driver.ColumnDef{
			{
				Name:          "id",
				Type:          "integer",
				AutoIncrement: true,
				Default:       &driver.DefaultValue{Value: "nextval('t3_create_id_seq'::regclass)", IsSQLExpression: true},
			},
			{Name: "name", Type: "varchar", Length: "64", Default: &driver.DefaultValue{Value: "x"}},
			{Name: "price", Type: "numeric", Length: "10,2", Nullable: true},
			{Name: "data", Type: "jsonb", Nullable: true},
			{Name: "created_at", Type: "timestamptz", Nullable: true, Default: &driver.DefaultValue{Value: "now()", IsSQLExpression: true}},
		},
		Indexes: []driver.Index{
			{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
			{Name: "t3_name_uidx", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "name"}}},
		},
		Options: driver.TableOptions{Comment: "created by SaveTable"},
	}

	if err := drv.SaveTable(ctx, testDatabase, "", desired); err != nil {
		t.Fatalf("SaveTable(create) error: %v", err)
	}

	got, err := drv.TableDef(ctx, testDatabase, "t3_create")
	if err != nil {
		t.Fatalf("TableDef(t3_create) error: %v", err)
	}

	cols := defColumnsByName(t, got)
	if len(got.Columns) != 5 {
		var names []string
		for _, c := range got.Columns {
			names = append(names, c.Name)
		}
		t.Fatalf("TableDef(t3_create) has %d columns (%v), want 5", len(got.Columns), names)
	}

	id := cols["id"]
	if id.Type != "integer" {
		t.Errorf("id Type = %q, want integer", id.Type)
	}
	if !id.AutoIncrement {
		t.Error("id AutoIncrement = false, want true (serial)")
	}
	if id.Default == nil || !id.Default.IsSQLExpression || !strings.HasPrefix(id.Default.Value, "nextval(") {
		t.Fatalf("id Default = %+v, want nextval(...) expression", id.Default)
	}
	// The serial contract: exactly one identity default, not a doubled
	// nextval from an (absent) second DEFAULT clause.
	if n := strings.Count(id.Default.Value, "nextval("); n != 1 {
		t.Errorf("id Default contains %d nextval() calls (%q), want exactly 1", n, id.Default.Value)
	}

	name := cols["name"]
	if name.Type != "varchar" || name.Length != "64" {
		t.Errorf("name = %q(%q), want varchar(64)", name.Type, name.Length)
	}
	if name.Default == nil || name.Default.Value != "x" || name.Default.IsSQLExpression {
		t.Errorf("name Default = %+v, want literal x", name.Default)
	}

	price := cols["price"]
	if price.Type != "numeric" || price.Length != "10,2" || !price.Nullable {
		t.Errorf("price = %q(%q) nullable=%v, want numeric(10,2) nullable", price.Type, price.Length, price.Nullable)
	}
	if data := cols["data"]; data.Type != "jsonb" {
		t.Errorf("data Type = %q, want jsonb", data.Type)
	}
	created := cols["created_at"]
	if created.Type != "timestamptz" {
		t.Errorf("created_at Type = %q, want timestamptz", created.Type)
	}
	if created.Default == nil || created.Default.Value != "now()" || !created.Default.IsSQLExpression {
		t.Errorf("created_at Default = %+v, want now() expression", created.Default)
	}

	idx := defIndexesByName(t, got)
	if len(got.Indexes) != 2 {
		t.Fatalf("TableDef(t3_create) has %d indexes (%v), want 2", len(got.Indexes), got.Indexes)
	}
	if pk := idx["t3_create_pkey"]; pk.Type != "PRIMARY" || !sameStrings(indexColumns(pk), []string{"id"}) {
		t.Errorf("t3_create_pkey = %+v, want PRIMARY [id]", pk)
	}
	if u := idx["t3_name_uidx"]; u.Type != "UNIQUE" || !sameStrings(indexColumns(u), []string{"name"}) {
		t.Errorf("t3_name_uidx = %+v, want UNIQUE [name]", u)
	}

	if got.Options.Comment != "created by SaveTable" {
		t.Errorf("Options.Comment = %q, want %q", got.Options.Comment, "created by SaveTable")
	}
}

// TestSaveTable_AlterColumns creates a base table, then sends a desired
// state that renames a column (CHANGE), drops one, adds one and toggles
// another to nullable; the serial id travels through with both markers,
// proving MODIFY never touches its sequence default.
func TestSaveTable_AlterColumns(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useSaveSchema(t, drv)
	ctx := context.Background()

	base := driver.TableDef{
		NewName: "t3_alter",
		Columns: []driver.ColumnDef{
			{Name: "id", Type: "integer", AutoIncrement: true},
			{Name: "name", Type: "varchar", Length: "32"},
			{Name: "extra", Type: "varchar", Length: "16", Nullable: true},
			{Name: "flag", Type: "boolean"},
		},
		Indexes: []driver.Index{
			{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
		},
	}
	if err := drv.SaveTable(ctx, testDatabase, "", base); err != nil {
		t.Fatalf("SaveTable(create base) error: %v", err)
	}

	read, err := drv.TableDef(ctx, testDatabase, "t3_alter")
	if err != nil {
		t.Fatalf("TableDef(t3_alter) error: %v", err)
	}
	byName := defColumnsByName(t, read)

	// The id comes through exactly as read (OldName, serial default and
	// all); flag flips to nullable.
	flag := byName["flag"]
	flag.Nullable = true

	desired := driver.TableDef{
		NewName: "t3_alter",
		Columns: []driver.ColumnDef{
			byName["id"], // MODIFY (serial keeps its default)
			{Name: "full_name", OldName: "name", Type: "varchar", Length: "64"}, // CHANGE
			{Name: "extra", OldName: "extra", Nullable: true},                   // DROP
			{Name: "score", Type: "numeric", Length: "8,2", Nullable: true},     // ADD
			flag, // MODIFY (DROP NOT NULL)
		},
		Options: driver.TableOptions{Comment: "altered"},
	}
	if err := drv.SaveTable(ctx, testDatabase, "t3_alter", desired); err != nil {
		t.Fatalf("SaveTable(alter) error: %v", err)
	}

	got, err := drv.TableDef(ctx, testDatabase, "t3_alter")
	if err != nil {
		t.Fatalf("TableDef(t3_alter) after alter: %v", err)
	}
	cols := defColumnsByName(t, got)
	if _, exists := cols["extra"]; exists {
		t.Error("column extra still exists after drop")
	}
	if _, exists := cols["name"]; exists {
		t.Error("column name still exists after rename to full_name")
	}
	if len(got.Columns) != 4 {
		t.Fatalf("TableDef(t3_alter) has %d columns, want 4", len(got.Columns))
	}

	id := cols["id"]
	if !id.AutoIncrement || id.Default == nil || strings.Count(id.Default.Value, "nextval(") != 1 {
		t.Errorf("id after modify = AutoIncrement %v, Default %+v — the serial default must survive untouched", id.AutoIncrement, id.Default)
	}
	fullName := cols["full_name"]
	if fullName.Type != "varchar" || fullName.Length != "64" || fullName.Nullable {
		t.Errorf("full_name = %q(%q) nullable=%v, want varchar(64) NOT NULL", fullName.Type, fullName.Length, fullName.Nullable)
	}
	score := cols["score"]
	if score.Type != "numeric" || score.Length != "8,2" || !score.Nullable {
		t.Errorf("score = %q(%q) nullable=%v, want numeric(8,2) nullable", score.Type, score.Length, score.Nullable)
	}
	if flagCol := cols["flag"]; !flagCol.Nullable {
		t.Errorf("flag Nullable = false after SET NOT NULL toggle, want true")
	}
	if got.Options.Comment != "altered" {
		t.Errorf("Options.Comment = %q, want %q", got.Options.Comment, "altered")
	}
	if len(got.Indexes) != 1 || got.Indexes[0].Type != "PRIMARY" {
		t.Errorf("indexes after alter = %+v, want the PRIMARY only", got.Indexes)
	}
}

// TestSaveTable_RenameTable renames a table through SaveTable and
// verifies it lives under the new name while the old one is gone.
func TestSaveTable_RenameTable(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useSaveSchema(t, drv)
	ctx := context.Background()

	execSaveDDL(t, drv, `CREATE TABLE "save schema".t3_rename (id integer PRIMARY KEY, x integer NULL)`)

	def, err := drv.TableDef(ctx, testDatabase, "t3_rename")
	if err != nil {
		t.Fatalf("TableDef(t3_rename) error: %v", err)
	}
	def.NewName = "t3_renamed"

	if err := drv.SaveTable(ctx, testDatabase, "t3_rename", def); err != nil {
		t.Fatalf("SaveTable(rename) error: %v", err)
	}

	got, err := drv.TableDef(ctx, testDatabase, "t3_renamed")
	if err != nil {
		t.Fatalf("TableDef(t3_renamed) after rename: %v", err)
	}
	if got.NewName != "t3_renamed" || len(got.Columns) != 2 {
		t.Errorf("TableDef(t3_renamed) = %+v, want 2 columns", got)
	}
	if _, err := drv.TableDef(ctx, testDatabase, "t3_rename"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("TableDef(t3_rename) after rename = %v, want not found", err)
	}
}

// TestSaveTable_TransactionRollback pins spec §4.6: a desired state
// whose second step fails (a varchar column re-typed to integer without
// USING) leaves the table exactly as it was — step 1's added column is
// rolled back with the rest — and the error names the failing step.
func TestSaveTable_TransactionRollback(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useSaveSchema(t, drv)
	ctx := context.Background()

	execSaveDDL(t, drv, `CREATE TABLE "save schema".t3_rb (a varchar(16) NOT NULL, b integer NOT NULL)`)

	before, err := drv.TableDef(ctx, testDatabase, "t3_rb")
	if err != nil {
		t.Fatalf("TableDef(t3_rb) error: %v", err)
	}

	desired := driver.TableDef{
		NewName: "t3_rb",
		Columns: []driver.ColumnDef{
			{Name: "c", Type: "text", Nullable: true},  // step 1: applies inside the tx
			{Name: "a", OldName: "a", Type: "integer"}, // step 2: varchar→integer fails server-side
		},
	}
	err = drv.SaveTable(ctx, testDatabase, "t3_rb", desired)
	if err == nil {
		t.Fatal("SaveTable() with un-castable type change: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "step 2") {
		t.Errorf("SaveTable() error %q, want it to contain %q", err, "step 2")
	}
	if strings.Contains(err.Error(), testPassword) {
		t.Errorf("SaveTable() error %q leaks the superuser password", err)
	}

	// Nothing partial survives: the definition is byte-identical to the
	// pre-save read (the added column c is gone).
	after, err := drv.TableDef(ctx, testDatabase, "t3_rb")
	if err != nil {
		t.Fatalf("TableDef(t3_rb) after rollback: %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("table changed despite rollback:\nafter:  %+v\nbefore: %+v", after, before)
	}
}

// TestSaveIndexes_RoundTrip adds PRIMARY and UNIQUE indexes to a bare
// table, re-targets the UNIQUE (same name, different column), drops it,
// then exercises the PRIMARY add and drop, and verifies the invalid
// inputs fail before any statement runs.
func TestSaveIndexes_RoundTrip(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useSaveSchema(t, drv)
	ctx := context.Background()

	execSaveDDL(t, drv, `CREATE TABLE "save schema".t3_idx (id integer NOT NULL, name varchar(64) NOT NULL, kind varchar(32) NOT NULL)`)

	desired := []driver.Index{
		{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
		{Name: "t3_name_uidx", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "name"}}},
	}
	if err := drv.SaveIndexes(ctx, testDatabase, "t3_idx", desired); err != nil {
		t.Fatalf("SaveIndexes(add) error: %v", err)
	}
	def, err := drv.TableDef(ctx, testDatabase, "t3_idx")
	if err != nil {
		t.Fatalf("TableDef(after add) error: %v", err)
	}
	idx := defIndexesByName(t, def)
	if len(def.Indexes) != 2 {
		t.Fatalf("indexes after add = %+v, want PRIMARY + UNIQUE", def.Indexes)
	}
	if pk := idx["t3_idx_pkey"]; pk.Type != "PRIMARY" || !sameStrings(indexColumns(pk), []string{"id"}) {
		t.Errorf("t3_idx_pkey = %+v, want PRIMARY [id]", pk)
	}
	if u := idx["t3_name_uidx"]; u.Type != "UNIQUE" || !sameStrings(indexColumns(u), []string{"name"}) {
		t.Errorf("t3_name_uidx = %+v, want UNIQUE [name]", u)
	}

	// Same UNIQUE name on a different column: drop + re-add in one call.
	changed := []driver.Index{
		{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
		{Name: "t3_name_uidx", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "kind"}}},
	}
	if err := drv.SaveIndexes(ctx, testDatabase, "t3_idx", changed); err != nil {
		t.Fatalf("SaveIndexes(change) error: %v", err)
	}
	def, err = drv.TableDef(ctx, testDatabase, "t3_idx")
	if err != nil {
		t.Fatalf("TableDef(after change) error: %v", err)
	}
	idx = defIndexesByName(t, def)
	if u := idx["t3_name_uidx"]; u.Type != "UNIQUE" || !sameStrings(indexColumns(u), []string{"kind"}) {
		t.Errorf("t3_name_uidx after change = %+v, want UNIQUE [kind]", u)
	}

	// Drop everything — the PRIMARY goes via DROP CONSTRAINT.
	if err := drv.SaveIndexes(ctx, testDatabase, "t3_idx", nil); err != nil {
		t.Fatalf("SaveIndexes(drop all) error: %v", err)
	}
	def, err = drv.TableDef(ctx, testDatabase, "t3_idx")
	if err != nil {
		t.Fatalf("TableDef(after drop) error: %v", err)
	}
	if len(def.Indexes) != 0 {
		t.Errorf("indexes after dropping all = %+v, want none", def.Indexes)
	}

	// The PRIMARY comes back through ADD PRIMARY KEY.
	if err := drv.SaveIndexes(ctx, testDatabase, "t3_idx", desired[:1]); err != nil {
		t.Fatalf("SaveIndexes(re-add primary) error: %v", err)
	}
	def, err = drv.TableDef(ctx, testDatabase, "t3_idx")
	if err != nil {
		t.Fatalf("TableDef(after re-add) error: %v", err)
	}
	if len(def.Indexes) != 1 || def.Indexes[0].Type != "PRIMARY" || !sameStrings(indexColumns(def.Indexes[0]), []string{"id"}) {
		t.Errorf("indexes after re-add primary = %+v, want PRIMARY [id]", def.Indexes)
	}

	// Invalid inputs fail before any statement.
	if err := drv.SaveIndexes(ctx, "", "t3_idx", nil); err == nil {
		t.Error(`SaveIndexes(ctx, "", ...) = nil error, want error`)
	}
	if err := drv.SaveIndexes(ctx, testDatabase, "", nil); err == nil {
		t.Error("SaveIndexes(empty table) error: expected error, got nil")
	}
	fulltext := []driver.Index{{Name: "ft", Type: "FULLTEXT", Columns: []driver.IndexColumn{{Name: "name"}}}}
	if err := drv.SaveIndexes(ctx, testDatabase, "t3_idx", fulltext); err == nil || !strings.Contains(err.Error(), "not supported on postgres") {
		t.Errorf("SaveIndexes(FULLTEXT) error = %v, want not supported on postgres", err)
	}
	unnamed := []driver.Index{{Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "name"}}}}
	if err := drv.SaveIndexes(ctx, testDatabase, "t3_idx", unnamed); err == nil {
		t.Error("SaveIndexes(unnamed unique) error: expected error, got nil")
	}
	dup := []driver.Index{
		{Name: "t3_dup", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "name"}}},
		{Name: "t3_dup", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "kind"}}},
	}
	if err := drv.SaveIndexes(ctx, testDatabase, "t3_idx", dup); err == nil {
		t.Error("SaveIndexes(duplicate names) error: expected error, got nil")
	}

	// The rejected calls must not have touched the table.
	def, err = drv.TableDef(ctx, testDatabase, "t3_idx")
	if err != nil {
		t.Fatalf("TableDef(after rejects) error: %v", err)
	}
	if len(def.Indexes) != 1 || def.Indexes[0].Type != "PRIMARY" {
		t.Errorf("indexes after rejected calls = %+v, want the PRIMARY untouched", def.Indexes)
	}
}
