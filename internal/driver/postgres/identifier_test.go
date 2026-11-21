package postgres

import (
	"errors"
	"testing"

	"github.com/dblama-dev/dblama/internal/sqlgen"
)

func TestQualify(t *testing.T) {
	got, err := qualify("public", "users")
	if err != nil {
		t.Fatalf("qualify() error = %v", err)
	}
	if want := `"public"."users"`; got != want {
		t.Errorf("qualify() = %q, want %q", got, want)
	}

	// Mixed-case and special-character names pass through QuoteIdentPg
	// verbatim — PG folds unquoted identifiers to lowercase, quoting is
	// what preserves them (plan review focus 1).
	got, err = qualify("Odd Schema", `My"Table`)
	if err != nil {
		t.Fatalf("qualify() error = %v", err)
	}
	if want := `"Odd Schema"."My""Table"`; got != want {
		t.Errorf("qualify() = %q, want %q", got, want)
	}
}

func TestQualifyEmptyPart(t *testing.T) {
	if _, err := qualify("", "users"); !errors.Is(err, sqlgen.ErrEmptyIdentifier) {
		t.Errorf("qualify(empty schema) error = %v, want ErrEmptyIdentifier", err)
	}
	if _, err := qualify("public", ""); !errors.Is(err, sqlgen.ErrEmptyIdentifier) {
		t.Errorf("qualify(empty table) error = %v, want ErrEmptyIdentifier", err)
	}
}
