package mysql

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"
)

// withDecompressedCap swaps maxDecompressedInput for capBytes for the
// duration of the test (the seam that keeps the bomb-guard unit-testable
// without building a 512 MiB stream).
func withDecompressedCap(t *testing.T, capBytes int64) {
	t.Helper()
	old := maxDecompressedInput
	maxDecompressedInput = capBytes
	t.Cleanup(func() { maxDecompressedInput = old })
}

// gzipped compresses s into a gzip stream.
func gzipped(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(s)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// TestReadImportInput_Plain passes plain (non-gzip) input through
// untouched, whatever the cap is.
func TestReadImportInput_Plain(t *testing.T) {
	withDecompressedCap(t, 8)
	got, err := readImportInput(strings.NewReader("INSERT INTO t VALUES (1);"))
	if err != nil {
		t.Fatalf("readImportInput error: %v", err)
	}
	if got != "INSERT INTO t VALUES (1);" {
		t.Errorf("readImportInput = %q, want the input verbatim", got)
	}
}

// TestReadImportInput_GzipUnderCap decompresses a gzip stream at exactly
// the cap: it must come through in full (LimitReader is cap+1, so the
// boundary is not an error).
func TestReadImportInput_GzipUnderCap(t *testing.T) {
	const script = "INSERT INTO t VALUES (1); INSERT INTO t VALUES (2);"
	withDecompressedCap(t, int64(len(script)))
	got, err := readImportInput(bytes.NewReader(gzipped(t, script)))
	if err != nil {
		t.Fatalf("readImportInput error: %v", err)
	}
	if got != script {
		t.Errorf("readImportInput = %q, want the decompressed script", got)
	}
}

// TestReadImportInput_GzipOverCap rejects a decompression bomb: a gzip
// stream that decompresses past the cap errors with the bomb message
// instead of buffering the whole payload.
func TestReadImportInput_GzipOverCap(t *testing.T) {
	withDecompressedCap(t, 64)
	// 128 zero bytes decompress to double the 64-byte cap.
	bomb := gzipped(t, strings.Repeat("\x00", 128))
	_, err := readImportInput(bytes.NewReader(bomb))
	if err == nil {
		t.Fatal("readImportInput over the cap: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "decompressed dump exceeds 512MB") {
		t.Errorf("error = %v, want the decompressed-size message (text pinned to the production cap)", err)
	}
}

// TestReadImportInput_GzipError propagates a corrupt gzip stream's error
// (a gzip magic header that fails to open).
func TestReadImportInput_GzipError(t *testing.T) {
	_, err := readImportInput(io.MultiReader(bytes.NewReader([]byte{gzipMagic0, gzipMagic1}),
		strings.NewReader("not a gzip stream")))
	if err == nil {
		t.Fatal("readImportInput on corrupt gzip: expected error, got nil")
	}
}
