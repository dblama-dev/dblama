package postgres

import (
	"encoding/json"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/dump"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

const exportBatchSize = 100

// ShowCreateTable generates the table's CREATE statement from the
// catalogs: the read TableDef (task 2) is re-rendered through the same
// buildCreateTable the save path uses, so a dump round-trips through
// this driver's own importer by construction.
func (d *PostgresDriver) ShowCreateTable(ctx context.Context, db, table string) (string, error) {
	def, err := d.TableDef(ctx, db, table)
	if err != nil {
		return "", fmt.Errorf("show create table: %w", err)
	}
	stmts, err := buildCreateTableQualified(sqlgen.QuoteIdentPg(table), driver.TableDef{
		NewName:     table,
		Columns:     def.Columns,
		Indexes:     def.Indexes,
		ForeignKeys: def.ForeignKeys,
		Options:     def.Options,
	})
	if err != nil {
		return "", fmt.Errorf("show create table: %w", err)
	}
	return strings.Join(stmts, "\n"), nil
}

// Export streams a dump of database db (the pool's login database; the
// handler validates the requested db equals it) to w in req.Format.
// Views are not exported in v0.1 (spec §7 narrowing): base tables only.
// A single-column primary key pages by keyset; any other shape pages by
// LIMIT/OFFSET (documented caveat). Nothing but the current row and
// statement is ever buffered.
func (d *PostgresDriver) Export(ctx context.Context, db string, req driver.ExportRequest, w io.Writer) error {
	if db == "" {
		return fmt.Errorf("export: no database to dump")
	}
	if db != d.loginDB {
		return fmt.Errorf("export: database %s is not the login database %s", db, d.loginDB)
	}
	var sep rune
	isSQL := req.Format == dump.FormatSQL
	if !isSQL {
		s, err := dump.Separator(req.Format)
		if err != nil {
			return fmt.Errorf("export: unknown format %q", req.Format)
		}
		sep = s
	}

	var tables []string
	switch {
	case isSQL && len(req.Tables) == 0:
		infos, err := d.ListTables(ctx, db)
		if err != nil {
			return fmt.Errorf("export: list tables of %s: %w", db, err)
		}
		for _, ti := range infos {
			if !ti.IsView {
				tables = append(tables, ti.Name)
			}
		}
	case isSQL:
		tables = req.Tables
	default:
		if len(req.Tables) != 1 {
			return fmt.Errorf("export: format %q requires exactly one table, got %d", req.Format, len(req.Tables))
		}
		tables = req.Tables
	}
	for _, table := range tables {
		if table == "" {
			return fmt.Errorf("export: %w: table name", sqlgen.ErrEmptyIdentifier)
		}
	}

	out := w
	closeOut := func() error { return nil }
	if req.Gzip {
		gz := gzip.NewWriter(w)
		out = gz
		closeOut = func() error {
			if err := gz.Close(); err != nil {
				return fmt.Errorf("export: close gzip stream: %w", err)
			}
			return nil
		}
	}

	var err error
	if isSQL {
		err = d.exportSQL(ctx, tables, req, out)
	} else {
		err = d.exportDelimited(ctx, tables[0], sep, out)
	}
	if cerr := closeOut(); err == nil {
		err = cerr
	}
	return err
}

// exportSQL writes the SQL dialect: a file header, then one section per
// table — optional DROP, the generated CREATE (rewritten with IF NOT
// EXISTS when requested) and the data as INSERT batches.
func (d *PostgresDriver) exportSQL(ctx context.Context, tables []string, req driver.ExportRequest, out io.Writer) error {
	schema := d.CurrentSchema()
	header := "-- db-manager dump\n" +
		"-- Database: " + sqlgen.QuoteIdentPg(d.loginDB) + "\n" +
		"-- Schema: " + sqlgen.QuoteIdentPg(schema) + "\n\n"
	if err := writeExport(out, header); err != nil {
		return err
	}

	for _, table := range tables {
		create, err := d.ShowCreateTable(ctx, d.loginDB, table)
		if err != nil {
			return fmt.Errorf("export: %w", err)
		}
		if req.IfNotExists {
			create, err = addIfNotExists(create)
			if err != nil {
				return fmt.Errorf("export: %w", err)
			}
		}

		qual, err := qualify(schema, table)
		if err != nil {
			return fmt.Errorf("export: %w", err)
		}
		section := "-- Table: " + qual + "\n"
		if req.DropIfExists {
			section += "DROP TABLE IF EXISTS " + qual + ";\n"
		}
		section += create + ";\n\n"
		if err := writeExport(out, section); err != nil {
			return err
		}

		if err := d.exportTableSQLRows(ctx, table, out); err != nil {
			return err
		}
	}
	return nil
}

// addIfNotExists rewrites the generated CREATE TABLE into CREATE TABLE
// IF NOT EXISTS. The builders emit a pinned prefix; anything else is an
// error instead of a silently unmodified statement.
func addIfNotExists(create string) (string, error) {
	const prefix = "CREATE TABLE "
	if !strings.HasPrefix(create, prefix) {
		return "", fmt.Errorf("add if not exists: unexpected statement %q", create)
	}
	return "CREATE TABLE IF NOT EXISTS " + strings.TrimPrefix(create, prefix), nil
}

// exportTableSQLRows streams one table's rows as INSERT statements of at
// most exportBatchSize tuples, qualified with the current schema.
func (d *PostgresDriver) exportTableSQLRows(ctx context.Context, table string, out io.Writer) error {
	pk, err := d.PrimaryKey(ctx, d.loginDB, table)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	first := true
	var insertPrefix string
	var meta []driver.ColumnMeta
	err = d.exportPages(ctx, table, pk, func(page driver.Page) error {
		if first {
			names := make([]string, len(page.Columns))
			meta = make([]driver.ColumnMeta, len(page.Columns))
			for i, c := range page.Columns {
				names[i] = sqlgen.QuoteIdentPg(c.Name)
				meta[i] = c
			}
			insertPrefix = "INSERT INTO " + sqlgen.QuoteIdentPg(table) +
				" (" + strings.Join(names, ",") + ") VALUES "
			first = false
		}

		var sb strings.Builder
		sb.WriteString(insertPrefix)
		for i, row := range page.Rows {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString("(")
			for j, v := range row {
				if j > 0 {
					sb.WriteString(",")
				}
				sb.WriteString(sqlLiteralPg(v, meta[j].Type))
			}
			sb.WriteString(")")
		}
		sb.WriteString(";\n")
		return writeExport(out, sb.String())
	})
	if err != nil {
		return fmt.Errorf("export data of %s.%s: %w", d.CurrentSchema(), table, err)
	}
	return nil
}

// exportDelimited writes one table as delimited text via the shared CSV
// writer: header row, NULL → empty field, enclosure only when needed.
func (d *PostgresDriver) exportDelimited(ctx context.Context, table string, sep rune, out io.Writer) error {
	columns, err := d.selectColumns(ctx, d.CurrentSchema(), table, nil)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	names := make([]string, len(columns))
	for i, c := range columns {
		names[i] = c.Name
	}

	pk, err := d.PrimaryKey(ctx, d.loginDB, table)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	cw := dump.NewCSVWriter(out, sep)
	if err := cw.WriteHeader(names); err != nil {
		return fmt.Errorf("export header of %s.%s: %w", d.CurrentSchema(), table, err)
	}
	if err := d.exportPages(ctx, table, pk, func(page driver.Page) error {
		for _, row := range page.Rows {
			if err := cw.WriteRow(row); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("export data of %s.%s: %w", d.CurrentSchema(), table, err)
	}
	return nil
}

// exportPages streams every row of table in pages of exportBatchSize.
// A single-column key pages by keyset (no skip, no repeat); any other
// shape pages by LIMIT/OFFSET — on a live table that may skip or
// duplicate rows (documented caveat).
func (d *PostgresDriver) exportPages(ctx context.Context, table string, orderBy []string, yield func(driver.Page) error) error {
	q := sqlgen.SelectQuery{Limit: exportBatchSize}
	for _, c := range orderBy {
		q.Sorts = append(q.Sorts, sqlgen.Sort{Column: c})
	}

	if len(orderBy) == 1 {
		for {
			page, err := d.SelectRows(ctx, d.loginDB, table, q)
			if err != nil {
				return err
			}
			if len(page.Rows) == 0 {
				return nil
			}
			if err := yield(page); err != nil {
				return err
			}
			if len(page.Rows) < exportBatchSize {
				return nil
			}
			last := page.Rows[len(page.Rows)-1]
			idx := slices.IndexFunc(page.Columns, func(c driver.ColumnMeta) bool { return c.Name == orderBy[0] })
			if idx < 0 || idx >= len(last) {
				return fmt.Errorf("key column %q not found in result of %s.%s", orderBy[0], d.CurrentSchema(), table)
			}
			q.Filters = []sqlgen.Filter{{
				Column:   orderBy[0],
				Operator: ">",
				Value:    pgKeysetText(last[idx]),
			}}
		}
	}

	for offset := 0; ; offset += exportBatchSize {
		q.Offset = offset
		page, err := d.SelectRows(ctx, d.loginDB, table, q)
		if err != nil {
			return err
		}
		if len(page.Rows) == 0 {
			return nil
		}
		if err := yield(page); err != nil {
			return err
		}
		if len(page.Rows) < exportBatchSize {
			return nil
		}
	}
}

// pgKeysetText renders one keyset key for the next page's filter bind.
// PG accepts ISO-8601 timestamps in timestamptz comparisons, so the
// read path's RFC3339 strings bind as-is — no normalization needed
// (unlike the MySQL dump).
func pgKeysetText(v any) string {
	switch val := v.(type) {
	case nil:
		return "" // a key column is never NULL
	case string:
		return val
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64)
	case bool:
		if val {
			return "t"
		}
		return "f"
	case time.Time:
		return val.Format(time.RFC3339)
	default:
		return fmt.Sprint(v)
	}
}

// sqlLiteralPg renders one page value as a PG SQL literal for an INSERT:
// NULL stays NULL; strings are ''-escaped (PG backslash is literal);
// bytea columns emit '\x…'::bytea hex literals (empty binary is
// ''::bytea); time values keep their RFC3339 form (PG parses it);
// arrays and jsonb arrive as their text rendering and are quoted.
func sqlLiteralPg(v any, colType string) string {
	if v == nil {
		return "NULL"
	}
	if isByteaTypeName(colType) {
		// bytea page values arrive as "0x…" hex strings; PG wants a
		// '\x…'::bytea literal (empty binary is ''::bytea).
		hex := strings.TrimPrefix(fmt.Sprint(v), "0x")
		return "'\\x" + hex + "'::bytea"
	}
	if strings.HasSuffix(colType, "[]") {
		// Array page values arrive JSON-encoded; the dump needs the PG
		// array literal form the server parses back.
		if lit, ok := jsonToPgArray(fmt.Sprint(v)); ok {
			return sqlQuoteStringPg(lit)
		}
	}
	// sqlQuoteStringPg escapes inner quotes ('' doubling) and wraps the
	// result in single quotes — exactly one literal.
	return sqlQuoteStringPg(fmt.Sprint(v))
}

// jsonUnmarshal is an indirection seam for tests.
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

// isByteaTypeName reports whether the column type is a binary PG type.
func isByteaTypeName(t string) bool {
	return t == "bytea"
}

// jsonToPgArray converts the read path's JSON rendering of an array
// column back to the PG array literal: ["a","b"] → {a,b}; elements
// containing separators, quotes or braces are double-quoted with \
// escaping; JSON null becomes the unquoted NULL element.
func jsonToPgArray(jsonText string) (string, bool) {
	var items []any
	if err := jsonUnmarshal([]byte(jsonText), &items); err != nil {
		return "", false
	}
	var sb strings.Builder
	sb.WriteString("{")
	for i, item := range items {
		if i > 0 {
			sb.WriteString(",")
		}
		if item == nil {
			sb.WriteString("NULL")
			continue
		}
		el := fmt.Sprint(item)
		if el == "" {
			sb.WriteString(`""`)
			continue
		}
		needsQuote := strings.ContainsAny(el, `,"{}\`) || strings.TrimSpace(el) != el
		if needsQuote {
			sb.WriteString(`"` + strings.ReplaceAll(strings.ReplaceAll(el, `\`, `\\`), `"`, `\"`) + `"`)
		} else {
			sb.WriteString(el)
		}
	}
	sb.WriteString("}")
	return sb.String(), true
}

func writeExport(w io.Writer, s string) error {
	if _, err := io.WriteString(w, s); err != nil {
		return fmt.Errorf("export: write: %w", err)
	}
	return nil
}

// TruncateTables truncates each table in the current schema; the first
// error aborts.
func (d *PostgresDriver) TruncateTables(ctx context.Context, _ string, tables []string) error {
	schema := d.CurrentSchema()
	for _, table := range tables {
		qual, err := qualify(schema, table)
		if err != nil {
			return fmt.Errorf("truncate: %w", err)
		}
		if _, err := d.db.ExecContext(ctx, "TRUNCATE TABLE "+qual); err != nil {
			return fmt.Errorf("truncate %s: %w", qual, err)
		}
	}
	return nil
}

// DropTables drops each table in the current schema; the first error
// aborts.
func (d *PostgresDriver) DropTables(ctx context.Context, _ string, tables []string) error {
	schema := d.CurrentSchema()
	for _, table := range tables {
		qual, err := qualify(schema, table)
		if err != nil {
			return fmt.Errorf("drop: %w", err)
		}
		if _, err := d.db.ExecContext(ctx, "DROP TABLE "+qual); err != nil {
			return fmt.Errorf("drop table %s: %w", qual, err)
		}
	}
	return nil
}

// DropViews drops each view in the current schema; the first error
// aborts.
func (d *PostgresDriver) DropViews(ctx context.Context, _ string, views []string) error {
	schema := d.CurrentSchema()
	for _, view := range views {
		qual, err := qualify(schema, view)
		if err != nil {
			return fmt.Errorf("drop view: %w", err)
		}
		if _, err := d.db.ExecContext(ctx, "DROP VIEW "+qual); err != nil {
			return fmt.Errorf("drop view %s: %w", qual, err)
		}
	}
	return nil
}

// csvBatchSize is the multi-row INSERT tuple count of the CSV importer.
const csvBatchSize = 100
