//go:build integration

package mysql

import (
	"context"
	"github.com/hitslab/db-manager/server/internal/driver"
	"strings"
	"testing"
)

func TestDatabaseOperations(t *testing.T) {
	for _, image := range testImages {
		t.Run(image.image, func(t *testing.T) {
			ctx := context.Background()
			d := connectOK(t, startDB(t, image.image)).(*MysqlDriver)
			if err := d.CreateDatabase(ctx, "p5_src", "utf8mb4_unicode_ci"); err != nil {
				t.Fatal(err)
			}
			if err := d.CreateDatabase(ctx, "p5_latin", "latin1_swedish_ci"); err != nil {
				t.Fatal(err)
			}
			var explicitCharset string
			if err := d.db.QueryRowContext(ctx, "SELECT DEFAULT_CHARACTER_SET_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = 'p5_latin'").Scan(&explicitCharset); err != nil || explicitCharset != "latin1" {
				t.Fatalf("explicit collation character set %q: %v", explicitCharset, err)
			}
			exec := func(q string) {
				t.Helper()
				if _, err := d.db.ExecContext(ctx, q); err != nil {
					t.Fatal(err)
				}
			}
			exec("CREATE TABLE p5_src.items (id INT PRIMARY KEY AUTO_INCREMENT, n INT, doubled INT GENERATED ALWAYS AS (n*2) STORED, UNIQUE KEY uq(n))")
			exec("INSERT INTO p5_src.items(n) VALUES (3),(7)")
			exec("CREATE TABLE p5_src.zero_id (id INT PRIMARY KEY AUTO_INCREMENT, n INT)")
			exec("SET SESSION sql_mode = CONCAT(@@sql_mode, ',NO_AUTO_VALUE_ON_ZERO')")
			exec("INSERT INTO p5_src.zero_id(id,n) VALUES (0,11)")
			exec("SET SESSION sql_mode = REPLACE(@@sql_mode, 'NO_AUTO_VALUE_ON_ZERO', '')")
			exec("ALTER TABLE p5_src.zero_id AUTO_INCREMENT=9000")

			if err := d.RenameDatabase(ctx, "p5_src", "p5_renamed"); err != nil {
				t.Fatal(err)
			}
			var n int
			if err := d.db.QueryRowContext(ctx, "SELECT SUM(doubled) FROM p5_renamed.items").Scan(&n); err != nil || n != 20 {
				t.Fatalf("renamed data %d: %v", n, err)
			}

			var zero int
			if err := d.db.QueryRowContext(ctx, "SELECT id FROM p5_renamed.zero_id WHERE n=11").Scan(&zero); err != nil || zero != 0 {
				t.Fatalf("zero autoincrement ID changed to %d: %v", zero, err)
			}
			exec("INSERT INTO p5_renamed.zero_id(n) VALUES (22)")
			if err := d.db.QueryRowContext(ctx, "SELECT id FROM p5_renamed.zero_id WHERE n=22").Scan(&zero); err != nil || zero != 9000 {
				t.Fatalf("autoincrement counter changed to %d: %v", zero, err)
			}
			if _, err := d.db.ExecContext(ctx, "SELECT * FROM p5_src.items"); err == nil {
				t.Fatal("source still exists after success")
			}
			if err := d.CreateDatabase(ctx, "p5_target", ""); err != nil {
				t.Fatal(err)
			}
			if err := d.CopyTables(ctx, "p5_renamed", []string{"items"}, "p5_target"); err != nil {
				t.Fatal(err)
			}
			if err := d.CopyTables(ctx, "p5_renamed", []string{"items"}, "p5_target"); err == nil {
				t.Fatal("existing target overwritten")
			}
			if err := d.CreateDatabase(ctx, "p5_moved", ""); err != nil {
				t.Fatal(err)
			}
			if err := d.MoveTables(ctx, "p5_target", []string{"items"}, "p5_moved"); err != nil {
				t.Fatal(err)
			}
			if err := d.RenameDatabase(ctx, "p5_renamed", "p5_moved"); err == nil {
				t.Fatal("rename to existing DB accepted")
			}
			if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM p5_renamed.items").Scan(&n); err != nil || n != 2 {
				t.Fatalf("source lost on failure: %d %v", n, err)
			}
			exec("CREATE VIEW p5_renamed.v AS SELECT n FROM p5_renamed.items")
			if err := d.RenameDatabase(ctx, "p5_renamed", "p5_refused"); err == nil {
				t.Fatal("rename silently lost view")
			}
			exec("CREATE TABLE p5_renamed.child (id INT PRIMARY KEY, item_id INT, FOREIGN KEY(item_id) REFERENCES p5_renamed.items(id))")
			if err := d.CopyTables(ctx, "p5_renamed", []string{"child"}, "p5_target"); err == nil {
				t.Fatal("copy silently lost foreign key")
			}
			meta, err := d.Metadata(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(meta.ColumnTypes) == 0 || len(meta.Collations) == 0 || len(meta.Operators) == 0 {
				t.Fatalf("missing metadata: %+v", meta)
			}
			if err := d.CreateDatabase(ctx, "p5_bad", "x; DROP DATABASE p5_moved"); err == nil {
				t.Fatal("unknown collation accepted")
			}
			if err := d.CopyTables(ctx, "p5_moved", []string{"items", "missing"}, "p5_target"); err == nil {
				t.Fatal("missing transfer table accepted")
			}
			if _, err := d.db.ExecContext(ctx, "SELECT * FROM p5_target.items"); err == nil {
				t.Fatal("partial copy left behind")
			}
			for _, name := range []string{"p5_renamed", "p5_target", "p5_moved", "p5_latin"} {
				if err := d.DropDatabase(ctx, name); err != nil {
					t.Fatal(err)
				}
			}
			dbs, err := d.ListDatabases(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, db := range dbs {
				if strings.HasPrefix(db.Name, "p5_") {
					t.Fatalf("unexpected database after cleanup: %s", db.Name)
				}
			}
		})
	}
}

func TestDatabaseRenamePreservesServerDefaultCollation(t *testing.T) {
	for _, image := range testImages {
		t.Run(image.image, func(t *testing.T) {
			ctx := context.Background()
			d := connectOK(t, startDB(t, image.image)).(*MysqlDriver)
			if err := d.CreateDatabase(ctx, "default_collation_source", ""); err != nil {
				t.Fatal(err)
			}
			var charset, collation string
			if err := d.db.QueryRowContext(ctx, "SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = 'default_collation_source'").Scan(&charset, &collation); err != nil {
				t.Fatal(err)
			}
			if _, err := d.db.ExecContext(ctx, "CREATE TABLE default_collation_source.items (id INT PRIMARY KEY, label VARCHAR(80))"); err != nil {
				t.Fatal(err)
			}
			if _, err := d.db.ExecContext(ctx, "INSERT INTO default_collation_source.items VALUES (7, 'preserved')"); err != nil {
				t.Fatal(err)
			}
			if err := d.RenameDatabase(ctx, "default_collation_source", "default_collation_renamed"); err != nil {
				t.Fatalf("rename database with server default %s/%s: %v", charset, collation, err)
			}
			var gotCharset, gotCollation, label string
			if err := d.db.QueryRowContext(ctx, "SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = 'default_collation_renamed'").Scan(&gotCharset, &gotCollation); err != nil {
				t.Fatal(err)
			}
			if gotCharset != charset || gotCollation != collation {
				t.Fatalf("database defaults changed from %s/%s to %s/%s", charset, collation, gotCharset, gotCollation)
			}
			if err := d.db.QueryRowContext(ctx, "SELECT label FROM default_collation_renamed.items WHERE id = 7").Scan(&label); err != nil || label != "preserved" {
				t.Fatalf("renamed row %q: %v", label, err)
			}
			var remaining int
			if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = 'default_collation_source'").Scan(&remaining); err != nil || remaining != 0 {
				t.Fatalf("source remains after rename: %d %v", remaining, err)
			}
		})
	}
}

var _ driver.DatabaseManager = (*MysqlDriver)(nil)
var _ driver.TableTransfer = (*MysqlDriver)(nil)
