package mysql

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/sqlgen"
)

// ExecSQL defaults (brief: ExecOpts zero value → 1000 rows / 60 s).
const (
	defaultExecMaxRows = 1000
	defaultExecTimeout = 60 * time.Second
)

// rowStatementKeywords is the pinned heuristic for "this statement
// returns rows" (brief): a statement whose first keyword is one of
// these runs through QueryContext, everything else through
// ExecContext. Edge (pinned): a CTE (WITH ... SELECT) counts as
// row-returning, as do the DESC/DESCRIBE/EXPLAIN forms; a row-returning
// statement outside this list (e.g. CHECK TABLE) comes back without
// rows and without affected counts.
var rowStatementKeywords = []string{"select", "show", "desc", "describe", "explain", "with"}

// errNoSuchThread is the MySQL error number for "Unknown thread id"
// (ER_NO_SUCH_THREAD): the KILL target already disconnected.
const errNoSuchThread = 1094

// queryEntry is one registered running console query: its dedicated
// connection plus that connection's server-side id, which CancelQuery
// needs for KILL QUERY.
type queryEntry struct {
	conn   *sql.Conn
	connID uint32
}

// queryRegistry maps running console query ids to their dedicated
// connections. A driver instance is shared across requests, so the map
// is mutex-guarded; the map itself lazily initializes so a zero-value
// driver stays safe (unit tests).
type queryRegistry struct {
	mu    sync.Mutex
	conns map[string]*queryEntry
}

// add registers queryID → (conn, connID). An empty queryID (a caller
// that does not intend to cancel) is ignored.
func (r *queryRegistry) add(queryID string, conn *sql.Conn, connID uint32) {
	if queryID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conns == nil {
		r.conns = make(map[string]*queryEntry)
	}
	r.conns[queryID] = &queryEntry{conn: conn, connID: connID}
}

// connID returns the server-side connection id registered under queryID
// and whether that id is currently registered.
func (r *queryRegistry) connID(queryID string) (uint32, bool) {
	if queryID == "" {
		return 0, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.conns[queryID]
	if !ok {
		return 0, false
	}
	return entry.connID, true
}

// remove drops queryID from the registry; removing an absent id is a
// no-op.
func (r *queryRegistry) remove(queryID string) {
	if queryID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.conns, queryID)
}

