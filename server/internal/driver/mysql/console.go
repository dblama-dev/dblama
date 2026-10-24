package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
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
// semantics. Pinned mechanism (brief): statements are split
// client-side by a minimal string/comment-aware splitter (no DELIMITER
// support — Task 3's sqlgen.SplitStatements replaces it) and executed
// one by one; row-returning statements (keyword heuristic) go through
// QueryContext, the rest through ExecContext for AffectedRows /
// LastInsertID. The first failing statement aborts the batch, but the
// Results accumulated so far are returned alongside the error.
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
		_ = conn.Close() // best-effort; the scan error is the real failure
		return nil, fmt.Errorf("exec sql: read connection id: %w", err)
	}
	d.queries.add(queryID, conn, connID)
	defer func() {
		d.queries.remove(queryID)
		_ = conn.Close()
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

// splitStatements splits a multi-statement query on ';' boundaries that
// sit outside single-quoted / double-quoted strings, backtick
// identifiers and comments, trimming each statement. Minimal Task 1
// version: no DELIMITER support (Task 3's sqlgen.SplitStatements
// replaces it) and comments are kept verbatim inside their statement.
// Only non-blank statements are returned; a query without a single
// non-blank statement yields nil.
func splitStatements(query string) []string {
	var statements []string
	start := 0
	i := 0
	for i < len(query) {
		switch c := query[i]; c {
		case '\'':
			i = scanQuoted(query, i, '\'', true)
		case '"':
			i = scanQuoted(query, i, '"', true)
		case '`':
			i = scanQuoted(query, i, '`', false)
		case '#':
			i = scanLineComment(query, i)
		case '-':
			// MySQL: "--" starts a comment only when followed by
			// whitespace or end of input ("1--2" is subtraction).
			if i+1 < len(query) && query[i+1] == '-' && (i+2 == len(query) || isSpaceByte(query[i+2])) {
				i = scanLineComment(query, i)
			} else {
				i++
			}
		case '/':
			if i+1 < len(query) && query[i+1] == '*' {
				i = scanBlockComment(query, i)
			} else {
				i++
			}
		case ';':
			if stmt := strings.TrimSpace(query[start:i]); stmt != "" {
				statements = append(statements, stmt)
			}
			i++
			start = i
		default:
			i++
		}
	}
	if stmt := strings.TrimSpace(query[start:]); stmt != "" {
		statements = append(statements, stmt)
	}
	return statements
}

// scanQuoted returns the index just past the quoted region opening at
// i (which holds the opening quote). backslash enables the backslash
// escapes MySQL applies inside '...' and "..." strings; inside
// backtick identifiers a backslash is literal. A doubled quote is an
// escaped quote and stays inside the region. An unterminated region
// runs to end of input.
func scanQuoted(q string, i int, quote byte, backslash bool) int {
	i++
	for i < len(q) {
		switch q[i] {
		case '\\':
			if backslash {
				i += 2
				continue
			}
		case quote:
			if i+1 < len(q) && q[i+1] == quote {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return i
}

// scanLineComment returns the index of the terminating '\n' (which the
// main loop then processes as ordinary text) or len(q).
func scanLineComment(q string, i int) int {
	for i < len(q) && q[i] != '\n' {
		i++
	}
	return i
}

// scanBlockComment returns the index just past the "*/" closing the
// block comment opening at i, or len(q) when unterminated.
func scanBlockComment(q string, i int) int {
	i += 2
	for i+1 < len(q) && (q[i] != '*' || q[i+1] != '/') {
		i++
	}
	if i+1 < len(q) {
		return i + 2
	}
	return len(q)
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
