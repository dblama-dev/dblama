//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// saveImageExpectations holds the per-image read-back expectations of the
// SaveTable tests (same reporting differences as the Task 3 round trip).
func saveImageExpectations(image string) (defImageExpectations, bool) {
	exp, ok := map[string]defImageExpectations{
		"mysql:8.4": {
			tableCollation:  "utf8mb4_0900_ai_ci",
			stringCollation: "utf8mb4_0900_ai_ci",
			intLength:       "",
			tsExpr:          "CURRENT_TIMESTAMP",
			autoIncrement:   0,
		},
		"mariadb:11.4": {
			tableCollation:  "utf8mb4_unicode_ci",
			stringCollation: "utf8mb4_unicode_ci",
			intLength:       "10",
			tsExpr:          "current_timestamp()",
			autoIncrement:   1,
		},
	}[image]
	return exp, ok
}

// aiIntLength is the display width MariaDB reports for an
// AUTO_INCREMENT int column (int(11)); MySQL 8 dropped display widths.
func aiIntLength(image string) string {
	if image == "mariadb:11.4" {
		return "11"
	}
	return ""
}

// TestSaveTable_CreateAndReadBack creates a table from a desired state
// and verifies the read-back definition matches it per image.
func TestSaveTable_CreateAndReadBack(t *testing.T) {
	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			exp, ok := saveImageExpectations(img.image)
			if !ok {
				t.Fatalf("no expectations for image %q", img.image)
			}
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			desired := driver.TableDef{
				NewName: "t4_create",
				Columns: []driver.ColumnDef{
					{Name: "id", Type: "int", AutoIncrement: true},
					// Backslashes in literals must survive (the DSN
					// leaves backslash escapes enabled on both servers).
					{Name: "name", Type: "varchar", Length: "64", Comment: `path\to\dir`},
					{Name: "kind", Type: "enum", Nullable: true, EnumValues: []string{"a", `a\b`}},
					{Name: "created_at", Type: "timestamp", Default: &driver.DefaultValue{Value: "CURRENT_TIMESTAMP", IsSQLExpression: true}, OnUpdate: "CURRENT_TIMESTAMP"},
				},
				Indexes: []driver.Index{
					{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
				},
				Options: driver.TableOptions{
					Engine:    "InnoDB",
					Collation: exp.tableCollation,
					Comment:   "created by SaveTable",
				},
			}
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS t4_create") })

			if err := drv.SaveTable(ctx, cfg.Database, "", desired); err != nil {
				t.Fatalf("SaveTable(create) error: %v", err)
			}

			got, err := drv.TableDef(ctx, cfg.Database, "t4_create")
			if err != nil {
				t.Fatalf("TableDef(t4_create) error: %v", err)
			}

			want := driver.TableDef{
				NewName: "t4_create",
				Columns: []driver.ColumnDef{
					{Name: "id", OldName: "id", Type: "int", Length: aiIntLength(img.image), AutoIncrement: true},
					{Name: "name", OldName: "name", Type: "varchar", Length: "64", Comment: `path\to\dir`, Collation: exp.stringCollation},
					{Name: "kind", OldName: "kind", Type: "enum", Nullable: true, Collation: exp.stringCollation, EnumValues: []string{"a", `a\b`}},
					{Name: "created_at", OldName: "created_at", Type: "timestamp", Default: &driver.DefaultValue{Value: exp.tsExpr, IsSQLExpression: true}, OnUpdate: exp.tsExpr},
				},
				Indexes: []driver.Index{
					{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
				},
				Options: driver.TableOptions{
					Engine:        "InnoDB",
					Collation:     exp.tableCollation,
					Comment:       "created by SaveTable",
					AutoIncrement: exp.autoIncrement,
				},
			}
			assertTableDef(t, got, want)
		})
	}
}

