package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/session"
)

// registerDatabaseRoutes mounts inside the authenticated /api group.
func (s *apiServer) registerDatabaseRoutes(r chi.Router) {
	r.Post("/databases", s.handleDatabaseCreate)
	r.Delete("/databases/{db}", s.handleDatabaseDrop)
	r.Patch("/databases/{db}", s.handleDatabaseRename)
	r.Post("/databases/{db}/table-transfer", s.handleTableTransfer)
	r.Get("/metadata", s.handleMetadata)
}
func databaseUnsupported(w http.ResponseWriter) {
	WriteErr(w, http.StatusNotFound, &Error{Code: "driver_not_supported", Message: "operation is not supported by this driver"})
}
func validDatabaseInput(name string) bool {
	return strings.TrimSpace(name) != "" && !strings.ContainsRune(name, 0)
}

// Database operations require exactly one JSON document; a trailing document
// must not accidentally execute a different mutation than the caller intended.
func decodeDatabaseBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, requestBodyLimit)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		WriteErr(w, 400, ErrBadRequest("malformed JSON body"))
		return false
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		WriteErr(w, 400, ErrBadRequest("expected one JSON document"))
		return false
	}
	return true
}
func databaseManager(w http.ResponseWriter, r *http.Request) (driver.DatabaseManager, bool) {
	sess, _ := session.FromContext(r.Context())
	d, ok := sess.Driver.(driver.DatabaseManager)
	if !ok {
		databaseUnsupported(w)
	}
	return d, ok
}
func (s *apiServer) handleDatabaseCreate(w http.ResponseWriter, r *http.Request) {
	d, ok := databaseManager(w, r)
	if !ok {
		return
	}
	var req struct {
		Name      string `json:"name"`
		Collation string `json:"collation"`
	}
	if !decodeDatabaseBody(w, r, &req) {
		return
	}
	if !validDatabaseInput(req.Name) {
		WriteErr(w, 400, ErrBadRequest("database name is required"))
		return
	}
	if err := d.CreateDatabase(r.Context(), req.Name, req.Collation); err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *apiServer) handleDatabaseDrop(w http.ResponseWriter, r *http.Request) {
	d, ok := databaseManager(w, r)
	if !ok {
		return
	}
	name := pathParam(r, "db")
	if !validDatabaseInput(name) {
		WriteErr(w, 400, ErrBadRequest("database name is required"))
		return
	}
	if err := d.DropDatabase(r.Context(), name); err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *apiServer) handleDatabaseRename(w http.ResponseWriter, r *http.Request) {
	d, ok := databaseManager(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeDatabaseBody(w, r, &req) {
		return
	}
	source := pathParam(r, "db")
	if !validDatabaseInput(req.Name) || !validDatabaseInput(source) || req.Name == source {
		WriteErr(w, 400, ErrBadRequest("a different database name is required"))
		return
	}
	if err := d.RenameDatabase(r.Context(), source, req.Name); err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *apiServer) handleTableTransfer(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	d, ok := sess.Driver.(driver.TableTransfer)
	if !ok || !sess.Driver.Caps().MoveCopy {
		databaseUnsupported(w)
		return
	}
	var req struct {
		Action   string   `json:"action"`
		Tables   []string `json:"tables"`
		TargetDB string   `json:"targetDB"`
	}
	if !decodeDatabaseBody(w, r, &req) {
		return
	}
	source := pathParam(r, "db")
	if !validDatabaseInput(source) || !validDatabaseInput(req.TargetDB) || source == req.TargetDB || len(req.Tables) == 0 || (req.Action != "copy" && req.Action != "move") {
		WriteErr(w, 400, ErrBadRequest("select tables, a different target database, and copy or move"))
		return
	}
	seen := map[string]bool{}
	for _, name := range req.Tables {
		if !validDatabaseInput(name) || seen[name] {
			WriteErr(w, 400, ErrBadRequest("table names must be nonempty and unique"))
			return
		}
		seen[name] = true
	}
	var err error
	if req.Action == "move" {
		err = d.MoveTables(r.Context(), source, req.Tables, req.TargetDB)
	} else {
		err = d.CopyTables(r.Context(), source, req.Tables, req.TargetDB)
	}
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *apiServer) handleMetadata(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	d, ok := sess.Driver.(driver.MetadataProvider)
	if !ok {
		databaseUnsupported(w)
		return
	}
	meta, err := d.Metadata(r.Context())
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	if meta.ColumnTypes == nil {
		meta.ColumnTypes = []driver.TypeInfo{}
	}
	if meta.Collations == nil {
		meta.Collations = []driver.Collation{}
	}
	if meta.Operators == nil {
		meta.Operators = []driver.Operator{}
	}
	WriteJSON(w, 200, meta)
}
