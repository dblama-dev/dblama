package api

import (
	"net/http"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/session"
)

func roleManager(w http.ResponseWriter, r *http.Request) (driver.RoleManager, bool) {
	sess, _ := session.FromContext(r.Context())
	d, ok := sess.Driver.(driver.RoleManager)
	if !ok || !sess.Driver.Caps().Roles {
		adminUnsupported(w)
		return nil, false
	}
	return d, true
}

func (s *apiServer) rolesList(w http.ResponseWriter, r *http.Request) {
	d, ok := roleManager(w, r)
	if !ok {
		return
	}
	roles, err := d.ListRoles(r.Context())
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	WriteJSON(w, 200, roles)
}

func (s *apiServer) rolesChange(w http.ResponseWriter, r *http.Request) {
	d, ok := roleManager(w, r)
	if !ok {
		return
	}
	var c driver.RoleChange
	if decodeJSONBody(w, r, &c) {
		adminDone(w, d.ChangeRole(r.Context(), c))
	}
}

func (s *apiServer) rolePrivileges(w http.ResponseWriter, r *http.Request) {
	d, ok := roleManager(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	obj := driver.RoleObject{Kind: q.Get("kind"), Schema: q.Get("schema"), Name: q.Get("name")}
	grants, err := d.RolePrivileges(r.Context(), q.Get("role"), obj)
	if err != nil {
		writeDriverErr(w, err)
		return
	}
	WriteJSON(w, 200, grants)
}

func (s *apiServer) rolePrivilegeChange(w http.ResponseWriter, r *http.Request) {
	d, ok := roleManager(w, r)
	if !ok {
		return
	}
	var req struct {
		Role      string            `json:"role"`
		Object    driver.RoleObject `json:"object"`
		Privilege string            `json:"privilege"`
		Grant     *bool             `json:"grant"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Grant == nil {
		WriteErr(w, 400, ErrBadRequest("grant flag required"))
		return
	}
	adminDone(w, d.ChangeRolePrivilege(r.Context(), req.Role, req.Object, req.Privilege, *req.Grant))
}