// TestSaveTable_AlterColumns creates two columns, then sends a desired
// state that renames one (CHANGE), drops the other, adds a new one and
// changes the table comment.
func TestSaveTable_AlterColumns(t *testing.T) {
	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			exp, ok := saveImageExpectations(img.image)
			if !ok {
				t.Fatalf("no expectations for image %q", img.image)
			}
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			base := driver.TableDef{
				NewName: "t4_alter",
				Columns: []driver.ColumnDef{
					{Name: "id", Type: "int", AutoIncrement: true},
					{Name: "name", Type: "varchar", Length: "32"},
					{Name: "extra", Type: "varchar", Length: "16", Nullable: true},
				},
				Indexes: []driver.Index{
					{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
				},
				Options: driver.TableOptions{Engine: "InnoDB", Collation: exp.tableCollation},
			}
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS t4_alter") })

			if err := drv.SaveTable(ctx, cfg.Database, "", base); err != nil {
				t.Fatalf("SaveTable(create base) error: %v", err)
			}

			desired := driver.TableDef{
				Columns: []driver.ColumnDef{
					{Name: "id", OldName: "id", Type: "int", AutoIncrement: true},                                              // MODIFY
					{Name: "full_name", OldName: "name", Type: "varchar", Length: "64"},                                        // CHANGE
					{Name: "extra", OldName: "extra", Nullable: true},                                                          // DROP
					{Name: "score", Type: "decimal", Length: "8,2", Nullable: true, Default: &driver.DefaultValue{Value: "0"}}, // ADD
				},
				Options: driver.TableOptions{Comment: "altered"},
			}
			if err := drv.SaveTable(ctx, cfg.Database, "t4_alter", desired); err != nil {
				t.Fatalf("SaveTable(alter) error: %v", err)
			}

			got, err := drv.TableDef(ctx, cfg.Database, "t4_alter")
			if err != nil {
				t.Fatalf("TableDef(t4_alter) error: %v", err)
			}

			want := driver.TableDef{
				NewName: "t4_alter",
				Columns: []driver.ColumnDef{
					{Name: "id", OldName: "id", Type: "int", Length: aiIntLength(img.image), AutoIncrement: true},
					{Name: "full_name", OldName: "full_name", Type: "varchar", Length: "64", Collation: exp.stringCollation},
					{Name: "score", OldName: "score", Type: "decimal", Length: "8,2", Nullable: true, Default: &driver.DefaultValue{Value: "0.00"}},
				},
				Indexes: []driver.Index{
					{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
				},
				Options: driver.TableOptions{
					Engine:    "InnoDB",
					Collation: exp.tableCollation,
					Comment:   "altered",
					// The column ALTER rebuilds the table, which
					// materializes AUTO_INCREMENT=1 on both servers.
					AutoIncrement: 1,
				},
			}
			assertTableDef(t, got, want)
		})
	}
}

// TestSaveTable_RenameTable sends a desired state whose NewName differs
// from the current table and verifies the table lives under the new name
// while the old one is gone.
func TestSaveTable_RenameTable(t *testing.T) {
	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			execDDL(t, cfg, "CREATE TABLE t4_rename (id INT PRIMARY KEY, x INT NULL)")
			t.Cleanup(func() {
				execDDL(t, cfg, "DROP TABLE IF EXISTS t4_rename")
				execDDL(t, cfg, "DROP TABLE IF EXISTS t4_renamed")
			})

			def, err := drv.TableDef(ctx, cfg.Database, "t4_rename")
			if err != nil {
				t.Fatalf("TableDef(t4_rename) error: %v", err)
			}
			def.NewName = "t4_renamed"

			if err := drv.SaveTable(ctx, cfg.Database, "t4_rename", def); err != nil {
				t.Fatalf("SaveTable(rename) error: %v", err)
			}

			got, err := drv.TableDef(ctx, cfg.Database, "t4_renamed")
			if err != nil {
				t.Fatalf("TableDef(t4_renamed) after rename: %v", err)
			}
			if got.NewName != "t4_renamed" {
				t.Errorf("NewName = %q, want %q", got.NewName, "t4_renamed")
			}
			if len(got.Columns) != 2 || got.Columns[0].Name != "id" || got.Columns[1].Name != "x" {
				t.Errorf("columns after rename = %v, want id,x", got.Columns)
			}

			tables, err := drv.ListTables(ctx, cfg.Database)
			if err != nil {
				t.Fatalf("ListTables() error: %v", err)
			}
			for _, tbl := range tables {
				if tbl.Name == "t4_rename" {
					t.Error("old table t4_rename still exists after rename")
				}
			}
		})
	}
}

