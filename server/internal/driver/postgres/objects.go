package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
	"github.com/jackc/pgx/v5/stdlib"
)

var _ driver.ViewManager = (*PostgresDriver)(nil)
var _ driver.TriggerManager = (*PostgresDriver)(nil)
var _ driver.RoutineManager = (*PostgresDriver)(nil)

func (d *PostgresDriver) objectTarget(ctx context.Context, db, name string) (string, error) {
	if db != d.database || db == "" || name == "" || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: database must match connection and object name is required", driver.ErrObjectValidation)
	}
	return sqlgen.QuoteIdentPg(d.schemaFor(ctx)) + "." + sqlgen.QuoteIdentPg(name), nil
}

// ListViews returns views with executable CREATE source.
func (d *PostgresDriver) ListViews(ctx context.Context, db string) ([]driver.ObjectInfo, error) {
	if _, err := d.objectTarget(ctx, db, "_"); err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT c.relname, pg_get_viewdef(c.oid,true) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relkind='v' ORDER BY c.relname`, d.schemaFor(ctx))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []driver.ObjectInfo{}
	for rows.Next() {
		var o driver.ObjectInfo
		var selectSQL string
		if err := rows.Scan(&o.Name, &selectSQL); err != nil {
			return nil, err
		}
		target, _ := d.objectTarget(ctx, db, o.Name)
		o.Kind = "VIEW"
		o.Source = "CREATE OR REPLACE VIEW " + target + " AS " + selectSQL
		out = append(out, o)
	}
	return out, rows.Err()
}

// ListTriggers returns trigger names, tables and CREATE source.
func (d *PostgresDriver) ListTriggers(ctx context.Context, db string) ([]driver.ObjectInfo, error) {
	if _, err := d.objectTarget(ctx, db, "_"); err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT t.tgname,c.relname,pg_get_triggerdef(t.oid,true) FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND NOT t.tgisinternal ORDER BY t.tgname,c.relname`, d.schemaFor(ctx))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []driver.ObjectInfo{}
	for rows.Next() {
		o := driver.ObjectInfo{Kind: "TRIGGER"}
		if err := rows.Scan(&o.Name, &o.Table, &o.Source); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ListRoutines returns functions and procedures with CREATE source.
func (d *PostgresDriver) ListRoutines(ctx context.Context, db string) ([]driver.ObjectInfo, error) {
	if _, err := d.objectTarget(ctx, db, "_"); err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT p.proname,CASE p.prokind WHEN 'p' THEN 'PROCEDURE' ELSE 'FUNCTION' END,pg_get_function_identity_arguments(p.oid),pg_get_functiondef(p.oid) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=$1 AND p.prokind IN ('f','p') ORDER BY p.proname,p.oid`, d.schemaFor(ctx))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []driver.ObjectInfo{}
	for rows.Next() {
		var o driver.ObjectInfo
		if err := rows.Scan(&o.Name, &o.Kind, &o.Identity, &o.Source); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func (d *PostgresDriver) saveObject(ctx context.Context, db, name, kind string, def driver.ObjectDef) error {
	if _, err := d.objectTarget(ctx, db, name); err != nil {
		return err
	}
	if err := driver.ValidateObjectSource(def.Source, kind, d.schemaFor(ctx), name); err != nil {
		return err
	}
	// Execute a single extended-protocol statement on a dedicated connection:
	// CREATE bodies can contain semicolons, while appended SQL is rejected.
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var triggerTarget string
	if kind == "TRIGGER" {
		triggers, err := d.ListTriggers(ctx, db)
		if err != nil {
			return err
		}
		matches := []driver.ObjectInfo{}
		for _, o := range triggers {
			if o.Name == name && (def.Table == "" || o.Table == def.Table) {
				matches = append(matches, o)
			}
		}
		if len(matches) > 1 {
			return fmt.Errorf("%w: trigger name is ambiguous; select its table", driver.ErrObjectValidation)
		}
		if len(matches) == 1 {
			triggerTarget, _ = d.objectTarget(ctx, db, matches[0].Table)
		}
	}
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer discardConnection(conn)
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "SET LOCAL search_path TO "+sqlgen.QuoteIdentPg(d.schemaFor(ctx))); err != nil {
		return err
	}
	if triggerTarget != "" {
		if _, err = tx.ExecContext(ctx, "DROP TRIGGER "+sqlgen.QuoteIdentPg(name)+" ON "+triggerTarget); err != nil {
			return err
		}
	}
	// pgx Exec forces simple protocol for zero-argument queries, even when a
	// mode is supplied. PgConn.ExecParams forces Parse/Bind and rejects batches.
	err = conn.Raw(func(raw any) error {
		sc, ok := raw.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("unexpected postgres connection %T", raw)
		}
		_, err := sc.Conn().PgConn().ExecParams(ctx, def.Source, nil, nil, nil, nil).Close()
		return err
	})
	if err != nil {
		return err
	}
	return tx.Commit()
}

// AlterView creates or replaces a view from explicitly edited SQL source.
func (d *PostgresDriver) AlterView(ctx context.Context, db, name string, def driver.ObjectDef) error {
	return d.saveObject(ctx, db, name, "VIEW", def)
}

// CreateTrigger creates or replaces a trigger from SQL source.
func (d *PostgresDriver) CreateTrigger(ctx context.Context, db, name string, def driver.ObjectDef) error {
	return d.saveObject(ctx, db, name, "TRIGGER", def)
}

// CreateRoutine saves a procedure or function from SQL source.
func (d *PostgresDriver) CreateRoutine(ctx context.Context, db, name string, def driver.ObjectDef) error {
	kind := strings.ToUpper(def.Kind)
	if kind != "FUNCTION" && kind != "PROCEDURE" {
		return fmt.Errorf("%w: routine kind required", driver.ErrObjectValidation)
	}
	if kind == "PROCEDURE" && !d.Caps().Procedures {
		return fmt.Errorf("%w: procedures unsupported", driver.ErrObjectValidation)
	}
	return d.saveObject(ctx, db, name, kind, def)
}

// DropView drops one quoted view name.
func (d *PostgresDriver) DropView(ctx context.Context, db, name string) error {
	target, err := d.objectTarget(ctx, db, name)
	if err != nil {
		return err
	}
	_, err = d.db.ExecContext(ctx, "DROP VIEW "+target)
	return err
}

// DropTrigger drops the selected trigger.
func (d *PostgresDriver) DropTrigger(ctx context.Context, db, name, table string) error {
	target, err := d.objectTarget(ctx, db, table)
	if err != nil {
		return err
	}
	if name == "" || strings.ContainsRune(name, 0) {
		return fmt.Errorf("%w: trigger name required", driver.ErrObjectValidation)
	}
	_, err = d.db.ExecContext(ctx, "DROP TRIGGER "+sqlgen.QuoteIdentPg(name)+" ON "+target)
	return err
}

// routineIdentity resolves signatures against the schema catalog. Only the
// server-returned identity is concatenated into DROP's type signature.
func (d *PostgresDriver) routineIdentity(ctx context.Context, db, name, kind, identity string) (driver.ObjectInfo, error) {
	routines, err := d.ListRoutines(ctx, db)
	if err != nil {
		return driver.ObjectInfo{}, err
	}
	matches := []driver.ObjectInfo{}
	for _, o := range routines {
		if o.Name == name && o.Kind == kind && identity == o.Identity {
			matches = append(matches, o)
		}
	}
	if len(matches) != 1 {
		return driver.ObjectInfo{}, fmt.Errorf("%w: routine not found or overloaded; select its exact identity", driver.ErrObjectValidation)
	}
	return matches[0], nil
}

// DropRoutine drops the selected procedure or function.
func (d *PostgresDriver) DropRoutine(ctx context.Context, db, name, kind, identity string) error {
	kind = strings.ToUpper(kind)
	if kind != "FUNCTION" && kind != "PROCEDURE" {
		return fmt.Errorf("%w: invalid routine kind", driver.ErrObjectValidation)
	}
	o, err := d.routineIdentity(ctx, db, name, kind, identity)
	if err != nil {
		return err
	}
	target, _ := d.objectTarget(ctx, db, name)
	_, err = d.db.ExecContext(ctx, "DROP "+kind+" "+target+"("+o.Identity+")")
	return err
}

// CallProcedure calls the selected procedure with bound scalar arguments.
func (d *PostgresDriver) CallProcedure(ctx context.Context, db, name, identity string, args []any) ([]driver.Result, error) {
	if !d.Caps().Procedures {
		return nil, fmt.Errorf("%w: procedures unsupported", driver.ErrObjectValidation)
	}
	o, err := d.routineIdentity(ctx, db, name, "PROCEDURE", identity)
	if err != nil {
		return nil, err
	}
	// Bound casts select the exact overload even for NULL or string inputs.
	var typeList []string
	rows, err := d.db.QueryContext(ctx, `SELECT format_type(a.type_oid,NULL) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace CROSS JOIN LATERAL unnest(COALESCE(p.proallargtypes,p.proargtypes::oid[])) WITH ORDINALITY a(type_oid,ord) WHERE n.nspname=$1 AND p.proname=$2 AND p.prokind='p' AND pg_get_function_identity_arguments(p.oid)=$3 ORDER BY a.ord`, d.schemaFor(ctx), name, o.Identity)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			_ = rows.Close()
			return nil, err
		}
		typeList = append(typeList, typ)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(args) != len(typeList) {
		return nil, fmt.Errorf("%w: expected %d procedure arguments (include null for OUT parameters)", driver.ErrObjectValidation, len(typeList))
	}
	marks := make([]string, len(args))
	for i := range args {
		marks[i] = fmt.Sprintf("$%d::%s", i+1, typeList[i])
	}
	target, _ := d.objectTarget(ctx, db, name)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer discardConnection(conn)
	if _, err = conn.ExecContext(ctx, "SET search_path TO "+sqlgen.QuoteIdentPg(d.schemaFor(ctx))); err != nil {
		return nil, err
	}
	resultRows, err := conn.QueryContext(ctx, "CALL "+target+"("+strings.Join(marks, ",")+")", args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resultRows.Close() }()
	result, err := readProcedureRows(resultRows)
	if err != nil {
		return nil, err
	}
	return []driver.Result{result}, nil
}
func readProcedureRows(rows *sql.Rows) (driver.Result, error) {
	columns, err := rows.Columns()
	if err != nil {
		return driver.Result{}, err
	}
	res := driver.Result{Columns: []driver.ColumnMeta{}, Rows: [][]any{}}
	for _, name := range columns {
		res.Columns = append(res.Columns, driver.ColumnMeta{Name: name})
	}
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return res, err
		}
		if len(res.Rows) >= 1000 {
			res.Truncated = true
			continue
		}
		row := make([]any, len(values))
		for i, v := range values {
			if b, ok := v.([]byte); ok {
				row[i] = string(b)
			} else {
				row[i] = v
			}
		}
		res.Rows = append(res.Rows, row)
	}
	return res, rows.Err()
}
