package mysql

import (
	"context"
	"reflect"
	"testing"
)

func TestSplitStatements(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "single", query: "SELECT 1", want: []string{"SELECT 1"}},
		{name: "two", query: "SELECT 1; SELECT 2", want: []string{"SELECT 1", "SELECT 2"}},
		{name: "trailing semicolon", query: "SELECT 1; SELECT 2;", want: []string{"SELECT 1", "SELECT 2"}},
		{name: "semicolons only", query: ";;;  ;", want: nil},
		{name: "blank", query: "   ", want: nil},
		{name: "semicolon in single quotes", query: "SELECT 'a;b'; SELECT 2", want: []string{"SELECT 'a;b'", "SELECT 2"}},
		{name: "doubled quote escape", query: "SELECT 'it''s; ok'; SELECT 2", want: []string{"SELECT 'it''s; ok'", "SELECT 2"}},
		{name: "backslash escape", query: `SELECT 'it\'s; ok'; SELECT 2`, want: []string{`SELECT 'it\'s; ok'`, "SELECT 2"}},
		{name: "semicolon in double quotes", query: `SELECT "a;b"; SELECT 2`, want: []string{`SELECT "a;b"`, "SELECT 2"}},
		{name: "semicolon in backticks", query: "SELECT `a;b` FROM t", want: []string{"SELECT `a;b` FROM t"}},
		{name: "hash comment", query: "SELECT 1 # ; not a break\n; SELECT 2", want: []string{"SELECT 1 # ; not a break", "SELECT 2"}},
		{name: "hash inside string", query: "SELECT '#';", want: []string{"SELECT '#'"}},
		{name: "dash dash comment", query: "SELECT 1 -- ; not a break\n; SELECT 2", want: []string{"SELECT 1 -- ; not a break", "SELECT 2"}},
		{"dash dash needs space", "SELECT 1--2; SELECT 2", []string{"SELECT 1--2", "SELECT 2"}},
		{name: "block comment", query: "SELECT 1 /* ; still one */; SELECT 2", want: []string{"SELECT 1 /* ; still one */", "SELECT 2"}},
		{name: "block comment multiline", query: "SELECT 1 /* ;\n; */ ; SELECT 2", want: []string{"SELECT 1 /* ;\n; */", "SELECT 2"}},
		{name: "crlf after comment", query: "SELECT 1 -- c\r\n; SELECT 2", want: []string{"SELECT 1 -- c", "SELECT 2"}},
		{name: "no space around semicolons", query: "SELECT 1;SELECT 2", want: []string{"SELECT 1", "SELECT 2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := splitStatements(tc.query)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitStatements(%q) = %#v, want %#v", tc.query, got, tc.want)
			}
		})
	}
}

func TestIsRowStatement(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want bool
	}{
		{"SELECT 1", true},
		{"  select * from t", true},
		{"SELECT(1)", true},
		{"SHOW TABLES", true},
		{"DESC t", true},
		{"DESCRIBE t", true},
		{"EXPLAIN SELECT 1", true},
		{"EXPLAIN ANALYZE SELECT 1", true},
		{"WITH c AS (SELECT 1) SELECT * FROM c", true},
		{"selection is not a keyword", false},
		{"INSERT INTO t VALUES (1)", false},
		{"UPDATE t SET a=1", false},
		{"DELETE FROM t", false},
		{"CREATE TABLE t (id INT)", false},
		{"SET @a=1", false},
		{"USE other", false},
		{"", false},
		{"/* leading */ SELECT 1", true},
		{"/* leading */\nSELECT 1", true},
		{"-- comment\nSELECT 1", true},
		{"# comment\nSHOW TABLES", true},
		{"/* unterminated SELECT", false},
		{"-- comment only", false},
	}
	for _, tc := range cases {
		if got := isRowStatement(tc.stmt); got != tc.want {
			t.Errorf("isRowStatement(%q) = %v, want %v", tc.stmt, got, tc.want)
		}
	}
}

// TestCancelQueryZeroValue pins that CancelQuery never panics and never
// errors on an unknown queryID — even on a driver instance without any
// registered query (the registry lazily initializes).
func TestCancelQueryZeroValue(t *testing.T) {
	t.Parallel()
	d := &MysqlDriver{}
	if err := d.CancelQuery(context.Background(), "no-such-query"); err != nil {
		t.Errorf("CancelQuery(unknown) error: %v, want nil", err)
	}
}