// TestSaveTable_ErrorStepReporting verifies that a failing CREATE (a
// foreign key to a nonexistent table) is reported with a step number,
// leaves no half-created table behind and leaks neither the password nor
// the DSN.
func TestSaveTable_ErrorStepReporting(t *testing.T) {
	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			desired := driver.TableDef{
				NewName: "t4_fkfail",
				Columns: []driver.ColumnDef{
					{Name: "id", Type: "int", AutoIncrement: true},
					{Name: "pid", Type: "int", Nullable: true},
				},
				Indexes: []driver.Index{
					{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
				},
				ForeignKeys: []driver.ForeignKey{
					{Name: "fk_missing", Columns: []string{"pid"}, RefTable: "t4_no_such_parent", RefColumns: []string{"id"}},
				},
			}
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS t4_fkfail") })

			err := drv.SaveTable(ctx, cfg.Database, "", desired)
			if err == nil {
				t.Fatal("SaveTable() with FK to missing table: expected error, got nil")
			}
			if !strings.Contains(err.Error(), "step") {
				t.Errorf("SaveTable() error %q, want it to contain %q", err, "step")
			}
			if strings.Contains(err.Error(), testRootPassword) {
				t.Errorf("SaveTable() error %q leaks the root password", err)
			}
			if strings.Contains(err.Error(), "password") {
				t.Errorf("SaveTable() error %q mentions the password", err)
			}

			// The failed CREATE must be atomic: no partial table left.
			_, defErr := drv.TableDef(ctx, cfg.Database, "t4_fkfail")
			if defErr == nil || !strings.Contains(defErr.Error(), "not found") {
				t.Errorf("TableDef(t4_fkfail) after failed create = %v, want a not-found error", defErr)
			}
		})
	}
}

// TestSaveTable_RoundTripPreservesColumns reads a definition, sends it
// back unchanged (with OldName mirroring Name, the read contract) and
// verifies the stored definition is byte-identical afterwards.
func TestSaveTable_RoundTripPreservesColumns(t *testing.T) {
	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			exp, ok := saveImageExpectations(img.image)
			if !ok {
				t.Fatalf("no expectations for image %q", img.image)
			}
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			desired := driver.TableDef{
				NewName: "t4_roundtrip",
				Columns: []driver.ColumnDef{
					{Name: "id", Type: "int", AutoIncrement: true},
					{Name: "name", Type: "varchar", Length: "64", Default: &driver.DefaultValue{Value: "x"}, Comment: "who"},
					// The generated expression is read from
					// GENERATION_EXPRESSION and re-emitted as AS (expr).
					{Name: "name_up", Type: "varchar", Length: "64", Nullable: true, Generated: "STORED",
						Default: &driver.DefaultValue{Value: "UPPER(name)", IsSQLExpression: true}},
					{Name: "kind", Type: "enum", Nullable: true, EnumValues: []string{"a", "b", "c"}},
					{Name: "price", Type: "decimal", Length: "10,2", Nullable: true},
					{Name: "size_code", Type: "char", Length: "2", Nullable: true, Collation: "utf8mb4_bin"},
					{Name: "created_at", Type: "timestamp", Default: &driver.DefaultValue{Value: "CURRENT_TIMESTAMP", IsSQLExpression: true}, OnUpdate: "CURRENT_TIMESTAMP"},
				},
				Indexes: []driver.Index{
					{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
				},
				Options: driver.TableOptions{
					Engine:    "InnoDB",
					Collation: exp.tableCollation,
					Comment:   "round trip",
				},
			}
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS t4_roundtrip") })

			if err := drv.SaveTable(ctx, cfg.Database, "", desired); err != nil {
				t.Fatalf("SaveTable(create) error: %v", err)
			}

			first, err := drv.TableDef(ctx, cfg.Database, "t4_roundtrip")
			if err != nil {
				t.Fatalf("TableDef(before round trip) error: %v", err)
			}
			// The web layer sends the read definition back with
			// OldName == Name (the read contract); without it every
			// column would be interpreted as an ADD.
			for i := range first.Columns {
				first.Columns[i].OldName = first.Columns[i].Name
			}

			if err := drv.SaveTable(ctx, cfg.Database, "t4_roundtrip", first); err != nil {
				t.Fatalf("SaveTable(round trip) error: %v", err)
			}

			second, err := drv.TableDef(ctx, cfg.Database, "t4_roundtrip")
			if err != nil {
				t.Fatalf("TableDef(after round trip) error: %v", err)
			}
			assertTableDef(t, second, first)
		})
	}
}

