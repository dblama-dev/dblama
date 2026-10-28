package mysql

import (
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/dump"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// errNoSuchTable is the MySQL error number for ER_NO_SUCH_TABLE ("Table
// ... doesn't exist").
const errNoSuchTable = 1146

// ShowCreateTable returns the server's own CREATE TABLE statement of one
// table. The statement addresses the schema db, regardless of the DSN's
// default database. A missing table yields an error containing "not
// found"; empty names fail before any query is issued. Some servers
// (MariaDB) append a fake empty second "Create Table" column to keep
// their client buffers happy; the first non-empty value wins, so that
// artifact is dropped.
func (d *MysqlDriver) ShowCreateTable(ctx context.Context, db, table string) (string, error) {
	if db == "" {
		return "", fmt.Errorf("show create table: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return "", fmt.Errorf("show create table: %w: table name", sqlgen.ErrEmptyIdentifier)
	}

	rows, err := d.db.QueryContext(ctx, "SHOW CREATE TABLE "+qualifiedIdent(db, table))
	if err != nil {
		var myErr *mysqldriver.MySQLError
		if errors.As(err, &myErr) && myErr.Number == errNoSuchTable {
			return "", fmt.Errorf("table %q not found", table)
		}
		return "", fmt.Errorf("show create table %s.%s: %w", db, table, err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", fmt.Errorf("show create table %s.%s: %w", db, table, err)
		}
		return "", fmt.Errorf("table %q not found", table)
	}

	// Column 0 is the table name; the CREATE statement follows —
	// possibly shadowed by a trailing fake empty duplicate column.
	cols, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("show create table %s.%s: %w", db, table, err)
	}
	values := make([]sql.NullString, len(cols)-1)
	ptrs := make([]any, len(cols))
	ptrs[0] = new(sql.NullString) // table name, discarded
	for i := range values {
		ptrs[i+1] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return "", fmt.Errorf("scan create statement of %s.%s: %w", db, table, err)
	}
	for _, v := range values {
		if v.Valid && v.String != "" {
			return v.String, nil
		}
	}
	return "", fmt.Errorf("show create table %s.%s: server returned no create statement", db, table)
}

// exportBatchSize is the row page size of an export and the number of
// VALUES tuples per emitted INSERT statement.
const exportBatchSize = 100

// Export streams a dump of the driver's login database to w. The sql
// format writes one section per table — an optional DROP TABLE IF
// EXISTS, the server's CREATE statement (rewritten with IF NOT EXISTS
// when requested) and the data as batched multi-row INSERTs. Statements
// are unqualified (mysqldump-style): the importer selects the target
// database, so the same dump imports into any schema. The delimited
// formats write a header row and one line per row. Rows stream in
// pages of exportBatchSize; nothing but the current row and statement
// is ever buffered. With Gzip the stream is wrapped in a gzip writer
// whose footer is flushed when Export returns.
//
// IncludeViews opts SQL dumps into dependency-ordered view definitions.
//
// Paging pins: a single-column primary key pages by keyset (WHERE pk >
// lastPK ORDER BY pk LIMIT 100) so a live table cannot skip or repeat
// rows; any other shape (no PK, composite PK) pages by LIMIT/OFFSET,
// which on a live table may skip or duplicate rows — a documented
// caveat, the same one mysqldump has.
func (d *MysqlDriver) Export(ctx context.Context, db string, req driver.ExportRequest, w io.Writer) error {
	// Validate the format before any byte is written: an unknown
	// format fails without touching w.
	var sep rune
	isSQL := req.Format == dump.FormatSQL
	if !isSQL {
		s, err := dump.Separator(req.Format)
		if err != nil {
			return fmt.Errorf("export: unknown format %q (want %q, %q, %q or %q)",
				req.Format, dump.FormatSQL, dump.FormatCSV, dump.FormatCSVSemicolon, dump.FormatTSV)
		}
		sep = s
	}

	if db == "" {
		return fmt.Errorf("export: no database to dump")
	}
	if db != d.loginDB {
		// The pool is bound to the login database; the handler already
		// validates this, the driver refuses rather than dump the wrong one.
		return fmt.Errorf("export: database %s is not the login database %s", db, d.loginDB)
	}

	// Resolve the table list: sql with an empty list dumps the whole
	// database (non-view tables); csv/tsv need exactly one table.
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
			return fmt.Errorf("export: format %q requires exactly one table in tables, got %d",
				req.Format, len(req.Tables))
		}
		tables = req.Tables
	}
	for _, table := range tables {
		if table == "" {
			return fmt.Errorf("export: %w: table name", sqlgen.ErrEmptyIdentifier)
		}
	}

	var views []dump.View
	if isSQL && req.Query == nil && req.IncludeViews {
		var err error
		tables, views, err = d.prepareViewExport(ctx, db, req.Tables)
		if err != nil {
			return fmt.Errorf("export views: %w", err)
		}
	}
	if isSQL && req.Query == nil {
		var err error
		tables, _, err = d.prepareTableExport(ctx, db, tables)
		if err != nil {
			return err
		}
	}

	var generated map[string]bool
	if isSQL && req.Query != nil && len(tables) == 1 {
		var err error
		generated, err = d.generatedExportColumns(ctx, db, tables[0])
		if err != nil {
			return err
		}
	}

	out := w
	closeOut := func() error { return nil }
	if req.Gzip {
		gz := gzip.NewWriter(w)
		out = gz
		// The footer is written by Close, so its error is part of the
		// export's outcome: a failure there means a truncated stream
		// and must not be swallowed.
		closeOut = func() error {
			if err := gz.Close(); err != nil {
				return fmt.Errorf("export: close gzip stream: %w", err)
			}
			return nil
		}
	}

	var err error
	if req.Query != nil {
		if len(tables) != 1 {
			return fmt.Errorf("selection export requires exactly one table")
		}
		if isSQL {
			err = writeExport(out, "SET @DBM_OLD_SQL_MODE=@@SESSION.sql_mode;\nSET SESSION sql_mode='NO_AUTO_VALUE_ON_ZERO';\n")
		}
		if err == nil {

			err = dump.WriteSelection(ctx, tables[0], req.Format, *req.Query, out,
				func(ctx context.Context, q driver.SelectQuery) (driver.Page, error) {
					page, err := d.SelectRows(ctx, db, tables[0], q)
					if err != nil {
						return page, err
					}
					if isSQL {
						return dump.WritablePage(page, generated)
					}
					return page, nil
				}, sqlgen.QuoteIdent, sqlLiteral, false)
		}
		if err == nil && isSQL {
			err = writeExport(out, "SET SESSION sql_mode=@DBM_OLD_SQL_MODE;\n")
		}
	} else if isSQL {
		if req.DropIfExists {
			err = dropViewsSQL(out, views)
		}
		if err == nil {
			err = d.exportSQL(ctx, db, tables, req, out)
		}
		if err == nil {
			err = writeViewsSQL(out, views, req)
		}
		if err == nil {
			err = writeExport(out, "SET SESSION foreign_key_checks=@DBM_OLD_FOREIGN_KEY_CHECKS;\nSET SESSION sql_mode=@DBM_OLD_SQL_MODE;\n")
		}
	} else {
		err = d.exportDelimited(ctx, db, tables[0], sep, out)
	}
	if cerr := closeOut(); err == nil {
		err = cerr
	}
	return err
}

