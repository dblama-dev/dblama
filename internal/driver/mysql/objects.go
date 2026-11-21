package mysql

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/sqlgen"
)

var _ driver.ViewManager = (*MysqlDriver)(nil)
var _ driver.TriggerManager = (*MysqlDriver)(nil)
var _ driver.RoutineManager = (*MysqlDriver)(nil)
var _ driver.EventManager = (*MysqlDriver)(nil)
var _ driver.PartitionManager = (*MysqlDriver)(nil)

func objectIdent(db, name string) (string, error) {
	if db == "" || name == "" || strings.ContainsRune(db, 0) || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: database and object name required", driver.ErrObjectValidation)
	}
	return sqlgen.QuoteIdent(db) + "." + sqlgen.QuoteIdent(name), nil
}

// ListViews returns views with executable CREATE source.
func (d *MysqlDriver) ListViews(ctx context.Context, db string) ([]driver.ObjectInfo, error) {
	return d.listObjects(ctx, db, "VIEW")
}

// ListTriggers returns trigger names, tables and CREATE source.
func (d *MysqlDriver) ListTriggers(ctx context.Context, db string) ([]driver.ObjectInfo, error) {
	return d.listObjects(ctx, db, "TRIGGER")
}

// ListRoutines returns functions and procedures with CREATE source.
func (d *MysqlDriver) ListRoutines(ctx context.Context, db string) ([]driver.ObjectInfo, error) {
	return d.listObjects(ctx, db, "ROUTINE")
}

// ListEvents returns scheduled events with CREATE source and status.
func (d *MysqlDriver) ListEvents(ctx context.Context, db string) ([]driver.ObjectInfo, error) {
	return d.listObjects(ctx, db, "EVENT")
}
func (d *MysqlDriver) listObjects(ctx context.Context, db, kind string) ([]driver.ObjectInfo, error) {
	if _, err := objectIdent(db, "_"); err != nil {
		return nil, err
	}
	query := ""
	switch kind {
	case "VIEW":
		query = "SELECT TABLE_NAME, 'VIEW', '', '' FROM information_schema.VIEWS WHERE TABLE_SCHEMA=? ORDER BY TABLE_NAME"
	case "TRIGGER":
		query = "SELECT TRIGGER_NAME, 'TRIGGER', EVENT_OBJECT_TABLE, '' FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=? ORDER BY TRIGGER_NAME"
	case "ROUTINE":
		query = "SELECT ROUTINE_NAME, ROUTINE_TYPE, '', '' FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA=? ORDER BY ROUTINE_NAME"
	case "EVENT":
		query = "SELECT EVENT_NAME, 'EVENT', '', STATUS FROM information_schema.EVENTS WHERE EVENT_SCHEMA=? ORDER BY EVENT_NAME"
	}
	rows, err := d.db.QueryContext(ctx, query, db)
	if err != nil {
		return nil, err
	}
	objects := []driver.ObjectInfo{}
	for rows.Next() {
		var o driver.ObjectInfo
		if err := rows.Scan(&o.Name, &o.Kind, &o.Table, &o.Status); err != nil {
			_ = rows.Close()
			return nil, err
		}
		objects = append(objects, o)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range objects {
		source, err := d.objectSource(ctx, db, objects[i].Name, objects[i].Kind)
		if err != nil {
			return nil, err
		}
		objects[i].Source = source
	}
	return objects, nil
}
func (d *MysqlDriver) objectSource(ctx context.Context, db, name, kind string) (string, error) {
	target, err := objectIdent(db, name)
	if err != nil {
		return "", err
	}
	rows, err := d.db.QueryContext(ctx, "SHOW CREATE "+kind+" "+target)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return "", err
	}
	if !rows.Next() {
		return "", fmt.Errorf("object not found")
	}
	values := make([]sql.NullString, len(columns))
	ptrs := make([]any, len(columns))
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err = rows.Scan(ptrs...); err != nil {
		return "", err
	}
	for i, col := range columns {
		if strings.EqualFold(col, "Create "+kind) || strings.EqualFold(col, "SQL Original Statement") {
			if !values[i].Valid {
				return "", fmt.Errorf("source access denied for %s", name)
			}
			return values[i].String, nil
		}
	}
	return "", fmt.Errorf("server omitted object source")
}
func (d *MysqlDriver) sourceConn(ctx context.Context, db string) (*sql.Conn, error) {
	if _, err := objectIdent(db, "_"); err != nil {
		return nil, err
	}
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = conn.ExecContext(ctx, "USE "+sqlgen.QuoteIdent(db)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}
func discardObjectConn(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return sqldriver.ErrBadConn })
	_ = conn.Close()
}
func (d *MysqlDriver) saveObject(ctx context.Context, db, name, kind string, def driver.ObjectDef, replace bool) error {
	if err := driver.ValidateObjectSource(def.Source, kind, db, name); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var previous string
	if replace {
		// Probe SHOW CREATE directly; absence and permission errors are distinguished
		// by the catalog so permission failures never cause a destructive drop.
		catalogKind := kind
		if kind == "FUNCTION" || kind == "PROCEDURE" {
			catalogKind = "ROUTINE"
		}
		list, err := d.listObjects(ctx, db, catalogKind)
		if err != nil {
			return err
		}
		for _, o := range list {
			if o.Name == name && o.Kind == kind {
				previous = o.Source
			}
		}
	}
	conn, err := d.sourceConn(ctx, db)
	if err != nil {
		return err
	}
	defer discardObjectConn(conn)
	target, _ := objectIdent(db, name)
	if previous != "" {
		if _, err = conn.ExecContext(ctx, "DROP "+kind+" "+target); err != nil {
			return err
		}
	}
	if _, err = conn.ExecContext(ctx, def.Source); err != nil {
		if previous != "" {
			if _, restoreErr := conn.ExecContext(ctx, previous); restoreErr != nil {
				return fmt.Errorf("save failed: %v; restoring previous source failed: %w", err, restoreErr)
			}
		}
		return err
	}
	return nil
}