// TestSaveTable_ReorderColumns proves that reordering existing columns
// is a real server-side effect now: [a,b,c] reordered to [c,a,b] (the
// frontend sends every column with a Position from the desired display
// order) reads back in the new ordinal order on both images.
func TestSaveTable_ReorderColumns(t *testing.T) {
	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			execDDL(t, cfg, "CREATE TABLE t4_reorder (a INT NOT NULL, b INT NOT NULL, c INT NOT NULL)")
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS t4_reorder") })

			def, err := drv.TableDef(ctx, cfg.Database, "t4_reorder")
			if err != nil {
				t.Fatalf("TableDef(t4_reorder) error: %v", err)
			}

			// Desired order c, a, b with positions from the display
			// order, exactly as the structure editor sends them.
			byName := map[string]driver.ColumnDef{}
			for _, c := range def.Columns {
				byName[c.Name] = c
			}
			moved := func(name, position string) driver.ColumnDef {
				c, ok := byName[name]
				if !ok {
					t.Fatalf("column %q missing from read-back definition", name)
				}
				c.Position = position
				return c
			}
			def.Columns = []driver.ColumnDef{
				moved("c", "FIRST"),
				moved("a", "c"),
				moved("b", "a"),
			}

			if err := drv.SaveTable(ctx, cfg.Database, "t4_reorder", def); err != nil {
				t.Fatalf("SaveTable(reorder) error: %v", err)
			}

			got, err := drv.TableDef(ctx, cfg.Database, "t4_reorder")
			if err != nil {
				t.Fatalf("TableDef(t4_reorder) after reorder: %v", err)
			}
			names := make([]string, len(got.Columns))
			for i, c := range got.Columns {
				names[i] = c.Name
			}
			want := []string{"c", "a", "b"}
			if !slices.Equal(names, want) {
				t.Errorf("column order after reorder = %v, want %v", names, want)
			}
		})
	}
}

// queryCount returns COUNT(*) of db.table read over a raw connection
// (TableInfo.Rows is only an estimate for InnoDB).
func queryCount(t *testing.T, cfg driver.ConnConfig, db, table string) int64 {
	t.Helper()
	dsnDB, err := sql.Open("mysql", DSN(cfg, cfg.Password))
	if err != nil {
		t.Fatalf("open count connection: %v", err)
	}
	defer func() { _ = dsnDB.Close() }()
	var n int64
	if err := dsnDB.QueryRow("SELECT COUNT(*) FROM " + qualifiedIdent(db, table)).Scan(&n); err != nil {
		t.Fatalf("count %s.%s: %v", db, table, err)
	}
	return n
}

