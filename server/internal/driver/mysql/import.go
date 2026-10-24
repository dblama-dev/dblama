package mysql

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// progressChunk is the read quantum of the input-consuming loop (the
// script is buffered in chunks of this size; progress is reported per
// executed statement, not per read chunk — see ImportSQL).
const progressChunk = 64 * 1024

// errExcerptLen caps the failing statement's text embedded in an import
// error.
const errExcerptLen = 100

// gzip magic bytes (RFC 1952): every gzip stream starts with 0x1f 0x8b.
const (
	gzipMagic0 = 0x1f
	gzipMagic1 = 0x8b
)

// ImportSQL executes an SQL script read from r against database db on a
// dedicated connection and reports byte progress. See the driver.Driver
// contract for the full semantics; the implementation pins are:
//
//   - the whole stream is read into memory once (an SQL script is not
//     chunk-executable: a statement may straddle any read boundary), so
//     the memory footprint is the script size; the gzip magic 1f 8b is
//     detected before reading, so .gz dumps decompress transparently;
//   - statements are split by sqlgen.SplitStatements — mysql-client
//     semantics with DELIMITER support, so a DELIMITER-wrapped
//     trigger/routine block runs as the single statement it must be —
//     and executed in order through ExecContext on one dedicated
//     connection; db other than the login database is selected via USE
//     on that connection (the ExecSQL mechanics). Result rows, if any,
//     are discarded;
//   - progress: done is the byte span of the script consumed by
//     executed statements, total is the full script size. The stream is
//     consumed before execution (its size is unknown until it ends, and
//     a read-phase done would regress to 0 when execution starts, so
//     progress reports execution position only); progress(done=pos,
//     total=N) is called at every statement boundary and the final call
//     reports done == total. A nil progress is allowed;
//   - the first failing statement aborts the import: statements before
//     it have been applied and are not rolled back, the error carries
//     the statement's 1-based index and its first errExcerptLen
//     characters, and the progress position stands at the failing
//     statement's start.
func (d *MysqlDriver) ImportSQL(ctx context.Context, db string, r io.Reader, progress func(done, total int64)) error {
	if db == "" {
		return fmt.Errorf("import sql: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if r == nil {
		return fmt.Errorf("import sql: nil reader")
	}

	script, err := readImportInput(r)
	if err != nil {
		return fmt.Errorf("import sql: read input: %w", err)
	}
	if strings.TrimSpace(script) == "" {
		return fmt.Errorf("import sql: empty script")
	}

	conn, err := d.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("import sql: acquire connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if db != d.loginDB {
		// The pool is bound to the login database; point the dedicated
		// connection at the target database first (the ExecSQL
		// mechanics), so the dump's unqualified statements land there.
		if _, err := conn.ExecContext(ctx, "USE "+sqlgen.QuoteIdent(db)); err != nil {
			return fmt.Errorf("import sql: switch to database %s: %w", db, err)
		}
	}

	statements := sqlgen.SplitStatements(script)
	pos := 0 // bytes of script consumed by executed statements
	for i, stmt := range statements {
		if progress != nil {
			progress(int64(pos), int64(len(script)))
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("import sql: statement %d: %w", i+1, err)
		}
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("import sql: statement %d: %s: %w", i+1, excerpt(stmt), err)
		}
		// Locate the executed statement's span so done tracks the
		// script position, not just a statement counter. Splitting
		// never alters statement text, so the text is always found;
		// the fallback keeps progress monotonic even then.
		if j := strings.Index(script[pos:], stmt); j >= 0 {
			pos += j + len(stmt)
		} else {
			pos += len(stmt)
		}
	}
	if progress != nil {
		progress(int64(len(script)), int64(len(script)))
	}
	return nil
}

// readImportInput consumes r once and returns the script text. A gzip
// stream (magic 1f 8b) is decompressed transparently. No progress is
// reported here: the stream's size is unknown until it ends and a
// read-phase done would regress to 0 when execution starts (see
// ImportSQL).
func readImportInput(r io.Reader) (string, error) {
	br := bufio.NewReader(r)
	src := io.Reader(br)
	if magic, err := br.Peek(2); err == nil && magic[0] == gzipMagic0 && magic[1] == gzipMagic1 {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return "", err
		}
		defer func() { _ = gz.Close() }()
		src = gz
	}

	var buf bytes.Buffer
	chunk := make([]byte, progressChunk)
	for {
		n, rerr := src.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	return buf.String(), nil
}

// excerpt returns s's first errExcerptLen characters for error context.
func excerpt(s string) string {
	if len(s) > errExcerptLen {
		return s[:errExcerptLen] + "…"
	}
	return s
}

// compile-time check lives in mysql.go (var _ driver.Driver).
