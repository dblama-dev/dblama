package mysql

import (
	"context"
	"encoding/csv"
	"encoding/hex"
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

// ImportCSV imports delimited rows in parameterized batches. Nil Columns
// retains ordinal mapping and legacy empty-field-to-NULL conversion. Explicit
// mapping permits reordering/ignored fields, keeps empty strings and decodes
// \N as NULL (leading doubled backslashes escape a literal backslash).
// Double quotes are the supported enclosure, including when left unspecified.
// Progress reports only successfully applied batches, total=0 until EOF.
// Earlier batches remain applied if parsing, execution or cancellation fails.
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
	fieldCount := len(columns)
	if opts.Columns != nil {
		fieldCount = len(opts.Columns)
	}
	columns, indexes, err := driver.CSVImportColumns(columns, opts.Columns)
	if err != nil {
		return err
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
	cr.FieldsPerRecord = fieldCount

	if opts.SkipHeader {
		if _, err := cr.Read(); err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("import csv: empty input")
			}
			return fmt.Errorf("import csv: header: %w", err)
		}
	}

	args := make([]any, 0, csvBatchSize*len(names))
	committed := 0
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
		committed = lastRow
		if progress != nil {
			progress(int64(committed), 0)
		}
		args = args[:0]
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("import csv: %w", err)
		}
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
		for i, source := range indexes {
			f := rec[source]
			if opts.Columns != nil {
				if f == `\N` {
					args = append(args, nil)
				} else {
					if strings.HasPrefix(f, `\\`) {
						f = f[1:]
					}
					if isBinaryTypeName(columns[i].Type) && strings.HasPrefix(f, "0x") {
						binary, err := hex.DecodeString(f[2:])
						if err != nil {
							return fmt.Errorf("import csv: row %d column %s: invalid binary hex: %w", nrows, columns[i].Name, err)
						}
						args = append(args, binary)
						continue
					}
					args = append(args, f)
				}
				continue
			}
			if f == "" {
				args = append(args, nil)
			} else {
				args = append(args, f)
			}
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
	if err := flush(nrows); err != nil {
		return err
	}
	if progress != nil {
		progress(int64(committed), int64(committed))
	}
	return nil
}

// compile-time check lives in mysql.go (var _ driver.Driver).
