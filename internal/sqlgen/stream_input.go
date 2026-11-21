package sqlgen

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
)

// ErrSQLImportTooLarge marks an input exceeding the import byte limit.
var ErrSQLImportTooLarge = errors.New("SQL import exceeds size limit")

// OpenSQLImport detects gzip by magic bytes and decompresses on demand. Gzip
// output is always bounded by maxBytes; capPlain also bounds uncompressed input.
// Closing the returned reader never closes the caller-owned source.
func OpenSQLImport(r io.Reader, maxBytes int64, capPlain bool) (io.ReadCloser, error) {
	if maxBytes < 0 {
		return nil, fmt.Errorf("negative SQL input limit")
	}
	if r == nil {
		return nil, fmt.Errorf("nil SQL input reader")
	}
	br := bufio.NewReaderSize(r, 64*1024)
	magic, err := br.Peek(2)
	if err != nil && err != io.EOF {
		return nil, err
	}
	src := io.NopCloser(br)
	compressed := len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b
	if compressed {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return nil, err
		}
		src = gz
	}
	if compressed || capPlain {
		return &boundedSQLImport{ReadCloser: src, remaining: maxBytes}, nil
	}
	return src, nil
}

type boundedSQLImport struct {
	io.ReadCloser
	remaining int64
	failed    bool
}

func (r *boundedSQLImport) Read(p []byte) (int, error) {
	if r.failed {
		return 0, ErrSQLImportTooLarge
	}
	if len(p) == 0 {
		return 0, nil
	}
	if int64(len(p)) > r.remaining+1 {
		p = p[:r.remaining+1]
	}
	n, err := r.ReadCloser.Read(p)
	if int64(n) > r.remaining {
		r.failed = true
		return 0, ErrSQLImportTooLarge
	}
	r.remaining -= int64(n)
	return n, err
}
