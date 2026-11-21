//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/dblama-dev/dblama/internal/driver"
)

// defColumnsByName turns def's column list into a name → ColumnDef map.
func defColumnsByName(t *testing.T, def driver.TableDef) map[string]driver.ColumnDef {
	t.Helper()
	cols := make(map[string]driver.ColumnDef, len(def.Columns))
	for _, col := range def.Columns {
		if _, dup := cols[col.Name]; dup {
			t.Fatalf("duplicate column %q in def", col.Name)
		}
		cols[col.Name] = col
	}
	return cols
}

// defIndexesByName turns def's index list into a name → Index map.
func defIndexesByName(t *testing.T, def driver.TableDef) map[string]driver.Index {
	t.Helper()
	idx := make(map[string]driver.Index, len(def.Indexes))
	for _, ix := range def.Indexes {
		idx[ix.Name] = ix
	}
	return idx
}

// indexColumns flattens an index's column names.
func indexColumns(ix driver.Index) []string {
	cols := make([]string, 0, len(ix.Columns))
	for _, c := range ix.Columns {
		cols = append(cols, c.Name)
	}
	return cols
}

func TestTableDef_RoundTrip(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	setupSchemaFixture(t, drv)
	useFixtureSchema(t, drv)
	ctx := context.Background()

	def, err := drv.TableDef(ctx, testDatabase, "t1")
	if err != nil {
		t.Fatalf("TableDef(t1) error: %v", err)
	}

	if def.NewName != "t1" {
		t.Errorf("NewName = %q, want %q", def.NewName, "t1")
	}

	cols := defColumnsByName(t, def)
	if len(def.Columns) != 9 {
		var names []string
		for _, c := range def.Columns {
			names = append(names, c.Name)
		}
		t.Fatalf("TableDef(t1) has %d columns (%v), want 9", len(def.Columns), names)
	}
	for _, col := range def.Columns {
		if col.OldName != col.Name {
			t.Errorf("column %q OldName = %q, want same as Name", col.Name, col.OldName)
		}
		if col.Position != "" {
			t.Errorf("column %q Position = %q, want empty", col.Name, col.Position)
		}
	}

	id := cols["id"]
	if id.Type != "integer" {
		t.Errorf("id Type = %q, want integer", id.Type)
	}
	if !id.AutoIncrement {
		t.Error("id AutoIncrement = false for serial, want true")
	}
	if id.Nullable {
		t.Error("id Nullable = true, want false (PRIMARY KEY)")
	}
	if id.Default == nil || !id.Default.IsSQLExpression || !strings.HasPrefix(id.Default.Value, "nextval(") {
		t.Errorf("id Default = %+v, want nextval(...) expression", id.Default)
	}

	name := cols["name"]
	if name.Type != "varchar" {
		t.Errorf("name Type = %q, want varchar", name.Type)
	}
	if name.Length != "64" {
		t.Errorf("name Length = %q, want 64", name.Length)
	}
	if name.Nullable {
		t.Error("name Nullable = true, want false")
	}
	if name.Default == nil || name.Default.Value != "x" || name.Default.IsSQLExpression {
		t.Errorf("name Default = %+v, want literal x", name.Default)
	}

	price := cols["price"]
	if price.Type != "numeric" {
		t.Errorf("price Type = %q, want numeric", price.Type)
	}
	if price.Length != "10,2" {
		t.Errorf("price Length = %q, want 10,2", price.Length)
	}
	if !price.Nullable {
		t.Error("price Nullable = false, want true")
	}
	if price.Default != nil {
		t.Errorf("price Default = %+v, want nil", price.Default)
	}

	if data := cols["data"]; data.Type != "jsonb" {
		t.Errorf("data Type = %q, want jsonb", data.Type)
	}

	ref := cols["ref"]
	if ref.Type != "uuid" {
		t.Errorf("ref Type = %q, want uuid", ref.Type)
	}
	if ref.Default == nil || ref.Default.Value != "gen_random_uuid()" || !ref.Default.IsSQLExpression {
		t.Errorf("ref Default = %+v, want gen_random_uuid() expression", ref.Default)
	}

	created := cols["created_at"]
	if created.Type != "timestamptz" {
		t.Errorf("created_at Type = %q, want timestamptz", created.Type)
	}
	if created.Default == nil || created.Default.Value != "now()" || !created.Default.IsSQLExpression {
		t.Errorf("created_at Default = %+v, want now() expression", created.Default)
	}

	tags := cols["tags"]
	if tags.Type != "text[]" {
		t.Errorf("tags Type = %q, want text[]", tags.Type)
	}
	if tags.EnumValues != nil {
		t.Errorf("tags EnumValues = %v, want nil", tags.EnumValues)
	}

	t2id := cols["t2_id"]
	if t2id.Type != "integer" {
		t.Errorf("t2_id Type = %q, want integer", t2id.Type)
	}
	if t2id.Nullable {
		t.Error("t2_id Nullable = true, want false")
	}

	doubleID := cols["double_id"]
	if doubleID.Generated != "STORED" {
		t.Errorf("double_id Generated = %q, want STORED", doubleID.Generated)
	}
	if doubleID.Default == nil || doubleID.Default.Value != "(id * 2)" || !doubleID.Default.IsSQLExpression {
		t.Errorf("double_id Default = %+v, want (id * 2) expression", doubleID.Default)
	}

	idx := defIndexesByName(t, def)
	if len(def.Indexes) != 4 {
		var names []string
		for _, ix := range def.Indexes {
			names = append(names, ix.Name)
		}
		t.Fatalf("TableDef(t1) has %d indexes (%v), want 4", len(def.Indexes), names)
	}
	if pk := idx["t1_pkey"]; pk.Type != "PRIMARY" || !sameStrings(indexColumns(pk), []string{"id"}) {
		t.Errorf("t1_pkey = %+v, want PRIMARY [id]", pk)
	}
	if u := idx["t1_name_uidx"]; u.Type != "UNIQUE" || !sameStrings(indexColumns(u), []string{"name"}) {
		t.Errorf("t1_name_uidx = %+v, want UNIQUE [name]", u)
	}
	if p := idx["t1_price_idx"]; p.Type != "INDEX" || !sameStrings(indexColumns(p), []string{"price"}) {
		t.Errorf("t1_price_idx = %+v, want INDEX [price]", p)
	}
	if e := idx["t1_expr_idx"]; e.Type != "INDEX" || len(e.Columns) != 1 || !strings.Contains(e.Columns[0].Name, "lower") {
		t.Errorf("t1_expr_idx = %+v, want INDEX on a lower(...) expression", e)
	}

	if len(def.ForeignKeys) != 1 {
		t.Fatalf("TableDef(t1) has %d foreign keys, want 1: %+v", len(def.ForeignKeys), def.ForeignKeys)
	}
	fk := def.ForeignKeys[0]
	if fk.Name != "t1_t2_id_fkey" {
		t.Errorf("fk Name = %q, want t1_t2_id_fkey", fk.Name)
	}
	if !sameStrings(fk.Columns, []string{"t2_id"}) {
		t.Errorf("fk Columns = %v, want [t2_id]", fk.Columns)
	}
	if fk.RefTable != "t2" {
		t.Errorf("fk RefTable = %q, want t2 (same schema stays bare)", fk.RefTable)
	}
	if !sameStrings(fk.RefColumns, []string{"id"}) {
		t.Errorf("fk RefColumns = %v, want [id]", fk.RefColumns)
	}
	if fk.OnDelete != "CASCADE" {
		t.Errorf("fk OnDelete = %q, want CASCADE", fk.OnDelete)
	}
	if fk.OnUpdate != "NO ACTION" {
		t.Errorf("fk OnUpdate = %q, want NO ACTION", fk.OnUpdate)
	}

	if def.Options.Engine != "" {
		t.Errorf("Options.Engine = %q, want empty", def.Options.Engine)
	}
	if def.Options.Collation != "" {
		t.Errorf("Options.Collation = %q, want empty", def.Options.Collation)
	}
	if def.Options.Comment != "t1 comment" {
		t.Errorf("Options.Comment = %q, want %q", def.Options.Comment, "t1 comment")
	}
	if def.Options.AutoIncrement != 0 {
		t.Errorf("Options.AutoIncrement = %d, want 0", def.Options.AutoIncrement)
	}

	// A mixed-case table name reaches the catalogs case-sensitively.
	mc, err := drv.TableDef(ctx, testDatabase, "MixedCase")
	if err != nil {
		t.Fatalf("TableDef(MixedCase) error: %v", err)
	}
	if mc.NewName != "MixedCase" || len(mc.Columns) != 2 || len(mc.Indexes) != 1 {
		t.Errorf("TableDef(MixedCase) = %+v, want 2 columns and 1 index", mc)
	}

	// A foreign key into another schema qualifies the referenced table.
	xref, err := drv.TableDef(ctx, testDatabase, "xref")
	if err != nil {
		t.Fatalf("TableDef(xref) error: %v", err)
	}
	if len(xref.ForeignKeys) != 1 || xref.ForeignKeys[0].RefTable != "t3" || xref.ForeignKeys[0].RefSchema != "public" {
		t.Errorf("xref ForeignKeys = %+v, want one FK into public.t3", xref.ForeignKeys)
	}

	// A missing table yields "not found"; empty names fail before any query.
	_, err = drv.TableDef(ctx, testDatabase, "missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("TableDef(missing) error = %v, want not found", err)
	}
	if _, err := drv.TableDef(ctx, "", "t1"); err == nil {
		t.Error(`TableDef(ctx, "", "t1") = nil error, want error`)
	}
	if _, err := drv.TableDef(ctx, testDatabase, ""); err == nil {
		t.Error(`TableDef(ctx, db, "") = nil error, want error`)
	}
}

// sameStrings compares two string slices for exact equality.
func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
