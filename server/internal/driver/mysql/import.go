package mysql

import (
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

// errExcerptLen caps SQL text embedded in an import error.
const errExcerptLen = 100

// maxDecompressedInputMB bounds gzip output to the API upload size.
const maxDecompressedInputMB = 512

// maxDecompressedInput is replaceable by the existing small-limit unit tests.
var maxDecompressedInput int64 = maxDecompressedInputMB << 20

// ImportSQL reads and executes one statement at a time, including DELIMITER
// bodies. Memory is bounded by the largest statement. Progress reports source
// bytes with total=0 until EOF, followed by (N,N). Earlier statements remain
// applied if input parsing, decompression or execution later fails.
func (d *MysqlDriver) ImportSQL(ctx context.Context, db string, r io.Reader, progress func(done, total int64)) error {
	if db == "" {
		return fmt.Errorf("import sql: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	input, err := sqlgen.OpenSQLImport(r, maxDecompressedInput, false)
	if err != nil {
		return fmt.Errorf("import sql: read input: %w", err)
	}
	defer func() { _ = input.Close() }()
	scanner := sqlgen.NewStatementStream(input)
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("import sql: acquire connection: %w", err)
	}
	defer discardConnection(conn)
	if db != d.loginDB {
		if _, err := conn.ExecContext(ctx, "USE "+sqlgen.QuoteIdent(db)); err != nil {
			return fmt.Errorf("import sql: switch to database %s: %w", db, err)
		}
	}
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("import sql: after %d statements: %w", count, err)
		}
		stmt, err := scanner.Next()
		if err == io.EOF {
			break
		}
		if errors.Is(err, sqlgen.ErrSQLImportTooLarge) {
			return fmt.Errorf("import sql: decompressed dump exceeds %dMB: %w", maxDecompressedInputMB, err)
		}
		if err != nil {
			return fmt.Errorf("import sql: read statement %d: %w", count+1, err)
		}
		count++
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("import sql: statement %d: %s: %w", count, excerpt(stmt), err)
		}
		if progress != nil {
			progress(scanner.Offset(), 0)
		}
	}
	if count == 0 {
		return fmt.Errorf("import sql: empty script")
	}
	if progress != nil {
		progress(scanner.Offset(), scanner.Offset())
	}
	return nil
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