// exportSQL writes the SQL dialect: the file header, then one section
// per table with the server's own SHOW CREATE TABLE statement and the
// table's data as INSERT batches.
func (d *MysqlDriver) exportSQL(ctx context.Context, db string, tables []string, req driver.ExportRequest, out io.Writer) error {
	header := "SET NAMES utf8mb4;\nSET @DBM_OLD_FOREIGN_KEY_CHECKS=@@SESSION.foreign_key_checks;\nSET SESSION foreign_key_checks=0;\nSET @DBM_OLD_SQL_MODE=@@SESSION.sql_mode;\nSET SESSION sql_mode='NO_AUTO_VALUE_ON_ZERO';\n" +
		"-- db-manager dump\n" +
		"-- Database: " + sqlgen.QuoteIdent(db) + "\n\n"
	if err := writeExport(out, header); err != nil {
		return err
	}

	if req.DropIfExists && len(tables) > 0 {
		names := make([]string, len(tables))
		for i, name := range tables {
			names[i] = sqlgen.QuoteIdent(name)
		}
		if err := writeExport(out, "DROP TABLE IF EXISTS "+strings.Join(names, ", ")+";\n"); err != nil {
			return err
		}
	}

	for _, table := range tables {
		create, err := d.ShowCreateTable(ctx, db, table)
		if err != nil {
			return fmt.Errorf("export: %w", err)
		}
		if req.IfNotExists {
			create, err = addIfNotExists(create)
			if err != nil {
				return fmt.Errorf("export: %w", err)
			}
		}

		section := "-- Table: " + sqlgen.QuoteIdent(table) + "\n"
		section += create + ";\n\n"
		if err := writeExport(out, section); err != nil {
			return err
		}

		if err := d.exportTableSQLRows(ctx, db, table, out); err != nil {
			return err
		}
	}
	return nil
}

