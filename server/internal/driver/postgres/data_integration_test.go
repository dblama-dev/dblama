//go:build integration

package postgres

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// dataSchemaName is the fresh schema every data test works in — a name
// with a space, so every generated statement must carry double-quoted
// schema-qualified identifiers.
const dataSchemaName = "data schema"

// useDataSchema drops and recreates the data schema and selects it as
// the driver's current schema.
func useDataSchema(t *testing.T, drv driver.Driver) {
	t.Helper()
	pg := drv.(*PostgresDriver) //nolint:forcetypeassert // test helper: connectOK only builds PostgresDriver
	ctx := context.Background()
	for _, stmt := range []string{
		`DROP SCHEMA IF EXISTS "data schema" CASCADE`,
		`CREATE SCHEMA "data schema"`,
	} {
		if _, err := pg.db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("data schema statement %q: %v", stmt, err)
		}
	}
	if err := schemaAware(t, drv).SetSchema(dataSchemaName); err != nil {
		t.Fatalf("SetSchema(%q) error: %v", dataSchemaName, err)
	}
}

// execDataDDL runs one fixture statement through the driver's own pool.
func execDataDDL(t *testing.T, drv driver.Driver, stmt string) {
	t.Helper()
	pg := drv.(*PostgresDriver) //nolint:forcetypeassert // test helper: connectOK only builds PostgresDriver
	if _, err := pg.db.ExecContext(context.Background(), stmt); err != nil {
		t.Fatalf("fixture statement %q: %v", stmt, err)
	}
}

// seedBrowseTable creates the browsing playground table browse_t4 in the
// data schema and fills it with seven rows; the cleanup drops the whole
// schema (useDataSchema drops it on the next run anyway). The rows
// alternate NULL and empty-string values so the tests can tell them
// apart, and row 4 carries a mixed-case 'Alpha' to pin that LIKE stays
// exact-case on PostgreSQL (no ILIKE rewriting).
//
//	id  a       b       bin
//	1   alpha   x       0x01aa
//	2   beta    ''      NULL
//	3   gamma   NULL    0x02bb
//	4   Alpha   y       NULL
//	5   delta   ''      0x03cc
//	6   alpha   NULL    NULL
//	7   NULL    z       0x04dd
func seedBrowseTable(t *testing.T, drv driver.Driver) {
	t.Helper()
	execDataDDL(t, drv, `CREATE TABLE "data schema".browse_t4 (id integer PRIMARY KEY, a varchar(50), b varchar(50), bin bytea)`)
	seedRows := []string{
		`(1, 'alpha', 'x', '\x01AA'::bytea)`,
		`(2, 'beta', '', NULL)`,
		`(3, 'gamma', NULL, '\x02BB'::bytea)`,
		`(4, 'Alpha', 'y', NULL)`,
		`(5, 'delta', '', '\x03CC'::bytea)`,
		`(6, 'alpha', NULL, NULL)`,
		`(7, NULL, 'z', '\x04DD'::bytea)`,
	}
	execDataDDL(t, drv, "INSERT INTO \"data schema\".browse_t4 (id, a, b, bin) VALUES "+strings.Join(seedRows, ", "))
}

// seedWriteTable creates the row-write playground table write_t4 (id
// serial PRIMARY KEY, name varchar(50), note varchar(50)). note is
// nullable with no default, so NULL and '' can be told apart after a
// write; the serial id exercises the RETURNING-based LastInsertID.
func seedWriteTable(t *testing.T, drv driver.Driver) {
	t.Helper()
	execDataDDL(t, drv, `CREATE TABLE "data schema".write_t4 (id SERIAL PRIMARY KEY, name varchar(50), note varchar(50))`)
}

