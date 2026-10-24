package sqlgen

import (
	"errors"
	"reflect"
	"testing"
)

// The Pg builders mirror the MySQL ones statement for statement — the
// goldens below are the MySQL goldens re-rendered in the PostgreSQL
// dialect: double-quoted identifiers and $1..$N placeholders.

func TestBuildSelectPg(t *testing.T) {
	q := SelectQuery{
		Columns: []string{"id", "name"},
		Filters: []Filter{
			{Column: "name", Operator: "LIKE", Value: "%a%"},
			{Column: "age", Operator: ">=", Value: "18"},
		},
		Sorts:  []Sort{{Column: "name", Desc: true}, {Column: "id"}},
		Limit:  10,
		Offset: 20,
	}
	want := `SELECT "id","name" FROM "users" WHERE "name" LIKE $1 AND "age" >= $2 ORDER BY "name" DESC, "id" LIMIT $3 OFFSET $4`
	wantArgs := []any{"%a%", "18", 10, 20}

	got, args, err := BuildSelectPg("users", q)
	if err != nil {
		t.Fatalf("BuildSelectPg() error = %v", err)
	}
	if got != want {
		t.Errorf("BuildSelectPg() query = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("BuildSelectPg() args = %v, want %v", args, wantArgs)
	}

	got2, args2, err := BuildSelectPg("users", q)
	if err != nil {
		t.Fatalf("BuildSelectPg() repeat error = %v", err)
	}
	if got2 != got || !reflect.DeepEqual(args2, args) {
		t.Errorf("BuildSelectPg() not deterministic:\nfirst  = %q %v\nrepeat = %q %v", got, args, got2, args2)
	}
}

func TestBuildSelectPg_DefaultLimitAndStar(t *testing.T) {
	got, args, err := BuildSelectPg("users", SelectQuery{})
	if err != nil {
		t.Fatalf("BuildSelectPg() error = %v", err)
	}
	want := `SELECT * FROM "users" LIMIT $1 OFFSET $2`
	if got != want {
		t.Errorf("BuildSelectPg() query = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, []any{50, 0}) {
		t.Errorf("BuildSelectPg() args = %v, want [50 0]", args)
	}
}

func TestBuildSelectPg_InAndNull(t *testing.T) {
	q := SelectQuery{
		Filters: []Filter{
			{Column: "id", Operator: "IN", Value: "1,2,3"},
			{Column: "deleted_at", Operator: "IS NULL"},
		},
		Sorts: []Sort{{Column: "id", Desc: true}},
		Limit: 5,
	}
	got, args, err := BuildSelectPg("t", q)
	if err != nil {
		t.Fatalf("BuildSelectPg() error = %v", err)
	}
	want := `SELECT * FROM "t" WHERE "id" IN ($1,$2,$3) AND "deleted_at" IS NULL ORDER BY "id" DESC LIMIT $4 OFFSET $5`
	if got != want {
		t.Errorf("BuildSelectPg() query = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, []any{"1", "2", "3", 5, 0}) {
		t.Errorf("BuildSelectPg() args = %v, want [1 2 3 5 0]", args)
	}
}

func TestBuildSelectQualifiedPg(t *testing.T) {
	q := SelectQuery{
		Filters: []Filter{{Column: "id", Operator: "IN", Value: "1,2"}},
		Sorts:   []Sort{{Column: "id", Desc: true}},
		Limit:   5,
		Offset:  10,
	}
	want := `SELECT * FROM "odd schema"."users" WHERE "id" IN ($1,$2) ORDER BY "id" DESC LIMIT $3 OFFSET $4`
	wantArgs := []any{"1", "2", 5, 10}

	got, args, err := BuildSelectQualifiedPg("odd schema", "users", q)
	if err != nil {
		t.Fatalf("BuildSelectQualifiedPg() error = %v", err)
	}
	if got != want {
		t.Errorf("BuildSelectQualifiedPg() query = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("BuildSelectQualifiedPg() args = %v, want %v", args, wantArgs)
	}
}

func TestBuildCountQualifiedPg(t *testing.T) {
	q := SelectQuery{
		Columns: []string{"id"}, // ignored by the count
		Filters: []Filter{
			{Column: "a", Operator: "LIKE", Value: "%x%"},
			{Column: "b", Operator: "IS NULL"},
		},
		Sorts: []Sort{{Column: "id"}}, // ignored: no ORDER BY in a count
		Limit: 3,
	}
	want := `SELECT COUNT(*) FROM "odd schema"."users" WHERE "a" LIKE $1 AND "b" IS NULL`
	wantArgs := []any{"%x%"}

	got, args, err := BuildCountQualifiedPg("odd schema", "users", q)
	if err != nil {
		t.Fatalf("BuildCountQualifiedPg() error = %v", err)
	}
	if got != want {
		t.Errorf("BuildCountQualifiedPg() query = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("BuildCountQualifiedPg() args = %v, want %v", args, wantArgs)
	}
}

func TestBuildInsertPg(t *testing.T) {
	got, placeholder, err := BuildInsertPg("users", []string{"id", "name", "email"})
	if err != nil {
		t.Fatalf("BuildInsertPg() error = %v", err)
	}
	want := `INSERT INTO "users" ("id","name","email") VALUES ($1,$2,$3)`
	if got != want {
		t.Errorf("BuildInsertPg() query = %q, want %q", got, want)
	}
	if placeholder != "" {
		t.Errorf("BuildInsertPg() argsPlaceholder = %q, want empty string", placeholder)
	}
}

func TestBuildUpdatePg_WhereSorted(t *testing.T) {
	// SET columns sorted by name; WHERE keys sorted; the placeholder
	// numbering runs through SET and WHERE (SET $1..$3, WHERE $4,$5);
	// args stay the where values only, in sorted-key order.
	got, args, err := BuildUpdatePg("users", []string{"name", "email", "age"}, map[string]any{"id": 1, "tenant": "acme"})
	if err != nil {
		t.Fatalf("BuildUpdatePg() error = %v", err)
	}
	want := `UPDATE "users" SET "age"=$1,"email"=$2,"name"=$3 WHERE "id"=$4 AND "tenant"=$5`
	if got != want {
		t.Errorf("BuildUpdatePg() query = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, []any{1, "acme"}) {
		t.Errorf("BuildUpdatePg() args = %v, want [1 acme]", args)
	}

	got2, args2, err := BuildUpdatePg("users", []string{"name", "email", "age"}, map[string]any{"id": 1, "tenant": "acme"})
	if err != nil {
		t.Fatalf("BuildUpdatePg() repeat error = %v", err)
	}
	if got2 != got || !reflect.DeepEqual(args2, args) {
		t.Errorf("BuildUpdatePg() not deterministic:\nfirst  = %q %v\nrepeat = %q %v", got, args, got2, args2)
	}
}

func TestBuildUpdatePg_EmptyWhere(t *testing.T) {
	got, args, err := BuildUpdatePg("t", []string{"a"}, nil)
	if err != nil {
		t.Fatalf("BuildUpdatePg() error = %v", err)
	}
	want := `UPDATE "t" SET "a"=$1`
	if got != want {
		t.Errorf("BuildUpdatePg() query = %q, want %q", got, want)
	}
	if len(args) != 0 {
		t.Errorf("BuildUpdatePg() args = %v, want empty", args)
	}
}

func TestBuildDeletePg(t *testing.T) {
	got, args, err := BuildDeletePg("users", map[string]any{"id": 1, "tenant": "acme"})
	if err != nil {
		t.Fatalf("BuildDeletePg() error = %v", err)
	}
	want := `DELETE FROM "users" WHERE "id"=$1 AND "tenant"=$2`
	if got != want {
		t.Errorf("BuildDeletePg() query = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, []any{1, "acme"}) {
		t.Errorf("BuildDeletePg() args = %v, want [1 acme]", args)
	}
}

func TestPgBuilders_EmptyIdentifier(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{"BuildSelectPg empty table", func() error {
			_, _, err := BuildSelectPg("", SelectQuery{})
			return err
		}},
		{"BuildSelectPg empty column", func() error {
			_, _, err := BuildSelectPg("t", SelectQuery{Columns: []string{"a", ""}})
			return err
		}},
		{"BuildSelectQualifiedPg empty schema", func() error {
			_, _, err := BuildSelectQualifiedPg("", "t", SelectQuery{})
			return err
		}},
		{"BuildSelectQualifiedPg empty table", func() error {
			_, _, err := BuildSelectQualifiedPg("s", "", SelectQuery{})
			return err
		}},
		{"BuildCountQualifiedPg empty schema", func() error {
			_, _, err := BuildCountQualifiedPg("", "t", SelectQuery{})
			return err
		}},
		{"BuildCountQualifiedPg empty table", func() error {
			_, _, err := BuildCountQualifiedPg("s", "", SelectQuery{})
			return err
		}},
		{"BuildInsertPg empty table", func() error {
			_, _, err := BuildInsertPg("", []string{"a"})
			return err
		}},
		{"BuildInsertPg empty column", func() error {
			_, _, err := BuildInsertPg("t", []string{"a", ""})
			return err
		}},
		{"BuildUpdatePg empty table", func() error {
			_, _, err := BuildUpdatePg("", []string{"a"}, nil)
			return err
		}},
		{"BuildDeletePg empty table", func() error {
			_, _, err := BuildDeletePg("", map[string]any{"id": 1})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if !errors.Is(err, ErrEmptyIdentifier) {
				t.Errorf("%s: err = %v, want ErrEmptyIdentifier", tt.name, err)
			}
		})
	}
}

func TestDialects(t *testing.T) {
	if got := MySQLDialect.QuoteIdent("a`b"); got != "`a``b`" {
		t.Errorf("MySQLDialect.QuoteIdent = %q, want backticks with doubling", got)
	}
	for n := 1; n <= 2; n++ {
		if got := MySQLDialect.Placeholder(n); got != "?" {
			t.Errorf("MySQLDialect.Placeholder(%d) = %q, want ?", n, got)
		}
	}
	if got := PgDialect.QuoteIdent(`a"b`); got != `"a""b"` {
		t.Errorf("PgDialect.QuoteIdent = %q, want double quotes with doubling", got)
	}
	for _, tc := range []struct {
		n    int
		want string
	}{
		{1, "$1"}, {2, "$2"}, {10, "$10"}, {123, "$123"},
	} {
		if got := PgDialect.Placeholder(tc.n); got != tc.want {
			t.Errorf("PgDialect.Placeholder(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