// ExecSQL runs the console query — possibly several ';'-separated
// statements — on a dedicated connection and returns one Result per
// statement, in order. See the driver.Driver contract for the full
// semantics. Pinned mechanism (brief): statements are split client-side
// by sqlgen.SplitStatements (string/comment-aware, with DELIMITER-block
// support; '/*! ... */' executable comments are pinned to split like
// plain block comments) and executed one by one; row-returning
// statements (keyword heuristic) go through QueryContext, the rest
// through ExecContext for AffectedRows / LastInsertID. The first
// failing statement aborts the batch, but the Results accumulated so
// far are returned alongside the error.
func (d *MysqlDriver) ExecSQL(ctx context.Context, db, query string, queryID string, opts driver.ExecOpts) ([]driver.Result, error) {
	if db == "" {
		return nil, fmt.Errorf("exec sql: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("exec sql: empty query")
	}

	timeout := defaultExecTimeout
	if opts.TimeoutSec > 0 {
		timeout = time.Duration(opts.TimeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// One dedicated connection per call: its default database can be
	// switched without touching the pool, and a cancelled/abandoned
	// batch never poisons a pooled connection.
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("exec sql: acquire connection: %w", err)
	}

	// Capture the dedicated connection's server-side id before
	// registering: CancelQuery issues KILL QUERY against it. KILL QUERY
	// is connection-level, so the id stays valid across a later USE
	// switch to another database.
	var connID uint32
	if err := conn.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&connID); err != nil {
		discardConnection(conn) // the scan error is the real failure
		return nil, fmt.Errorf("exec sql: read connection id: %w", err)
	}
	d.queries.add(queryID, conn, connID)
	defer func() {
		d.queries.remove(queryID)
		discardConnection(conn)
	}()

	if db != d.loginDB {
		// The pool is bound to the login database; point the dedicated
		// connection at the requested one first.
		if _, err := conn.ExecContext(ctx, "USE "+sqlgen.QuoteIdent(db)); err != nil {
			return nil, fmt.Errorf("exec sql: switch to database %s: %w", db, err)
		}
	}

	maxRows := opts.MaxRows
	if maxRows <= 0 {
		maxRows = defaultExecMaxRows
	}

	statements := splitStatements(query)
	results := make([]driver.Result, 0, len(statements))
	for i, stmt := range statements {
		res, err := execStatement(ctx, conn, stmt, maxRows)
		if err != nil {
			// Brief ruling: the results before the failing statement
			// stay available to the caller.
			return results, fmt.Errorf("exec sql: statement %d: %w", i+1, err)
		}
		results = append(results, res)
	}
	return results, nil
}

// execStatement runs one statement and stamps its own execution time
// into Seconds.
func execStatement(ctx context.Context, conn *sql.Conn, stmt string, maxRows int) (driver.Result, error) {
	start := time.Now()
	var (
		res driver.Result
		err error
	)
	if isRowStatement(stmt) {
		res, err = execRowStatement(ctx, conn, stmt, maxRows)
	} else {
		res, err = execUpdateStatement(ctx, conn, stmt)
	}
	if err != nil {
		return driver.Result{}, err
	}
	res.Seconds = time.Since(start).Seconds()
	return res, nil
}

// execRowStatement runs a row-returning statement through QueryContext
// and buffers at most maxRows rows (flagging Truncated when more
// arrive).
func execRowStatement(ctx context.Context, conn *sql.Conn, stmt string, maxRows int) (driver.Result, error) {
	rows, err := conn.QueryContext(ctx, stmt)
	if err != nil {
		return driver.Result{}, fmt.Errorf("query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	columns, err := resultColumns(rows)
	if err != nil {
		return driver.Result{}, fmt.Errorf("read columns: %w", err)
	}

	// Binary columns (per this result's own type metadata) scan to
	// []byte that is rendered as hex; text columns that also scan to
	// []byte are rendered as strings. Unreadable type metadata treats
	// everything as text.
	binary := binaryColumns(rows)
	if binary == nil {
		binary = make([]bool, len(columns))
	}

	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}

	res := driver.Result{Columns: columns}
	for rows.Next() {
		if len(res.Rows) == maxRows {
			res.Truncated = true
			break
		}
		if err := rows.Scan(pointers...); err != nil {
			return res, fmt.Errorf("scan row: %w", err)
		}
		row := make([]any, len(values))
		for i, v := range values {
			row[i] = convertValue(v, binary[i])
		}
		res.Rows = append(res.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return res, fmt.Errorf("iterate rows: %w", err)
	}
	return res, nil
}

// execUpdateStatement runs a non-row statement through ExecContext and
// reports the affected row count and the generated id (0 when the
// statement generated none or the driver cannot report one).
func execUpdateStatement(ctx context.Context, conn *sql.Conn, stmt string) (driver.Result, error) {
	sqlRes, err := conn.ExecContext(ctx, stmt)
	if err != nil {
		return driver.Result{}, fmt.Errorf("exec: %w", err)
	}
	res := driver.Result{}
	if n, err := sqlRes.RowsAffected(); err == nil {
		res.AffectedRows = n
	}
	if id, err := sqlRes.LastInsertId(); err == nil {
		res.LastInsertID = id
	}
	return res, nil
}

// resultColumns reads the result's column metadata: the column name,
// the driver's base type label (lowercased to match the labels used by
// the browse path) and nullability when the driver reports it.
func resultColumns(rows *sql.Rows) ([]driver.ColumnMeta, error) {
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	columns := make([]driver.ColumnMeta, len(types))
	for i, ct := range types {
		columns[i].Name = ct.Name()
		columns[i].Type = strings.ToLower(ct.DatabaseTypeName())
		if nullable, ok := ct.Nullable(); ok {
			columns[i].Nullable = nullable
		}
	}
	return columns, nil
}

// CancelQuery cancels the console query registered under queryID by
// issuing KILL QUERY against the dedicated connection's server-side id
// through the shared pool (KILL QUERY aborts only the running query,
// leaving the connection usable). An unknown queryID means the query
// already finished — not an error (brief ruling); the same holds when
// the query finishes between the registry lookup and the KILL and the
// server reports an unknown thread id.
func (d *MysqlDriver) CancelQuery(ctx context.Context, queryID string) error {
	connID, ok := d.queries.connID(queryID)
	if !ok {
		return nil
	}
	if _, err := d.db.ExecContext(ctx, "KILL QUERY "+strconv.FormatUint(uint64(connID), 10)); err != nil {
		var myErr *mysqldriver.MySQLError
		if errors.As(err, &myErr) && myErr.Number == errNoSuchThread {
			return nil
		}
		return fmt.Errorf("cancel query %s: kill %d: %w", queryID, connID, err)
	}
	return nil
}

// splitStatements splits a multi-statement query into statements,
// delegating to sqlgen.SplitStatements (Task 3): string/comment-aware
// splitting plus mysql-client DELIMITER-block support. Pinned Task 3
// difference from the mysql client: '/*! ... */' executable comments
// split exactly like plain '/* ... */' block comments. Only non-blank
// statements are returned; a query without a single non-blank
// statement yields nil.
func splitStatements(query string) []string {
	return sqlgen.SplitStatements(query)
}

func isSpaceByte(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

// isRowStatement applies the pinned keyword heuristic: whether the
// statement's first keyword (after any leading comments) is one that
// returns rows to the client.
func isRowStatement(stmt string) bool {
	s := strings.TrimSpace(stmt)
	// Skip leading comments so "/* note */ SELECT 1" is recognized.
	for {
		switch {
		case strings.HasPrefix(s, "/*"):
			end := strings.Index(s, "*/")
			if end < 0 {
				return false
			}
			s = strings.TrimSpace(s[end+2:])
		case strings.HasPrefix(s, "--"):
			rest := s[2:]
			if rest != "" && !isSpaceByte(rest[0]) {
				return false // subtraction, not a comment
			}
			nl := strings.IndexByte(s, '\n')
			if nl < 0 {
				return false
			}
			s = strings.TrimSpace(s[nl+1:])
		case strings.HasPrefix(s, "#"):
			nl := strings.IndexByte(s, '\n')
			if nl < 0 {
				return false
			}
			s = strings.TrimSpace(s[nl+1:])
		default:
			lower := strings.ToLower(s)
			for _, kw := range rowStatementKeywords {
				if !strings.HasPrefix(lower, kw) {
					continue
				}
				rest := lower[len(kw):]
				if rest == "" || isSpaceByte(rest[0]) || rest[0] == '(' {
					return true
				}
			}
			return false
		}
	}
}

// discardConnection prevents SQL console/import session state (USE, SET,
// temporary tables and transactions) from leaking into later pooled requests.
func discardConnection(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return sqldriver.ErrBadConn })
	_ = conn.Close()
}
