package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/session"
)

func (s *apiServer) registerSchemaRoutes(r chi.Router, prefix string) {
	r.Get(prefix+"/schemas", s.handleSchemasList)
	r.Get(prefix+"/sequences", s.handleSequencesList)
	r.Post(prefix+"/sequences", s.handleSequenceCreate)
	r.Route(prefix+"/sequences/{name}", func(seq chi.Router) {
		seq.Delete("/", s.handleSequenceDrop)
		seq.Patch("/", s.handleSequenceUpdate)
	})
}

// requestSchema binds schema to the request, including console and transfer
// routes. No mutable session-wide selection is changed.
func requestSchema(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, _ := session.FromContext(r.Context())
		if _, ok := sess.Driver.(driver.SchemaAware); ok {
			r = r.WithContext(driver.WithSchema(r.Context(), r.URL.Query().Get("schema")))
		}
		next.ServeHTTP(w, r)
	})
}

// sequenceAware extracts the SequenceManager capability or answers with
// the driver_not_supported envelope (spec §4.4).
func sequenceAware(w http.ResponseWriter, r *http.Request) (driver.SequenceManager, bool) {
	sess, _ := session.FromContext(r.Context())
	sm, ok := sess.Driver.(driver.SequenceManager)
	if !ok {
		WriteErr(w, http.StatusNotFound, &Error{
			Code:    "driver_not_supported",
			Message: "sequences are not supported by this driver",
		})
		return nil, false
	}
	return sm, true
}

// handleSchemasList lists the schemas of the request's database.
func (s *apiServer) handleSchemasList(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	sa, ok := sess.Driver.(driver.SchemaAware)
	if !ok {
		WriteErr(w, http.StatusNotFound, &Error{
			Code:    "driver_not_supported",
			Message: "schemas are not supported by this driver",
		})
		return
	}
	schemas, err := sa.ListSchemas(r.Context())
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	if schemas == nil {
		schemas = []string{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"schemas": schemas})
}

type sequenceRequest struct {
	Name      string `json:"name"`
	Increment int64  `json:"increment"`
	Restart   *int64 `json:"restart"`
}

// handleSequencesList lists the sequences of ?schema= (default current).
func (s *apiServer) handleSequencesList(w http.ResponseWriter, r *http.Request) {
	sm, ok := sequenceAware(w, r)
	if !ok {
		return
	}
	schema := driver.SchemaFromContext(r.Context())
	if schema == "" {
		schema = "public"
	}
	seqs, err := sm.ListSequences(r.Context(), schema)
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	if seqs == nil {
		seqs = []driver.SequenceInfo{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"sequences": seqs})
}

// handleSequenceCreate creates a sequence in the current schema.
func (s *apiServer) handleSequenceCreate(w http.ResponseWriter, r *http.Request) {
	sm, ok := sequenceAware(w, r)
	if !ok {
		return
	}
	var req sequenceRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("sequence name is required"))
		return
	}
	if err := sm.CreateSequence(r.Context(), req.Name, req.Increment); err != nil {
		writeDriverErr(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

// handleSequenceDrop drops the {name} sequence.
func (s *apiServer) handleSequenceDrop(w http.ResponseWriter, r *http.Request) {
	sm, ok := sequenceAware(w, r)
	if !ok {
		return
	}
	name := pathParam(r, "name")
	if name == "" {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("sequence name is required"))
		return
	}
	if err := sm.DropSequence(r.Context(), name); err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSequenceUpdate alters the {name} sequence (restart/increment).
func (s *apiServer) handleSequenceUpdate(w http.ResponseWriter, r *http.Request) {
	sm, ok := sequenceAware(w, r)
	if !ok {
		return
	}
	name := pathParam(r, "name")
	if name == "" {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("sequence name is required"))
		return
	}
	var req sequenceRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Restart == nil && req.Increment == 0 {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("restart or increment is required"))
		return
	}
	var inc *int64
	if req.Increment != 0 {
		inc = &req.Increment
	}
	if err := sm.AlterSequence(r.Context(), name, req.Restart, inc); err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
