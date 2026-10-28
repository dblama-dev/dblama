//go:build integration

package postgres

import (
	"context"
	"github.com/hitslab/db-manager/server/internal/driver"
	"testing"
)

func TestOriginalRowKeysAndCopy(t *testing.T) {
	cfg := startDB(t)
	d := connectOK(t, cfg).(*PostgresDriver)
	ctx := context.Background()
	if _, err := d.db.ExecContext(ctx, `CREATE TABLE original_keys(label text,data bytea,ts timestamp(6),nullable text,document jsonb); INSERT INTO original_keys VALUES ('old',decode('00ff','hex'),'2026-10-28 12:34:56.123456',NULL,'{"a":1}')`); err != nil {
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
	if err := d.db.QueryRowContext(ctx, "SELECT count(*) FROM original_keys WHERE label='new'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("update matched %d rows err=%v", count, err)
	}
	key["label"] = "new"
	if err := d.CopyRows(ctx, cfg.Database, "original_keys", []driver.PK{key}); err != nil {
		t.Fatal(err)
	}
	if err := d.db.QueryRowContext(ctx, "SELECT count(*) FROM original_keys").Scan(&count); err != nil || count != 2 {
		t.Fatalf("copy made %d rows err=%v", count, err)
	}
	if err := d.DeleteRows(ctx, cfg.Database, "original_keys", []driver.PK{key}); err != nil {
		t.Fatal(err)
	}
	if err := d.db.QueryRowContext(ctx, "SELECT count(*) FROM original_keys").Scan(&count); err != nil || count != 0 {
		t.Fatalf("delete left %d rows err=%v", count, err)
	}
}
