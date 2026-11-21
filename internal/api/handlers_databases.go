package api

import (
	"net/http"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/session"
)

// handleDrivers lists the registered drivers for the login screen. It is
// public: no session is required.
func (s *apiServer) handleDrivers(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, s.drivers.List())
}

// handleDatabases lists the databases on the connected server.
func (s *apiServer) handleDatabases(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())

	dbs, err := sess.Driver.ListDatabases(r.Context())
	if err != nil {
		WriteErr(w, http.StatusInternalServerError, ErrDriver(err.Error()))
		return
	}
	if dbs == nil {
		dbs = []driver.DatabaseInfo{}
	}
	WriteJSON(w, http.StatusOK, databasesResponse{Databases: dbs})
}

// databasesResponse is the GET /api/databases body.
type databasesResponse struct {
	Databases []driver.DatabaseInfo `json:"databases"`
}
