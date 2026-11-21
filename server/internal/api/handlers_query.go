package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// Query limits: maxRows is hard-capped (spec §9.7) and timeoutSec is
// capped at one hour; a larger value is a bad_request, not a silent
// clamp. The EXPLAIN endpoint fixes its own timeout.
const (
	queryMaxRowsCap   = 100000
	queryTimeoutCap   = 3600
	explainTimeoutSec = 60
)

// queryRequest is the POST /api/databases/{db}/query body.
type queryRequest struct {
	SQL        string `json:"sql"`
	MaxRows    int    `json:"maxRows,omitempty"`
	TimeoutSec int    `json:"timeoutSec,omitempty"`
	QueryID    string `json:"queryID"`
}

// queryResponse is the query endpoint body: one entry per statement.
type queryResponse struct {
	Results []driver.Result `json:"results"`
}

type queryKey struct{ session, query string }

// handleQuery runs a raw SQL batch on one database and returns every
// statement's result. The queryID binds the run to the client-side cancel
// button via POST /api/query/{queryID}/cancel.
func (s *apiServer) handleQuery(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.SQL) == "" {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("sql is required"))
		return
	}
	if req.QueryID == "" {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("queryID is required"))
		return
	}
	if req.MaxRows < 0 || req.MaxRows > queryMaxRowsCap {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest(
			"maxRows exceeds the cap of "+strconv.Itoa(queryMaxRowsCap)))
		return
	}
	if req.TimeoutSec < 0 || req.TimeoutSec > queryTimeoutCap {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest(
			"timeoutSec exceeds the cap of "+strconv.Itoa(queryTimeoutCap)))
		return
	}

	sess, _ := session.FromContext(r.Context())
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	key := queryKey{session: sess.ID, query: req.QueryID}
	if _, running := s.queries.LoadOrStore(key, cancel); running {
		WriteErr(w, http.StatusConflict, ErrBadRequest("queryID is already running"))
		return
	}
	defer s.queries.Delete(key)
	db := pathParam(r, "db")
	d, release, err := databaseDriver(ctx, sess.Driver, db)
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	defer release()
	results, err := d.ExecSQL(ctx, db, req.SQL, req.QueryID,
		driver.ExecOpts{MaxRows: req.MaxRows, TimeoutSec: req.TimeoutSec})
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	if results == nil {
		results = []driver.Result{}
	}
	WriteJSON(w, http.StatusOK, queryResponse{Results: results})
}

// handleCancel cancels the query registered under the queryID. It always
// answers 204: CancelQuery is idempotent (nil for an unknown queryID) and
// a query that already finished is indistinguishable from a cancelled one
// from the client's point of view.
func (s *apiServer) handleCancel(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	id := pathParam(r, "queryID")
	cancel, running := s.queries.Load(queryKey{session: sess.ID, query: id})
	_ = sess.Driver.CancelQuery(r.Context(), id)
	if running {
		cancel.(context.CancelFunc)()
	}
	w.WriteHeader(http.StatusNoContent)
}

// explainRequest is the POST /api/query body. Unlike handleQuery the
// database travels in the body — the console explains a statement typed
// against the connected database before running it.
type explainRequest struct {
	DB  string `json:"db"`
	SQL string `json:"sql"`
}

// explainResponse wraps the single plan result.
type explainResponse struct {
	Result *driver.Result `json:"result"`
}

// handleExplain runs "EXPLAIN " + the raw SQL on the named database with
// a fixed 60s timeout and returns the first (only) statement result. The
// SQL must be a single statement: ExecSQL would execute every trailing
// statement too, and a trailing UPDATE behind a harmless-looking SELECT
// must not fire because the user pressed EXPLAIN. The generated queryID
// is internal — an EXPLAIN is never cancellable from the console.
func (s *apiServer) handleExplain(w http.ResponseWriter, r *http.Request) {
	var req explainRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.DB == "" {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("db is required"))
		return
	}
	if strings.TrimSpace(req.SQL) == "" {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("sql is required"))
		return
	}
	stmts := sqlgen.SplitStatements(req.SQL)
	if len(stmts) > 1 {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("explain accepts a single statement"))
		return
	}
	if len(stmts) == 0 {
		// Whitespace or comments only: nothing to explain.
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("sql is required"))
		return
	}

	sess, _ := session.FromContext(r.Context())
	d, release, err := databaseDriver(r.Context(), sess.Driver, req.DB)
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	defer release()
	results, err := d.ExecSQL(r.Context(), req.DB, "EXPLAIN "+stmts[0], newQueryID(),
		driver.ExecOpts{TimeoutSec: explainTimeoutSec})
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	var res *driver.Result
	if len(results) > 0 {
		res = &results[0]
	}
	WriteJSON(w, http.StatusOK, explainResponse{Result: res})
}

// newQueryID generates a fresh query id for server-initiated statements.
func newQueryID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("explain-%d", time.Now().UnixNano())
	}
	return "explain-" + hex.EncodeToString(b[:])
}