// assertIndexes compares got with want as name-keyed sets (STATISTICS
// ordering differs between servers), so index order is irrelevant.
func assertIndexes(t *testing.T, got, want []driver.Index) {
	t.Helper()
	sortByName := func(idx []driver.Index) []driver.Index {
		out := slices.Clone(idx)
		slices.SortFunc(out, func(a, b driver.Index) int { return strings.Compare(a.Name, b.Name) })
		return out
	}
	gotSorted, wantSorted := sortByName(got), sortByName(want)
	if !reflect.DeepEqual(gotSorted, wantSorted) {
		t.Errorf("indexes = %+v, want %+v", gotSorted, wantSorted)
	}
}

// TestSaveIndexes_RoundTrip adds PRIMARY and UNIQUE indexes to a bare
// table, verifies the read-back definition, then changes the UNIQUE
// (same name, different columns and a sub-part) and finally removes it.
func TestSaveIndexes_RoundTrip(t *testing.T) {
	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			execDDL(t, cfg, "CREATE TABLE t5_idx (id INT NOT NULL, name VARCHAR(64) NOT NULL, kind VARCHAR(32) NOT NULL)")
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS t5_idx") })

			desired := []driver.Index{
				{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
				{Name: "name", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "name"}}},
			}
			if err := drv.SaveIndexes(ctx, cfg.Database, "t5_idx", desired); err != nil {
				t.Fatalf("SaveIndexes(add) error: %v", err)
			}

			def, err := drv.TableDef(ctx, cfg.Database, "t5_idx")
			if err != nil {
				t.Fatalf("TableDef(after add) error: %v", err)
			}
			assertIndexes(t, def.Indexes, desired)

			// Same UNIQUE name with different columns and a sub-part:
			// SaveIndexes must DROP the old definition and ADD the new.
			changed := []driver.Index{
				{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
				{Name: "name", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "kind"}, {Name: "name", SubPart: 10}}},
			}
			if err := drv.SaveIndexes(ctx, cfg.Database, "t5_idx", changed); err != nil {
				t.Fatalf("SaveIndexes(change) error: %v", err)
			}
			def2, err := drv.TableDef(ctx, cfg.Database, "t5_idx")
			if err != nil {
				t.Fatalf("TableDef(after change) error: %v", err)
			}
			assertIndexes(t, def2.Indexes, changed)

			// Desired without the UNIQUE index: it must disappear.
			if err := drv.SaveIndexes(ctx, cfg.Database, "t5_idx", desired[:1]); err != nil {
				t.Fatalf("SaveIndexes(drop unique) error: %v", err)
			}
			def3, err := drv.TableDef(ctx, cfg.Database, "t5_idx")
			if err != nil {
				t.Fatalf("TableDef(after drop) error: %v", err)
			}
			assertIndexes(t, def3.Indexes, desired[:1])
		})
	}
}

