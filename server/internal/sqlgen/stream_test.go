package sqlgen

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type tinySQLReader struct {
	text  string
	width int
}

func (r *tinySQLReader) Read(p []byte) (int, error) {
	if len(r.text) == 0 {
		return 0, io.EOF
	}
	n := min(len(p), r.width, len(r.text))
	copy(p, r.text[:n])
	r.text = r.text[n:]
	return n, nil
}
func collectStream(r io.Reader) ([]string, error) {
	s := NewStatementStream(r)
	var out []string
	for {
		stmt, err := s.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, stmt)
	}
}
func TestStatementStreamChunkBoundaries(t *testing.T) {
	script := "-- heading;\nDELIMITER $$\nCREATE PROCEDURE p() BEGIN SELECT 'a;''b'; SELECT 2; END$$\nDELIMITER ;\nSELECT 1--2; # comment\nSELECT `a``b`, \"x\\\";\"; /* semi; */ SELECT 4;"
	want := []string{"CREATE PROCEDURE p() BEGIN SELECT 'a;''b'; SELECT 2; END", "SELECT 1--2", "# comment\nSELECT `a``b`, \"x\\\";\"", "/* semi; */ SELECT 4"}
	for width := 1; width <= 17; width++ {
		got, err := collectStream(&tinySQLReader{text: script, width: width})
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("width %d: %#v %v", width, got, err)
		}
	}
}
func TestStatementStreamPostgresDollarQuotesAndNestedComments(t *testing.T) {
	script := "CREATE FUNCTION f() RETURNS text AS $tag$ BEGIN RETURN '; $$'; END; $tag$ LANGUAGE plpgsql; /* outer /* inner; */ still ; */ SELECT $1, '$2'; SELECT $$a;b$$;"
	want := []string{"CREATE FUNCTION f() RETURNS text AS $tag$ BEGIN RETURN '; $$'; END; $tag$ LANGUAGE plpgsql", "/* outer /* inner; */ still ; */ SELECT $1, '$2'", "SELECT $$a;b$$"}
	for width := 1; width < 12; width++ {
		got, err := collectStream(&tinySQLReader{text: script, width: width})
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("width %d: %#v %v", width, got, err)
		}
	}
}
func TestStatementStreamRejectsIncompleteFinalLexicalRegions(t *testing.T) {
	for _, tail := range []string{"SELECT 'unfinished", "SELECT \"unfinished", "SELECT `unfinished", "SELECT $$unfinished", "SELECT $tag$unfinished", "SELECT 1 /* unfinished", "SELECT 1 /* outer /* inner */"} {
		got, err := collectStream(&tinySQLReader{text: "SELECT 7;" + tail, width: 1})
		if err == nil || !reflect.DeepEqual(got, []string{"SELECT 7"}) {
			t.Fatalf("incomplete %q executed: %#v %v", tail, got, err)
		}
	}
}
func TestStatementStreamDoesNotReadWholeScriptBeforeFirstStatement(t *testing.T) {
	r := &tinySQLReader{text: "SELECT 1;" + strings.Repeat("SELECT 2;", 100000), width: 1}
	s := NewStatementStream(r)
	stmt, err := s.Next()
	if stmt != "SELECT 1" || err != nil {
		t.Fatalf("%q %v", stmt, err)
	}
	if len(r.text) < 800000 {
		t.Fatal("stream buffered the script")
	}
	if s.Offset() != 9 {
		t.Fatalf("offset=%d", s.Offset())
	}
}

type failingSQLReader struct{}

func (failingSQLReader) Read([]byte) (int, error) { return 0, errors.New("broken input") }
func TestStatementStreamPropagatesReaderError(t *testing.T) {
	_, err := NewStatementStream(failingSQLReader{}).Next()
	if err == nil || !strings.Contains(err.Error(), "broken input") {
		t.Fatalf("reader error=%v", err)
	}
}

func TestPostgresStatementStreamCommentAndStringRules(t *testing.T) {
	script := `SELECT '{}'::jsonb #> '{a}'; --comment without space;
SELECT 'backslash\'; SELECT E'escape\';still'; SELECT "name\";`
	s := NewPostgresStatementStream(&tinySQLReader{text: script, width: 1})
	var got []string
	for {
		stmt, err := s.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, stmt)
	}
	want := []string{`SELECT '{}'::jsonb #> '{a}'`, `--comment without space;
SELECT 'backslash\'`, `SELECT E'escape\';still'`, `SELECT "name\"`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PG lexical rules: %#v", got)
	}
}
