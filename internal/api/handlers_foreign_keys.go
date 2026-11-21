package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/session"
)

func (s *apiServer) registerForeignKeyRoutes(r chi.Router) {
	r.Get("/databases/{db}/relationships", s.handleRelationships)
	r.Post("/databases/{db}/tables/{table}/foreign-keys", s.handleForeignKeysSave)
}

func (s *apiServer) handleRelationships(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	reader, ok := sess.Driver.(driver.RelationshipReader)
	if !ok {
		objectUnsupported(w)
		return
	}
	relations, err := reader.ListRelationships(r.Context(), pathParam(r, "db"))
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	if relations == nil {
		relations = []driver.TableRelationship{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"relationships": relations})
}

func (s *apiServer) handleForeignKeysSave(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	m, ok := sess.Driver.(driver.ForeignKeyManager)
	if !ok {
		objectUnsupported(w)
		return
	}
	var keys []driver.ForeignKey
	if !decodeJSONBody(w, r, &keys) {
		return
	}
	if keys == nil {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("foreign keys must be an array; use [] to remove all constraints"))
		return
	}
	if err := driver.ValidateForeignKeys(keys, true); err != nil {
		writeObjectErr(w, err)
		return
	}
	if err := m.SaveForeignKeys(r.Context(), pathParam(r, "db"), pathParam(r, "table"), keys); err != nil {
		writeObjectErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
