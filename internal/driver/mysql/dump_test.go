package mysql

import "testing"

// TestKeysetText pins the keyset cursor rendering per column type: only
// datetime-typed values (which the read path renders as RFC3339) are
// normalized to SQL datetime text; a string-typed key passes through
// verbatim even when it happens to look like an RFC3339 timestamp.
func TestKeysetText(t *testing.T) {
	tests := []struct {
		name    string
		in      any
		colType string
		want    string
	}{
		{"int", int64(42), "int", "42"},
		{"negative int", int64(-7), "bigint", "-7"},
		{"float", 1.5, "double", "1.5"},
		{"bool true", true, "tinyint", "1"},
		{"bool false", false, "tinyint", "0"},
		{"string passthrough", "abc", "varchar", "abc"},
		{"string datetime", "2026-01-02 10:20:30", "datetime", "2026-01-02 10:20:30"},
		{"rfc3339 on datetime column", "2026-01-02T10:20:30Z", "datetime", "2026-01-02 10:20:30"},
		{"rfc3339 on varchar column", "2026-01-02T10:20:30Z", "varchar", "2026-01-02T10:20:30Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keysetText(tt.in, tt.colType); got != tt.want {
				t.Errorf("keysetText(%v, %q) = %q, want %q", tt.in, tt.colType, got, tt.want)
			}
		})
	}
}

// TestSQLLiteral pins the INSERT literal rendering: NULL, raw numbers,
// SQL-escaped strings, datetime normalization and 0x hex for binary
// columns (with the empty value's X'' form).
func TestSQLLiteral(t *testing.T) {
	tests := []struct {
		name    string
		in      any
		colType string
		want    string
	}{
		{"nil", nil, "int", "NULL"},
		{"int", int64(42), "int", "42"},
		{"float", 2.25, "double", "2.25"},
		{"bool", true, "tinyint", "1"},
		{"string", "it's", "varchar", `'it''s'`},
		{"plain datetime", "2026-01-02 10:20:30", "datetime", `'2026-01-02 10:20:30'`},
		{"rfc3339 datetime", "2026-01-02T10:20:30Z", "datetime", `'2026-01-02 10:20:30'`},
		{"binary hex", "0xDEADBEEF", "varbinary", "0xDEADBEEF"},
		{"binary empty", "0x", "varbinary", "X''"},
		{"hex-looking text", "0xDEADBEEF", "varchar", `'0xDEADBEEF'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sqlLiteral(tt.in, tt.colType); got != tt.want {
				t.Errorf("sqlLiteral(%v, %q) = %s, want %s", tt.in, tt.colType, got, tt.want)
			}
		})
	}
}

// TestAddIfNotExists pins the SHOW CREATE TABLE rewrite and that any
// other layout is an error instead of a silently unmodified statement.
func TestAddIfNotExists(t *testing.T) {
	in := "CREATE TABLE `t` (\n  `id` int NOT NULL,\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB"
	want := "CREATE TABLE IF NOT EXISTS `t` (\n  `id` int NOT NULL,\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB"
	got, err := addIfNotExists(in)
	if err != nil {
		t.Fatalf("addIfNotExists() error: %v", err)
	}
	if got != want {
		t.Errorf("addIfNotExists() = %q, want %q", got, want)
	}

	if _, err := addIfNotExists("CREATE VIEW v AS SELECT 1"); err == nil {
		t.Error("addIfNotExists(CREATE VIEW …) error: expected error, got nil")
	}
}