// exportTableSQLRows streams one table's rows as INSERT statements of
// at most exportBatchSize VALUES tuples each. The statement is built
// once from the first page's columns; a table with no rows produces no
// INSERT at all. Binary columns emit their "0x…" hex strings as
// unquoted hex literals.
func (d *MysqlDriver) exportTableSQLRows(ctx context.Context, db, table string, out io.Writer) error {
	pk, err := d.PrimaryKey(ctx, db, table)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	generated, err := d.generatedExportColumns(ctx, db, table)
	if err != nil {
		return err
	}

	first := true
	var insertPrefix string
	var meta []driver.ColumnMeta
	err = d.exportPages(ctx, db, table, pk, func(page driver.Page) error {
		page, err = dump.WritablePage(page, generated)
		if err != nil {
			return err
		}
		if first {
			names := make([]string, len(page.Columns))
			meta = make([]driver.ColumnMeta, len(page.Columns))
			for i, c := range page.Columns {
				names[i] = sqlgen.QuoteIdent(c.Name)
				meta[i] = c
			}
			insertPrefix = "INSERT INTO " + sqlgen.QuoteIdent(table) +
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
				sb.WriteString(sqlLiteral(v, meta[j].Type))
			}
			sb.WriteString(")")
		}
		sb.WriteString(";\n")
		return writeExport(out, sb.String())
	})
	if err != nil {
		return fmt.Errorf("export data of %s.%s: %w", db, table, err)
	}
	return nil
}