// readAll returns the full content of table ordered by the given column
// — the verification helper of the write tests.
func readAll(t *testing.T, drv driver.Driver, table, order string) driver.Page {
	t.Helper()
	page, err := drv.SelectRows(context.Background(), testDatabase, table, sqlgen.SelectQuery{
		Sorts: []sqlgen.Sort{{Column: order}},
	})
	if err != nil {
		t.Fatalf("SelectRows(%s) after write: %v", table, err)
	}
	return page
}

func TestSelectRows_Pagination(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useDataSchema(t, drv)
	seedBrowseTable(t, drv)

	page, err := drv.SelectRows(context.Background(), testDatabase, "browse_t4", sqlgen.SelectQuery{
		Sorts:  []sqlgen.Sort{{Column: "id", Desc: true}},
		Limit:  3,
		Offset: 2,
	})
	if err != nil {
		t.Fatalf("SelectRows() error = %v", err)
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
	// Columns come back in ordinal order with base type labels.
	wantNames := []string{"id", "a", "b", "bin"}
	gotNames := make([]string, len(page.Columns))
	for i, c := range page.Columns {
		gotNames[i] = c.Name
	}
	if !slices.Equal(gotNames, wantNames) {
		t.Errorf("Columns = %v, want %v", gotNames, wantNames)
	}
	wantTypes := []string{"integer", "varchar", "varchar", "bytea"}
	for i, c := range page.Columns {
		if c.Type != wantTypes[i] {
			t.Errorf("column %q Type = %q, want %q", c.Name, c.Type, wantTypes[i])
		}
	}
	// id is the primary key (NOT NULL); a, b, bin are nullable.
	if page.Columns[0].Nullable {
		t.Error("column id Nullable = true, want false")
	}
	if !page.Columns[2].Nullable {
		t.Error("column b Nullable = false, want true")
	}
}

func TestSelectRows_Filters(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useDataSchema(t, drv)
	seedBrowseTable(t, drv)
	ctx := context.Background()

	cases := []struct {
		name    string
		filter  sqlgen.Filter
		wantIDs []int64
	}{
		{"eq", sqlgen.Filter{Column: "a", Operator: "=", Value: "beta"}, []int64{2}},
		// PIN: LIKE is exact-case on PostgreSQL (the operator whitelist
		// says LIKE, not ILIKE) — row 4's 'Alpha' must not match 'alp%'.
		{"like exact case", sqlgen.Filter{Column: "a", Operator: "LIKE", Value: "alp%"}, []int64{1, 6}},
		{"isNull", sqlgen.Filter{Column: "b", Operator: "IS NULL"}, []int64{3, 6}},
		{"in", sqlgen.Filter{Column: "id", Operator: "IN", Value: "1,3,5"}, []int64{1, 3, 5}},
	}
	for _, f := range cases {
		t.Run(f.name, func(t *testing.T) {
			page, err := drv.SelectRows(ctx, testDatabase, "browse_t4", sqlgen.SelectQuery{
				Filters: []sqlgen.Filter{f.filter},
				Sorts:   []sqlgen.Sort{{Column: "id"}},
			})
			if err != nil {
				t.Fatalf("SelectRows() error = %v", err)
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
}

func TestSelectRows_NullVsEmpty(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useDataSchema(t, drv)
	seedBrowseTable(t, drv)

	// Row 2 has b='' and row 3 has b=NULL: the two must scan to distinct
	// values ("" vs nil).
	page, err := drv.SelectRows(context.Background(), testDatabase, "browse_t4", sqlgen.SelectQuery{
		Filters: []sqlgen.Filter{{Column: "id", Operator: "IN", Value: "2,3"}},
		Sorts:   []sqlgen.Sort{{Column: "id"}},
	})
	if err != nil {
		t.Fatalf("SelectRows() error = %v", err)
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
}

func TestSelectRows_InjectionValue(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useDataSchema(t, drv)
	seedBrowseTable(t, drv)
	ctx := context.Background()

	page, err := drv.SelectRows(ctx, testDatabase, "browse_t4", sqlgen.SelectQuery{
		Filters: []sqlgen.Filter{{Column: "a", Operator: "=", Value: "'; DROP TABLE browse_t4; --"}},
	})
	if err != nil {
		t.Fatalf("SelectRows() error = %v", err)
	}
	if len(page.Rows) != 0 {
		t.Errorf("len(Rows) = %d, want 0", len(page.Rows))
	}
	if page.TotalRows != 0 {
		t.Errorf("TotalRows = %d, want 0", page.TotalRows)
	}

	// The table must still be there with all seven rows.
	after, err := drv.SelectRows(ctx, testDatabase, "browse_t4", sqlgen.SelectQuery{})
	if err != nil {
		t.Fatalf("SelectRows() after injection attempt: %v", err)
	}
	if after.TotalRows != 7 {
		t.Errorf("TotalRows after injection attempt = %d, want 7 (table was damaged)", after.TotalRows)
	}
}

// TestSelectRows_ArrayAndJsonb pins the PostgreSQL value rendering: text[]
// and int[] arrays arrive as native slices and render as JSON strings,
// jsonb as its text form, bytea as "0x" hex (Go hex is lowercase),
// numeric and uuid as strings (pgx's driver.Valuer forms), timestamptz
// as RFC3339 and NULLs as nil.
func TestSelectRows_ArrayAndJsonb(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useDataSchema(t, drv)
	execDataDDL(t, drv, `CREATE TABLE "data schema".rich_t4 (
		id integer PRIMARY KEY,
		tags text[],
		nums integer[],
		doc jsonb,
		blob bytea,
		price numeric(10,2),
		uid uuid,
		ts timestamptz,
		flag boolean)`)
	execDataDDL(t, drv, `INSERT INTO "data schema".rich_t4 VALUES
		(1, ARRAY['a','b'], ARRAY[1,2], '{"a": 1}'::jsonb, '\xFF00'::bytea,
		 12.34, '123e4567-e89b-12d3-a456-426614174000'::uuid,
		 '2026-10-24 10:30:00+00'::timestamptz, true),
		(2, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL)`)

	page, err := drv.SelectRows(context.Background(), testDatabase, "rich_t4", sqlgen.SelectQuery{
		Sorts: []sqlgen.Sort{{Column: "id"}},
	})
	if err != nil {
		t.Fatalf("SelectRows() error = %v", err)
	}
	if len(page.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(page.Rows))
	}

	row := page.Rows[0]
	checks := []struct {
		name string
		got  any
		want string
	}{
		{"tags text[] as JSON string", row[1], `["a","b"]`},
		{"nums int[] as JSON string", row[2], `[1,2]`},
		{"doc jsonb as string", row[3], `{"a": 1}`},
		{"blob bytea as 0x hex", row[4], "0xff00"},
		{"price numeric as string", row[5], "12.34"},
		{"uid uuid as string", row[6], "123e4567-e89b-12d3-a456-426614174000"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %#v (%T), want %q", c.name, c.got, c.got, c.want)
		}
	}
	if ts, ok := row[7].(string); !ok {
		t.Errorf("ts timestamptz = %#v (%T), want an RFC3339 string", row[7], row[7])
	} else {
		parsed, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			t.Errorf("ts timestamptz = %q, not RFC3339: %v", ts, err)
		} else if want := time.Date(2026, 10, 24, 10, 30, 0, 0, time.UTC); !parsed.Equal(want) {
			t.Errorf("ts timestamptz = %q, want the instant 2026-10-24T10:30:00Z", ts)
		}
	}
	if row[8] != true {
		t.Errorf("flag boolean = %#v (%T), want true", row[8], row[8])
	}

	// Row 2 is all NULLs — every composite rendering must give way to nil.
	for i, got := range page.Rows[1] {
		if i == 0 {
			continue // id
		}
		if got != nil {
			t.Errorf("row 2 column %d = %#v (%T), want nil", i, got, got)
		}
	}
}

func TestSelectRows_EmptyNames(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	ctx := context.Background()

	if _, err := drv.SelectRows(ctx, "", "t", sqlgen.SelectQuery{}); err == nil {
		t.Error("SelectRows(empty db) error: expected error, got nil")
	}
	if _, err := drv.SelectRows(ctx, "db", "", sqlgen.SelectQuery{}); err == nil {
		t.Error("SelectRows(empty table) error: expected error, got nil")
	}
}

// --- Row write operations ---

func TestInsertRow_NullVsEmpty(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useDataSchema(t, drv)
	seedWriteTable(t, drv)
	ctx := context.Background()

	// JSON null arrives as nil and must land as SQL NULL; "" must land
	// as an empty string. The two are never conflated.
	if err := drv.InsertRow(ctx, testDatabase, "write_t4", driver.RowValues{
		"id": int64(1), "name": "with-null", "note": nil,
	}); err != nil {
		t.Fatalf("InsertRow(NULL note) error: %v", err)
	}
	if err := drv.InsertRow(ctx, testDatabase, "write_t4", driver.RowValues{
		"id": int64(2), "name": "with-empty", "note": "",
	}); err != nil {
		t.Fatalf("InsertRow(empty note) error: %v", err)
	}
	if err := drv.InsertRow(ctx, testDatabase, "write_t4", nil); err == nil {
		t.Error("InsertRow(empty vals) error = nil, want error")
	}

	page := readAll(t, drv, "write_t4", "id")
	if len(page.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(page.Rows))
	}
	if got := page.Rows[0][2]; got != nil {
		t.Errorf("row 1 note = %#v (%T), want nil (SQL NULL)", got, got)
	}
	if got := page.Rows[1][2]; got != "" {
		t.Errorf("row 2 note = %#v (%T), want \"\"", got, got)
	}
}

func TestInsertRow_LastInsertID(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useDataSchema(t, drv)
	seedWriteTable(t, drv)
	ctx := context.Background()

	// serial table: RETURNING captures the generated id.
	if err := drv.InsertRow(ctx, testDatabase, "write_t4", driver.RowValues{"name": "first"}); err != nil {
		t.Fatalf("InsertRow(first) error: %v", err)
	}
	serialID, err := drv.LastInsertID(ctx)
	if err != nil {
		t.Fatalf("LastInsertID() error: %v", err)
	}
	if serialID < 1 {
		t.Errorf("LastInsertID() after serial insert = %d, want >= 1", serialID)
	}

	// Non-serial primary key: no RETURNING, LastInsertID is 0.
	execDataDDL(t, drv, `CREATE TABLE "data schema".plain_t4 (id integer PRIMARY KEY, name text)`)
	if err := drv.InsertRow(ctx, testDatabase, "plain_t4", driver.RowValues{
		"id": int64(1), "name": "explicit",
	}); err != nil {
		t.Fatalf("InsertRow(plain_t4) error: %v", err)
	}
	id, err := drv.LastInsertID(ctx)
	if err != nil {
		t.Fatalf("LastInsertID() error: %v", err)
	}
	if id != 0 {
		t.Errorf("LastInsertID() after non-serial insert = %d, want 0", id)
	}

	// The serial row must carry exactly the captured id.
	if got := readAll(t, drv, "write_t4", "id").Rows[0][0]; got != serialID {
		t.Errorf("serial row id = %v, want the generated id %v", got, serialID)
	}
}

func TestUpdateRow_ByPK(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useDataSchema(t, drv)
	seedWriteTable(t, drv)
	ctx := context.Background()

	for i := int64(1); i <= 2; i++ {
		if err := drv.InsertRow(ctx, testDatabase, "write_t4", driver.RowValues{
			"id": i, "name": fmt.Sprintf("old%d", i), "note": "keep",
		}); err != nil {
			t.Fatalf("InsertRow(row %d) error: %v", i, err)
		}
	}

	// Update by PK: several SET values (one of them NULL), only the
	// addressed row changes.
	if err := drv.UpdateRow(ctx, testDatabase, "write_t4",
		driver.PK{"id": int64(1)},
		driver.RowValues{"name": "new", "note": nil},
	); err != nil {
		t.Fatalf("UpdateRow() error: %v", err)
	}

	page := readAll(t, drv, "write_t4", "id")
	if len(page.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(page.Rows))
	}
	if page.Rows[0][1] != "new" || page.Rows[0][2] != nil {
		t.Errorf("row 1 = (%v, %v), want (new, nil)", page.Rows[0][1], page.Rows[0][2])
	}
	if page.Rows[1][1] != "old2" || page.Rows[1][2] != "keep" {
		t.Errorf("row 2 = (%v, %v), want (old2, keep) — untouched", page.Rows[1][1], page.Rows[1][2])
	}

	// An empty values set or an empty where must be rejected before any
	// statement: no unqualified full-table UPDATE.
	if err := drv.UpdateRow(ctx, testDatabase, "write_t4",
		driver.PK{"id": int64(1)}, driver.RowValues{}); err == nil {
		t.Error("UpdateRow(empty vals) error = nil, want error")
	}
	if err := drv.UpdateRow(ctx, testDatabase, "write_t4",
		driver.PK{}, driver.RowValues{"name": "boom"}); err == nil {
		t.Error("UpdateRow(empty where) error = nil, want error")
	}
	if page := readAll(t, drv, "write_t4", "id"); len(page.Rows) != 2 || page.Rows[1][1] != "old2" {
		t.Errorf("table changed by a rejected UpdateRow: %v rows", len(page.Rows))
	}
}

func TestUpdateRow_NoPK(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useDataSchema(t, drv)
	execDataDDL(t, drv, `CREATE TABLE "data schema".write_nopk_t4 (a integer, b varchar(50))`)
	ctx := context.Background()

	// Two identical rows: the table has no primary key, so the where is
	// all columns and a single UPDATE matches both.
	for i := 0; i < 2; i++ {
		if err := drv.InsertRow(ctx, testDatabase, "write_nopk_t4", driver.RowValues{
			"a": int64(1), "b": "x",
		}); err != nil {
			t.Fatalf("InsertRow(%d) error: %v", i+1, err)
		}
	}

	if err := drv.UpdateRow(ctx, testDatabase, "write_nopk_t4",
		driver.PK{"a": int64(1), "b": "x"},
		driver.RowValues{"b": "y"},
	); err != nil {
		t.Fatalf("UpdateRow() error: %v", err)
	}

	// Affected-rows semantics: every row matching the where changes —
	// both duplicates end up updated.
	page := readAll(t, drv, "write_nopk_t4", "b")
	if len(page.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(page.Rows))
	}
	for i, row := range page.Rows {
		if row[0] != int64(1) || row[1] != "y" {
			t.Errorf("row %d = (%v, %v), want (1, y)", i, row[0], row[1])
		}
	}
}

func TestDeleteRows_Multiple(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useDataSchema(t, drv)
	seedWriteTable(t, drv)
	ctx := context.Background()

	for i := int64(1); i <= 4; i++ {
		if err := drv.InsertRow(ctx, testDatabase, "write_t4", driver.RowValues{
			"id": i, "name": fmt.Sprintf("row%d", i),
		}); err != nil {
			t.Fatalf("InsertRow(%d) error: %v", i, err)
		}
	}

	// Two rows in one call, transactional.
	if err := drv.DeleteRows(ctx, testDatabase, "write_t4", []driver.PK{
		{"id": int64(1)},
		{"id": int64(3)},
	}); err != nil {
		t.Fatalf("DeleteRows() error: %v", err)
	}

	page := readAll(t, drv, "write_t4", "id")
	gotIDs := make([]int64, len(page.Rows))
	for i, row := range page.Rows {
		id, ok := row[0].(int64)
		if !ok {
			t.Fatalf("row %d id = %v (%T), want int64", i, row[0], row[0])
		}
		gotIDs[i] = id
	}
	if !slices.Equal(gotIDs, []int64{2, 4}) {
		t.Errorf("remaining ids = %v, want [2 4]", gotIDs)
	}

	// Pinned guards: an empty key inside the list and an empty list are
	// both errors — a DELETE without WHERE never leaves this driver.
	if err := drv.DeleteRows(ctx, testDatabase, "write_t4", []driver.PK{{}}); err == nil {
		t.Error("DeleteRows(empty key) error = nil, want error")
	}
	if err := drv.DeleteRows(ctx, testDatabase, "write_t4", nil); err == nil {
		t.Error("DeleteRows(empty list) error = nil, want error")
	}
	if page := readAll(t, drv, "write_t4", "id"); len(page.Rows) != 2 {
		t.Errorf("rejected DeleteRows damaged the table: %d rows left", len(page.Rows))
	}
}

func TestUpsertRows(t *testing.T) {
	cfg := startDB(t)
	drv := connectOK(t, cfg)
	useDataSchema(t, drv)
	seedWriteTable(t, drv)
	ctx := context.Background()

	if err := drv.InsertRow(ctx, testDatabase, "write_t4", driver.RowValues{
		"id": int64(1), "name": "old", "note": "keep",
	}); err != nil {
		t.Fatalf("InsertRow(existing) error: %v", err)
	}

	// One row hits the existing PK (updated via ON CONFLICT ... DO
	// UPDATE), one is new (inserted) — a single statement.
	if err := drv.UpsertRows(ctx, testDatabase, "write_t4", []driver.RowValues{
		{"id": int64(1), "name": "updated", "note": "replaced"},
		{"id": int64(2), "name": "inserted", "note": "fresh"},
	}); err != nil {
		t.Fatalf("UpsertRows() error: %v", err)
	}

	page := readAll(t, drv, "write_t4", "id")
	if len(page.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(page.Rows))
	}
	if page.Rows[0][1] != "updated" || page.Rows[0][2] != "replaced" {
		t.Errorf("row 1 = (%v, %v), want (updated, replaced)", page.Rows[0][1], page.Rows[0][2])
	}
	if page.Rows[1][1] != "inserted" || page.Rows[1][2] != "fresh" {
		t.Errorf("row 2 = (%v, %v), want (inserted, fresh)", page.Rows[1][1], page.Rows[1][2])
	}

	// Rows with different column sets cannot share one INSERT — rejected
	// before any statement. An empty row list is an error too.
	if err := drv.UpsertRows(ctx, testDatabase, "write_t4", []driver.RowValues{
		{"id": int64(5), "name": "a", "note": "n"},
		{"id": int64(6), "name": "b"},
	}); err == nil {
		t.Error("UpsertRows(mixed columns) error = nil, want error")
	}
	if err := drv.UpsertRows(ctx, testDatabase, "write_t4", nil); err == nil {
		t.Error("UpsertRows(empty list) error = nil, want error")
	}
	if page := readAll(t, drv, "write_t4", "id"); len(page.Rows) != 2 {
		t.Errorf("rejected UpsertRows changed the table: %d rows", len(page.Rows))
	}

	// A table without a primary key has no conflict target — an error,
	// not a fallback to a plain INSERT.
	execDataDDL(t, drv, `CREATE TABLE "data schema".upsert_nopk_t4 (a integer, b text)`)
	if err := drv.UpsertRows(ctx, testDatabase, "upsert_nopk_t4", []driver.RowValues{
		{"a": int64(1), "b": "x"},
	}); err == nil {
		t.Error("UpsertRows(no primary key) error = nil, want error")
	}
}
