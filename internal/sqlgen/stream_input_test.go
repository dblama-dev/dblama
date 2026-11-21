package sqlgen

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestSQLImportReaderCompressionAndLimit(t *testing.T) {
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write([]byte("SELECT 1;")); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		input     io.Reader
		limit     int64
		wantError bool
	}{
		{"gzip exact", bytes.NewReader(compressed.Bytes()), 9, false},
		{"gzip over", bytes.NewReader(compressed.Bytes()), 8, true},
		{"plain exact", strings.NewReader("SELECT 1;"), 9, false},
		{"plain over", strings.NewReader("SELECT 1;"), 8, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := OpenSQLImport(tc.input, tc.limit, true)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = r.Close() }()
			got, err := io.ReadAll(r)
			if tc.wantError {
				if !errors.Is(err, ErrSQLImportTooLarge) {
					t.Fatalf("limit error=%v", err)
				}
			} else if err != nil || string(got) != "SELECT 1;" {
				t.Fatalf("input=%q err=%v", got, err)
			}
		})
	}
}
