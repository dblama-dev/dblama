//go:build integration

package mysql

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// driverConnConfig keeps the seed helper signature short.
type driverConnConfig = driver.ConnConfig

// seedBrowseTable creates the browsing playground table browse_t6 in the
// container's default schema and fills it with seven rows; the cleanup
// drops it. The rows alternate NULL and empty-string values so the tests
// can tell them apart.
//
//	id  a       b       bin
//	1   alpha   x       0x01AA
//	2   beta    ''      NULL
//	3   gamma   NULL    0x02BB
//	4   alpha   y       NULL
//	5   delta   ''      0x03CC
//	6   alpha   NULL    NULL
//	7   NULL    z       0x04DD
func seedBrowseTable(t *testing.T, cfg driverConnConfig) {
	t.Helper()
	execDDL(t, cfg, "CREATE TABLE browse_t6 (id INT PRIMARY KEY, a VARCHAR(50), b VARCHAR(50), bin VARBINARY(16))")
	seedRows := []string{
		"(1, 'alpha', 'x', 0x01AA)",
		"(2, 'beta', '', NULL)",
		"(3, 'gamma', NULL, 0x02BB)",
		"(4, 'alpha', 'y', NULL)",
		"(5, 'delta', '', 0x03CC)",
		"(6, 'alpha', NULL, NULL)",
		"(7, NULL, 'z', 0x04DD)",
	}
	execDDL(t, cfg, "INSERT INTO browse_t6 (id, a, b, bin) VALUES "+strings.Join(seedRows, ", "))
	t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS browse_t6") })
}

func TestSelectRows_Pagination(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)
			seedBrowseTable(t, cfg)

			page, err := drv.SelectRows(context.Background(), cfg.Database, "browse_t6", sqlgen.SelectQuery{
				Sorts:  []sqlgen.Sort{{Column: "id", Desc: true}},
				Limit:  3,
				Offset: 2,
			})
			if err != nil {
				t.Fatalf("SelectRows() error: %v", err)
			}

			if page.TotalRows != 7 {
				t.Errorf("TotalRows = %d, want 7", page.TotalRows)
			}
			if len(page.Rows) != 3 {
				t.Fatalf("len(Rows) = %d, want 3", len(page.Rows))
			}
			// Ordered by id DESC starting at offset 2: ids 5, 4, 3.
			wantIDs := []any{int64(5), int64(4), int64(3)}
			for i, row := range page.Rows {
				if got := row[0]; got != wantIDs[i] {
					t.Errorf("row %d id = %v (%T), want %v", i, got, got, wantIDs[i])
				}
			}
			// Columns come back in ordinal order with base types.
			wantNames := []string{"id", "a", "b", "bin"}
			gotNames := make([]string, len(page.Columns))
			for i, c := range page.Columns {
				gotNames[i] = c.Name
				if c.Type == "" {
					t.Errorf("column %q Type is empty", c.Name)
				}
			}
			if !slices.Equal(gotNames, wantNames) {
				t.Errorf("Columns = %v, want %v", gotNames, wantNames)
			}
			// id is the primary key (implicitly NOT NULL); a, b, bin
			// are declared without NOT NULL and are nullable.
			if page.Columns[0].Nullable {
				t.Error("column id Nullable = true, want false")
			}
			if !page.Columns[2].Nullable {
				t.Error("column b Nullable = false, want true")
			}
		})
	}
}

