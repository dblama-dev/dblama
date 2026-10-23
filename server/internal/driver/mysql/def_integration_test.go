//go:build integration

package mysql

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// defImageExpectations holds the per-image expectations that differ
// because MySQL 8 and MariaDB report information_schema differently
// (display widths, string default quoting, timestamp expression case,
// AUTO_INCREMENT of a fresh table).
type defImageExpectations struct {
	tableCollation  string // COLLATE used in the fixture DDL
	stringCollation string // collation reported for table-default string columns
	intLength       string // length parsed from the COLUMN_TYPE of INT UNSIGNED
	tsExpr          string // the CURRENT_TIMESTAMP expression as reported
	autoIncrement   int64  // TABLES.AUTO_INCREMENT of a fresh table
}

// assertTableDef compares got against want section by section so a
// failure names the mismatching column, index or foreign key.
func assertTableDef(t *testing.T, got, want driver.TableDef) {
	t.Helper()
	if got.NewName != want.NewName {
		t.Errorf("NewName = %q, want %q", got.NewName, want.NewName)
	}
	if len(got.Columns) != len(want.Columns) {
		t.Errorf("Columns: got %d, want %d:\ngot  %#v\nwant %#v", len(got.Columns), len(want.Columns), got.Columns, want.Columns)
	} else {
		for i := range want.Columns {
			if !reflect.DeepEqual(got.Columns[i], want.Columns[i]) {
				t.Errorf("column %d:\ngot  %#v\nwant %#v", i, got.Columns[i], want.Columns[i])
			}
		}
	}
	if len(got.Indexes) != len(want.Indexes) {
		t.Errorf("Indexes: got %d, want %d:\ngot  %#v\nwant %#v", len(got.Indexes), len(want.Indexes), got.Indexes, want.Indexes)
	} else {
		for i := range want.Indexes {
			if !reflect.DeepEqual(got.Indexes[i], want.Indexes[i]) {
				t.Errorf("index %d:\ngot  %#v\nwant %#v", i, got.Indexes[i], want.Indexes[i])
			}
		}
	}
	if len(got.ForeignKeys) != len(want.ForeignKeys) {
		t.Errorf("ForeignKeys: got %d, want %d:\ngot  %#v\nwant %#v", len(got.ForeignKeys), len(want.ForeignKeys), got.ForeignKeys, want.ForeignKeys)
	} else {
		for i := range want.ForeignKeys {
			if !reflect.DeepEqual(got.ForeignKeys[i], want.ForeignKeys[i]) {
				t.Errorf("foreign key %d:\ngot  %#v\nwant %#v", i, got.ForeignKeys[i], want.ForeignKeys[i])
			}
		}
	}
	if !reflect.DeepEqual(got.Options, want.Options) {
		t.Errorf("Options:\ngot  %#v\nwant %#v", got.Options, want.Options)
	}
}

