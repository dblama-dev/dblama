package mysql

import (
	"strings"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// fullCreateFixture is the Task 3 round-trip table rebuilt as a desired
// state: every column feature (unsigned, literal and expression defaults,
// enum, collation, comment, ON UPDATE) plus inline PRIMARY/UNIQUE and a
// foreign key.
func fullCreateFixture() driver.TableDef {
	return driver.TableDef{
		NewName: "child",
		Columns: []driver.ColumnDef{
			{Name: "id", Type: "int", Unsigned: true, AutoIncrement: true},
			{Name: "name", Type: "varchar", Length: "64", Default: &driver.DefaultValue{Value: "x"}, Comment: "user name"},
			{Name: "kind", Type: "enum", Nullable: true, EnumValues: []string{"a", "b", "c"}},
			{Name: "price", Type: "decimal", Length: "10,2", Nullable: true},
			{Name: "size_code", Type: "char", Length: "2", Nullable: true, Collation: "utf8mb4_bin"},
			{Name: "created_at", Type: "timestamp", Default: &driver.DefaultValue{Value: "CURRENT_TIMESTAMP", IsSQLExpression: true}, OnUpdate: "CURRENT_TIMESTAMP"},
			{Name: "parent_id", Type: "int", Unsigned: true, Nullable: true},
		},
		Indexes: []driver.Index{
			{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
			{Name: "uq_price", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "price"}}},
		},
		ForeignKeys: []driver.ForeignKey{
			{
				Name:       "fk_parent",
				Columns:    []string{"parent_id"},
				RefTable:   "parent",
				RefColumns: []string{"id"},
				OnDelete:   "CASCADE",
				OnUpdate:   "RESTRICT",
			},
		},
		Options: driver.TableOptions{
			Engine:        "InnoDB",
			Collation:     "utf8mb4_unicode_ci",
			Comment:       "child table",
			AutoIncrement: 42,
		},
	}
}

const wantFullCreateDDL = "CREATE TABLE `appdb`.`child` (\n" +
	"  `id` int unsigned NOT NULL AUTO_INCREMENT,\n" +
	"  `name` varchar(64) NOT NULL DEFAULT 'x' COMMENT 'user name',\n" +
	"  `kind` enum('a','b','c') NULL,\n" +
	"  `price` decimal(10,2) NULL,\n" +
	"  `size_code` char(2) NULL COLLATE `utf8mb4_bin`,\n" +
	"  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,\n" +
	"  `parent_id` int unsigned NULL,\n" +
	"  PRIMARY KEY (`id`),\n" +
	"  UNIQUE KEY `uq_price` (`price`),\n" +
	"  CONSTRAINT `fk_parent` FOREIGN KEY (`parent_id`) REFERENCES `parent` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT\n" +
	") ENGINE=InnoDB AUTO_INCREMENT=42 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='child table'"

func TestBuildCreateTable(t *testing.T) {
	got, err := buildCreateTable("appdb", fullCreateFixture())
	if err != nil {
		t.Fatalf("buildCreateTable() error: %v", err)
	}
	if got != wantFullCreateDDL {
		t.Errorf("buildCreateTable() DDL mismatch:\ngot:\n%s\nwant:\n%s", got, wantFullCreateDDL)
	}
}

func TestBuildAlterTable(t *testing.T) {
	desired := driver.TableDef{
		NewName: "child_new",
		Columns: []driver.ColumnDef{
			// CHANGE: rename + redefine.
			{Name: "title", OldName: "name", Type: "varchar", Length: "128", Default: &driver.DefaultValue{Value: "x"}, Comment: "renamed"},
			// DROP: existing column marked with an empty type.
			{Name: "kind", OldName: "kind", Nullable: true},
			// ADD FIRST / ADD AFTER.
			{Name: "size_code", Type: "char", Length: "2", Nullable: true, Collation: "utf8mb4_bin", Position: "FIRST"},
			{Name: "parent_id", Type: "int", Unsigned: true, Nullable: true, Position: "title"},
			// MODIFY: same name, new definition.
			{Name: "price", OldName: "price", Type: "decimal", Length: "12,4", Nullable: true},
		},
		Options: driver.TableOptions{Engine: "InnoDB", Comment: "new comment"},
	}

	got, err := buildAlterTable("appdb", "child", desired)
	if err != nil {
		t.Fatalf("buildAlterTable() error: %v", err)
	}
	want := []string{
		"ALTER TABLE `appdb`.`child` RENAME TO `appdb`.`child_new`",
		// Steps after the rename must address the new table name.
		"ALTER TABLE `appdb`.`child_new` CHANGE COLUMN `name` `title` varchar(128) NOT NULL DEFAULT 'x' COMMENT 'renamed'",
		"ALTER TABLE `appdb`.`child_new` DROP COLUMN `kind`",
		"ALTER TABLE `appdb`.`child_new` ADD COLUMN `size_code` char(2) NULL COLLATE `utf8mb4_bin` FIRST",
		"ALTER TABLE `appdb`.`child_new` ADD COLUMN `parent_id` int unsigned NULL AFTER `title`",
		"ALTER TABLE `appdb`.`child_new` MODIFY COLUMN `price` decimal(12,4) NULL",
		"ALTER TABLE `appdb`.`child_new` ENGINE=InnoDB COMMENT='new comment'",
	}
	if len(got) != len(want) {
		t.Fatalf("buildAlterTable() produced %d statements, want %d:\n%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d:\ngot:  %s\nwant: %s", i+1, got[i], want[i])
		}
	}
}

