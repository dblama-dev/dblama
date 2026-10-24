package sqlgen

import (
	"reflect"
	"testing"
)

// TestSplitStatements pins the string/comment-aware statement splitting
// of SplitStatements. DELIMITER behavior has its own test below.
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
		{name: "empty", query: "", want: nil},
		{name: "semicolon in single quotes", query: "SELECT 'a;b'; SELECT 2", want: []string{"SELECT 'a;b'", "SELECT 2"}},
		{name: "doubled quote escape", query: "SELECT 'it''s; ok'; SELECT 2", want: []string{"SELECT 'it''s; ok'", "SELECT 2"}},
		{name: "backslash escape", query: `SELECT 'it\'s; ok'; SELECT 2`, want: []string{`SELECT 'it\'s; ok'`, "SELECT 2"}},
		{name: "semicolon in double quotes", query: `SELECT "a;b"; SELECT 2`, want: []string{`SELECT "a;b"`, "SELECT 2"}},
		{name: "backslash in double quotes", query: `SELECT "a\"; b"; SELECT 2`, want: []string{`SELECT "a\"; b"`, "SELECT 2"}},
		{name: "semicolon in backticks", query: "SELECT `a;b` FROM t", want: []string{"SELECT `a;b` FROM t"}},
		{name: "backslash literal in backticks", query: "SELECT `a\\;b` FROM t", want: []string{"SELECT `a\\;b` FROM t"}},
		{name: "hash comment", query: "SELECT 1 # ; not a break\n; SELECT 2", want: []string{"SELECT 1 # ; not a break", "SELECT 2"}},
		{name: "hash inside string", query: "SELECT '#';", want: []string{"SELECT '#'"}},
		{name: "dash dash comment", query: "SELECT 1 -- ; not a break\n; SELECT 2", want: []string{"SELECT 1 -- ; not a break", "SELECT 2"}},
		{"dash dash needs space", "SELECT 1--2; SELECT 2", []string{"SELECT 1--2", "SELECT 2"}},
		{name: "block comment", query: "SELECT 1 /* ; still one */; SELECT 2", want: []string{"SELECT 1 /* ; still one */", "SELECT 2"}},
		{name: "block comment multiline", query: "SELECT 1 /* ;\n; */ ; SELECT 2", want: []string{"SELECT 1 /* ;\n; */", "SELECT 2"}},
		{
			// Pinned Task 3 difference from the mysql client: the
			// executable-comment marker /*! is treated exactly like /*
			// for splitting; its content never toggles a split.
			name:  "executable comment as block comment",
			query: "SELECT 1 /*! ; still one */; SELECT 2",
			want:  []string{"SELECT 1 /*! ; still one */", "SELECT 2"},
		},
		{name: "crlf after comment", query: "SELECT 1 -- c\r\n; SELECT 2", want: []string{"SELECT 1 -- c", "SELECT 2"}},
		{name: "no space around semicolons", query: "SELECT 1;SELECT 2", want: []string{"SELECT 1", "SELECT 2"}},
		{
			// Pinned Task 1 semantics: comments stay verbatim inside
			// their statement, so trailing comment text is emitted as
			// the final statement.
			name:  "only comments",
			query: "-- nothing ; here\n# nor ; here\n/* nor ; here */",
			want:  []string{"-- nothing ; here\n# nor ; here\n/* nor ; here */"},
		},
		{name: "delimiter word inside string is not a directive", query: "SELECT 'delimiter ;;'", want: []string{"SELECT 'delimiter ;;'"}},
		{
			// Pinned: a DELIMITER directive is recognized only as the
			// first word of a line; mid-line "delimiter" is statement
			// text, and the default ';' inside it still splits.
			name:  "delimiter must start the line",
			query: "SELECT 1; delimiter ;; SELECT 2",
			want:  []string{"SELECT 1", "delimiter", "SELECT 2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SplitStatements(tc.query)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SplitStatements(%q) = %#v, want %#v", tc.query, got, tc.want)
			}
		})
	}
}

// TestSplitStatementsDelimiter pins the mysql-client DELIMITER
// semantics: a line whose first word is "delimiter" (case-insensitive)
// switches the statement terminator; the directive line itself is not a
// statement; the previous terminator stops splitting until the next
// directive.
func TestSplitStatementsDelimiter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name: "double semicolon procedure block",
			query: "DELIMITER ;;\n" +
				"CREATE PROCEDURE p() BEGIN SELECT 1; SELECT 2; END;;\n" +
				"DELIMITER ;\n" +
				"SELECT 3;",
			want: []string{"CREATE PROCEDURE p() BEGIN SELECT 1; SELECT 2; END", "SELECT 3"},
		},
		{
			name: "custom dollar token",
			query: "DELIMITER $$\n" +
				"CREATE TRIGGER tr BEFORE INSERT ON t FOR EACH ROW SET @a = 1; SET @b = 2$$\n" +
				"DELIMITER ;\n" +
				"SELECT 1;",
			want: []string{"CREATE TRIGGER tr BEFORE INSERT ON t FOR EACH ROW SET @a = 1; SET @b = 2", "SELECT 1"},
		},
		{
			name:  "case insensitive keyword",
			query: "delimiter ;;\nSELECT 1;;\nDeLiMiTeR ;\nSELECT 2",
			want:  []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:  "leading whitespace on directive line",
			query: "   DELIMITER ;;\nSELECT 1;;SELECT 2;;",
			want:  []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:  "trailing block without terminator",
			query: "DELIMITER ;;\nSELECT 1;;\nSELECT 2",
			want:  []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:  "directive only input",
			query: "DELIMITER ;;",
			want:  nil,
		},
		{
			name: "directive with quotes around token",
			query: "DELIMITER ';;'\nSELECT 1;;\nDELIMITER \";\"\nSELECT 2;",
			want:  []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:  "delimiter directive inside block comment is not a directive",
			query: "SELECT 1 /* delimiter ;; */; SELECT 2",
			want:  []string{"SELECT 1 /* delimiter ;; */", "SELECT 2"},
		},
		{
			name:  "pending text before directive is flushed first",
			query: "SELECT 1\nDELIMITER ;;\nSELECT 2;;",
			want:  []string{"SELECT 1", "SELECT 2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SplitStatements(tc.query)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SplitStatements(%q) = %#v, want %#v", tc.query, got, tc.want)
			}
		})
	}
}
