//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// execDDL runs one DDL statement against the test server.
func execDDL(t *testing.T, cfg driver.ConnConfig, query string) {
	t.Helper()
	db, err := sql.Open("mysql", DSN(cfg, cfg.Password))
	if err != nil {
		t.Fatalf("open DDL connection: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func TestListTables(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE t_p2 (id INT PRIMARY KEY) COMMENT='p2 test table'")
			execDDL(t, cfg, "CREATE VIEW v_p2 AS SELECT 1")
			t.Cleanup(func() {
				execDDL(t, cfg, "DROP VIEW IF EXISTS v_p2")
				execDDL(t, cfg, "DROP TABLE IF EXISTS t_p2")
			})

			tables, err := drv.ListTables(context.Background(), cfg.Database)
			if err != nil {
				t.Fatalf("ListTables(%q) error: %v", cfg.Database, err)
			}

			byName := make(map[string]driver.TableInfo, len(tables))
			names := make([]string, 0, len(tables))
			for _, tbl := range tables {
				if tbl.SizeBytes < 0 {
					t.Errorf("table %q SizeBytes = %d, want >= 0", tbl.Name, tbl.SizeBytes)
				}
				byName[tbl.Name] = tbl
				names = append(names, tbl.Name)
			}
			if !slices.IsSorted(names) {
				t.Errorf("ListTables() not ordered by name: %v", names)
			}

			tbl, ok := byName["t_p2"]
			if !ok {
				t.Fatalf("ListTables() missing table %q, got %v", "t_p2", names)
			}
			if tbl.IsView {
				t.Error("t_p2 IsView = true, want false")
			}
			if tbl.Engine == "" {
				t.Error("t_p2 Engine is empty, want non-empty")
			}
			if tbl.Comment != "p2 test table" {
				t.Errorf("t_p2 Comment = %q, want %q", tbl.Comment, "p2 test table")
			}

			view, ok := byName["v_p2"]
			if !ok {
				t.Fatalf("ListTables() missing view %q, got %v", "v_p2", names)
			}
			if !view.IsView {
				t.Error("v_p2 IsView = false, want true")
			}
		})
	}
}

func TestListTables_EmptyName(t *testing.T) {
	cfg := startDB(t, "mysql:8.4")
	drv := connectOK(t, cfg)

	if _, err := drv.ListTables(context.Background(), ""); err == nil {
		t.Fatal(`ListTables("") error: expected error, got nil`)
	}
}
