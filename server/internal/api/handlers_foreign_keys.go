package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
	"net/http"
)

func (s *apiServer) registerForeignKeyRoutes(r chi.Router) {
	r.Post("/databases/{db}/tables/{table}/foreign-keys", s.handleForeignKeysSave)
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
	if err := m.SaveForeignKeys(r.Context(), chi.URLParam(r, "db"), chi.URLParam(r, "table"), keys); err != nil {
		writeObjectErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