// TestSaveIndexes_PrimaryDrop removes the PRIMARY KEY of a table without
// AUTO_INCREMENT columns (desired state without PRIMARY) and verifies
// empty identifiers fail before any statement is issued.
func TestSaveIndexes_PrimaryDrop(t *testing.T) {
	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			execDDL(t, cfg, "CREATE TABLE t5_prim (id INT NOT NULL, x INT NOT NULL, PRIMARY KEY (id))")
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS t5_prim") })

			if err := drv.SaveIndexes(ctx, cfg.Database, "t5_prim", nil); err != nil {
				t.Fatalf("SaveIndexes(drop primary) error: %v", err)
			}
			def, err := drv.TableDef(ctx, cfg.Database, "t5_prim")
			if err != nil {
				t.Fatalf("TableDef(after drop) error: %v", err)
			}
			if len(def.Indexes) != 0 {
				t.Errorf("indexes after dropping all = %+v, want none", def.Indexes)
			}

			// Empty identifiers fail before any statement.
			if err := drv.SaveIndexes(ctx, "", "t5_prim", nil); err == nil {
				t.Error("SaveIndexes(empty db) error: expected error, got nil")
			}
			if err := drv.SaveIndexes(ctx, cfg.Database, "", nil); err == nil {
				t.Error("SaveIndexes(empty table) error: expected error, got nil")
			}
			unnamed := []driver.Index{{Name: "", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "x"}}}}
			if err := drv.SaveIndexes(ctx, cfg.Database, "t5_prim", unnamed); err == nil {
				t.Error("SaveIndexes(unnamed unique) error: expected error, got nil")
			}
			// Unknown types are rejected instead of silently rendered as
			// plain INDEX.
			weird := []driver.Index{{Name: "uq", Type: "UNIQUE KEY", Columns: []driver.IndexColumn{{Name: "x"}}}}
			if err := drv.SaveIndexes(ctx, cfg.Database, "t5_prim", weird); err == nil {
				t.Error("SaveIndexes(unknown type) error: expected error, got nil")
			}
			// Duplicate names within one desired list are a hard error.
			dup := []driver.Index{
				{Name: "uq", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "x"}}},
				{Name: "UQ", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "id"}}},
			}
			if err := drv.SaveIndexes(ctx, cfg.Database, "t5_prim", dup); err == nil {
				t.Error("SaveIndexes(duplicate name) error: expected error, got nil")
			}
		})
	}
}

// TestTruncateTables empties two tables, verifies the first error
// aborts the remaining tables untouched and that empty names fail
// before any statement.
func TestTruncateTables(t *testing.T) {
	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			execDDL(t, cfg, "CREATE TABLE t5_trunc_a (id INT NOT NULL)")
			execDDL(t, cfg, "CREATE TABLE t5_trunc_b (id INT NOT NULL)")
			t.Cleanup(func() {
				execDDL(t, cfg, "DROP TABLE IF EXISTS t5_trunc_a")
				execDDL(t, cfg, "DROP TABLE IF EXISTS t5_trunc_b")
			})
			for i := 0; i < 3; i++ {
				execDDL(t, cfg, "INSERT INTO t5_trunc_a VALUES (1)")
				execDDL(t, cfg, "INSERT INTO t5_trunc_b VALUES (1)")
			}

			if err := drv.TruncateTables(ctx, cfg.Database, []string{"t5_trunc_a", "t5_trunc_b"}); err != nil {
				t.Fatalf("TruncateTables() error: %v", err)
			}
			for _, table := range []string{"t5_trunc_a", "t5_trunc_b"} {
				if n := queryCount(t, cfg, cfg.Database, table); n != 0 {
					t.Errorf("count of %s after truncate = %d, want 0", table, n)
				}
			}

			// The first error aborts: the table after the missing one
			// keeps its rows.
			execDDL(t, cfg, "INSERT INTO t5_trunc_a VALUES (2)")
			err := drv.TruncateTables(ctx, cfg.Database, []string{"t5_missing", "t5_trunc_a"})
			if err == nil {
				t.Fatal("TruncateTables(missing first) error: expected error, got nil")
			}
			if !strings.Contains(err.Error(), "t5_missing") {
				t.Errorf("TruncateTables() error %q, want it to mention %q", err, "t5_missing")
			}
			if n := queryCount(t, cfg, cfg.Database, "t5_trunc_a"); n != 1 {
				t.Errorf("count of t5_trunc_a after aborted truncate = %d, want 1", n)
			}

			// An empty name fails before any statement.
			err = drv.TruncateTables(ctx, cfg.Database, []string{"t5_trunc_a", ""})
			if err == nil {
				t.Fatal("TruncateTables(empty name) error: expected error, got nil")
			}
			if n := queryCount(t, cfg, cfg.Database, "t5_trunc_a"); n != 1 {
				t.Errorf("count of t5_trunc_a after rejected call = %d, want 1", n)
			}
		})
	}
}

