package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/dblama-dev/dblama/internal/driver"
)

// Each catalog query reads a single object's ACL, including its built-in
// default when the owner has never issued GRANT or REVOKE.
type privilegeTarget struct {
	sqlName    string
	aclQuery   string
	args       []any
	privileges []string
}

func (d *PostgresDriver) privilegeTarget(obj driver.RoleObject) (privilegeTarget, error) {
	var t privilegeTarget
	switch obj.Kind {
	case "database":
		name, err := checkedIdentifier(d.database)
		if err != nil {
			return t, err
		}
		t.sqlName = "DATABASE " + name
		t.aclQuery = `SELECT coalesce(datacl,pg_catalog.acldefault('d',datdba)) FROM pg_catalog.pg_database WHERE datname=current_database()`
		t.privileges = []string{"CONNECT", "CREATE", "TEMPORARY"}
	case "schema", "table":
		schema, err := checkedIdentifier(obj.Schema)
		if err != nil {
			return t, err
		}
		t.args = []any{obj.Schema}
		if obj.Kind == "schema" {
			t.sqlName = "SCHEMA " + schema
			t.aclQuery = `SELECT coalesce(nspacl,pg_catalog.acldefault('n',nspowner)) FROM pg_catalog.pg_namespace WHERE nspname=$1`
			t.privileges = []string{"USAGE", "CREATE"}
		} else {
			table, err := checkedIdentifier(obj.Name)
			if err != nil {
				return t, err
			}
			t.sqlName = "TABLE " + schema + "." + table
			t.args = append(t.args, obj.Name)
			t.aclQuery = `SELECT coalesce(c.relacl,pg_catalog.acldefault('r',c.relowner))
FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname=$1 AND c.relname=$2 AND c.relkind IN ('r','p','v','m','f')`
			t.privileges = []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"}
			major, _, _ := strings.Cut(d.info.Version, ".")
			if version, _ := strconv.Atoi(major); version >= 17 {
				t.privileges = append(t.privileges, "MAINTAIN")
			}
		}
	default:
		return t, fmt.Errorf("%w: unknown object type", driver.ErrRoleInput)
	}
	return t, nil
}

type privilegeReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readRolePrivileges(ctx context.Context, db privilegeReader, role string, t privilegeTarget) ([]driver.ObjectPrivilege, error) {
	if _, err := roleIdentifier(role); err != nil {
		return nil, err
	}
	roleParam := fmt.Sprintf("$%d", len(t.args)+1)
	// pg_has_role(...,'USAGE') accounts for inheritance on every link.
	// Superusers can use every role; display their bypass status in the UI.
	query := `WITH object_acl(acl) AS (` + t.aclQuery + `)
SELECT DISTINCT a.privilege_type,coalesce(g.rolname,''),a.grantee=0
FROM object_acl CROSS JOIN LATERAL pg_catalog.aclexplode(acl) a
LEFT JOIN pg_catalog.pg_roles g ON g.oid=a.grantee
JOIN pg_catalog.pg_roles r ON r.rolname=` + roleParam + `
WHERE CASE WHEN a.grantee IN (0,r.oid) THEN true WHEN r.rolsuper THEN false
ELSE pg_catalog.pg_has_role(r.oid,a.grantee,'USAGE') END ORDER BY 1,2`
	rows, err := db.QueryContext(ctx, query, append(t.args, role)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	grants := []driver.ObjectPrivilege{}
	for rows.Next() {
		var g driver.ObjectPrivilege
		if err := rows.Scan(&g.Privilege, &g.Source, &g.Public); err != nil {
			return nil, err
		}
		grants = append(grants, g)
	}
	return grants, rows.Err()
}

// RolePrivileges lists applicable object ACL entries and supported changes.
func (d *PostgresDriver) RolePrivileges(ctx context.Context, role string, obj driver.RoleObject) (driver.RoleObjectPrivileges, error) {
	t, err := d.privilegeTarget(obj)
	if err != nil {
		return driver.RoleObjectPrivileges{}, err
	}
	grants, err := readRolePrivileges(ctx, d.db, role, t)
	return driver.RoleObjectPrivileges{Editable: t.privileges, Grants: grants}, err
}

// ChangeRolePrivilege changes one direct object grant in the bound database.
func (d *PostgresDriver) ChangeRolePrivilege(ctx context.Context, role string, obj driver.RoleObject, privilege string, grant bool) error {
	name, err := roleIdentifier(role)
	if err != nil {
		return err
	}
	t, err := d.privilegeTarget(obj)
	if err != nil {
		return err
	}
	if !slices.Contains(t.privileges, privilege) {
		return fmt.Errorf("%w: unsupported privilege", driver.ErrRoleInput)
	}
	stmt := "REVOKE " + privilege + " ON " + t.sqlName + " FROM " + name
	if grant {
		stmt = "GRANT " + privilege + " ON " + t.sqlName + " TO " + name
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, stmt); err != nil {
		return err
	}
	// PostgreSQL may only issue a warning when the caller cannot grant a
	// privilege. Report success only when the direct ACL matches the request.
	grants, err := readRolePrivileges(ctx, tx, role, t)
	if err != nil {
		return err
	}
	direct := slices.ContainsFunc(grants, func(g driver.ObjectPrivilege) bool { return g.Source == role && g.Privilege == privilege })
	if direct != grant {
		return errors.New("direct privilege did not change; check grant options and other grantors")
	}
	return tx.Commit()
}
