//go:build integration

package mysql

import (
	"context"
	"github.com/dblama-dev/dblama/internal/driver"
	"testing"
)

func TestOriginalRowKeysAndCopy(t *testing.T) {
	for _, image := range testImages {
		t.Run(image.flavor, func(t *testing.T) {
			cfg := startDB(t, image.image)
			d := connectOK(t, cfg).(*MysqlDriver)
			ctx := context.Background()
			_, err := d.db.ExecContext(ctx, "CREATE TABLE original_keys (label VARCHAR(50), data VARBINARY(10), ts DATETIME(6), nullable TEXT, document JSON)")
			if err != nil {
				t.Fatal(err)
			}
			_, err = d.db.ExecContext(ctx, "INSERT INTO original_keys VALUES ('old',X'00FF','2026-10-28 12:34:56.123456',NULL,'{\"a\": 1}')")
			if err != nil {
				t.Fatal(err)
			}
			page, err := d.SelectRows(ctx, cfg.Database, "original_keys", driver.SelectQuery{})
			if err != nil {
				t.Fatal(err)
			}
			key := driver.PK{}
			for i, col := range page.Columns {
				key[col.Name] = page.Rows[0][i]
			}
			if err := d.UpdateRow(ctx, cfg.Database, "original_keys", key, driver.RowValues{"label": "new"}); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM original_keys WHERE label='new'").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("update using original typed row key matched %d rows", count)
			}
			key["label"] = "new"
			if err := d.CopyRows(ctx, cfg.Database, "original_keys", []driver.PK{key}); err != nil {
				t.Fatal(err)
			}
			if err := d.DeleteRows(ctx, cfg.Database, "original_keys", []driver.PK{key}); err != nil {
				t.Fatal(err)
			}
			if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM original_keys").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("original key deletion left %d rows", count)
			}
			_, err = d.db.ExecContext(ctx, "CREATE TABLE copy_rows (id INT AUTO_INCREMENT PRIMARY KEY, value INT, generated_value INT GENERATED ALWAYS AS (value*2) STORED)")
			if err != nil {
				t.Fatal(err)
			}
			_, err = d.db.ExecContext(ctx, "INSERT INTO copy_rows(value) VALUES (3)")
			if err != nil {
				t.Fatal(err)
			}
			if err := d.CopyRows(ctx, cfg.Database, "copy_rows", []driver.PK{{"id": 1}}); err != nil {
				t.Fatal(err)
			}
			if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM copy_rows WHERE generated_value=6").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 2 {
				t.Fatalf("copied rows %d", count)
			}
		})
	}
}
