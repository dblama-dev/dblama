package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/sqlgen"
)

var _ driver.RoleManager = (*PostgresDriver)(nil)

// ListRoles returns cluster roles and their direct memberships.
func (d *PostgresDriver) ListRoles(ctx context.Context) ([]driver.RoleInfo, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT DISTINCT r.rolname,r.rolcanlogin,r.rolsuper,coalesce(g.rolname,'')
FROM pg_catalog.pg_roles r LEFT JOIN pg_catalog.pg_auth_members m ON m.member=r.oid
LEFT JOIN pg_catalog.pg_roles g ON g.oid=m.roleid ORDER BY 1,4`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	roles := []driver.RoleInfo{}
	for rows.Next() {
		var role driver.RoleInfo
		var group string
		if err := rows.Scan(&role.Name, &role.Login, &role.Superuser, &group); err != nil {
			return nil, err
		}
		if len(roles) == 0 || roles[len(roles)-1].Name != role.Name {
			role.Memberships = []string{}
			roles = append(roles, role)
		}
		if group != "" {
			last := &roles[len(roles)-1]
			last.Memberships = append(last.Memberships, group)
		}
	}
	return roles, rows.Err()
}

// Reject names PostgreSQL would truncate before looking up the target.
func checkedIdentifier(name string) (string, error) {
	if name == "" || len(name) > 63 || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: expected an identifier of 1 to 63 bytes", driver.ErrRoleInput)
	}
	return sqlgen.QuoteIdentPg(name), nil
}

func roleIdentifier(name string) (string, error) {
	if name == "public" {
		return "", fmt.Errorf("%w: PUBLIC is not an individual role", driver.ErrRoleInput)
	}
	return checkedIdentifier(name)
}

// ChangeRole applies one cluster-wide change with the current session privileges.
func (d *PostgresDriver) ChangeRole(ctx context.Context, c driver.RoleChange) error {
	name, err := roleIdentifier(c.Name)
	if err != nil {
		return err
	}
	var stmt string
	switch c.Action {
	case "create", "login":
		if c.Login == nil {
			return fmt.Errorf("%w: login flag required", driver.ErrRoleInput)
		}
		stmt = "ALTER ROLE " + name
		if c.Action == "create" {
			stmt = "CREATE ROLE " + name
		}
		if *c.Login {
			stmt += " LOGIN"
		} else {
			stmt += " NOLOGIN"
		}
	case "password":
		if c.Password == "" {
			return fmt.Errorf("%w: password required", driver.ErrRoleInput)
		}
		stmt = "ALTER ROLE " + name
	case "join", "leave":
		if c.Password != "" {
			return driver.ErrRoleInput
		}
		return d.changeMembership(ctx, c.Name, c.Group, c.Action == "join")
	case "drop":
		stmt = "DROP ROLE " + name
	default:
		return driver.ErrRoleInput
	}
	if c.Password != "" {
		if (c.Action != "create" && c.Action != "password") || strings.ContainsRune(c.Password, 0) {
			return driver.ErrRoleInput
		}
		// E literals interpret backslashes explicitly regardless of session settings.
		stmt += " PASSWORD E'" + strings.NewReplacer(`\`, `\\`, "'", "''").Replace(c.Password) + "'"
	}
	_, err = d.db.ExecContext(ctx, stmt)
	if err != nil && c.Password != "" {
		return errors.New("role update failed; check the role name, password policy and permissions")
	}
	return err
}

func (d *PostgresDriver) changeMembership(ctx context.Context, member, parent string, join bool) error {
	name, err := roleIdentifier(parent)
	if err != nil {
		return err
	}
	target := sqlgen.QuoteIdentPg(member) // validated by ChangeRole
	stmt := "REVOKE " + name + " FROM " + target
	if join {
		stmt = "GRANT " + name + " TO " + target
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, stmt); err != nil {
		return err
	}
	// A membership can have several grantors. REVOKE may leave it intact.
	var present bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT FROM pg_catalog.pg_auth_members m
		JOIN pg_catalog.pg_roles r ON r.oid=m.member
		JOIN pg_catalog.pg_roles g ON g.oid=m.roleid
		WHERE r.rolname=$1 AND g.rolname=$2)`, member, parent).Scan(&present)
	if err != nil {
		return err
	}
	if present != join {
		return errors.New("membership did not change; check administration rights and other grantors")
	}
	return tx.Commit()
}
