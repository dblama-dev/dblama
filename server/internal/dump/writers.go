// Package dump holds the export format helpers shared by the driver
// implementations: the delimited-text (CSV/TSV) writer and the value
// rendering rules of the delimited dialects. The SQL format is written
// by the driver itself; this package only carries what is format
// plumbing rather than database access. Everything here is pure text
// transformation over an io.Writer — nothing buffers a whole dump.
package dump

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// The export formats. The delimited trio shares one writer keyed by its
// separator; FormatSQL is driver-side.
const (
	FormatSQL          = "sql"
	FormatCSV          = "csv"
	FormatCSVSemicolon = "csv_semicolon"
	FormatTSV          = "tsv"
)

// Separator maps a delimited format to its field separator. A format
// outside the delimited trio yields an error — the caller uses it to
// validate the format before any byte is written.
func Separator(format string) (rune, error) {
	switch format {
	case FormatCSV:
		return ',', nil
	case FormatCSVSemicolon:
		return ';', nil
	case FormatTSV:
		return '\t', nil
	default:
		return 0, fmt.Errorf("dump: unknown delimited format %q", format)
	}
}

// CSVWriter writes delimited text: a header row (the caller always
// writes one first) and one row per call, with minimal quoting — a
// field is enclosed in double quotes only when it contains the
// separator, a double quote or a line break; inside an enclosed field a
// double quote is doubled. Lines end with "\n". Every row is a single
// Write on the underlying writer, so nothing but the current row is
// ever held in memory.
type CSVWriter struct {
	w   io.Writer
	sep rune
}

// NewCSVWriter returns a writer emitting the given dialect over w.
func NewCSVWriter(w io.Writer, sep rune) *CSVWriter {
	return &CSVWriter{w: w, sep: sep}
}

// WriteHeader writes one line of column names.
func (c *CSVWriter) WriteHeader(names []string) error {
	return c.writeLine(names)
}

// WriteRow writes one data line; the values go through CSVValue.
func (c *CSVWriter) WriteRow(values []any) error {
	fields := make([]string, len(values))
	for i, v := range values {
		fields[i] = CSVValue(v)
	}
	return c.writeLine(fields)
}

// writeLine renders and writes one line, separator logic included.
func (c *CSVWriter) writeLine(fields []string) error {
	var sb strings.Builder
	for i, f := range fields {
		if i > 0 {
			sb.WriteRune(c.sep)
		}
		sb.WriteString(quoteField(f, c.sep))
	}
	sb.WriteByte('\n')
	_, err := io.WriteString(c.w, sb.String())
	if err != nil {
		return fmt.Errorf("dump: write row: %w", err)
	}
	return nil
}

// quoteField encloses field in double quotes when it contains the
// separator, a double quote or a line break, doubling inner quotes.
func quoteField(field string, sep rune) string {
	if !strings.ContainsRune(field, sep) &&
		!strings.ContainsAny(field, "\"\n\r") {
		return field
	}
	return `"` + strings.ReplaceAll(field, `"`, `""`) + `"`
}

// CSVValue renders one cell value as delimited text, over the value
// kinds a read page carries: nil (SQL NULL) → "", strings pass through
// unchanged (binary columns arrive as "0x…" hex strings and stay as
// they are), int64 and float64 in their natural text form, bool as
// 1/0, time.Time as RFC3339. Everything else falls back to its default
// string form.
func CSVValue(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64)
	case bool:
		if val {
			return "1"
		}
		return "0"
	case time.Time:
		return val.Format(time.RFC3339)
	default:
		return fmt.Sprint(v)
	}
}

// EscapedCSVRow encodes the reversible CSV value convention: NULL is \N,
// empty text stays empty, and literal leading backslashes gain one backslash.
// Binary values already arrive as 0x hex strings from the driver's row reader.
func EscapedCSVRow(row []any) []any {
	values := append([]any(nil), row...)
	for i, v := range values {
		if v == nil {
			values[i] = `\N`
		} else if text, ok := v.(string); ok && strings.HasPrefix(text, `\`) {
			values[i] = `\` + text
		}
	}
	return values
}
