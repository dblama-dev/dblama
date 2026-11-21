package postgres

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/sqlgen"
)

// Console execution runs on a dedicated connection per ExecSQL call,
// mirroring the MySQL console. The multi-statement mechanics were pinned
// by a spike against postgres:17 (task 5 report): database/sql
// QueryContext+NextResultSet silently returns ONE result set for a
// multi-statement batch under simple_protocol, so the batch instead goes
// through the raw pgx connection — conn.Raw → stdlib.Conn →
// PgConn().Exec — whose MultiResultReader yields every result set with
// its CommandTag ("SELECT n", "INSERT 0 n", ...).
//
// SET search_path TO <current schema> runs on that dedicated connection:
// the spec's §13.5 ban targets the SHARED pool, and this connection is
// one-shot and closed under defer.

// consoleRegistry tracks the dedicated PostgreSQL connection for each running query.
type consoleRegistry struct {
	mu       sync.Mutex
	registry map[string]*pgconn.PgConn
}

func (r *consoleRegistry) add(id string, conn *pgconn.PgConn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.registry == nil {
		r.registry = make(map[string]*pgconn.PgConn)
	}
	r.registry[id] = conn
}

func (r *consoleRegistry) remove(id string, conn *pgconn.PgConn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.registry[id] == conn {
		delete(r.registry, id)
	}
}

func (r *consoleRegistry) lookup(id string) *pgconn.PgConn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.registry[id]
}

// ExecSQL implements driver.Driver: one Result per statement, MaxRows
// row caps with Truncated, per-statement Seconds, mid-error returns the
// accumulated Results together with the error.
func (d *PostgresDriver) ExecSQL(ctx context.Context, db, query, queryID string, opts driver.ExecOpts) ([]driver.Result, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("exec sql: empty query")
	}
	if db == "" {
		return nil, fmt.Errorf("exec sql: %w", sqlgen.ErrEmptyIdentifier)
	}
	if db != d.database {
		return nil, fmt.Errorf("exec sql: database %q does not match the connection database %q", db, d.database)
	}
	maxRows := opts.MaxRows
	if maxRows <= 0 {
		maxRows = 1000
	}
	timeout := time.Duration(opts.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	schema := d.schemaFor(ctx)

	conn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("exec sql: conn: %w", err)
	}
	defer discardConnection(conn)

	results := make([]driver.Result, 0, 4)
	start := time.Now()
	err = conn.Raw(func(driverConn any) error {
		sc, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("exec sql: raw conn is %T", driverConn)
		}
		pgConn := sc.Conn().PgConn()
		if _, err := pgConn.Exec(ctx, "SET search_path TO "+sqlgen.QuoteIdentPg(schema)).ReadAll(); err != nil {
			return fmt.Errorf("exec sql: set search_path: %w", err)
		}
		mr := pgConn.Exec(ctx, query)
		if queryID != "" {
			d.session.console.add(queryID, pgConn)
			defer d.session.console.remove(queryID, pgConn)
		}
		set := 0
		for mr.NextResult() {
			rr := mr.ResultReader()
			res, rerr := readResultSet(rr, maxRows)
			if rerr != nil {
				_ = mr.Close()
				return fmt.Errorf("statement %d: %w", set+1, rerr)
			}
			res.Seconds = time.Since(start).Seconds()
			results = append(results, res)
			set++
		}
		if err := mr.Close(); err != nil {
			// mr.Close surfaces the batch-level error (syntax, cancel,
			// timeout) after the statements read so far.
			return err
		}
		return nil
	})
	if err != nil {
		return results, err
	}
	return results, nil
}

// readResultSet drains one pgconn ResultReader into a driver.Result:
// column metadata with rows (capped at maxRows, Truncated flagged), or
// the CommandTag's affected count for rowless statements.
func readResultSet(rr *pgconn.ResultReader, maxRows int) (driver.Result, error) {
	var res driver.Result
	fds := rr.FieldDescriptions()
	if len(fds) == 0 {
		tag, err := rr.Close()
		if err != nil {
			return res, err
		}
		res.AffectedRows = tag.RowsAffected()
		return res, nil
	}
	for _, fd := range fds {
		res.Columns = append(res.Columns, driver.ColumnMeta{
			Name: string(fd.Name),
		})
	}
	rows := make([][]any, 0, 16)
	for rr.NextRow() {
		if len(rows) >= maxRows {
			res.Truncated = true
			break
		}
		vals := rr.Values()
		row := make([]any, len(vals))
		for i, v := range vals {
			row[i] = consoleValue(v)
		}
		rows = append(rows, row)
	}
	_, err := rr.Close()
	if err != nil {
		return res, err
	}
	res.Rows = rows
	return res, nil
}

// consoleValue converts one pgconn text-format cell to a JSON-compatible
// value: nil → nil, otherwise the text bytes as a string (numbers,
// timestamps, array and jsonb literals arrive as text under
// simple_protocol; the data grid path does richer conversion, the
// console intentionally shows raw text).
func consoleValue(v []byte) any {
	if v == nil {
		return nil
	}
	return string(v)
}

// CancelQuery sends a cancel request to the running query's backend. pgx
// uses a separate socket and the backend's secret, so this works without
// a pool connection. The console discards its dedicated connection after
// execution, so a late cancel can only target that original backend.
func (d *PostgresDriver) CancelQuery(ctx context.Context, queryID string) error {
	conn := d.session.console.lookup(queryID)
	if conn == nil {
		return nil
	}
	if err := conn.CancelRequest(ctx); err != nil {
		return fmt.Errorf("cancel query: %w", err)
	}
	return nil
}

// Returning ErrBadConn from Raw tells database/sql to discard the physical
// connection rather than reuse arbitrary SET/search_path state from SQL.
func discardConnection(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return sqldriver.ErrBadConn })
	_ = conn.Close()
}