func TestBuildAlterTable_NoRenameWhenSameName(t *testing.T) {
	desired := driver.TableDef{
		NewName: "same_t4",
		Columns: []driver.ColumnDef{
			{Name: "id", OldName: "id", Type: "int"},
		},
		// Zero options must not produce an options step at all.
	}
	got, err := buildAlterTable("appdb", "same_t4", desired)
	if err != nil {
		t.Fatalf("buildAlterTable() error: %v", err)
	}
	want := []string{"ALTER TABLE `appdb`.`same_t4` MODIFY COLUMN `id` int NOT NULL"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("buildAlterTable() = %v, want %v", got, want)
	}
}

func TestBuildCreateTable_EnumEscape(t *testing.T) {
	desired := driver.TableDef{
		NewName: "odd`names",
		Columns: []driver.ColumnDef{
			{Name: "we`ird", Type: "enum", Nullable: true, EnumValues: []string{"it's", "plain"}},
			// A trailing backslash in a value would previously swallow
			// the closing quote; it must come back doubled.
			{Name: "bs", Type: "enum", Nullable: true, EnumValues: []string{"a", `a\b`, `path\`}},
		},
	}
	got, err := buildCreateTable("appdb", desired)
	if err != nil {
		t.Fatalf("buildCreateTable() error: %v", err)
	}
	want := "CREATE TABLE `appdb`.`odd``names` (\n" +
		"  `we``ird` enum('it''s','plain') NULL,\n" +
		"  `bs` enum('a','a\\\\b','path\\\\') NULL\n" +
		")"
	if got != want {
		t.Errorf("buildCreateTable() DDL mismatch:\ngot:  %s\nwant: %s", got, want)
	}
}

func TestBuildAlterTable_ColumnFragmentDefaults(t *testing.T) {
	desired := driver.TableDef{
		Columns: []driver.ColumnDef{
			// NULL literal default → DEFAULT NULL.
			{Name: "nullable_int", Type: "int", Nullable: true, Default: &driver.DefaultValue{Value: "NULL", IsSQLExpression: true}},
			// Expression default verbatim.
			{Name: "ts3", Type: "datetime", Default: &driver.DefaultValue{Value: "CURRENT_TIMESTAMP(3)", IsSQLExpression: true}},
			// ON UPDATE verbatim.
			{Name: "upd", Type: "timestamp", Default: &driver.DefaultValue{Value: "CURRENT_TIMESTAMP", IsSQLExpression: true}, OnUpdate: "CURRENT_TIMESTAMP(6)"},
			// Generated stored: expression in Default.Value, no DEFAULT and
			// no nullability clause (MariaDB rejects attributes after AS).
			{Name: "full", Type: "varchar", Length: "128", Nullable: true, Generated: "STORED", Default: &driver.DefaultValue{Value: "CONCAT(`upd`,'!')", IsSQLExpression: true}},
			// Generated virtual.
			{Name: "virt", Type: "int", Nullable: true, Generated: "VIRTUAL", Default: &driver.DefaultValue{Value: "1 + 1", IsSQLExpression: true}},
			// Literal default with a quote must be escaped.
			{Name: "lit", Type: "varchar", Length: "10", Nullable: true, Default: &driver.DefaultValue{Value: "it's"}},
		},
	}
	got, err := buildAlterTable("appdb", "t4frag", desired)
	if err != nil {
		t.Fatalf("buildAlterTable() error: %v", err)
	}
	want := []string{
		"ALTER TABLE `appdb`.`t4frag` ADD COLUMN `nullable_int` int NULL DEFAULT NULL",
		"ALTER TABLE `appdb`.`t4frag` ADD COLUMN `ts3` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP(3)",
		"ALTER TABLE `appdb`.`t4frag` ADD COLUMN `upd` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP(6)",
		"ALTER TABLE `appdb`.`t4frag` ADD COLUMN `full` varchar(128) AS (CONCAT(`upd`,'!')) STORED",
		"ALTER TABLE `appdb`.`t4frag` ADD COLUMN `virt` int AS (1 + 1) VIRTUAL",
		"ALTER TABLE `appdb`.`t4frag` ADD COLUMN `lit` varchar(10) NULL DEFAULT 'it''s'",
	}
	if len(got) != len(want) {
		t.Fatalf("buildAlterTable() produced %d statements, want %d:\n%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d:\ngot:  %s\nwant: %s", i+1, got[i], want[i])
		}
	}
}

func TestBuildCreateTable_Errors(t *testing.T) {
	t.Run("empty table name", func(t *testing.T) {
		_, err := buildCreateTable("appdb", driver.TableDef{Columns: []driver.ColumnDef{{Name: "id", Type: "int"}}})
		if err == nil {
			t.Fatal("expected error for empty table name, got nil")
		}
	})
	t.Run("no columns", func(t *testing.T) {
		_, err := buildCreateTable("appdb", driver.TableDef{NewName: "t4"})
		if err == nil {
			t.Fatal("expected error for empty column list, got nil")
		}
	})
	t.Run("enum without values", func(t *testing.T) {
		_, err := buildCreateTable("appdb", driver.TableDef{
			NewName: "t4",
			Columns: []driver.ColumnDef{{Name: "kind", Type: "enum"}},
		})
		if err == nil {
			t.Fatal("expected error for enum without values, got nil")
		}
	})
}

func TestBuildAlterTable_Errors(t *testing.T) {
	t.Run("empty table", func(t *testing.T) {
		_, err := buildAlterTable("appdb", "", driver.TableDef{Columns: []driver.ColumnDef{{Name: "id", OldName: "id", Type: "int"}}})
		if err == nil {
			t.Fatal("expected error for empty table name, got nil")
		}
	})
	t.Run("column with neither type nor old name", func(t *testing.T) {
		_, err := buildAlterTable("appdb", "t4", driver.TableDef{Columns: []driver.ColumnDef{{Name: "ghost"}}})
		if err == nil {
			t.Fatal("expected error for column without type and old name, got nil")
		}
	})
	t.Run("bad length", func(t *testing.T) {
		_, err := buildAlterTable("appdb", "t4", driver.TableDef{Columns: []driver.ColumnDef{
			{Name: "bad", Type: "varchar", Length: "64) ENGINE=InnoDB; DROP TABLE x"},
		}})
		if err == nil {
			t.Fatal("expected error for malicious length, got nil")
		}
	})
	t.Run("generated without expression", func(t *testing.T) {
		_, err := buildAlterTable("appdb", "t4", driver.TableDef{Columns: []driver.ColumnDef{
			{Name: "gen", Type: "int", Nullable: true, Generated: "STORED"},
		}})
		if err == nil {
			t.Fatal("expected error for generated column without expression, got nil")
		}
	})
	t.Run("generated not null", func(t *testing.T) {
		_, err := buildAlterTable("appdb", "t4", driver.TableDef{Columns: []driver.ColumnDef{
			{Name: "gen", Type: "int", Generated: "STORED", Default: &driver.DefaultValue{Value: "1", IsSQLExpression: true}},
		}})
		if err == nil {
			t.Fatal("expected error for NOT NULL generated column, got nil")
		}
	})
}

// sqlQuoteString is exercised indirectly above; this test pins the
// escaping helper directly since every string literal depends on it.
// Backslashes are doubled: neither test server sets NO_BACKSLASH_ESCAPES,
// so a trailing "\" would otherwise swallow the closing quote.
func TestSqlQuoteString(t *testing.T) {
	cases := map[string]string{
		"x":       "'x'",
		"":        "''",
		"it's":    "'it''s'",
		"a''b'c":  "'a''''b''c'",
		"back\\x": "'back\\\\x'", // literal backslash doubled
		"path\\":  "'path\\\\'",  // trailing backslash cannot swallow the quote
		"a\\'b":   "'a\\\\''b'",  // backslash and quote doubled independently
	}
	for in, want := range cases {
		if got := sqlQuoteString(in); got != want {
			t.Errorf("sqlQuoteString(%q) = %s, want %s", in, got, want)
		}
	}
	if !strings.Contains(sqlQuoteString("'"), "''") {
		t.Error("sqlQuoteString must double inner single quotes")
	}
}
