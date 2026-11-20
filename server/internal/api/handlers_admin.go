package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
	"net/http"
	"strconv"
	"strings"
)

func (s *apiServer) registerAdminRoutes(r chi.Router) {
	r.Get("/users", s.adminUsers)
	r.Post("/users", s.adminCreateUser)
	r.Get("/users/privileges", s.adminPrivileges)
	r.Route("/users/{user}", func(u chi.Router) {
		u.Delete("/", s.adminDropUser)
		u.Patch("/", s.adminRenameUser)
		u.Put("/password", s.adminPassword)
		u.Put("/limits", s.adminLimits)
		u.Get("/grants", s.adminGrants)
		u.Post("/grants", s.adminGrant)
		u.Delete("/grants", s.adminGrant)
	})
	r.Get("/processes", s.adminProcesses)
	r.Post("/processes/{id}/kill", s.adminKill)
	r.Get("/status", s.adminStatus)
	r.Get("/variables", s.adminVariables)
}
func adminUnsupported(w http.ResponseWriter) {
	WriteErr(w, http.StatusNotFound, &Error{Code: "driver_not_supported", Message: "administration feature is not supported by this driver"})
}
func adminUsersDriver(w http.ResponseWriter, r *http.Request) (driver.UserManager, bool) {
	sess, ok := session.FromContext(r.Context())
	if !ok {
		WriteErr(w, 401, ErrAuthRequired())
		return nil, false
	}
	d, ok := sess.Driver.(driver.UserManager)
	if !ok || !sess.Driver.Caps().Users {
		adminUnsupported(w)
		return nil, false
	}
	return d, true
}
func adminMonitorDriver(w http.ResponseWriter, r *http.Request) (driver.MonitorManager, bool) {
	sess, ok := session.FromContext(r.Context())
	if !ok {
		WriteErr(w, 401, ErrAuthRequired())
		return nil, false
	}
	d, ok := sess.Driver.(driver.MonitorManager)
	if !ok || !sess.Driver.Caps().Monitor {
		adminUnsupported(w)
		return nil, false
	}
	return d, true
}
func adminAccount(w http.ResponseWriter, r *http.Request) (driver.UserAccount, bool) {
	a := driver.UserAccount{User: pathParam(r, "user"), Host: r.URL.Query().Get("host")}
	if a.User == "_anonymous" && r.URL.Query().Get("anonymous") == "true" {
		a.User = ""
	}
	return a, adminValidAccount(w, a)
}
func adminValidAccount(w http.ResponseWriter, a driver.UserAccount) bool {
	if a.Host == "" || strings.ContainsRune(a.User+a.Host, 0) {
		WriteErr(w, 400, ErrBadRequest("host is required"))
		return false
	}
	return true
}
func adminDone(w http.ResponseWriter, err error) {
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *apiServer) adminUsers(w http.ResponseWriter, r *http.Request) {
	d, ok := adminUsersDriver(w, r)
	if !ok {
		return
	}
	v, err := d.ListUsers(r.Context())
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	if v == nil {
		v = []driver.UserInfo{}
	}
	WriteJSON(w, 200, map[string]any{"users": v})
}
func (s *apiServer) adminPrivileges(w http.ResponseWriter, r *http.Request) {
	d, ok := adminUsersDriver(w, r)
	if !ok {
		return
	}
	catalog, ok := adminCatalog(w, r, d)
	if !ok {
		return
	}
	patterns, err := d.DatabasePatternsSupported(r.Context())
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	WriteJSON(w, 200, map[string]any{"privileges": catalog, "databasePatterns": patterns})
}
func (s *apiServer) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	d, ok := adminUsersDriver(w, r)
	if !ok {
		return
	}
	var req struct {
		driver.UserAccount
		Password string `json:"password"`
	}
	if !decodeJSONBody(w, r, &req) || !adminValidAccount(w, req.UserAccount) {
		return
	}
	if err := d.CreateUser(r.Context(), req.UserAccount, req.Password); err != nil {
		WriteErr(w, 500, ErrDriver("create user failed; check account, password policy and permissions"))
		return
	}
	WriteJSON(w, 201, map[string]any{"ok": true})
}
func (s *apiServer) adminDropUser(w http.ResponseWriter, r *http.Request) {
	d, ok := adminUsersDriver(w, r)
	if !ok {
		return
	}
	a, ok := adminAccount(w, r)
	if !ok {
		return
	}
	adminDone(w, d.DropUser(r.Context(), a))
}
func (s *apiServer) adminRenameUser(w http.ResponseWriter, r *http.Request) {
	d, ok := adminUsersDriver(w, r)
	if !ok {
		return
	}
	a, ok := adminAccount(w, r)
	if !ok {
		return
	}
	var target driver.UserAccount
	if !decodeJSONBody(w, r, &target) || !adminValidAccount(w, target) {
		return
	}
	adminDone(w, d.RenameUser(r.Context(), a, target))
}
func (s *apiServer) adminPassword(w http.ResponseWriter, r *http.Request) {
	d, ok := adminUsersDriver(w, r)
	if !ok {
		return
	}
	a, ok := adminAccount(w, r)
	if !ok {
		return
	}
	var req struct {
		Password *string `json:"password"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Password == nil {
		WriteErr(w, 400, ErrBadRequest("password is required"))
		return
	}
	if err := d.SetPassword(r.Context(), a, *req.Password); err != nil {
		WriteErr(w, 500, ErrDriver("change password failed; check account, password policy and permissions"))
		return
	}
	w.WriteHeader(204)
}
func (s *apiServer) adminLimits(w http.ResponseWriter, r *http.Request) {
	d, ok := adminUsersDriver(w, r)
	if !ok {
		return
	}
	a, ok := adminAccount(w, r)
	if !ok {
		return
	}
	var limits driver.UserLimits
	if !decodeJSONBody(w, r, &limits) {
		return
	}
	adminDone(w, d.SetUserLimits(r.Context(), a, limits))
}
func (s *apiServer) adminGrants(w http.ResponseWriter, r *http.Request) {
	d, ok := adminUsersDriver(w, r)
	if !ok {
		return
	}
	a, ok := adminAccount(w, r)
	if !ok {
		return
	}
	v, err := d.UserGrants(r.Context(), a)
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	if v == nil {
		v = []driver.GrantInfo{}
	}
	WriteJSON(w, 200, map[string]any{"grants": v})
}
func (s *apiServer) adminGrant(w http.ResponseWriter, r *http.Request) {
	d, ok := adminUsersDriver(w, r)
	if !ok {
		return
	}
	a, ok := adminAccount(w, r)
	if !ok {
		return
	}
	var req driver.GrantChange
	if !decodeJSONBody(w, r, &req) {
		return
	}
	catalog, ok := adminCatalog(w, r, d)
	if !ok {
		return
	}
	if !adminValidGrant(w, catalog, req) {
		return
	}
	if r.Method == http.MethodDelete {
		adminDone(w, d.RevokePrivileges(r.Context(), a, req))
	} else {
		adminDone(w, d.GrantPrivileges(r.Context(), a, req))
	}
}
func adminValidGrant(w http.ResponseWriter, catalog []driver.PrivilegeInfo, g driver.GrantChange) bool {
	scope := g.Scope
	if !scope.Valid() || len(g.Privileges) == 0 && !g.GrantOption {
		WriteErr(w, 400, ErrBadRequest("invalid privilege scope or empty privileges"))
		return false
	}
	for _, p := range g.Privileges {
		found := false
		for _, entry := range catalog {
			if p == entry.Name {
				for _, level := range entry.Levels {
					if level == scope.PrivilegeLevel() {
						found = true
					}
				}
			}
		}
		if !found {
			WriteErr(w, 400, ErrBadRequest("invalid privilege for scope"))
			return false
		}
	}
	return true
}
func (s *apiServer) adminProcesses(w http.ResponseWriter, r *http.Request) {
	d, ok := adminMonitorDriver(w, r)
	if !ok {
		return
	}
	v, err := d.Processes(r.Context())
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	if v == nil {
		v = []driver.ProcessInfo{}
	}
	WriteJSON(w, 200, map[string]any{"processes": v})
}
func (s *apiServer) adminKill(w http.ResponseWriter, r *http.Request) {
	d, ok := adminMonitorDriver(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(pathParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		WriteErr(w, 400, ErrBadRequest("invalid process id"))
		return
	}
	adminDone(w, d.KillProcess(r.Context(), id))
}
func (s *apiServer) adminStatus(w http.ResponseWriter, r *http.Request)    { s.adminValues(w, r, false) }
func (s *apiServer) adminVariables(w http.ResponseWriter, r *http.Request) { s.adminValues(w, r, true) }
func (s *apiServer) adminValues(w http.ResponseWriter, r *http.Request, variables bool) {
	d, ok := adminMonitorDriver(w, r)
	if !ok {
		return
	}
	key := "status"
	fn := d.ServerStatus
	if variables {
		key = "variables"
		fn = d.ServerVariables
	}
	v, err := fn(r.Context())
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	if v == nil {
		v = []driver.ServerValue{}
	}
	WriteJSON(w, 200, map[string]any{key: v})
}

func adminCatalog(w http.ResponseWriter, r *http.Request, d driver.UserManager) ([]driver.PrivilegeInfo, bool) {
	var catalog []driver.PrivilegeInfo
	if reader, ok := d.(driver.PrivilegeCatalogReader); ok {
		var err error
		catalog, err = reader.ReadPrivilegeCatalog(r.Context())
		if err != nil {
			writeDriverErr(w, err)
			return nil, false
		}
	} else {
		catalog = d.PrivilegeCatalog()
	}
	if len(catalog) == 0 {
		WriteErr(w, 500, ErrDriver("privilege catalog unavailable"))
		return nil, false
	}
	return catalog, true
}