// AlterView creates or replaces a view from explicitly edited SQL source.
func (d *MysqlDriver) AlterView(ctx context.Context, db, name string, def driver.ObjectDef) error {
	if err := driver.ValidateObjectSource(def.Source, "VIEW", db, name); err != nil {
		return err
	}
	source, err := ensureViewReplacement(def.Source)
	if err != nil {
		return err
	}
	def.Source = source
	return d.saveObject(ctx, db, name, "VIEW", def, false)
}

// ensureViewReplacement inserts OR REPLACE after the CREATE header token.
// The body may itself contain that phrase in a literal or comment.
func ensureViewReplacement(source string) (string, error) {
	start, err := skipObjectTrivia(source, 0)
	if err != nil {
		return "", err
	}
	if !objectKeywordAt(source, start, "CREATE") {
		return "", fmt.Errorf("%w: missing CREATE header", driver.ErrObjectValidation)
	}
	end := start + len("CREATE")
	next, err := skipObjectTrivia(source, end)
	if err != nil {
		return "", err
	}
	if objectKeywordAt(source, next, "OR") {
		return source, nil
	}
	return source[:end] + " OR REPLACE" + source[end:], nil
}

func skipObjectTrivia(source string, index int) (int, error) {
	for index < len(source) {
		switch {
		case unicode.IsSpace(rune(source[index])):
			index++
		case strings.HasPrefix(source[index:], "/*"):
			end := strings.Index(source[index+2:], "*/")
			if end < 0 {
				return 0, fmt.Errorf("%w: unterminated comment", driver.ErrObjectValidation)
			}
			index += end + 4
		case strings.HasPrefix(source[index:], "--") || source[index] == '#':
			end := strings.IndexByte(source[index:], '\n')
			if end < 0 {
				return len(source), nil
			}
			index += end + 1
		default:
			return index, nil
		}
	}
	return index, nil
}

func objectKeywordAt(source string, index int, word string) bool {
	end := index + len(word)
	if index < 0 || end > len(source) || !strings.EqualFold(source[index:end], word) {
		return false
	}
	if end == len(source) {
		return true
	}
	return !objectWordByte(source[end])
}

