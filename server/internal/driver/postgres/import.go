package postgres

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// The PG importer mirrors the MySQL one (mysql/import.go): the script is
// fully buffered (documented v0.1 limitation — a 512 MiB API cap bounds
// it; a chunk-safe streaming splitter is a recorded follow-up), gzip is
// detected by magic bytes, statements are split by sqlgen.SplitStatements
// and executed in order on a dedicated connection with the current schema
// set via search_path (allowed there — the connection is one-shot).

// errExcerptLen caps the failing statement's text in an import error.
const errExcerptLen = 100

// maxDecompressedInputMB is the decompressed-size cap (same 512 MiB the
// API layer caps the upload body at) — a decompression-bomb guard.
const maxDecompressedInputMB = 512

// ImportSQL executes an SQL script read from r against database db on a
// dedicated connection. PG dumps carry no DELIMITER directives, but the
// splitter is mysql-client compatible and handles any that appear.
func (d *PostgresDriver) ImportSQL(ctx context.Context, db string, r io.Reader, progress func(done, total int64)) error {
	if db == "" {
		return fmt.Errorf("import sql: %w", sqlgen.ErrEmptyIdentifier)
	}
	if db != d.loginDB {
		return fmt.Errorf("import sql: database %s is not the login database %s", db, d.loginDB)
	}
	script, err := readImportInput(r)
	if err != nil {
		return fmt.Errorf("import sql: %w", err)
	}
	stmts := sqlgen.SplitStatements(script)
	if len(stmts) == 0 {
		return fmt.Errorf("import sql: no statements in the input")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("import sql: conn: %w", err)
	}
	defer func() { _ = conn.Close() }()

	schema := d.CurrentSchema()
	if _, err := conn.ExecContext(ctx, "SET search_path TO "+sqlgen.QuoteIdentPg(schema)); err != nil {
		return fmt.Errorf("import sql: set search_path: %w", err)
	}

	pos := 0
	for i, stmt := range stmts {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("import sql: cancelled after %d statements: %w", i, err)
		}
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("import sql: statement %d (%s): %w",
				i+1, excerptAt(script, pos), err)
		}
		if idx := strings.Index(script[pos:], stmt); idx >= 0 {
			pos += idx + len(stmt)
		} else {
			pos = len(script)
		}
		pos = min(pos, len(script))
		if progress != nil {
			progress(int64(pos), int64(len(script)))
		}
	}
	if progress != nil {
		progress(int64(len(script)), int64(len(script)))
	}
	return nil
}

// readImportInput buffers the whole script, decompressing gzip (magic
// 0x1f 0x8b) through a decompressed-size cap.
func readImportInput(r io.Reader) (string, error) {
	br := bufio.NewReader(r)
	magic, err := br.Peek(2)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read input: %w", err)
	}
	var src io.Reader = br
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, gerr := gzip.NewReader(br)
		if gerr != nil {
			return "", fmt.Errorf("read input: gzip: %w", gerr)
		}
		defer func() { _ = gz.Close() }()
		src = io.LimitReader(gz, int64(maxDecompressedInputMB)*1024*1024+1)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, src); err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}
	script := buf.String()
	if int64(len(script)) > int64(maxDecompressedInputMB)*1024*1024 {
		return "", fmt.Errorf("read input: decompressed dump exceeds %dMB", maxDecompressedInputMB)
	}
	return script, nil
}

// excerptAt returns up to errExcerptLen bytes of script starting at pos.
func excerptAt(script string, pos int) string {
	end := min(pos+errExcerptLen, len(script))
	return strings.TrimSpace(script[pos:end])
}

// ImportCSV loads a delimited file into table in the current schema:
// batches of csvBatchSize rows inserted as one multi-row INSERT; an
// empty field becomes SQL NULL (the documented CSV convention — an
// empty string is not expressible); the first failing batch aborts with
// its row range, earlier batches stay applied (one statement each).
func (d *PostgresDriver) ImportCSV(ctx context.Context, db, table string, r io.Reader, opts driver.CSVImportOpts, progress func(done, total int64)) error {
	if db == "" || table == "" {
		return fmt.Errorf("import csv: %w", sqlgen.ErrEmptyIdentifier)
	}
	if db != d.loginDB {
		return fmt.Errorf("import csv: database %s is not the login database %s", db, d.loginDB)
	}
	sep, err := csvSeparator(opts.Separator)
	if err != nil {
		return fmt.Errorf("import csv: %w", err)
	}

	schema := d.CurrentSchema()
	columns, err := d.selectColumns(ctx, schema, table, nil)
	if err != nil {
		return fmt.Errorf("import csv: %w", err)
	}
	names := make([]string, len(columns))
	for i, c := range columns {
		names[i] = c.Name
	}

	cr := csv.NewReader(r)
	cr.Comma = sep
	cr.FieldsPerRecord = len(names)
	if opts.SkipHeader {
		if _, err := cr.Read(); err != nil {
			return fmt.Errorf("import csv: header: %w", err)
		}
	}

	qual, err := qualify(schema, table)
	if err != nil {
		return fmt.Errorf("import csv: %w", err)
	}

	nrows := 0
	var args []any
	batchFirst := 0
	flush := func() error {
		if len(args) == 0 {
			return nil
		}
		query, aerr := buildMultiInsertPg(qual, names, len(args)/len(names))
		if aerr != nil {
			return aerr
		}
		if _, err := d.db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("import csv: rows %d-%d: %w", batchFirst, nrows, err)
		}
		args = args[:0]
		batchFirst = 0
		return nil
	}

	for {
		record, err := cr.Read()
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
		for _, field := range record {
			if field == "" {
				args = append(args, nil)
			} else {
				args = append(args, field)
			}
		}
		if progress != nil {
			progress(int64(nrows), int64(nrows))
		}
		if len(args)/len(names) >= csvBatchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}
	if progress != nil {
		progress(int64(nrows), int64(nrows))
	}
	return nil
}

// csvSeparator maps the option to a rune; an empty value defaults to
// the comma, any multi-char or unsupported value fails before reading.
func csvSeparator(s string) (rune, error) {
	switch s {
	case "":
		return ',', nil
	case ",":
		return ',', nil
	case ";":
		return ';', nil
	case "\t":
		return '\t', nil
	default:
		return 0, fmt.Errorf("unsupported separator %q (want \",\", \";\" or tab)", s)
	}
}

// buildMultiInsertPg builds one INSERT with valueTuples tuples of the
// given columns: INSERT INTO "s"."t" ("a","b") VALUES ($1,$2),($3,$4).
func buildMultiInsertPg(qual string, cols []string, valueTuples int) (string, error) {
	if valueTuples <= 0 {
		return "", fmt.Errorf("import csv: empty batch")
	}
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = sqlgen.QuoteIdentPg(c)
	}
	var sb strings.Builder
	sb.WriteString("INSERT INTO " + qual + " (" + strings.Join(quoted, ",") + ") VALUES ")
	n := 0
	for t := 0; t < valueTuples; t++ {
		if t > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(")
		for i := range cols {
			if i > 0 {
				sb.WriteString(",")
			}
			n++
			sb.WriteString(fmt.Sprintf("$%d", n))
		}
		sb.WriteString(")")
	}
	return sb.String(), nil
}
