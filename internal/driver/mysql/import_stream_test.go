package mysql

import (
	"errors"
	"fmt"
	"github.com/dblama-dev/dblama/internal/sqlgen"
	"io"
)

const (
	gzipMagic0 = 0x1f
	gzipMagic1 = 0x8b
)

// Legacy input fixtures now exercise the same incremental reader as ImportSQL.
// Buffering here belongs only to these small test fixtures.
func readImportInput(r io.Reader) (string, error) {
	input, err := sqlgen.OpenSQLImport(r, maxDecompressedInput, false)
	if err != nil {
		return "", err
	}
	defer func() { _ = input.Close() }()
	data, err := io.ReadAll(input)
	if errors.Is(err, sqlgen.ErrSQLImportTooLarge) {
		return "", fmt.Errorf("decompressed dump exceeds %dMB: %w", maxDecompressedInputMB, err)
	}
	return string(data), err
}
