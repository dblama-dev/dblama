package postgres

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// errExcerptLen caps the failing statement's text in an import error.
const errExcerptLen = 100

// maxDecompressedInputMB bounds plain and decompressed SQL input.
const maxDecompressedInputMB = 512

// ImportSQL incrementally executes SQL statements on one dedicated connection.
// Dollar-quoted functions and nested comments are kept intact. The current
// schema is selected only on that connection, which is discarded afterwards.
// Progress total stays unknown (0) until the final (N,N) callback.
func (d *PostgresDriver) ImportSQL(ctx context.Context, db string, r io.Reader, progress func(done, total int64)) error {
	if db == "" {
		return fmt.Errorf("import sql: %w", sqlgen.ErrEmptyIdentifier)
	}
	if db != d.loginDB {
		return fmt.Errorf("import sql: database %s is not the login database %s", db, d.loginDB)
	}
	input, err := sqlgen.OpenSQLImport(r, int64(maxDecompressedInputMB)<<20, true)
	if err != nil {
		return fmt.Errorf("import sql: read input: %w", err)
	}
	defer func() { _ = input.Close() }()
	scanner := sqlgen.NewPostgresStatementStream(input)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("import sql: conn: %w", err)
	}
	defer discardConnection(conn)
	if _, err := conn.ExecContext(ctx, "SET search_path TO "+sqlgen.QuoteIdentPg(d.schemaFor(ctx))); err != nil {
		return fmt.Errorf("import sql: set search_path: %w", err)
	}
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("import sql: cancelled after %d statements: %w", count, err)
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
			return fmt.Errorf("import sql: statement %d (%s): %w", count, strings.TrimSpace(stmt[:min(len(stmt), errExcerptLen)]), err)
		}
		if progress != nil {
			progress(scanner.Offset(), 0)
		}
	}
	if count == 0 {
		return fmt.Errorf("import sql: no statements in the input")
	}
	if progress != nil {
		progress(scanner.Offset(), scanner.Offset())
	}
	return nil
}

// ImportCSV loads a delimited file into table in the current schema:
// batches of csvBatchSize rows inserted as one multi-row INSERT; an
// \N field becomes SQL NULL, empty stays empty, leading backslashes
// are escaped. The first failing batch aborts with
// its row range, earlier batches stay applied (one statement each).
func (d *PostgresDriver) ImportCSV(ctx context.Context, db, table string, r io.Reader, opts driver.CSVImportOpts, progress func(done, total int64)) error {
	if opts.CSVEncoding != "" && opts.CSVEncoding != "escaped" && opts.CSVEncoding != "literal" {
		return fmt.Errorf("import csv: invalid csvEncoding %q", opts.CSVEncoding)
	}

	if db == "" || table == "" {
		return fmt.Errorf("import csv: %w", sqlgen.ErrEmptyIdentifier)
	}
	if db != d.loginDB {
		return fmt.Errorf("import csv: database %s is not the login database %s", db, d.loginDB)
	}
	if opts.Enclosure != "" && opts.Enclosure != `"` {
		return fmt.Errorf("import csv: unsupported enclosure %q", opts.Enclosure)
	}
	sep, err := csvSeparator(opts.Separator)
	if err != nil {
		return fmt.Errorf("import csv: %w", err)
	}

	schema := d.schemaFor(ctx)
	columns, err := d.selectColumns(ctx, schema, table, nil)
	if err != nil {
		return fmt.Errorf("import csv: %w", err)
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

	cr := csv.NewReader(r)
	cr.Comma = sep
	cr.FieldsPerRecord = fieldCount
	if opts.SkipHeader {
		if _, err := cr.Read(); err != nil {
			return fmt.Errorf("import csv: header: %w", err)
		}
	}

	qual, err := qualify(schema, table)
	if err != nil {
		return fmt.Errorf("import csv: %w", err)
	}

	committed := 0
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
		committed = nrows
		if progress != nil {
			progress(int64(committed), 0)
		}
		args = args[:0]
		batchFirst = 0
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("import csv: %w", err)
		}
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
		for i, source := range indexes {
			field := record[source]
			if opts.CSVEncoding == "literal" {
				args = append(args, field)
				continue
			}
			if field == `\N` {
				args = append(args, nil)
				continue
			}
			if strings.HasPrefix(field, `\\`) {
				field = field[1:]
			}
			if strings.HasSuffix(columns[i].Type, "[]") {
				if literal, ok := jsonToPgArray(field); ok {
					field = literal
				}
			}
			if columns[i].Type == "bytea" && strings.HasPrefix(field, "0x") {
				field = `\x` + field[2:]
			}
			args = append(args, field)
		}
		if len(args)/len(names) >= csvBatchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if nrows == 0 {
		return fmt.Errorf("import csv: empty input")
	}
	if err := flush(); err != nil {
		return err
	}
	if progress != nil {
		progress(int64(committed), int64(committed))
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
			fmt.Fprintf(&sb, "$%d", n)
		}
		sb.WriteString(")")
	}
	return sb.String(), nil
}