// TestDropTables removes two tables and verifies they disappear from
// ListTables, that dropping a missing table errors and empty names are
// rejected.
func TestDropTables(t *testing.T) {
	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			execDDL(t, cfg, "CREATE TABLE t5_drop_a (id INT NOT NULL)")
			execDDL(t, cfg, "CREATE TABLE t5_drop_b (id INT NOT NULL)")
			t.Cleanup(func() {
				execDDL(t, cfg, "DROP TABLE IF EXISTS t5_drop_a")
				execDDL(t, cfg, "DROP TABLE IF EXISTS t5_drop_b")
			})

			if err := drv.DropTables(ctx, cfg.Database, []string{"t5_drop_a", "t5_drop_b"}); err != nil {
				t.Fatalf("DropTables() error: %v", err)
			}
			tables, err := drv.ListTables(ctx, cfg.Database)
			if err != nil {
				t.Fatalf("ListTables() error: %v", err)
			}
			for _, tbl := range tables {
				if tbl.Name == "t5_drop_a" || tbl.Name == "t5_drop_b" {
					t.Errorf("table %q still exists after DropTables", tbl.Name)
				}
			}

			// No IF EXISTS: a missing table is an error.
			if err := drv.DropTables(ctx, cfg.Database, []string{"t5_drop_a"}); err == nil {
				t.Error("DropTables(missing) error: expected error, got nil")
			}
			if err := drv.DropTables(ctx, cfg.Database, []string{""}); err == nil {
				t.Error("DropTables(empty name) error: expected error, got nil")
			}
		})
	}
}

// TestDropViews removes a view and verifies it disappears from
// ListTables, that dropping a missing view errors and empty names are
// rejected.
func TestDropViews(t *testing.T) {
	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			execDDL(t, cfg, "CREATE TABLE t5_view_src (id INT NOT NULL)")
			execDDL(t, cfg, "CREATE VIEW v5_drop AS SELECT id FROM t5_view_src")
			t.Cleanup(func() {
				execDDL(t, cfg, "DROP VIEW IF EXISTS v5_drop")
				execDDL(t, cfg, "DROP TABLE IF EXISTS t5_view_src")
			})

			if err := drv.DropViews(ctx, cfg.Database, []string{"v5_drop"}); err != nil {
				t.Fatalf("DropViews() error: %v", err)
			}
			tables, err := drv.ListTables(ctx, cfg.Database)
			if err != nil {
				t.Fatalf("ListTables() error: %v", err)
			}
			for _, tbl := range tables {
				if tbl.Name == "v5_drop" {
					t.Error("view v5_drop still exists after DropViews")
				}
			}

			// No IF EXISTS: a missing view is an error.
			if err := drv.DropViews(ctx, cfg.Database, []string{"v5_drop"}); err == nil {
				t.Error("DropViews(missing) error: expected error, got nil")
			}
			if err := drv.DropViews(ctx, cfg.Database, []string{""}); err == nil {
				t.Error("DropViews(empty name) error: expected error, got nil")
			}
		})
	}
}

