package mysql

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/hitslab/db-manager/server/internal/driver"
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

// maxDecompressedInputMB is the decompressed-size cap in megabytes —
// the same 512 MiB the API layer caps the upload body at. A small .gz
// can decompress to gigabytes (a decompression bomb), and the script is
// fully buffered, so the gzip stream is read through a LimitReader of
// cap+1 and an input that exceeds the cap is rejected.
const maxDecompressedInputMB = 512

// maxDecompressedInput is maxDecompressedInputMB in bytes. A var only so
// the cap logic can be unit-tested with a tiny value (the error text
// stays pinned to the production cap); production code must not write it.
var maxDecompressedInput int64 = maxDecompressedInputMB << 20

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
// stream (magic 1f 8b) is decompressed transparently, capped at
// maxDecompressedInput so a decompression bomb cannot exhaust memory.
// No progress is reported here: the stream's size is unknown until it
// ends and a read-phase done would regress to 0 when execution starts
// (see ImportSQL).
func readImportInput(r io.Reader) (string, error) {
	br := bufio.NewReader(r)
	src := io.Reader(br)
	gzipped := false
	if magic, err := br.Peek(2); err == nil && magic[0] == gzipMagic0 && magic[1] == gzipMagic1 {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return "", err
		}
		defer func() { _ = gz.Close() }()
		// cap+1: reading one byte past the cap is what tells "over the
		// limit" apart from "exactly at it".
		src = io.LimitReader(gz, maxDecompressedInput+1)
		gzipped = true
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
	if gzipped && int64(buf.Len()) > maxDecompressedInput {
		return "", fmt.Errorf("decompressed dump exceeds %dMB", maxDecompressedInputMB)
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

// csvBatchSize is the row count of one multi-row INSERT statement of the
// CSV import — the same batch size the dump writer streams in.
const csvBatchSize = 100

// progressRowStride is the row stride of ImportCSV's progress reports:
// per-row callbacks on a big file would flood the NDJSON stream (and the
// browser painting it), so progress is reported every 1000th row plus
// the final row (done == total on success).
const progressRowStride = 1000

// ImportCSV imports delimited rows read from r into table in database db
// and reports row progress. See the driver.Driver contract for the full
// semantics; the implementation pins are:
//
//   - the fields of every record map onto the table's columns in ordinal
//     order (read from information_schema, so a missing table fails
//     before anything is read) and every record must carry exactly the
//     table's column count — encoding/csv enforces it from the first
//     record on; values are always bound as parameters (no SQL is ever
//     built from the file's content);
//   - an empty field becomes SQL NULL, any other field is bound as a
//     string and coerced server-side (MySQL numeric/date semantics);
//   - rows are written in multi-row INSERT batches of csvBatchSize, the
//     remainder flushed at EOF; a batch is one statement, so on a
//     transactional table it lands atomically. The first failing batch
//     aborts the import: earlier batches stay applied, the error names
//     the batch's first row number (1-based, header excluded);
//   - options: Separator is the encoding/csv field separator (default
//     ","), Enclosure is the field-enclosing character — encoding/csv
//     quotes with "\"" only, so an empty enclosure means the standard
//     "\"" semantics still apply and any other character fails before
//     reading; SkipHeader discards the first record;
//   - progress: the row count is unknown until the stream ends, so
//     done tracks rows read: progress(done=n, total=n) is reported
//     every 1000th row (progressRowStride) plus the final row — per-row
//     callbacks would flood the progress stream — and the final count
//     is the number of imported rows; a nil progress is allowed. An
//     input with no data rows (nothing at all, or only the
//     skipped header) is an error, mirroring ImportSQL's empty script.
func (d *MysqlDriver) ImportCSV(ctx context.Context, db, table string, r io.Reader, opts driver.CSVImportOpts, progress func(done, total int64)) error {
	if db == "" {
		return fmt.Errorf("import csv: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return fmt.Errorf("import csv: %w: table name", sqlgen.ErrEmptyIdentifier)
	}
	if r == nil {
		return fmt.Errorf("import csv: nil reader")
	}

	sep := ","
	if opts.Separator != "" {
		if utf8.RuneCountInString(opts.Separator) != 1 {
			return fmt.Errorf("import csv: separator %q must be a single character", opts.Separator)
		}
		sep = opts.Separator
	}
	// encoding/csv quotes with the double quote only; anything else the
	// caller names cannot be honored, so it fails before reading.
	if opts.Enclosure != "" && opts.Enclosure != `"` {
		return fmt.Errorf("import csv: enclosure %q is not supported: only \" or none", opts.Enclosure)
	}

	columns, err := d.selectColumns(ctx, db, table, nil)
	if err != nil {
		return fmt.Errorf("import csv into %s.%s: %w", db, table, err)
	}
	names := make([]string, len(columns))
	for i, c := range columns {
		names[i] = c.Name
	}

	base, _, err := sqlgen.BuildInsert(table, names)
	if err != nil {
		return fmt.Errorf("import csv into %s.%s: %w", db, table, err)
	}
	// Qualify once on the single-tuple statement; the per-batch tuples
	// are appended after it (they never touch the table reference).
	qualified, err := qualifyWrite("INSERT INTO ", base, db, table)
	if err != nil {
		return fmt.Errorf("import csv into %s.%s: %w", db, table, err)
	}
	tuple := "(" + strings.TrimSuffix(strings.Repeat("?,", len(names)), ",") + ")"

	cr := csv.NewReader(r)
	cr.Comma = []rune(sep)[0]
	cr.FieldsPerRecord = len(names)

	if opts.SkipHeader {
		if _, err := cr.Read(); err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("import csv: empty input")
			}
			return fmt.Errorf("import csv: header: %w", err)
		}
	}

	args := make([]any, 0, csvBatchSize*len(names))
	nrows := 0      // data rows read so far
	batchFirst := 0 // 1-based row number of the current batch's first row
	flush := func(lastRow int) error {
		if len(args) == 0 {
			return nil
		}
		tuples := len(args) / len(names)
		query := qualified
		if tuples > 1 {
			query += strings.Repeat(", "+tuple, tuples-1)
		}
		if _, err := d.db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("import csv: rows %d-%d: %w", batchFirst, lastRow, err)
		}
		args = args[:0]
		return nil
	}

	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("import csv: row %d: %w", nrows+1, err)
		}
		nrows++
		if batchFirst == 0 {
			batchFirst = nrows
		}
		for _, f := range rec {
			if f == "" {
				args = append(args, nil)
			} else {
				args = append(args, f)
			}
		}
		if progress != nil && nrows%progressRowStride == 0 {
			progress(int64(nrows), int64(nrows))
		}
		if nrows-batchFirst+1 == csvBatchSize {
			if err := flush(nrows); err != nil {
				return err
			}
			batchFirst = 0
		}
	}
	if nrows == 0 {
		return fmt.Errorf("import csv: empty input")
	}
	// The final row always reports, even when it is not on the stride —
	// the done == total contract of a successful import.
	if progress != nil && nrows%progressRowStride != 0 {
		progress(int64(nrows), int64(nrows))
	}
	if err := flush(nrows); err != nil {
		return err
	}
	return nil
}

// compile-time check lives in mysql.go (var _ driver.Driver).
