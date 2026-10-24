package dump

import (
	"strings"
	"testing"
)

// TestCSVWriter_Enclosure pins the minimal-quoting rule: a field is
// enclosed in double quotes only when it contains the separator, a
// double quote or a line break; inside an enclosed field a double quote
// is doubled. A field without any of those stays bare.
func TestCSVWriter_Enclosure(t *testing.T) {
	var sb strings.Builder
	w := NewCSVWriter(&sb, ',')
	if err := w.WriteHeader([]string{"id", "val"}); err != nil {
		t.Fatalf("WriteHeader() error: %v", err)
	}
	rows := [][]any{
		{int64(1), "plain"},                    // nothing to enclose
		{int64(2), "has,comma"},                // separator inside
		{int64(3), `has"quote`},                // quote inside
		{int64(4), "line1\nline2"},             // newline inside
		{int64(5), "line1\r\nline2"},           // carriage return inside
		{int64(6), `both,"and` + "\n" + `mix`}, // quote and newline inside
	}
	for i, row := range rows {
		if err := w.WriteRow(row); err != nil {
			t.Fatalf("WriteRow(%d) error: %v", i, err)
		}
	}

	want := strings.Join([]string{
		"id,val",
		"1,plain",
		`2,"has,comma"`,
		`3,"has""quote"`,
		`4,"line1` + "\n" + `line2"`,
		`5,"line1` + "\r\n" + `line2"`,
		"6,\"both,\"\"and" + "\n" + `mix"`,
	}, "\n") + "\n"
	if got := sb.String(); got != want {
		t.Errorf("CSVWriter output =\n%q\nwant\n%q", got, want)
	}
}

// TestCSVWriter_NullAndBinary pins the value rendering: SQL NULL (nil)
// becomes an empty field, a binary hex string ("0x…") passes through
// unquoted and unchanged, numbers render in their natural text form.
func TestCSVWriter_NullAndBinary(t *testing.T) {
	var sb strings.Builder
	w := NewCSVWriter(&sb, ',')
	if err := w.WriteHeader([]string{"bin", "n", "f", "s"}); err != nil {
		t.Fatalf("WriteHeader() error: %v", err)
	}
	if err := w.WriteRow([]any{nil, int64(42), 1.5, "0xDEADBEEF"}); err != nil {
		t.Fatalf("WriteRow() error: %v", err)
	}
	if err := w.WriteRow([]any{"0x", nil, nil, nil}); err != nil {
		t.Fatalf("WriteRow() error: %v", err)
	}

	want := "bin,n,f,s\n" +
		",42,1.5,0xDEADBEEF\n" +
		"0x,,,\n"
	if got := sb.String(); got != want {
		t.Errorf("CSVWriter output =\n%q\nwant\n%q", got, want)
	}
}

// TestCSVWriter_Separators pins the three delimited dialects: comma,
// semicolon and tab — and that only the dialect's own separator
// triggers enclosure (a comma never quotes a semicolon dialect row).
func TestCSVWriter_Separators(t *testing.T) {
	tests := []struct {
		name string
		sep  rune
		want string
	}{
		{"csv", ',', "a,b\n1,\"x,y\"\n"},
		{"csv_semicolon", ';', "a;b\n1;\"x;y\"\n"},
		{"tsv", '\t', "a\tb\n1\t\"x\ty\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sb strings.Builder
			w := NewCSVWriter(&sb, tt.sep)
			if err := w.WriteHeader([]string{"a", "b"}); err != nil {
				t.Fatalf("WriteHeader() error: %v", err)
			}
			// The comma-containing value must stay bare in the
			// semicolon and tab dialects; the value below carries
			// each dialect's own separator once.
			val := map[rune]string{',': "x,y", ';': "x;y", '\t': "x\ty"}[tt.sep]
			if err := w.WriteRow([]any{"1", val}); err != nil {
				t.Fatalf("WriteRow() error: %v", err)
			}
			if got := sb.String(); got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSeparator pins the format → separator mapping and that an
// unknown delimited format is rejected.
func TestSeparator(t *testing.T) {
	tests := []struct {
		format string
		sep    rune
		ok     bool
	}{
		{FormatCSV, ',', true},
		{FormatCSVSemicolon, ';', true},
		{FormatTSV, '\t', true},
		{"xml", 0, false},
		{"SQL", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		sep, err := Separator(tt.format)
		if tt.ok && err != nil {
			t.Errorf("Separator(%q) error: %v, want nil", tt.format, err)
			continue
		}
		if !tt.ok {
			if err == nil {
				t.Errorf("Separator(%q) = %q, want error", tt.format, sep)
			}
			continue
		}
		if sep != tt.sep {
			t.Errorf("Separator(%q) = %q, want %q", tt.format, sep, tt.sep)
		}
	}
}

// TestCSVValue pins the cell rendering of the value kinds a read page
// carries: nil → "", strings pass through (hex included), int64 and
// float64 render in their natural forms, bool as 1/0.
func TestCSVValue(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string", "abc", "abc"},
		{"hex string", "0x00FF10", "0x00FF10"},
		{"empty string", "", ""},
		{"int64", int64(-17), "-17"},
		{"float64", 2.25, "2.25"},
		{"bool true", true, "1"},
		{"bool false", false, "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CSVValue(tt.in); got != tt.want {
				t.Errorf("CSVValue(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