// TestWrites_CrossSchemaTargeting proves the write operations address
// the schema named by their db argument, not the DSN's default
// database: identical tables live in the login database and a second
// one, every operation runs with db = second schema, and the login
// database's table must come out untouched.
func TestWrites_CrossSchemaTargeting(t *testing.T) {
	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			execDDL(t, cfg, "CREATE DATABASE t5_other")
			t.Cleanup(func() {
				execDDL(t, cfg, "DROP DATABASE IF EXISTS t5_other")
				execDDL(t, cfg, "DROP TABLE IF EXISTS t5_x")
			})
			execDDL(t, cfg, "CREATE TABLE t5_x (id INT NOT NULL, name VARCHAR(32) NOT NULL)")
			execDDL(t, cfg, "CREATE TABLE t5_other.t5_x (id INT NOT NULL, name VARCHAR(32) NOT NULL)")
			execDDL(t, cfg, "INSERT INTO t5_x (id, name) VALUES (1, 'x')")
			execDDL(t, cfg, "INSERT INTO t5_other.t5_x (id, name) VALUES (1, 'x')")

			// TRUNCATE empties only the second schema's table.
			if err := drv.TruncateTables(ctx, "t5_other", []string{"t5_x"}); err != nil {
				t.Fatalf("TruncateTables(t5_other) error: %v", err)
			}
			if n := queryCount(t, cfg, "t5_other", "t5_x"); n != 0 {
				t.Errorf("count of t5_other.t5_x after truncate = %d, want 0", n)
			}
			if n := queryCount(t, cfg, cfg.Database, "t5_x"); n != 1 {
				t.Errorf("count of login-db t5_x after cross-schema truncate = %d, want 1", n)
			}

			// SaveTable adds a column only to the second schema's table.
			desired := driver.TableDef{
				Columns: []driver.ColumnDef{
					{Name: "id", OldName: "id", Type: "int"},
					{Name: "name", OldName: "name", Type: "varchar", Length: "32"},
					{Name: "extra", Type: "int", Nullable: true},
				},
			}
			if err := drv.SaveTable(ctx, "t5_other", "t5_x", desired); err != nil {
				t.Fatalf("SaveTable(t5_other) error: %v", err)
			}
			otherDef, err := drv.TableDef(ctx, "t5_other", "t5_x")
			if err != nil {
				t.Fatalf("TableDef(t5_other.t5_x) error: %v", err)
			}
			if len(otherDef.Columns) != 3 || otherDef.Columns[2].Name != "extra" {
				t.Errorf("t5_other.t5_x columns = %+v, want a new %q column", otherDef.Columns, "extra")
			}
			loginDef, err := drv.TableDef(ctx, cfg.Database, "t5_x")
			if err != nil {
				t.Fatalf("TableDef(login t5_x) error: %v", err)
			}
			if len(loginDef.Columns) != 2 {
				t.Errorf("login-db t5_x columns = %+v, want the original 2 (untouched)", loginDef.Columns)
			}

			// SaveIndexes adds the index only to the second schema's table.
			idxs := []driver.Index{
				{Name: "uq_name", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "name"}}},
			}
			if err := drv.SaveIndexes(ctx, "t5_other", "t5_x", idxs); err != nil {
				t.Fatalf("SaveIndexes(t5_other) error: %v", err)
			}
			otherDef, err = drv.TableDef(ctx, "t5_other", "t5_x")
			if err != nil {
				t.Fatalf("TableDef(t5_other.t5_x) after SaveIndexes error: %v", err)
			}
			assertIndexes(t, otherDef.Indexes, idxs)
			loginDef, err = drv.TableDef(ctx, cfg.Database, "t5_x")
			if err != nil {
				t.Fatalf("TableDef(login t5_x) after SaveIndexes error: %v", err)
			}
			if len(loginDef.Indexes) != 0 {
				t.Errorf("login-db t5_x indexes = %+v, want none (untouched)", loginDef.Indexes)
			}

			// DROP TABLE removes only the second schema's table.
			if err := drv.DropTables(ctx, "t5_other", []string{"t5_x"}); err != nil {
				t.Fatalf("DropTables(t5_other) error: %v", err)
			}
			if _, err := drv.TableDef(ctx, "t5_other", "t5_x"); err == nil || !strings.Contains(err.Error(), "not found") {
				t.Errorf("TableDef(t5_other.t5_x) after drop = %v, want a not-found error", err)
			}
			if _, err := drv.TableDef(ctx, cfg.Database, "t5_x"); err != nil {
				t.Errorf("TableDef(login t5_x) after cross-schema drop: %v, want the table intact", err)
			}
		})
	}
}
