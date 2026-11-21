package sqlgen

import (
	"reflect"
	"testing"
)

func TestBuildInsert(t *testing.T) {
	got, placeholder, err := BuildInsert("users", []string{"id", "name", "email"})
	if err != nil {
		t.Fatalf("BuildInsert() error = %v", err)
	}
	want := "INSERT INTO `users` (`id`,`name`,`email`) VALUES (?,?,?)"
	if got != want {
		t.Errorf("BuildInsert() query = %q, want %q", got, want)
	}
	if placeholder != "" {
		t.Errorf("BuildInsert() argsPlaceholder = %q, want empty string", placeholder)
	}
}

func TestBuildInsert_SingleColumn(t *testing.T) {
	got, _, err := BuildInsert("t", []string{"a"})
	if err != nil {
		t.Fatalf("BuildInsert() error = %v", err)
	}
	want := "INSERT INTO `t` (`a`) VALUES (?)"
	if got != want {
		t.Errorf("BuildInsert() query = %q, want %q", got, want)
	}
}

func TestBuildUpdate_WhereSorted(t *testing.T) {
	// SET columns sorted by name; WHERE keys sorted; where values land in args in sorted-key order.
	got, args, err := BuildUpdate("users", []string{"name", "email", "age"}, map[string]any{"id": 1, "tenant": "acme"})
	if err != nil {
		t.Fatalf("BuildUpdate() error = %v", err)
	}
	want := "UPDATE `users` SET `age`=?,`email`=?,`name`=? WHERE `id`=? AND `tenant`=?"
	if got != want {
		t.Errorf("BuildUpdate() query = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, []any{1, "acme"}) {
		t.Errorf("BuildUpdate() args = %v, want [1 acme] (where values in sorted-key order)", args)
	}

	// Determinism: map iteration must not leak into output.
	got2, args2, err := BuildUpdate("users", []string{"name", "email", "age"}, map[string]any{"id": 1, "tenant": "acme"})
	if err != nil {
		t.Fatalf("BuildUpdate() repeat error = %v", err)
	}
	if got2 != got || !reflect.DeepEqual(args2, args) {
		t.Errorf("BuildUpdate() not deterministic:\nfirst  = %q %v\nrepeat = %q %v", got, args, got2, args2)
	}
}

func TestBuildUpdate_EmptyWhere(t *testing.T) {
	got, args, err := BuildUpdate("t", []string{"a"}, nil)
	if err != nil {
		t.Fatalf("BuildUpdate() error = %v", err)
	}
	want := "UPDATE `t` SET `a`=?"
	if got != want {
		t.Errorf("BuildUpdate() query = %q, want %q", got, want)
	}
	if len(args) != 0 {
		t.Errorf("BuildUpdate() args = %v, want empty", args)
	}
}

func TestBuildDelete(t *testing.T) {
	got, args, err := BuildDelete("users", map[string]any{"id": 1, "tenant": "acme"})
	if err != nil {
		t.Fatalf("BuildDelete() error = %v", err)
	}
	want := "DELETE FROM `users` WHERE `id`=? AND `tenant`=?"
	if got != want {
		t.Errorf("BuildDelete() query = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, []any{1, "acme"}) {
		t.Errorf("BuildDelete() args = %v, want [1 acme]", args)
	}

	got2, args2, err := BuildDelete("users", map[string]any{"id": 1, "tenant": "acme"})
	if err != nil {
		t.Fatalf("BuildDelete() repeat error = %v", err)
	}
	if got2 != got || !reflect.DeepEqual(args2, args) {
		t.Errorf("BuildDelete() not deterministic:\nfirst  = %q %v\nrepeat = %q %v", got, args, got2, args2)
	}
}

func TestBuildDelete_EmptyWhere(t *testing.T) {
	got, args, err := BuildDelete("t", nil)
	if err != nil {
		t.Fatalf("BuildDelete() error = %v", err)
	}
	want := "DELETE FROM `t`"
	if got != want {
		t.Errorf("BuildDelete() query = %q, want %q", got, want)
	}
	if len(args) != 0 {
		t.Errorf("BuildDelete() args = %v, want empty", args)
	}
}

func TestWriteWhereNullUsesISNULLAndCorrectPlaceholders(t *testing.T) {
	query, args, err := BuildUpdatePg("t", []string{"changed"}, map[string]any{"a": nil, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	if query != `UPDATE "t" SET "changed"=$1 WHERE "a" IS NULL AND "b"=$2` || len(args) != 1 || args[0] != 2 {
		t.Fatalf("query %s args %#v", query, args)
	}
	query, args, err = BuildDelete("t", map[string]any{"a": nil, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	if query != "DELETE FROM `t` WHERE `a` IS NULL AND `b`=?" || len(args) != 1 || args[0] != 2 {
		t.Fatalf("query %s args %#v", query, args)
	}
}
