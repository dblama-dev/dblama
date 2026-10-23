//go:build integration

package mysql

import (
	"context"
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
					{Name: "name", Type: "varchar", Length: "64", Comment: "user name"},
					{Name: "kind", Type: "enum", Nullable: true, EnumValues: []string{"a", "b"}},
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
					{Name: "name", OldName: "name", Type: "varchar", Length: "64", Comment: "user name", Collation: exp.stringCollation},
					{Name: "kind", OldName: "kind", Type: "enum", Nullable: true, Collation: exp.stringCollation, EnumValues: []string{"a", "b"}},
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
