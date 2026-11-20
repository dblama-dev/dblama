package sqlgen

import (
	"errors"
	"reflect"
	"testing"
)

func TestQuoteIdent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple", "a", "`a`"},
		{"inner backtick doubled", "a`b", "`a``b`"},
		{"reserved word", "order", "`order`"},
		{"empty yields empty backticks", "", "``"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := QuoteIdent(tt.in); got != tt.want {
				t.Errorf("QuoteIdent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestQuoteIdentPg(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple", "a", `"a"`},
		{"inner double quote doubled", `a"b`, `"a""b"`},
		{"mixed case preserved", "MixedCase", `"MixedCase"`},
		{"reserved word", "order", `"order"`},
		{"empty yields empty quotes", "", `""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := QuoteIdentPg(tt.in); got != tt.want {
				t.Errorf("QuoteIdentPg(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAllowedOperators(t *testing.T) {
	want := map[string]bool{
		"=": true, "!=": true, "<": true, ">": true, "<=": true, ">=": true,
		"LIKE": true, "NOT LIKE": true, "REGEXP": true, "NOT REGEXP": true, "CONTAINS": true,
		"IN": true, "NOT IN": true, "IS NULL": true, "IS NOT NULL": true,
	}
	got := AllowedOperators()
	if len(got) != len(want) {
		t.Fatalf("AllowedOperators() = %v, want exactly %d operators", got, len(want))
	}
	for _, op := range got {
		if !want[op] {
			t.Errorf("AllowedOperators() has unexpected operator %q", op)
		}
	}
}

func TestBuildSelect(t *testing.T) {
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
	want := "SELECT `id`,`name` FROM `users` WHERE `name` LIKE ? AND `age` >= ? ORDER BY `name` DESC, `id` LIMIT ? OFFSET ?"
	wantArgs := []any{"%a%", "18", 10, 20}

	got, args, err := BuildSelect("users", q)
	if err != nil {
		t.Fatalf("BuildSelect() error = %v", err)
	}
	if got != want {
		t.Errorf("BuildSelect() query = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("BuildSelect() args = %v, want %v", args, wantArgs)
	}

	// Determinism: same input must produce identical output on repeat calls.
	got2, args2, err := BuildSelect("users", q)
	if err != nil {
		t.Fatalf("BuildSelect() repeat error = %v", err)
	}
	if got2 != got || !reflect.DeepEqual(args2, args) {
		t.Errorf("BuildSelect() not deterministic:\nfirst  = %q %v\nrepeat = %q %v", got, args, got2, args2)
	}
}

func TestBuildSelect_DefaultLimitAndStar(t *testing.T) {
	got, args, err := BuildSelect("users", SelectQuery{})
	if err != nil {
		t.Fatalf("BuildSelect() error = %v", err)
	}
	want := "SELECT * FROM `users` LIMIT ? OFFSET ?"
	if got != want {
		t.Errorf("BuildSelect() query = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, []any{50, 0}) {
		t.Errorf("BuildSelect() args = %v, want [50 0] (Limit<=0 -> 50)", args)
	}
}

func TestBuildSelect_InOperator(t *testing.T) {
	tests := []struct {
		name     string
		operator string
		want     string
		wantArgs []any
	}{
		{"IN", "IN", "SELECT * FROM `t` WHERE `id` IN (?,?,?) LIMIT ? OFFSET ?", []any{"1", "2", "3", 50, 0}},
		{"NOT IN", "NOT IN", "SELECT * FROM `t` WHERE `id` NOT IN (?,?,?) LIMIT ? OFFSET ?", []any{"1", "2", "3", 50, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := SelectQuery{Filters: []Filter{{Column: "id", Operator: tt.operator, Value: "1,2,3"}}}
			got, args, err := BuildSelect("t", q)
			if err != nil {
				t.Fatalf("BuildSelect() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("BuildSelect() query = %q, want %q", got, tt.want)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("BuildSelect() args = %v, want %v", args, tt.wantArgs)
			}
		})
	}
}

func TestBuildSelect_NullFilter(t *testing.T) {
	for _, op := range []string{"IS NULL", "IS NOT NULL"} {
		q := SelectQuery{Filters: []Filter{{Column: "deleted_at", Operator: op, Value: "ignored"}}}
		got, args, err := BuildSelect("t", q)
		if err != nil {
			t.Fatalf("BuildSelect() op %q error = %v", op, err)
		}
		want := "SELECT * FROM `t` WHERE `deleted_at` " + op + " LIMIT ? OFFSET ?"
		if got != want {
			t.Errorf("BuildSelect() op %q query = %q, want %q", op, got, want)
		}
		if !reflect.DeepEqual(args, []any{50, 0}) {
			t.Errorf("BuildSelect() op %q args = %v, want [50 0] (no placeholder for NULL filter)", op, args)
		}
	}
}

func TestBuildSelect_InjectionSafe(t *testing.T) {
	q := SelectQuery{
		Filters: []Filter{{Column: "name", Operator: "=", Value: "x'; DROP TABLE t--"}},
		Limit:   1,
	}
	got, args, err := BuildSelect("t", q)
	if err != nil {
		t.Fatalf("BuildSelect() error = %v", err)
	}
	want := "SELECT * FROM `t` WHERE `name` = ? LIMIT ? OFFSET ?"
	if got != want {
		t.Errorf("BuildSelect() query = %q, want %q (value must never leak into SQL)", got, want)
	}
	if !reflect.DeepEqual(args, []any{"x'; DROP TABLE t--", 1, 0}) {
		t.Errorf("BuildSelect() args = %v, want value only in args", args)
	}
}

func TestBuildSelect_UnknownOperator(t *testing.T) {
	q := SelectQuery{Filters: []Filter{{Column: "a", Operator: "<>", Value: "1"}}}
	if _, _, err := BuildSelect("t", q); err == nil {
		t.Error("BuildSelect() with unknown operator <> must return error, got nil")
	}
}

func TestBuildSelect_EmptyComparison(t *testing.T) {
	for _, build := range []func(string, SelectQuery) (string, []any, error){BuildSelect, BuildSelectPg} {
		for _, op := range []string{"=", "!="} {
			q := SelectQuery{Filters: []Filter{{Column: "note", Operator: op, Value: ""}}}
			_, args, err := build("items", q)
			if err != nil || !reflect.DeepEqual(args, []any{"", 50, 0}) {
				t.Errorf("empty string %s comparison: args=%#v, err=%v", op, args, err)
			}
		}
	}
	q := SelectQuery{Filters: []Filter{{Column: "a", Operator: ">", Value: ""}}}
	if _, _, err := BuildSelect("t", q); err == nil {
		t.Error("empty ordering filter accepted")
	}
}

func TestBuildSelectQualified(t *testing.T) {
	q := SelectQuery{
		Filters: []Filter{{Column: "id", Operator: "IN", Value: "1,2"}},
		Sorts:   []Sort{{Column: "id", Desc: true}},
		Limit:   5,
		Offset:  10,
	}
	want := "SELECT * FROM `mydb`.`users` WHERE `id` IN (?,?) ORDER BY `id` DESC LIMIT ? OFFSET ?"
	wantArgs := []any{"1", "2", 5, 10}

	got, args, err := BuildSelectQualified("mydb", "users", q)
	if err != nil {
		t.Fatalf("BuildSelectQualified() error = %v", err)
	}
	if got != want {
		t.Errorf("BuildSelectQualified() query = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("BuildSelectQualified() args = %v, want %v", args, wantArgs)
	}
}

func TestBuildCountQualified(t *testing.T) {
	q := SelectQuery{
		Columns: []string{"id"}, // ignored: COUNT(*) has no column list
		Filters: []Filter{
			{Column: "a", Operator: "LIKE", Value: "%x%"},
			{Column: "b", Operator: "IS NULL"},
		},
		Sorts:  []Sort{{Column: "id"}}, // ignored: no ORDER BY in a count
		Limit:  3,
		Offset: 2,
	}
	want := "SELECT COUNT(*) FROM `mydb`.`users` WHERE `a` LIKE ? AND `b` IS NULL"
	wantArgs := []any{"%x%"}

	got, args, err := BuildCountQualified("mydb", "users", q)
	if err != nil {
		t.Fatalf("BuildCountQualified() error = %v", err)
	}
	if got != want {
		t.Errorf("BuildCountQualified() query = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("BuildCountQualified() args = %v, want %v", args, wantArgs)
	}
}

func TestQuoteIdent_Empty(t *testing.T) {
	// Build* functions must reject empty table and column names with ErrEmptyIdentifier.
	tests := []struct {
		name string
		call func() error
	}{
		{"BuildSelect empty table", func() error {
			_, _, err := BuildSelect("", SelectQuery{})
			return err
		}},
		{"BuildSelectQualified empty db", func() error {
			_, _, err := BuildSelectQualified("", "t", SelectQuery{})
			return err
		}},
		{"BuildSelectQualified empty table", func() error {
			_, _, err := BuildSelectQualified("db", "", SelectQuery{})
			return err
		}},
		{"BuildCountQualified empty db", func() error {
			_, _, err := BuildCountQualified("", "t", SelectQuery{})
			return err
		}},
		{"BuildCountQualified empty table", func() error {
			_, _, err := BuildCountQualified("db", "", SelectQuery{})
			return err
		}},
		{"BuildSelect empty column", func() error {
			_, _, err := BuildSelect("t", SelectQuery{Columns: []string{"a", ""}})
			return err
		}},
		{"BuildSelect empty filter column", func() error {
			_, _, err := BuildSelect("t", SelectQuery{Filters: []Filter{{Operator: "=", Value: "1"}}})
			return err
		}},
		{"BuildSelect empty sort column", func() error {
			_, _, err := BuildSelect("t", SelectQuery{Sorts: []Sort{{Desc: true}}})
			return err
		}},
		{"BuildInsert empty table", func() error {
			_, _, err := BuildInsert("", []string{"a"})
			return err
		}},
		{"BuildInsert empty column", func() error {
			_, _, err := BuildInsert("t", []string{"a", ""})
			return err
		}},
		{"BuildUpdate empty table", func() error {
			_, _, err := BuildUpdate("", []string{"a"}, nil)
			return err
		}},
		{"BuildDelete empty table", func() error {
			_, _, err := BuildDelete("", map[string]any{"id": 1})
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