func TestSelectRows_Filters(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)
			seedBrowseTable(t, cfg)
			ctx := context.Background()

			cases := []struct {
				name    string
				filter  sqlgen.Filter
				wantIDs []int64
			}{
				{"eq", sqlgen.Filter{Column: "a", Operator: "=", Value: "beta"}, []int64{2}},
				{"like", sqlgen.Filter{Column: "a", Operator: "LIKE", Value: "alp%"}, []int64{1, 4, 6}},
				{"isNull", sqlgen.Filter{Column: "b", Operator: "IS NULL"}, []int64{3, 6}},
				{"in", sqlgen.Filter{Column: "id", Operator: "IN", Value: "1,3,5"}, []int64{1, 3, 5}},
			}
			for _, f := range cases {
				t.Run(f.name, func(t *testing.T) {
					page, err := drv.SelectRows(ctx, cfg.Database, "browse_t6", sqlgen.SelectQuery{
						Filters: []sqlgen.Filter{f.filter},
						Sorts:   []sqlgen.Sort{{Column: "id"}},
					})
					if err != nil {
						t.Fatalf("SelectRows() error: %v", err)
					}
					gotIDs := make([]int64, len(page.Rows))
					for i, row := range page.Rows {
						id, ok := row[0].(int64)
						if !ok {
							t.Fatalf("row %d id = %v (%T), want int64", i, row[0], row[0])
						}
						gotIDs[i] = id
					}
					if !slices.Equal(gotIDs, f.wantIDs) {
						t.Errorf("ids = %v, want %v (TotalRows=%d)", gotIDs, f.wantIDs, page.TotalRows)
					}
					if page.TotalRows != int64(len(f.wantIDs)) {
						t.Errorf("TotalRows = %d, want %d", page.TotalRows, len(f.wantIDs))
					}
				})
			}
		})
	}
}

func TestSelectRows_NullVsEmpty(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)
			seedBrowseTable(t, cfg)

			// Row 2 has b='' and row 3 has b=NULL: the two must scan to
			// distinct values ("" vs nil).
			page, err := drv.SelectRows(context.Background(), cfg.Database, "browse_t6", sqlgen.SelectQuery{
				Filters: []sqlgen.Filter{{Column: "id", Operator: "IN", Value: "2,3"}},
				Sorts:   []sqlgen.Sort{{Column: "id"}},
			})
			if err != nil {
				t.Fatalf("SelectRows() error: %v", err)
			}
			if len(page.Rows) != 2 {
				t.Fatalf("len(Rows) = %d, want 2", len(page.Rows))
			}
			if got := page.Rows[0][2]; got != "" {
				t.Errorf("row 2 b = %#v (%T), want \"\"", got, got)
			}
			if got := page.Rows[1][2]; got != nil {
				t.Errorf("row 3 b = %#v (%T), want nil", got, got)
			}
		})
	}
}

func TestSelectRows_InjectionValue(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)
			seedBrowseTable(t, cfg)
			ctx := context.Background()

			page, err := drv.SelectRows(ctx, cfg.Database, "browse_t6", sqlgen.SelectQuery{
				Filters: []sqlgen.Filter{{Column: "a", Operator: "=", Value: "'; DROP TABLE browse_t6; --"}},
			})
			if err != nil {
				t.Fatalf("SelectRows() error: %v", err)
			}
			if len(page.Rows) != 0 {
				t.Errorf("len(Rows) = %d, want 0", len(page.Rows))
			}
			if page.TotalRows != 0 {
				t.Errorf("TotalRows = %d, want 0", page.TotalRows)
			}

			// The table must still be there with all seven rows.
			after, err := drv.SelectRows(ctx, cfg.Database, "browse_t6", sqlgen.SelectQuery{})
			if err != nil {
				t.Fatalf("SelectRows() after injection attempt: %v", err)
			}
			if after.TotalRows != 7 {
				t.Errorf("TotalRows after injection attempt = %d, want 7 (table was damaged)", after.TotalRows)
			}
		})
	}
}

