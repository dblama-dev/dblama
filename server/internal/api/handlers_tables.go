package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// requestBodyLimit caps every JSON request body at 1 MiB: table defs,
// index lists and row values are small documents, and an unbounded decode
// lets a client exhaust server memory.
const requestBodyLimit = 1 << 20

// classifyDriverErr maps a driver error to an HTTP status and error code:
// validation failures — empty identifiers wrapped from sqlgen — are a
// client problem and map to bad_request; everything else is a driver
// failure on an authenticated connection and maps to driver_error.
func classifyDriverErr(err error) (int, string) {
	if errors.Is(err, sqlgen.ErrEmptyIdentifier) {
		return http.StatusBadRequest, "bad_request"
	}
	return http.StatusInternalServerError, "driver_error"
}

// writeDriverErr answers with the classified envelope for a failed
// driver call.
func writeDriverErr(w http.ResponseWriter, err error) {
	status, code := classifyDriverErr(err)
	if code == "bad_request" {
		WriteErr(w, status, ErrBadRequest(err.Error()))
		return
	}
	WriteErr(w, status, ErrDriver(err.Error()))
}

// decodeJSONBody decodes the request body into v, answering bad_request
// on malformed or oversized input. It reports whether decoding succeeded.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, requestBodyLimit)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("malformed JSON body"))
		return false
	}
	return true
}

// tablesResponse is the GET /api/databases/{db}/tables body.
type tablesResponse struct {
	Tables []driver.TableInfo `json:"tables"`
}

// handleTablesList lists the tables and views of one database.
func (s *apiServer) handleTablesList(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	applySchemaParam(r, sess)

	tables, err := sess.Driver.ListTables(r.Context(), chi.URLParam(r, "db"))
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	if tables == nil {
		tables = []driver.TableInfo{}
	}
	WriteJSON(w, http.StatusOK, tablesResponse{Tables: tables})
}

// handleTableDef returns the full structure of one table.
func (s *apiServer) handleTableDef(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	applySchemaParam(r, sess)

	def, err := sess.Driver.TableDef(r.Context(), chi.URLParam(r, "db"), chi.URLParam(r, "table"))
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, def)
}

// handleSaveTable applies the desired table state. The path table "_new"
// means create: the driver receives an empty current name and takes the
// new name from the body; any other table name alters the existing one.
func (s *apiServer) handleSaveTable(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	applySchemaParam(r, sess)
	var def driver.TableDef
	if !decodeJSONBody(w, r, &def) {
		return
	}

	table := chi.URLParam(r, "table")
	if table == "_new" {
		table = ""
	}
	if err := sess.Driver.SaveTable(r.Context(), chi.URLParam(r, "db"), table, def); err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDropTable drops the table named in the path, or the view when
// the query carries isView=true.
func (s *apiServer) handleDropTable(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	applySchemaParam(r, sess)

	db := chi.URLParam(r, "db")
	table := chi.URLParam(r, "table")
	var err error
	if r.URL.Query().Get("isView") == "true" {
		err = sess.Driver.DropViews(r.Context(), db, []string{table})
	} else {
		err = sess.Driver.DropTables(r.Context(), db, []string{table})
	}
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTruncateTable empties the table named in the path.
func (s *apiServer) handleTruncateTable(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	applySchemaParam(r, sess)

	if err := sess.Driver.TruncateTables(r.Context(), chi.URLParam(r, "db"),
		[]string{chi.URLParam(r, "table")}); err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSaveIndexes applies the full desired index state of one table.
func (s *apiServer) handleSaveIndexes(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	applySchemaParam(r, sess)
	var indexes []driver.Index
	if !decodeJSONBody(w, r, &indexes) {
		return
	}

	if err := sess.Driver.SaveIndexes(r.Context(), chi.URLParam(r, "db"),
		chi.URLParam(r, "table"), indexes); err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// bulkTablesRequest is the POST /api/databases/{db}/tables body.
type bulkTablesRequest struct {
	Action string   `json:"action"`
	Tables []string `json:"tables"`
}

// handleBulkTables truncates or drops the named tables in one call.
func (s *apiServer) handleBulkTables(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	applySchemaParam(r, sess)
	var req bulkTablesRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	var err error
	switch req.Action {
	case "truncate":
		err = sess.Driver.TruncateTables(r.Context(), chi.URLParam(r, "db"), req.Tables)
	case "drop":
		err = sess.Driver.DropTables(r.Context(), chi.URLParam(r, "db"), req.Tables)
	default:
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("unknown action "+req.Action))
		return
	}
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pkResponse is the GET /api/databases/{db}/tables/{table}/pk body.
type pkResponse struct {
	Columns []string `json:"columns"`
}

// handlePrimaryKey returns the primary key column names of one table in
// key order; an empty result marshals as [].
func (s *apiServer) handlePrimaryKey(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	applySchemaParam(r, sess)

	cols, err := sess.Driver.PrimaryKey(r.Context(), chi.URLParam(r, "db"), chi.URLParam(r, "table"))
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	if cols == nil {
		cols = []string{}
	}
	WriteJSON(w, http.StatusOK, pkResponse{Columns: cols})
}
