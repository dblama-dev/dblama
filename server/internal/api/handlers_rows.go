package api

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// rowsDefaultLimit is the page size used when the request carries no
// limit; rowsMaxLimit is the hard cap — a larger limit is bad_request.
const (
	rowsDefaultLimit = 50
	rowsMaxLimit     = 1000
)

// parseOrder parses order=col:asc|col:desc. The column may itself not
// contain the direction suffix, so the split is on the last ':'.
func parseOrder(s string) (sqlgen.Sort, bool) {
	i := strings.LastIndex(s, ":")
	if i <= 0 {
		return sqlgen.Sort{}, false
	}
	switch s[i+1:] {
	case "asc":
		return sqlgen.Sort{Column: s[:i]}, true
	case "desc":
		return sqlgen.Sort{Column: s[:i], Desc: true}, true
	default:
		return sqlgen.Sort{}, false
	}
}

// parseFilter parses filter=col:op:value. The value may contain colons,
// so the string is split into exactly three parts on the first two ':'.
// IS NULL and IS NOT NULL ignore the value. ok is false for a malformed
// parameter or an operator outside the sqlgen whitelist.
func parseFilter(s string, allowed map[string]bool) (sqlgen.Filter, bool) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 || parts[0] == "" {
		return sqlgen.Filter{}, false
	}
	if !allowed[parts[1]] {
		return sqlgen.Filter{}, false
	}
	return sqlgen.Filter{Column: parts[0], Operator: parts[1], Value: parts[2]}, true
}

// parsePaging reads limit and offset: limit defaults to 50 and is capped
// at 1000, offset defaults to 0. A non-empty message means the request
// was invalid and names the offending parameter.
func parsePaging(query url.Values) (limit, offset int, msg string) {
	limit = rowsDefaultLimit
	if s := query.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return 0, 0, "invalid limit " + s
		}
		if n > rowsMaxLimit {
			return 0, 0, "limit exceeds the cap of " + strconv.Itoa(rowsMaxLimit)
		}
		limit = n
	}
	offset = 0
	if s := query.Get("offset"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return 0, 0, "invalid offset " + s
		}
		offset = n
	}
	return limit, offset, ""
}

// handleRowsSelect returns one page of browsed rows. The query string is
// parsed into a structured SelectQuery before it reaches the driver, so
// malformed parameters are rejected with bad_request and never sent on.
func (s *apiServer) handleRowsSelect(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())

	query := r.URL.Query()
	q := sqlgen.SelectQuery{}

	if o := query.Get("order"); o != "" {
		sort, ok := parseOrder(o)
		if !ok {
			WriteErr(w, http.StatusBadRequest, ErrBadRequest("invalid order parameter "+o))
			return
		}
		q.Sorts = []sqlgen.Sort{sort}
	}

	allowed := make(map[string]bool, 16)
	for _, op := range sqlgen.AllowedOperators() {
		allowed[op] = true
	}
	for _, f := range query["filter"] {
		flt, ok := parseFilter(f, allowed)
		if !ok {
			WriteErr(w, http.StatusBadRequest, ErrBadRequest("invalid filter parameter "+f))
			return
		}
		q.Filters = append(q.Filters, flt)
	}

	limit, offset, msg := parsePaging(query)
	if msg != "" {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest(msg))
		return
	}
	q.Limit, q.Offset = limit, offset

	page, err := sess.Driver.SelectRows(r.Context(), chi.URLParam(r, "db"), chi.URLParam(r, "table"), q)
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	if page.Rows == nil {
		page.Rows = [][]any{}
	}
	WriteJSON(w, http.StatusOK, page)
}

// lastInsertIDResponse is the POST /api/databases/{db}/tables/{table}/rows body.
type lastInsertIDResponse struct {
	LastInsertID int64 `json:"lastInsertId"`
}

// handleRowInsert inserts one row and reports the auto_increment id the
// insert generated. LastInsertID is driver state, not per-request
// state: the last insert wins across concurrent requests, so the
// reported id is only guaranteed to belong to this request's insert
// when no other insert ran against the same driver between the insert
// and the read back below.
func (s *apiServer) handleRowInsert(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	var vals driver.RowValues
	if !decodeJSONBody(w, r, &vals) {
		return
	}

	if err := sess.Driver.InsertRow(r.Context(), chi.URLParam(r, "db"), chi.URLParam(r, "table"), vals); err != nil {
		writeDriverErr(w, err)
		return
	}
	id, err := sess.Driver.LastInsertID(r.Context())
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, lastInsertIDResponse{LastInsertID: id})
}

// updateRowRequest is the PATCH rows body: the exact-match key of the row
// and the values to set.
type updateRowRequest struct {
	Where  driver.PK        `json:"where"`
	Values driver.RowValues `json:"values"`
}

// handleRowUpdate updates every row matching the where key exactly.
func (s *apiServer) handleRowUpdate(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	var req updateRowRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err := sess.Driver.UpdateRow(r.Context(), chi.URLParam(r, "db"),
		chi.URLParam(r, "table"), req.Where, req.Values); err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteRowsRequest is the DELETE rows body: one key per row to delete.
type deleteRowsRequest struct {
	Where []driver.PK `json:"where"`
}

// handleRowDelete deletes one row per key in a single driver transaction.
func (s *apiServer) handleRowDelete(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	var req deleteRowsRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err := sess.Driver.DeleteRows(r.Context(), chi.URLParam(r, "db"),
		chi.URLParam(r, "table"), req.Where); err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
