//go:build integration

package postgres

import (
	"context"
	"github.com/dblama-dev/dblama/internal/driver"
	"strings"
	"testing"
)

func TestDatabaseOperations(t *testing.T) {
	ctx := context.Background()
	d := connectOK(t, startDB(t)).(*PostgresDriver)
	if err := d.CreateDatabase(ctx, "p5_\"db", "C"); err != nil {
		t.Fatal(err)
	}
	if err := d.RenameDatabase(ctx, "p5_\"db", "p5_renamed"); err != nil {
		t.Fatal(err)
	}
	dbs, err := d.ListDatabases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, db := range dbs {
		if db.Name == "p5_renamed" {
			found = true
			if db.Collation != "C" {
				t.Fatalf("collation %s", db.Collation)
			}
		}
	}
	if !found {
		t.Fatal("renamed database missing")
	}
	if err := d.DropDatabase(ctx, "p5_renamed"); err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{d.DropDatabase(ctx, testDatabase), d.RenameDatabase(ctx, testDatabase, "bad")} {
		if err == nil || !strings.Contains(err.Error(), "reconnect") {
			t.Fatalf("current DB needs reconnect guidance: %v", err)
		}
	}
	meta, err := d.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.ColumnTypes) == 0 || len(meta.Collations) == 0 || len(meta.Operators) == 0 {
		t.Fatalf("incomplete metadata: %+v", meta)
	}
	if err := d.CreateDatabase(ctx, "p5_bad", "C'; DROP DATABASE postgres; --"); err == nil {
		t.Fatal("invalid collation accepted")
	}
}

var _ driver.DatabaseManager = (*PostgresDriver)(nil)

func TestP4DatabaseDebts(t *testing.T) {
	ctx := context.Background()
	cfg := startDB(t)
	d := connectOK(t, cfg).(*PostgresDriver)
	if err := d.CreateSequence(ctx, "descending_seq", -3); err != nil {
		t.Fatal(err)
	}
	sequences, err := d.ListSequences(ctx, "public")
	if err != nil {
		t.Fatal(err)
	}
	if len(sequences) != 1 || sequences[0].Increment != -3 {
		t.Fatalf("negative increment lost: %+v", sequences)
	}
	desired := driver.TableDef{NewName: "commented", Columns: []driver.ColumnDef{{Name: "id", Type: "int", Comment: "column's note"}}}
	if err := d.SaveTable(ctx, cfg.Database, "", desired); err != nil {
		t.Fatal(err)
	}
	def, err := d.TableDef(ctx, cfg.Database, "commented")
	if err != nil {
		t.Fatal(err)
	}
	if def.Columns[0].Comment != "column's note" {
		t.Fatalf("column comment lost: %+v", def.Columns[0])
	}
	for _, q := range []string{"CREATE ROLE p5_reader LOGIN PASSWORD 'reader'", "CREATE DATABASE p5_private", "REVOKE CONNECT ON DATABASE p5_private FROM PUBLIC"} {
		if _, err := d.db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	cfg.User = "p5_reader"
	cfg.Password = "reader"
	limited := connectOK(t, cfg)
	dbs, err := limited.ListDatabases(ctx)
	if err != nil {
		t.Fatalf("restricted catalog listing failed: %v", err)
	}
	for _, db := range dbs {
		if db.Name == "p5_private" && db.SizeBytes != 0 {
			t.Fatalf("inaccessible database has known size %d", db.SizeBytes)
		}
	}
}
