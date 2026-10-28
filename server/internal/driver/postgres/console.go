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

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
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

// consoleQuery is one registered running console query.
type consoleQuery struct {
	pid int64
}

// consoleRegistry maps queryID → running console query.
type consoleRegistry struct {
	mu       sync.Mutex
	registry map[string]consoleQuery
}

func (r *consoleRegistry) add(id string, q consoleQuery) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.registry == nil {
		r.registry = make(map[string]consoleQuery)
	}
	r.registry[id] = q
}

func (r *consoleRegistry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.registry, id)
}

func (r *consoleRegistry) lookup(id string) (consoleQuery, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.registry[id]
	return q, ok
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
	if db != d.loginDB {
		return nil, fmt.Errorf("exec sql: database %s is not the login database %s", db, d.loginDB)
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

	if queryID != "" {
		var pid int64
		if err := conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
			return nil, fmt.Errorf("exec sql: pg_backend_pid: %w", err)
		}
		d.console.add(queryID, consoleQuery{pid: pid})
		defer d.console.remove(queryID)
	}

	results := make([]driver.Result, 0, 4)
	start := time.Now()
	err = conn.Raw(func(driverConn any) error {
		sc, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("exec sql: raw conn is %T", driverConn)
		}
		if _, err := sc.Conn().PgConn().Exec(ctx, "SET search_path TO "+sqlgen.QuoteIdentPg(schema)).ReadAll(); err != nil {
			return fmt.Errorf("exec sql: set search_path: %w", err)
		}
		mr := sc.Conn().PgConn().Exec(ctx, query)
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

// CancelQuery cancels the backend running the query registered under
// queryID. An unknown queryID is not an error (already finished); a
// backend that vanished before the cancel lands (57P01) is the same
// not-an-error case.
func (d *PostgresDriver) CancelQuery(ctx context.Context, queryID string) error {
	q, ok := d.console.lookup(queryID)
	if !ok {
		return nil
	}
	if _, err := d.db.ExecContext(ctx, "SELECT pg_cancel_backend($1)", q.pid); err != nil {
		if strings.Contains(err.Error(), "57P01") {
			return nil
		}
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