// exportDelimited writes one table as delimited text: a header row of
// column names, then one line per row. NULL renders as an empty field,
// binary values as their "0x…" hex strings, and only fields containing
// the separator, a quote or a line break are enclosed.
func (d *MysqlDriver) exportDelimited(ctx context.Context, db, table string, sep rune, out io.Writer) error {
	columns, err := d.selectColumns(ctx, db, table, nil)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	names := make([]string, len(columns))
	for i, c := range columns {
		names[i] = c.Name
	}

	pk, err := d.PrimaryKey(ctx, db, table)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	cw := dump.NewCSVWriter(out, sep)
	if err := cw.WriteHeader(names); err != nil {
		return fmt.Errorf("export header of %s.%s: %w", db, table, err)
	}
	if err := d.exportPages(ctx, db, table, pk, func(page driver.Page) error {
		for _, row := range page.Rows {
			if err := cw.WriteRow(row); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("export data of %s.%s: %w", db, table, err)
	}
	return nil
}

// exportPages streams every row of table in pages of exportBatchSize,
// ordered by orderBy when it is not empty. A single-column key pages by
// keyset (no skip, no repeat); any other shape pages by LIMIT/OFFSET —
// on a live table that may skip or duplicate rows (documented caveat).
// yield receives each non-empty page in order.
func (d *MysqlDriver) exportPages(ctx context.Context, db, table string, orderBy []string, yield func(driver.Page) error) error {
	q := sqlgen.SelectQuery{Limit: exportBatchSize}
	for _, c := range orderBy {
		q.Sorts = append(q.Sorts, sqlgen.Sort{Column: c})
	}

	if len(orderBy) == 1 {
		// Keyset paging: one filter keeps the WHERE on the primary key.
		for {
			page, err := d.SelectRows(ctx, db, table, q)
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
				return fmt.Errorf("key column %q not found in result of %s.%s", orderBy[0], db, table)
			}
			q.Filters = []sqlgen.Filter{{
				Column:   orderBy[0],
				Operator: ">",
				Value:    keysetText(last[idx], page.Columns[idx].Type),
			}}
		}
	}

	// Offset paging (no PK, or a composite one — row-value comparison
	// is not expressible through the filter builder).
	for offset := 0; ; offset += exportBatchSize {
		q.Offset = offset
		page, err := d.SelectRows(ctx, db, table, q)
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

// keysetText renders one keyset key as the string the next page's
// filter binds: numbers in their decimal form, strings as they are —
// except on a datetime-typed column, whose values arrive as RFC3339
// from the read page and are normalized to SQL datetime text so the
// comparison matches the column's values. The column type gates the
// normalization: a string key is never rewritten, even when it happens
// to look like an RFC3339 timestamp.
func keysetText(v any, colType string) string {
	switch val := v.(type) {
	case nil:
		return "" // a key column is never NULL
	case string:
		if isDateTimeTypeName(colType) {
			if t, err := time.Parse(time.RFC3339Nano, val); err == nil {
				return t.Format("2006-01-02 15:04:05.999999999")
			}
		}
		return val
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64)
	case bool:
		if val {
			return "1"
		}
		return "0"
	case time.Time:
		return val.Format("2006-01-02 15:04:05.999999999")
	default:
		return fmt.Sprint(v)
	}
}

// sqlLiteral renders one page value as a SQL literal for an INSERT of
// the column with base type colType: NULL stays NULL, strings are
// SQL-escaped, binary columns emit their "0x…" hex strings as unquoted
// hex literals (an empty binary value has no 0x form — "0x" alone is
// not a literal — and is written X”), date/time columns have their
// RFC3339 page values (the read path's rendering) normalized back to
// SQL datetime text — the servers reject "T"/"Z" in a DATETIME literal
// — and everything else takes its natural numeric form.
func sqlLiteral(v any, colType string) string {
	switch val := v.(type) {
	case nil:
		return "NULL"
	case string:
		if isBinaryTypeName(colType) && strings.HasPrefix(val, "0x") {
			if hex := strings.TrimPrefix(val, "0x"); hex != "" {
				return "0x" + hex
			}
			return "X''"
		}
		if isDateTimeTypeName(colType) {
			if t, err := time.Parse(time.RFC3339Nano, val); err == nil {
				return sqlQuoteString(t.Format("2006-01-02 15:04:05.999999999"))
			}
		}
		return sqlQuoteString(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64)
	case bool:
		if val {
			return "1"
		}
		return "0"
	case time.Time:
		return sqlQuoteString(val.Format("2006-01-02 15:04:05.999999999"))
	default:
		return sqlQuoteString(fmt.Sprint(val))
	}
}

// isDateTimeTypeName reports whether a database type name is one whose
// values arrive as RFC3339 strings from the read path and must be
// normalized to SQL datetime text in a dump literal.
func isDateTimeTypeName(name string) bool {
	switch strings.ToUpper(name) {
	case "DATE", "DATETIME", "TIMESTAMP":
		return true
	}
	return false
}

// addIfNotExists rewrites a SHOW CREATE TABLE statement into its
// CREATE TABLE IF NOT EXISTS form. The transform is pinned to the one
// shape both flavors emit: the statement starts with "CREATE TABLE
// `name` (", so "IF NOT EXISTS " is inserted right after "CREATE
// TABLE ". Any other layout is an error instead of a silently
// unmodified statement.
func addIfNotExists(create string) (string, error) {
	const verb = "CREATE TABLE "
	if !strings.HasPrefix(create, verb) {
		head := create
		if i := strings.IndexByte(head, '\n'); i >= 0 {
			head = head[:i]
		}
		return "", fmt.Errorf("unexpected SHOW CREATE TABLE layout: %q", head)
	}
	return verb + "IF NOT EXISTS " + create[len(verb):], nil
}

// writeExport writes one chunk of dump text, wrapping write errors.
func writeExport(w io.Writer, s string) error {
	if _, err := io.WriteString(w, s); err != nil {
		return fmt.Errorf("export: write: %w", err)
	}
	return nil
}
