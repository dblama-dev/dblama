package driver

import (
	"context"
	"errors"
)

// ErrRoleInput identifies a malformed role or object privilege request.
var ErrRoleInput = errors.New("invalid role operation")

// RoleInfo describes a cluster role and its direct group memberships.
type RoleInfo struct {
	Name        string   `json:"name"`
	Login       bool     `json:"login"`
	Superuser   bool     `json:"superuser"`
	Memberships []string `json:"memberships"`
}

// RoleChange requests one cluster-level role or membership operation.
type RoleChange struct {
	Action   string `json:"action"`
	Name     string `json:"name"`
	Login    *bool  `json:"login,omitempty"`
	Password string `json:"password,omitempty"`
	Group    string `json:"group,omitempty"`
}

// RoleObject identifies an object within the request-scoped database.
type RoleObject struct {
	Kind   string `json:"kind"`
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

// ObjectPrivilege records a privilege and the role or PUBLIC grant that supplies it.
type ObjectPrivilege struct {
	Privilege string `json:"privilege"`
	Source    string `json:"source"`
	Public    bool   `json:"public"`
}

// RoleObjectPrivileges lists the grants and editable privileges for one object.
type RoleObjectPrivileges struct {
	Editable []string          `json:"editable"`
	Grants   []ObjectPrivilege `json:"grants"`
}

// RoleManager administers PostgreSQL-style roles and object privileges.
type RoleManager interface {
	ListRoles(context.Context) ([]RoleInfo, error)
	ChangeRole(context.Context, RoleChange) error
	RolePrivileges(context.Context, string, RoleObject) (RoleObjectPrivileges, error)
	ChangeRolePrivilege(ctx context.Context, role string, object RoleObject, privilege string, grant bool) error
}