func TestSelectRows_ColumnSubset(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)
			seedBrowseTable(t, cfg)

			page, err := drv.SelectRows(context.Background(), cfg.Database, "browse_t6", sqlgen.SelectQuery{
				Columns: []string{"b"},
			})
			if err != nil {
				t.Fatalf("SelectRows() error: %v", err)
			}
			if len(page.Columns) != 1 || page.Columns[0].Name != "b" {
				t.Fatalf("Columns = %+v, want only b", page.Columns)
			}
			if page.Columns[0].Type != "varchar" {
				t.Errorf("b Type = %q, want varchar", page.Columns[0].Type)
			}
			if len(page.Rows) != 7 {
				t.Fatalf("len(Rows) = %d, want 7", len(page.Rows))
			}
			for i, row := range page.Rows {
				if len(row) != 1 {
					t.Errorf("row %d has %d values, want 1", i, len(row))
				}
			}
		})
	}
}

func TestPrimaryKey(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE pk_t6 (id INT, part INT, v INT, PRIMARY KEY (part, id))")
			execDDL(t, cfg, "CREATE TABLE nopk_t6 (id INT)")
			t.Cleanup(func() {
				execDDL(t, cfg, "DROP TABLE IF EXISTS pk_t6")
				execDDL(t, cfg, "DROP TABLE IF EXISTS nopk_t6")
			})

			pk, err := drv.PrimaryKey(context.Background(), cfg.Database, "pk_t6")
			if err != nil {
				t.Fatalf("PrimaryKey(pk_t6) error: %v", err)
			}
			// SEQ_IN_INDEX order, not declaration order.
			if !slices.Equal(pk, []string{"part", "id"}) {
				t.Errorf("PrimaryKey(pk_t6) = %v, want [part id]", pk)
			}

			none, err := drv.PrimaryKey(context.Background(), cfg.Database, "nopk_t6")
			if err != nil {
				t.Fatalf("PrimaryKey(nopk_t6) error: %v", err)
			}
			if len(none) != 0 {
				t.Errorf("PrimaryKey(nopk_t6) = %v, want empty", none)
			}
		})
	}
}

func TestSelectRows_BlobHex(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)
			seedBrowseTable(t, cfg)

			page, err := drv.SelectRows(context.Background(), cfg.Database, "browse_t6", sqlgen.SelectQuery{
				Filters: []sqlgen.Filter{{Column: "id", Operator: "=", Value: "1"}},
			})
			if err != nil {
				t.Fatalf("SelectRows() error: %v", err)
			}
			got, ok := page.Rows[0][3].(string)
			if !ok || !strings.HasPrefix(got, "0x") {
				t.Fatalf("bin = %#v, want a 0x-prefixed hex string", page.Rows[0][3])
			}
			if !slices.Contains([]string{"0x01aa", "0x01AA"}, got) {
				t.Errorf("bin = %q, want hex of 0x01AA", got)
			}

			// NULL in the binary column stays nil.
			page, err = drv.SelectRows(context.Background(), cfg.Database, "browse_t6", sqlgen.SelectQuery{
				Filters: []sqlgen.Filter{{Column: "id", Operator: "=", Value: "2"}},
			})
			if err != nil {
				t.Fatalf("SelectRows() error: %v", err)
			}
			if got := page.Rows[0][3]; got != nil {
				t.Errorf("bin (NULL) = %#v, want nil", got)
			}
		})
	}
}

func TestSelectRows_EmptyNames(t *testing.T) {
	cfg := startDB(t, "mysql:8.4")
	drv := connectOK(t, cfg)
	ctx := context.Background()

	if _, err := drv.SelectRows(ctx, "", "t", sqlgen.SelectQuery{}); err == nil {
		t.Error("SelectRows(empty db) error: expected error, got nil")
	}
	if _, err := drv.SelectRows(ctx, "db", "", sqlgen.SelectQuery{}); err == nil {
		t.Error("SelectRows(empty table) error: expected error, got nil")
	}
	if _, err := drv.PrimaryKey(ctx, "", "t"); err == nil {
		t.Error("PrimaryKey(empty db) error: expected error, got nil")
	}
	if _, err := drv.PrimaryKey(ctx, "db", ""); err == nil {
		t.Error("PrimaryKey(empty table) error: expected error, got nil")
	}
}
