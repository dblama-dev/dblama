package mysql

import (
	"context"
	"fmt"
	"github.com/hitslab/db-manager/server/internal/driver"
	"strings"
)

var _ driver.UserManager = (*MysqlDriver)(nil)

func validAccount(a driver.UserAccount) error {
	if a.Host == "" || strings.ContainsRune(a.User+a.Host, 0) {
		return fmt.Errorf("host is required; user and host must not contain NUL")
	}
	return nil
}

// ListUsers lists accounts, authentication plugins and resource limits.
func (d *MysqlDriver) ListUsers(ctx context.Context) ([]driver.UserInfo, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT User, Host, COALESCE(plugin,''), max_questions, max_updates, max_connections, max_user_connections FROM mysql.user ORDER BY User,Host")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	users := []driver.UserInfo{}
	for rows.Next() {
		var u driver.UserInfo
		if err := rows.Scan(&u.User, &u.Host, &u.Plugin, &u.Limits.QueriesPerHour, &u.Limits.UpdatesPerHour, &u.Limits.ConnectionsPerHour, &u.Limits.UserConnections); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// CreateUser creates an account using bound credentials.
func (d *MysqlDriver) CreateUser(ctx context.Context, a driver.UserAccount, password string) error {
	if err := validAccount(a); err != nil {
		return err
	}
	_, err := d.db.ExecContext(ctx, "CREATE USER ?@? IDENTIFIED BY ?", a.User, a.Host, password)
	if err != nil {
		return fmt.Errorf("create user failed; check account existence, password policy and permissions")
	}
	return nil
}

// DropUser removes an account.
func (d *MysqlDriver) DropUser(ctx context.Context, a driver.UserAccount) error {
	if err := validAccount(a); err != nil {
		return err
	}
	_, err := d.db.ExecContext(ctx, "DROP USER ?@?", a.User, a.Host)
	return err
}

// RenameUser renames an account and its host.
func (d *MysqlDriver) RenameUser(ctx context.Context, a, b driver.UserAccount) error {
	if err := validAccount(a); err != nil {
		return err
	}
	if err := validAccount(b); err != nil {
		return err
	}
	_, err := d.db.ExecContext(ctx, "RENAME USER ?@? TO ?@?", a.User, a.Host, b.User, b.Host)
	return err
}

// SetPassword changes an account password without exposing it in errors.
func (d *MysqlDriver) SetPassword(ctx context.Context, a driver.UserAccount, password string) error {
	if err := validAccount(a); err != nil {
		return err
	}
	_, err := d.db.ExecContext(ctx, "ALTER USER ?@? IDENTIFIED BY ?", a.User, a.Host, password)
	if err != nil {
		return fmt.Errorf("change password failed; check account existence, password policy and permissions")
	}
	return nil
}

// SetUserLimits sets per-account resource limits.
func (d *MysqlDriver) SetUserLimits(ctx context.Context, a driver.UserAccount, l driver.UserLimits) error {
	if err := validAccount(a); err != nil {
		return err
	}
	_, err := d.db.ExecContext(ctx, "ALTER USER ?@? WITH MAX_QUERIES_PER_HOUR ? MAX_UPDATES_PER_HOUR ? MAX_CONNECTIONS_PER_HOUR ? MAX_USER_CONNECTIONS ?", a.User, a.Host, l.QueriesPerHour, l.UpdatesPerHour, l.ConnectionsPerHour, l.UserConnections)
	return err
}

// UserGrants returns parsed grants without authentication clauses or hashes.
func (d *MysqlDriver) UserGrants(ctx context.Context, a driver.UserAccount) ([]driver.GrantInfo, error) {
	if err := validAccount(a); err != nil {
		return nil, err
	}
	literal, err := d.grantDatabaseLiteralMode(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, "SHOW GRANTS FOR ?@?", a.User, a.Host)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	grants := []driver.GrantInfo{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		grants = append(grants, parseGrant(raw, literal))
	}
	return grants, rows.Err()
}

// GrantPrivileges grants whitelisted privileges on a structured scope.
func (d *MysqlDriver) GrantPrivileges(ctx context.Context, a driver.UserAccount, g driver.GrantChange) error {
	return d.changePrivileges(ctx, a, g, false)
}

// RevokePrivileges revokes whitelisted privileges on a structured scope.
func (d *MysqlDriver) RevokePrivileges(ctx context.Context, a driver.UserAccount, g driver.GrantChange) error {
	return d.changePrivileges(ctx, a, g, true)
}
func (d *MysqlDriver) changePrivileges(ctx context.Context, a driver.UserAccount, g driver.GrantChange, revoke bool) error {
	if err := validAccount(a); err != nil {
		return err
	}
	literal := false
	if g.Scope.Level == "database" {
		var err error
		literal, err = d.grantDatabaseLiteralMode(ctx)
		if err != nil {
			return err
		}
	}
	scope, err := grantScopeSQL(g.Scope, literal)
	if err != nil {
		return err
	}
	if len(g.Privileges) == 0 && g.GrantOption {
		statement := "GRANT USAGE ON " + scope + " TO ?@? WITH GRANT OPTION"
		if revoke {
			statement = "REVOKE GRANT OPTION ON " + scope + " FROM ?@?"
		}
		_, err := d.db.ExecContext(ctx, statement, a.User, a.Host)
		return err
	}
	catalog, err := d.ReadPrivilegeCatalog(ctx)
	if err != nil {
		return err
	}
	privileges, err := privilegesSQL(g.Privileges, g.Scope, catalog)
	if err != nil {
		return err
	}
	statement := "GRANT " + privileges + " ON " + scope + " TO ?@?"
	if revoke {
		statement = "REVOKE " + privileges + " ON " + scope + " FROM ?@?"
	} else if g.GrantOption {
		statement += " WITH GRANT OPTION"
	}
	if _, err := d.db.ExecContext(ctx, statement, a.User, a.Host); err != nil {
		return err
	}
	if revoke && g.GrantOption {
		_, err = d.db.ExecContext(ctx, "REVOKE GRANT OPTION ON "+scope+" FROM ?@?", a.User, a.Host)
	}
	return err
}
