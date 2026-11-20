package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
)

// registerObjectRoutes is mounted inside the authenticated, CSRF-protected,
// request-schema group. Every extension requires both its interface and cap.
func (s *apiServer) registerObjectRoutes(r chi.Router) {
	for _, kind := range []string{"views", "triggers", "routines", "events"} {
		path := "/databases/{db}/" + kind
		r.Get(path, s.handleObjects(kind))
		r.Put(path+"/{name}", s.handleObjects(kind))
		r.Delete(path+"/{name}", s.handleObjects(kind))
	}
	r.Post("/databases/{db}/routines/{name}/call", s.handleProcedureCall)
	path := "/databases/{db}/tables/{table}/partitions"
	r.Get(path, s.handlePartitions)
	r.Post(path, s.handlePartitions)
	r.Delete(path+"/{name}", s.handlePartitions)
	r.Post(path+"/{name}/truncate", s.handlePartitions)
}
func objectUnsupported(w http.ResponseWriter) {
	WriteErr(w, http.StatusNotFound, &Error{Code: "driver_not_supported", Message: "object capability is not supported by this driver"})
}
func writeObjectErr(w http.ResponseWriter, err error) {
	if errors.Is(err, driver.ErrObjectValidation) {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest(err.Error()))
		return
	}
	writeDriverErr(w, err)
}
func (s *apiServer) handleObjects(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, _ := session.FromContext(r.Context())
		db, name := pathParam(r, "db"), pathParam(r, "name")
		ctx := r.Context()
		var list func() ([]driver.ObjectInfo, error)
		var save func(driver.ObjectDef) error
		var drop func(driver.ObjectDef) error
		switch kind {
		case "views":
			m, ok := sess.Driver.(driver.ViewManager)
			if !ok || !sess.Driver.Caps().Views {
				objectUnsupported(w)
				return
			}
			list = func() ([]driver.ObjectInfo, error) { return m.ListViews(ctx, db) }
			save = func(def driver.ObjectDef) error { return m.AlterView(ctx, db, name, def) }
			drop = func(driver.ObjectDef) error { return m.DropView(ctx, db, name) }
		case "triggers":
			m, ok := sess.Driver.(driver.TriggerManager)
			if !ok || !sess.Driver.Caps().Triggers {
				objectUnsupported(w)
				return
			}
			list = func() ([]driver.ObjectInfo, error) { return m.ListTriggers(ctx, db) }
			save = func(def driver.ObjectDef) error { return m.CreateTrigger(ctx, db, name, def) }
			drop = func(def driver.ObjectDef) error { return m.DropTrigger(ctx, db, name, def.Table) }
		case "routines":
			m, ok := sess.Driver.(driver.RoutineManager)
			if !ok || !sess.Driver.Caps().Routines {
				objectUnsupported(w)
				return
			}
			list = func() ([]driver.ObjectInfo, error) { return m.ListRoutines(ctx, db) }
			save = func(def driver.ObjectDef) error { return m.CreateRoutine(ctx, db, name, def) }
			drop = func(def driver.ObjectDef) error { return m.DropRoutine(ctx, db, name, def.Kind, def.Identity) }
		case "events":
			m, ok := sess.Driver.(driver.EventManager)
			if !ok || !sess.Driver.Caps().Events {
				objectUnsupported(w)
				return
			}
			list = func() ([]driver.ObjectInfo, error) { return m.ListEvents(ctx, db) }
			save = func(def driver.ObjectDef) error { return m.CreateEvent(ctx, db, name, def) }
			drop = func(driver.ObjectDef) error { return m.DropEvent(ctx, db, name) }
		}
		if r.Method == http.MethodGet {
			objects, err := list()
			if err != nil {
				writeObjectErr(w, err)
				return
			}
			if objects == nil {
				objects = []driver.ObjectInfo{}
			}
			WriteJSON(w, http.StatusOK, map[string]any{"objects": objects})
			return
		}
		var def driver.ObjectDef
		if r.Method == http.MethodPut {
			if !decodeJSONBody(w, r, &def) {
				return
			}
			if strings.TrimSpace(def.Source) == "" {
				WriteErr(w, http.StatusBadRequest, ErrBadRequest("SQL source is required"))
				return
			}
		} else {
			def.Kind = r.URL.Query().Get("kind")
			def.Table = r.URL.Query().Get("table")
			def.Identity = r.URL.Query().Get("identity")
		}
		if kind == "routines" {
			def.Kind = strings.ToUpper(def.Kind)
			if def.Kind != "FUNCTION" && def.Kind != "PROCEDURE" {
				WriteErr(w, http.StatusBadRequest, ErrBadRequest("routine kind must be FUNCTION or PROCEDURE"))
				return
			}
			if def.Kind == "PROCEDURE" && !sess.Driver.Caps().Procedures {
				objectUnsupported(w)
				return
			}
		}
		var err error
		if r.Method == http.MethodPut {
			err = save(def)
		} else {
			err = drop(def)
		}
		if err != nil {
			writeObjectErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
func (s *apiServer) handleProcedureCall(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	m, ok := sess.Driver.(driver.RoutineManager)
	if !ok || !sess.Driver.Caps().Routines || !sess.Driver.Caps().Procedures {
		objectUnsupported(w)
		return
	}
	var req struct {
		Identity string `json:"identity"`
		Args     []any  `json:"args"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.Args) > 256 {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("too many procedure arguments"))
		return
	}
	for _, v := range req.Args {
		switch v.(type) {
		case nil, string, float64, bool:
		default:
			WriteErr(w, http.StatusBadRequest, ErrBadRequest("procedure arguments must be scalar values or null"))
			return
		}
	}
	results, err := m.CallProcedure(r.Context(), pathParam(r, "db"), pathParam(r, "name"), req.Identity, req.Args)
	if err != nil {
		writeObjectErr(w, err)
		return
	}
	if results == nil {
		results = []driver.Result{}
	}
	for i := range results {
		if results[i].Columns == nil {
			results[i].Columns = []driver.ColumnMeta{}
		}
		if results[i].Rows == nil {
			results[i].Rows = [][]any{}
		}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"results": results})
}
func (s *apiServer) handlePartitions(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	m, ok := sess.Driver.(driver.PartitionManager)
	if !ok || !sess.Driver.Caps().Partitions {
		objectUnsupported(w)
		return
	}
	db, table, name := pathParam(r, "db"), pathParam(r, "table"), pathParam(r, "name")
	ctx := r.Context()
	if r.Method == http.MethodGet {
		parts, err := m.ListPartitions(ctx, db, table)
		if err != nil {
			writeObjectErr(w, err)
			return
		}
		if parts == nil {
			parts = []driver.PartitionInfo{}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"partitions": parts})
		return
	}
	var err error
	switch {
	case r.Method == http.MethodDelete:
		err = m.DropPartition(ctx, db, table, name)
	case name != "":
		err = m.TruncatePartition(ctx, db, table, name)
	default:
		var req driver.ObjectDef
		if !decodeJSONBody(w, r, &req) {
			return
		}
		err = m.AddPartition(ctx, db, table, req.Source)
	}
	if err != nil {
		writeObjectErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
