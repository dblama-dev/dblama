//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
)

func TestLargeIntegerBrowseAndOriginalKey(t *testing.T) {
	cfg := startDB(t)
	d := connectOK(t, cfg).(*PostgresDriver)
	ctx := context.Background()
	if _, err := d.db.ExecContext(ctx, "CREATE TABLE exact_number (id BIGINT PRIMARY KEY, label TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.db.ExecContext(ctx, "INSERT INTO exact_number VALUES (9007199254740993,'before')"); err != nil {
		t.Fatal(err)
	}
	page, err := d.SelectRows(ctx, cfg.Database, "exact_number", driver.SelectQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if got := page.Rows[0][0]; got != "9007199254740993" {
		t.Fatalf("browse id %#v", got)
	}
	if err := d.UpdateRow(ctx, cfg.Database, "exact_number", driver.PK{"id": page.Rows[0][0]}, driver.RowValues{"label": "after"}); err != nil {
		t.Fatal(err)
	}
	var label string
	if err := d.db.QueryRowContext(ctx, "SELECT label FROM exact_number WHERE id=9007199254740993").Scan(&label); err != nil || label != "after" {
		t.Fatalf("label %q: %v", label, err)
	}
}
