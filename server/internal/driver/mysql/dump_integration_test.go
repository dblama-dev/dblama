//go:build integration

package mysql

import (
	"context"
	"strings"
	"testing"
)

func TestShowCreateTable(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE t_sc (id INT PRIMARY KEY, name VARCHAR(50)) COMMENT='show create test'")
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS t_sc") })

			create, err := drv.ShowCreateTable(context.Background(), cfg.Database, "t_sc")
			if err != nil {
				t.Fatalf("ShowCreateTable(%q, %q) error: %v", cfg.Database, "t_sc", err)
			}
			if !strings.Contains(create, "CREATE TABLE") {
				t.Errorf("ShowCreateTable() = %q, want it to contain %q", create, "CREATE TABLE")
			}
			if !strings.Contains(create, "t_sc") {
				t.Errorf("ShowCreateTable() = %q, want it to contain the table name %q", create, "t_sc")
			}
			if !strings.Contains(create, "show create test") {
				t.Errorf("ShowCreateTable() = %q, want it to contain the table comment", create)
			}
		})
	}
}

func TestShowCreateTable_NotFound(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			_, err := drv.ShowCreateTable(context.Background(), cfg.Database, "t_missing_sc")
			if err == nil {
				t.Fatal("ShowCreateTable(missing table) error: expected error, got nil")
			}
			if !strings.Contains(err.Error(), "not found") {
				t.Errorf("ShowCreateTable(missing table) error = %q, want it to contain %q", err, "not found")
			}
		})
	}
}

func TestShowCreateTable_EmptyName(t *testing.T) {
	cfg := startDB(t, "mysql:8.4")
	drv := connectOK(t, cfg)

	if _, err := drv.ShowCreateTable(context.Background(), "", "t_sc"); err == nil {
		t.Fatal("ShowCreateTable(empty db) error: expected error, got nil")
	}
	if _, err := drv.ShowCreateTable(context.Background(), cfg.Database, ""); err == nil {
		t.Fatal("ShowCreateTable(empty table) error: expected error, got nil")
	}
}