func TestTableDef_RoundTrip(t *testing.T) {
	expect := map[string]defImageExpectations{
		"mysql:8.4": {
			tableCollation:  "utf8mb4_0900_ai_ci",
			stringCollation: "utf8mb4_0900_ai_ci",
			intLength:       "", // MySQL 8 dropped the display width
			tsExpr:          "CURRENT_TIMESTAMP",
			autoIncrement:   0, // MySQL reports NULL until the first insert
		},
		"mariadb:11.4": {
			tableCollation:  "utf8mb4_unicode_ci",
			stringCollation: "utf8mb4_unicode_ci",
			intLength:       "10", // MariaDB still reports int(10)
			tsExpr:          "current_timestamp()",
			autoIncrement:   1,
		},
	}

	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			exp, ok := expect[img.image]
			if !ok {
				t.Fatalf("no expectations for image %q", img.image)
			}
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)
			ctx := context.Background()

			execDDL(t, cfg, fmt.Sprintf(`
CREATE TABLE parent_t3 (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  label VARCHAR(32) NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=%s COMMENT='t3 parent'`, exp.tableCollation))
			execDDL(t, cfg, fmt.Sprintf(`
CREATE TABLE child_t3 (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(64) NOT NULL DEFAULT 'x',
  kind ENUM('a','b','c') NULL,
  price DECIMAL(10,2) NULL,
  size_code CHAR(2) COLLATE utf8mb4_bin NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  full_name VARCHAR(128) GENERATED ALWAYS AS (CONCAT(name,'!')) STORED,
  parent_id INT UNSIGNED NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_price (price),
  FULLTEXT KEY ft_name (name),
  KEY idx_kind (kind),
  CONSTRAINT fk_parent FOREIGN KEY (parent_id) REFERENCES parent_t3 (id)
    ON DELETE CASCADE ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=%s COMMENT='t3 child'`, exp.tableCollation))
			t.Cleanup(func() {
				execDDL(t, cfg, "DROP TABLE IF EXISTS child_t3")
				execDDL(t, cfg, "DROP TABLE IF EXISTS parent_t3")
			})

			got, err := drv.TableDef(ctx, cfg.Database, "child_t3")
			if err != nil {
				t.Fatalf("TableDef(child_t3) error: %v", err)
			}

			want := driver.TableDef{
				NewName: "child_t3",
				Columns: []driver.ColumnDef{
					{Name: "id", OldName: "id", Type: "int", Length: exp.intLength, Unsigned: true, AutoIncrement: true},
					{Name: "name", OldName: "name", Type: "varchar", Length: "64", Default: &driver.DefaultValue{Value: "x"}, Collation: exp.stringCollation},
					{Name: "kind", OldName: "kind", Type: "enum", Nullable: true, Collation: exp.stringCollation, EnumValues: []string{"a", "b", "c"}},
					{Name: "price", OldName: "price", Type: "decimal", Length: "10,2", Nullable: true},
					{Name: "size_code", OldName: "size_code", Type: "char", Length: "2", Nullable: true, Collation: "utf8mb4_bin"},
					{Name: "created_at", OldName: "created_at", Type: "timestamp", Default: &driver.DefaultValue{Value: exp.tsExpr, IsSQLExpression: true}, OnUpdate: exp.tsExpr},
					{Name: "full_name", OldName: "full_name", Type: "varchar", Length: "128", Nullable: true, Generated: "STORED", Collation: exp.stringCollation},
					{Name: "parent_id", OldName: "parent_id", Type: "int", Length: exp.intLength, Unsigned: true, Nullable: true},
				},
				Indexes: []driver.Index{
					// The FK needs its own index; both servers create it.
					{Name: "fk_parent", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "parent_id"}}},
					{Name: "ft_name", Type: "FULLTEXT", Columns: []driver.IndexColumn{{Name: "name"}}},
					{Name: "idx_kind", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "kind"}}},
					{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
					{Name: "uq_price", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "price"}}},
				},
				ForeignKeys: []driver.ForeignKey{
					{
						Name:       "fk_parent",
						Columns:    []string{"parent_id"},
						RefTable:   "parent_t3",
						RefColumns: []string{"id"},
						OnDelete:   "CASCADE",
						OnUpdate:   "RESTRICT",
					},
				},
				Options: driver.TableOptions{
					Engine:        "InnoDB",
					Collation:     exp.tableCollation,
					Comment:       "t3 child",
					AutoIncrement: exp.autoIncrement,
				},
			}
			assertTableDef(t, got, want)
		})
	}
}

func TestTableDef_NotFound(t *testing.T) {
	for _, img := range testImages {
		t.Run(img.image, func(t *testing.T) {
			cfg := startDB(t, img.image)
			drv := connectOK(t, cfg)

			_, err := drv.TableDef(context.Background(), cfg.Database, "missing_t3")
			if err == nil {
				t.Fatal("TableDef(missing_t3) error: expected error, got nil")
			}
			if !strings.Contains(err.Error(), "not found") {
				t.Errorf("TableDef(missing_t3) error = %q, want it to contain %q", err, "not found")
			}
		})
	}
}