func objectWordByte(next byte) bool {
	return (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') || (next >= '0' && next <= '9') || next == '_' || next >= 128
}

// CreateTrigger creates or replaces a trigger from SQL source.
func (d *MysqlDriver) CreateTrigger(ctx context.Context, db, name string, def driver.ObjectDef) error {
	return d.saveObject(ctx, db, name, "TRIGGER", def, true)
}

// CreateRoutine saves a procedure or function from SQL source.
func (d *MysqlDriver) CreateRoutine(ctx context.Context, db, name string, def driver.ObjectDef) error {
	kind := strings.ToUpper(def.Kind)
	if kind != "FUNCTION" && kind != "PROCEDURE" {
		return fmt.Errorf("%w: routine kind must be FUNCTION or PROCEDURE", driver.ErrObjectValidation)
	}
	return d.saveObject(ctx, db, name, kind, def, true)
}

// CreateEvent saves a scheduled event from SQL source.
func (d *MysqlDriver) CreateEvent(ctx context.Context, db, name string, def driver.ObjectDef) error {
	return d.saveObject(ctx, db, name, "EVENT", def, true)
}
func (d *MysqlDriver) dropObject(ctx context.Context, db, name, kind string) error {
	target, err := objectIdent(db, name)
	if err != nil {
		return err
	}
	_, err = d.db.ExecContext(ctx, "DROP "+kind+" "+target)
	return err
}

// DropView drops one quoted view name.
func (d *MysqlDriver) DropView(ctx context.Context, db, name string) error {
	return d.dropObject(ctx, db, name, "VIEW")
}

// DropTrigger drops the selected trigger.
func (d *MysqlDriver) DropTrigger(ctx context.Context, db, name, _ string) error {
	return d.dropObject(ctx, db, name, "TRIGGER")
}

// DropRoutine drops the selected procedure or function.
func (d *MysqlDriver) DropRoutine(ctx context.Context, db, name, kind, _ string) error {
	kind = strings.ToUpper(kind)
	if kind != "FUNCTION" && kind != "PROCEDURE" {
		return fmt.Errorf("%w: invalid routine kind", driver.ErrObjectValidation)
	}
	return d.dropObject(ctx, db, name, kind)
}

// DropEvent drops one scheduled event.
func (d *MysqlDriver) DropEvent(ctx context.Context, db, name string) error {
	return d.dropObject(ctx, db, name, "EVENT")
}

// CallProcedure binds every value and consumes all result sets, including the
// final status packet. Rows are capped just like the SQL console.
func (d *MysqlDriver) CallProcedure(ctx context.Context, db, name, _ string, args []any) ([]driver.Result, error) {
	target, err := objectIdent(db, name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	meta, err := d.db.QueryContext(ctx, `SELECT COALESCE(PARAMETER_MODE,'IN'),COALESCE(PARAMETER_NAME,'') FROM information_schema.PARAMETERS WHERE SPECIFIC_SCHEMA=? AND SPECIFIC_NAME=? AND ORDINAL_POSITION>0 ORDER BY ORDINAL_POSITION`, db, name)
	if err != nil {
		return nil, err
	}
	type parameter struct{ mode, name string }
	parameters := []parameter{}
	for meta.Next() {
		var p parameter
		if err := meta.Scan(&p.mode, &p.name); err != nil {
			_ = meta.Close()
			return nil, err
		}
		parameters = append(parameters, p)
	}
	err = meta.Err()
	_ = meta.Close()
	if err != nil {
		return nil, err
	}
	if len(args) != len(parameters) {
		return nil, fmt.Errorf("%w: expected %d procedure arguments (include null for OUT parameters)", driver.ErrObjectValidation, len(parameters))
	}
	marks := make([]string, len(args))
	inputs := []any{}
	outputs := []string{}
	conn, err := d.sourceConn(ctx, db)
	if err != nil {
		return nil, err
	}
	defer discardObjectConn(conn)
	for i, p := range parameters {
		if p.mode == "IN" {
			marks[i] = "?"
			inputs = append(inputs, args[i])
			continue
		}
		variable := fmt.Sprintf("@dblama_out_%d", i)
		marks[i] = variable
		if _, err := conn.ExecContext(ctx, "SET "+variable+" = ?", args[i]); err != nil {
			return nil, err
		}
		outputs = append(outputs, variable+" AS "+sqlgen.QuoteIdent(p.name))
	}
	rows, err := conn.QueryContext(ctx, "CALL "+target+"("+strings.Join(marks, ",")+")", inputs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	results := []driver.Result{}
	for {
		res, err := readObjectRows(rows)
		if err != nil {
			return results, err
		}
		results = append(results, res)
		if !rows.NextResultSet() {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return results, err
	}
	_ = rows.Close()
	if len(outputs) > 0 {
		outRows, err := conn.QueryContext(ctx, "SELECT "+strings.Join(outputs, ","))
		if err != nil {
			return results, err
		}
		defer func() { _ = outRows.Close() }()
		out, err := readObjectRows(outRows)
		if err != nil {
			return results, err
		}
		results = append(results, out)
	}
	return results, nil
}
func readObjectRows(rows *sql.Rows) (driver.Result, error) {
	columns, err := rows.Columns()
	if err != nil {
		return driver.Result{}, err
	}
	res := driver.Result{Rows: [][]any{}, Columns: []driver.ColumnMeta{}}
	for _, name := range columns {
		res.Columns = append(res.Columns, driver.ColumnMeta{Name: name})
	}
	vals := make([]any, len(columns))
	ptrs := make([]any, len(columns))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return res, err
		}
		if len(res.Rows) >= 1000 {
			res.Truncated = true
			continue
		}
		row := make([]any, len(vals))
		for i, v := range vals {
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

// ListPartitions returns partition boundaries and estimated row counts.
func (d *MysqlDriver) ListPartitions(ctx context.Context, db, table string) ([]driver.PartitionInfo, error) {
	if _, err := objectIdent(db, table); err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT PARTITION_NAME, COALESCE(PARTITION_METHOD,''), COALESCE(PARTITION_EXPRESSION,''), COALESCE(PARTITION_DESCRIPTION,''), COALESCE(TABLE_ROWS,0) FROM information_schema.PARTITIONS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? AND PARTITION_NAME IS NOT NULL ORDER BY PARTITION_ORDINAL_POSITION`, db, table)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []driver.PartitionInfo{}
	for rows.Next() {
		var p driver.PartitionInfo
		if err := rows.Scan(&p.Name, &p.Method, &p.Expression, &p.Description, &p.Rows); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AddPartition executes a definition from the explicit SQL source editor. The MySQL
// connection rejects extra statements; ordinary operation names are quoted.
func (d *MysqlDriver) AddPartition(ctx context.Context, db, table, definition string) error {
	target, err := objectIdent(db, table)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(definition)), "PARTITION ") {
		return fmt.Errorf("%w: expected PARTITION definition", driver.ErrObjectValidation)
	}
	_, err = d.db.ExecContext(ctx, "ALTER TABLE "+target+" ADD PARTITION ("+definition+")")
	return err
}
func (d *MysqlDriver) partitionOp(ctx context.Context, db, table, name, op string) error {
	target, err := objectIdent(db, table)
	if err != nil {
		return err
	}
	if name == "" || strings.ContainsRune(name, 0) {
		return fmt.Errorf("%w: partition name required", driver.ErrObjectValidation)
	}
	_, err = d.db.ExecContext(ctx, "ALTER TABLE "+target+" "+op+" PARTITION "+sqlgen.QuoteIdent(name))
	return err
}

// DropPartition drops a partition and its data.
func (d *MysqlDriver) DropPartition(ctx context.Context, db, table, name string) error {
	return d.partitionOp(ctx, db, table, name, "DROP")
}

// TruncatePartition removes all rows from one partition.
func (d *MysqlDriver) TruncatePartition(ctx context.Context, db, table, name string) error {
	return d.partitionOp(ctx, db, table, name, "TRUNCATE")
}
