package driver

import (
	"context"
	"errors"
	"strings"
)

// UserManager is an optional MySQL/MariaDB administration capability.
type UserManager interface {
	ListUsers(context.Context) ([]UserInfo, error)
	CreateUser(context.Context, UserAccount, string) error
	DropUser(context.Context, UserAccount) error
	RenameUser(context.Context, UserAccount, UserAccount) error
	SetPassword(context.Context, UserAccount, string) error
	SetUserLimits(context.Context, UserAccount, UserLimits) error
	UserGrants(context.Context, UserAccount) ([]GrantInfo, error)
	PrivilegeCatalog() []PrivilegeInfo
	DatabasePatternsSupported(context.Context) (bool, error)
	GrantPrivileges(context.Context, UserAccount, GrantChange) error
	RevokePrivileges(context.Context, UserAccount, GrantChange) error
}

// UserAccount identifies a MySQL account by user and host.
type UserAccount struct {
	User string `json:"user"`
	Host string `json:"host"`
}

// UserInfo describes an account without credentials or password hashes.
type UserInfo struct {
	UserAccount
	Plugin string     `json:"plugin"`
	Limits UserLimits `json:"limits"`
}

// UserLimits contains nonnegative account resource limits; zero is unlimited.
type UserLimits struct {
	QueriesPerHour     uint32 `json:"queriesPerHour"`
	UpdatesPerHour     uint32 `json:"updatesPerHour"`
	ConnectionsPerHour uint32 `json:"connectionsPerHour"`
	UserConnections    uint32 `json:"userConnections"`
}

// DatabasePatternLevel is a native wildcard grant on a database name.
const DatabasePatternLevel = "databasePattern"

// ErrDatabasePatternsDisabled reports a server mode that requires exact schema names.
var ErrDatabasePatternsDisabled = errors.New("database privilege patterns are unavailable when partial_revokes is enabled")

// GrantScope identifies a global, literal database, database pattern or table grant target.
type GrantScope struct {
	Level    string `json:"level"`
	Database string `json:"database,omitempty"`
	Table    string `json:"table,omitempty"`
}

// PrivilegeLevel returns the server catalog level used for validation.
func (s GrantScope) PrivilegeLevel() string {
	if s.Level == DatabasePatternLevel {
		return "database"
	}
	return s.Level
}

// Valid checks the requested target before it can be used in a GRANT or REVOKE.
func (s GrantScope) Valid() bool {
	if strings.ContainsRune(s.Database+s.Table, 0) {
		return false
	}
	switch s.Level {
	case "global":
		return s.Database == "" && s.Table == ""
	case "database":
		return s.Database != "" && s.Table == ""
	case DatabasePatternLevel:
		parsed, valid := ParseDatabaseGrantName(s.Database)
		return s.Table == "" && valid && parsed.Level == DatabasePatternLevel
	case "table":
		return s.Database != "" && s.Table != ""
	}
	return false
}

// ParseDatabaseGrantName decodes a database-level name when the server enables
// wildcards. Preserve patterns exactly so their escapes survive a revoke.
// Unrecognized escapes retain their original target but cannot be edited.
func ParseDatabaseGrantName(name string) (GrantScope, bool) {
	scope := GrantScope{Level: "database", Database: name}
	valid := name != "" && !strings.ContainsRune(name, 0)
	var literal strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch c {
		case '%', '_':
			scope.Level = DatabasePatternLevel
		case '\\':
			i++
			if i == len(name) {
				return scope, false
			}
			c = name[i]
			if c != '\\' && c != '%' && c != '_' {
				valid = false
			}
		}
		literal.WriteByte(c)
	}
	if scope.Level == "database" && valid {
		scope.Database = literal.String()
	}
	return scope, valid
}

// PrivilegeInfo declares a privilege and the levels where it is valid.
type PrivilegeInfo struct {
	Name   string   `json:"name"`
	Levels []string `json:"levels"`
}

// GrantInfo describes a parsed grant; unsupported forms are not editable.
type GrantInfo struct {
	Scope       GrantScope `json:"scope"`
	Privileges  []string   `json:"privileges"`
	GrantOption bool       `json:"grantOption"`
	Editable    bool       `json:"editable"`
}

// GrantChange requests changes to whitelisted privileges at one scope.
type GrantChange struct {
	Scope       GrantScope `json:"scope"`
	Privileges  []string   `json:"privileges"`
	GrantOption bool       `json:"grantOption"`
}

// MonitorManager exposes read-only metrics and explicit process termination.
type MonitorManager interface {
	Processes(context.Context) ([]ProcessInfo, error)
	KillProcess(context.Context, int64) error
	ServerStatus(context.Context) ([]ServerValue, error)
	ServerVariables(context.Context) ([]ServerValue, error)
}

// ProcessInfo describes one server connection and its current activity.
type ProcessInfo struct {
	ID       int64  `json:"id"`
	User     string `json:"user"`
	Host     string `json:"host"`
	Database string `json:"database"`
	Command  string `json:"command"`
	Seconds  int64  `json:"seconds"`
	State    string `json:"state"`
	Query    string `json:"query"`
}

// ServerValue is a named status counter or configuration variable.
type ServerValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// PrivilegeCatalogReader allows request cancellation and precise read errors
// while retaining the context-free UserManager metadata compatibility method.
type PrivilegeCatalogReader interface {
	ReadPrivilegeCatalog(context.Context) ([]PrivilegeInfo, error)
}
