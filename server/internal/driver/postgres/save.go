package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// The save path whitelists every piece of a column definition that
// arrives as a raw client string, mirroring the MySQL driver: the type
// must be words of letters/digits/underscore (single spaces between
// words — "double precision", "timestamp with time zone") optionally
// carrying [] array markers, and the length a numeric pair — anything
// else (comments, parens, semicolons) is rejected instead of
// concatenated into the DDL.
var (
	pgWordRe   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	pgLengthRe = regexp.MustCompile(`^\d+(,\d+)?$`)
)

// SaveTable applies the desired table state: an empty table name
// creates the table in the current schema (SetSchema selects it; NewName
// is a bare name), a non-empty one alters it — rename first, then the
// column operations in desired order, then the table comment. All ALTER
// steps run inside ONE transaction (spec §4.6 — PostgreSQL DDL is
// transactional): a failing step rolls back everything, so nothing
// partial survives, and the error carries the step number. Index and
// foreign key changes on an existing table belong to SaveIndexes and
// are intentionally ignored here. Contract carried in from the Task 2
// review: a column marked AutoIncrement carries its identity default
// (serial on create, the nextval(...) expression on read), so the save
// path never renders a DEFAULT clause for it — rendering both would
// duplicate the default. The db argument is validated non-empty to keep
// the driver contract uniform; it selects nothing (the schema does).
func (d *PostgresDriver) SaveTable(ctx context.Context, db, table string, desired driver.TableDef) error {
	if err := d.checkDatabase(db); err != nil {
		return err
	}
	if db == "" {
		return fmt.Errorf("save table: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	schema := d.schemaFor(ctx)
	name := table
	var err error
	if table == "" {
		var stmts []string
		stmts, err = buildCreateTable(schema, desired)
		if err == nil {
			err = d.execDDL(ctx, stmts)
		}
		name = desired.NewName
	} else {
		err = d.prepareAndExecDDL(ctx, func(tx *sql.Tx) ([]string, error) {
			qual, err := qualify(schema, table)
			if err != nil {
				return nil, err
			}
			// An unchanged type can still reject ALTER TYPE if a generated column
			// depends on it. Keep the comparison and ALTER under one lock.
			if _, err := tx.ExecContext(ctx, "LOCK TABLE "+qual+" IN ACCESS EXCLUSIVE MODE"); err != nil {
				return nil, err
			}
			columns, err := readTableColumns(ctx, tx, schema, table)
			if err != nil {
				return nil, err
			}
			original := make(map[string]driver.ColumnDef, len(columns))
			for _, column := range columns {
				original[column.Name] = column
			}
			return buildAlterTable(schema, table, desired, original)
		})
	}
	if err != nil {
		return fmt.Errorf("save table %s.%s: %w", schema, name, err)
	}
	// The ALTERs may have changed which column (if any) the server
	// generates — drop the cached serial-key detection under every name
	// the table carried through the save.
	d.invalidateSerialPK(schema, name)
	if desired.NewName != "" && desired.NewName != name {
		d.invalidateSerialPK(schema, desired.NewName)
	}
	return nil
}

// SaveIndexes applies the full desired index state of one table (the
// Adminer alter_indexes model): current indexes missing from desired
// are dropped — the PRIMARY via DROP CONSTRAINT (its backing index name
// comes from pg_index, e.g. <table>_pkey), the rest via a
// schema-qualified DROP INDEX — and missing or changed indexes are
// added; changed means the same name with a different type or column
// list. The one PRIMARY is matched by type, not name (PostgreSQL names
// it <table>_pkey while the desired state says PRIMARY). Every
// statement runs inside one transaction, numbered "step %d" on error.
// FULLTEXT and SPATIAL indexes do not exist on PostgreSQL and fail
// with "not supported on postgres".
func (d *PostgresDriver) SaveIndexes(ctx context.Context, db, table string, desired []driver.Index) error {
	if err := d.checkDatabase(db); err != nil {
		return err
	}
	if db == "" {
		return fmt.Errorf("save indexes: %w: database name", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return fmt.Errorf("save indexes: %w: table name", sqlgen.ErrEmptyIdentifier)
	}
	schema := d.schemaFor(ctx)

	current, err := d.tableIndexes(ctx, schema, table)
	if err != nil {
		return fmt.Errorf("save indexes %s.%s: %w", schema, table, err)
	}
	stmts, err := buildAlterIndexes(schema, table, current, desired)
	if err != nil {
		return fmt.Errorf("save indexes %s.%s: %w", schema, table, err)
	}
	if err := d.execDDL(ctx, stmts); err != nil {
		return fmt.Errorf("save indexes %s.%s: %w", schema, table, err)
	}
	// A changed PRIMARY may have changed the single-column generated key.
	d.invalidateSerialPK(schema, table)
	return nil
}

// execDDL runs every statement of one save call inside a single
// transaction (spec §4.6): BEGIN; step 1; ...; step n; COMMIT. The
// first failing statement rolls the transaction back — nothing partial
// survives — and is reported as "step %d: %w". An empty statement list
// opens no transaction at all.
func (d *PostgresDriver) execDDL(ctx context.Context, stmts []string) error {
	if len(stmts) == 0 {
		return nil
	}
	return d.prepareAndExecDDL(ctx, func(*sql.Tx) ([]string, error) { return stmts, nil })
}

func (d *PostgresDriver) prepareAndExecDDL(ctx context.Context, prepare func(*sql.Tx) ([]string, error)) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ddl transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmts, err := prepare(tx)
	if err != nil {
		return err
	}
	for i, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback() // best-effort; the step error is the real failure
			return fmt.Errorf("step %d: %w", i+1, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ddl transaction: %w", err)
	}
	return nil
}

// buildCreateTable renders the statements of a CREATE: the CREATE TABLE
// itself (columns, PRIMARY and UNIQUE constraints and foreign keys
// inline — PostgreSQL has no inline plain index, so those become
// separate CREATE INDEX statements), then the index statements, then
// COMMENT ON TABLE and COMMENT ON COLUMN. Identifiers are
// double-quoted and every literal SQL-escaped from structured values.
func buildCreateTable(schema string, d driver.TableDef) ([]string, error) {
	if d.NewName == "" {
		return nil, fmt.Errorf("create table: table name is empty")
	}
	if len(d.Columns) == 0 {
		return nil, fmt.Errorf("create table %s: at least one column is required", d.NewName)
	}
	qual, err := qualify(schema, d.NewName)
	if err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}
	d.ForeignKeys = slices.Clone(d.ForeignKeys)
	for i := range d.ForeignKeys {
		if d.ForeignKeys[i].RefSchema == "" {
			d.ForeignKeys[i].RefSchema = schema
		}
	}
	return buildCreateTableQualified(qual, d)
}

// buildCreateTableQualified renders the CREATE against an explicit
// qualified table name — ShowCreateTable passes an unqualified name so
// a dump can be imported into any schema the importer selects.
func buildCreateTableQualified(qual string, d driver.TableDef) ([]string, error) {
	if err := driver.ValidateForeignKeys(d.ForeignKeys, false); err != nil {
		return nil, err
	}

	parts := make([]string, 0, len(d.Columns)+len(d.Indexes)+len(d.ForeignKeys))
	for _, c := range d.Columns {
		frag, err := columnFragmentPg(c)
		if err != nil {
			return nil, fmt.Errorf("create table %s: %w", d.NewName, err)
		}
		parts = append(parts, frag)
	}

	// Validate the entire desired index set before rendering any CREATE.
	seenIndexes := map[string]bool{}
	primarySeen := false
	for _, idx := range d.Indexes {
		if strings.EqualFold(idx.Type, "PRIMARY") {
			if primarySeen {
				return nil, fmt.Errorf("more than one PRIMARY index")
			}
			primarySeen = true
			continue
		}
		if seenIndexes[idx.Name] {
			return nil, fmt.Errorf("duplicate index name %q", idx.Name)
		}
		seenIndexes[idx.Name] = true
	}

	var indexStmts []string
	for _, idx := range d.Indexes {
		cols, err := indexColumnsFragmentPg(idx.Columns)
		if err != nil {
			return nil, fmt.Errorf("create table %s: index %s: %w", d.NewName, idx.Name, err)
		}
		switch strings.ToUpper(idx.Type) {
		case "PRIMARY":
			// The server names the constraint <table>_pkey itself; the
			// desired name (always PRIMARY) is not rendered.
			parts = append(parts, "PRIMARY KEY ("+cols+")")
		case "UNIQUE":
			if idx.Name == "" {
				return nil, fmt.Errorf("create table %s: unique index without a name", d.NewName)
			}
			parts = append(parts, "CONSTRAINT "+sqlgen.QuoteIdentPg(idx.Name)+" UNIQUE ("+cols+")")
		case "INDEX":
			if idx.Name == "" {
				return nil, fmt.Errorf("create table %s: index without a name", d.NewName)
			}
			indexStmts = append(indexStmts, "CREATE INDEX "+sqlgen.QuoteIdentPg(idx.Name)+" ON "+qual+" ("+cols+")")
		case "FULLTEXT", "SPATIAL":
			return nil, fmt.Errorf("create table %s: index %s: %s indexes are not supported on postgres",
				d.NewName, idx.Name, strings.ToUpper(idx.Type))
		default:
			return nil, fmt.Errorf("create table %s: index %s: unknown type %q", d.NewName, idx.Name, idx.Type)
		}
	}

	for _, fk := range d.ForeignKeys {
		frag, err := foreignKeyFragmentPg(fk)
		if err != nil {
			return nil, fmt.Errorf("create table %s: %w", d.NewName, err)
		}
		parts = append(parts, frag)
	}

	stmts := []string{"CREATE TABLE " + qual + " (\n  " + strings.Join(parts, ",\n  ") + "\n)"}
	stmts = append(stmts, indexStmts...)
	if d.Options.Comment != "" {
		stmts = append(stmts, "COMMENT ON TABLE "+qual+" IS "+sqlQuoteStringPg(d.Options.Comment))
	}
	for _, c := range d.Columns {
		if c.Comment != "" {
			stmts = append(stmts, columnCommentStmt(qual, c.Name, c.Comment))
		}
	}
	return stmts, nil
}

// buildAlterTable renders the ALTER statements of the desired state in
// execution order: the table rename (step 1, if NewName differs — the
// target is a bare name, PostgreSQL renames within the table's schema),
// then the column operations in the order the columns are listed, then
// the table comment. Later statements address the renamed table. The
// FIRST/AFTER placement of ColumnDef.Position is ignored: PostgreSQL
// has no column placement (phase 4 plan ruling). The engine, collation
// and auto-increment table options are no-ops on PostgreSQL (no
// engines; the counter is a sequence property), and an empty comment is
// not re-rendered — a comment cannot be cleared through SaveTable
// (v0.1 limitation, same as the MySQL path).
func buildAlterTable(schema, table string, d driver.TableDef, original map[string]driver.ColumnDef) ([]string, error) {
	if table == "" {
		return nil, fmt.Errorf("alter table: table name is empty")
	}
	qual, err := qualify(schema, table)
	if err != nil {
		return nil, fmt.Errorf("alter table: %w", err)
	}

	var stmts []string
	current := qual
	if d.NewName != "" && d.NewName != table {
		stmts = append(stmts, "ALTER TABLE "+qual+" RENAME TO "+sqlgen.QuoteIdentPg(d.NewName))
		current, err = qualify(schema, d.NewName)
		if err != nil {
			return nil, fmt.Errorf("alter table: %w", err)
		}
	}

	for i, c := range d.Columns {
		switch {
		case c.OldName == "" && c.Type != "": // ADD
			frag, err := columnFragmentPg(c)
			if err != nil {
				return nil, fmt.Errorf("alter table %s: column %d: %w", table, i+1, err)
			}
			stmts = append(stmts, "ALTER TABLE "+current+" ADD COLUMN "+frag)
			if c.Comment != "" {
				stmts = append(stmts, columnCommentStmt(current, c.Name, c.Comment))
			}
		case c.OldName != "" && c.Type == "": // DROP
			if c.Name == "" {
				return nil, fmt.Errorf("alter table %s: column %d is marked for drop but has no name", table, i+1)
			}
			stmts = append(stmts, "ALTER TABLE "+current+" DROP COLUMN "+sqlgen.QuoteIdentPg(c.Name))
		case c.OldName != "" && c.OldName != c.Name: // CHANGE: rename + redefine
			sub, err := columnAlterStmts(current, c, original[c.OldName])
			if err != nil {
				return nil, fmt.Errorf("alter table %s: column %d: %w", table, i+1, err)
			}
			stmts = append(stmts, "ALTER TABLE "+current+" RENAME COLUMN "+sqlgen.QuoteIdentPg(c.OldName)+" TO "+sqlgen.QuoteIdentPg(c.Name))
			stmts = append(stmts, sub...)
			if c.Comment != "" {
				stmts = append(stmts, columnCommentStmt(current, c.Name, c.Comment))
			}
		case c.OldName != "": // MODIFY (OldName == Name)
			sub, err := columnAlterStmts(current, c, original[c.OldName])
			if err != nil {
				return nil, fmt.Errorf("alter table %s: column %d: %w", table, i+1, err)
			}
			stmts = append(stmts, sub...)
			if c.Comment != "" {
				stmts = append(stmts, columnCommentStmt(current, c.Name, c.Comment))
			}
		default:
			return nil, fmt.Errorf("alter table %s: column %d (%q) has neither an old name nor a type", table, i+1, c.Name)
		}
	}

	if d.Options.Comment != "" {
		stmts = append(stmts, "COMMENT ON TABLE "+current+" IS "+sqlQuoteStringPg(d.Options.Comment))
	}
	return stmts, nil
}

// columnAlterStmts renders the per-aspect statements of one MODIFY or
// the redefine-half of a CHANGE (the rename itself is rendered by the
// caller first, so these statements carry the NEW column name): the
// type, then nullability, then the default — each as its own ALTER
// TABLE statement, all of them inside the save call's one transaction.
// A nil Default means the desired column has no default, so DROP
// DEFAULT is rendered (desired-state semantics, like the MySQL MODIFY
// that omits the clause). A column marked AutoIncrement renders no
// default statement — its identity default (nextval) must survive.
// Generated columns have a generation expression instead of a DEFAULT.
func columnAlterStmts(qual string, c, original driver.ColumnDef) ([]string, error) {
	col := sqlgen.QuoteIdentPg(c.Name)
	alter := "ALTER TABLE " + qual + " ALTER COLUMN " + col + " "
	var stmts []string
	if c.Generated == "" && (!strings.EqualFold(c.Type, original.Type) || c.Length != original.Length) {
		typePart, err := columnTypeFragmentPg(c, false)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, alter+"TYPE "+typePart)
	}
	if c.Nullable {
		stmts = append(stmts, alter+"DROP NOT NULL")
	} else {
		stmts = append(stmts, alter+"SET NOT NULL")
	}
	switch {
	case c.Generated != "" || c.AutoIncrement:
		// Generation and identity expressions must survive.
	case c.Default != nil && c.Default.IsSQLExpression:
		stmts = append(stmts, alter+"SET DEFAULT "+c.Default.Value)
	case c.Default != nil:
		stmts = append(stmts, alter+"SET DEFAULT "+sqlQuoteStringPg(c.Default.Value))
	default:
		stmts = append(stmts, alter+"DROP DEFAULT")
	}
	return stmts, nil
}

// columnFragmentPg renders one column definition for CREATE TABLE and
// ADD COLUMN. Unsigned, OnUpdate, Collation and EnumValues are no-ops:
// PostgreSQL has no unsigned integers or ON UPDATE, and per-column
// collations and enum types cannot round-trip through the shared
// cross-flavor model in v0.1 (the read path still reports them).
// Generated columns are STORED-only (PostgreSQL has no VIRTUAL); their
// expression travels in Default.Value and is rendered as the
// GENERATED ALWAYS AS clause — never as a DEFAULT, which PostgreSQL
// rejects on generated columns.
func columnFragmentPg(c driver.ColumnDef) (string, error) {
	if c.Name == "" {
		return "", fmt.Errorf("column definition has no name")
	}
	typePart, err := columnTypeFragmentPg(c, true)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(sqlgen.QuoteIdentPg(c.Name))
	sb.WriteString(" ")
	sb.WriteString(typePart)
	if c.Nullable {
		sb.WriteString(" NULL")
	} else {
		sb.WriteString(" NOT NULL")
	}

	if c.Generated != "" {
		if c.Default == nil || c.Default.Value == "" {
			return "", fmt.Errorf("generated column %s requires an expression", c.Name)
		}
		sb.WriteString(" GENERATED ALWAYS AS (" + c.Default.Value + ") STORED")
		return sb.String(), nil
	}

	if c.Default != nil && !c.AutoIncrement {
		// Contract: a column marked AutoIncrement never gets a DEFAULT
		// clause — its serial pseudo-type already carries the identity
		// default, and rendering both would duplicate it.
		if c.Default.IsSQLExpression {
			sb.WriteString(" DEFAULT " + c.Default.Value)
		} else {
			sb.WriteString(" DEFAULT " + sqlQuoteStringPg(c.Default.Value))
		}
	}
	return sb.String(), nil
}

// columnTypeFragmentPg renders the type part of a column: the base type
// with its numeric length suffix. When allowSerial is set (CREATE and
// ADD COLUMN) and the column is AutoIncrement, int/integer/bigint map
// to the serial pseudo-type — the sequence default comes with it, which
// is exactly why no DEFAULT clause may be rendered too. MODIFY renders
// the base type unchanged: PostgreSQL cannot add or drop identity
// through an ALTER COLUMN TYPE, so modifying an AutoIncrement column
// keeps its identity (v0.1 limitation); any other type with
// AutoIncrement set keeps the base type as well.
func columnTypeFragmentPg(c driver.ColumnDef, allowSerial bool) (string, error) {
	if !validPgType(c.Type) {
		return "", fmt.Errorf("column %s: invalid type %q", c.Name, c.Type)
	}
	base := c.Type
	if c.AutoIncrement && allowSerial {
		if serial, ok := pgSerialType(base); ok {
			base = serial
		}
	}
	if c.Length != "" {
		if !pgLengthRe.MatchString(c.Length) {
			return "", fmt.Errorf("column %s: invalid length %q", c.Name, c.Length)
		}
		return base + "(" + c.Length + ")", nil
	}
	return base, nil
}

// pgSerialType maps an AutoIncrement integer type to its serial
// pseudo-type; false means the type has no serial form.
func pgSerialType(base string) (string, bool) {
	switch strings.ToLower(base) {
	case "int", "integer":
		return "serial", true
	case "bigint":
		return "bigserial", true
	default:
		return "", false
	}
}

// validPgType whitelists the type word(s): letters, digits and
// underscore per word, single spaces between words (multi-word types
// like "double precision" survive a read round-trip), optional []
// array markers. Any other character — parens, semicolons, quotes,
// tabs, newlines — is rejected so no raw client string reaches the DDL.
func validPgType(t string) bool {
	for strings.HasSuffix(t, "[]") {
		t = strings.TrimSuffix(t, "[]")
	}
	if t == "" {
		return false
	}
	for _, w := range strings.Split(t, " ") {
		if !pgWordRe.MatchString(w) {
			return false
		}
	}
	return true
}

// columnCommentStmt renders one COMMENT ON COLUMN statement; an empty
// comment is never rendered, so a comment cannot be cleared through
// SaveTable (v0.1 limitation, same as the table comment).
func columnCommentStmt(qual, column, comment string) string {
	return "COMMENT ON COLUMN " + qual + "." + sqlgen.QuoteIdentPg(column) + " IS " + sqlQuoteStringPg(comment)
}

// indexColumnsFragmentPg renders the quoted column list of an index. A
// column name starting with "(" is an expression carried verbatim from
// a read-back pg_get_indexdef (e.g. "(lower((name)::text))") and is not
// quoted. Prefix sub-parts do not exist on PostgreSQL.
func indexColumnsFragmentPg(cols []driver.IndexColumn) (string, error) {
	if len(cols) == 0 {
		return "", fmt.Errorf("index without columns")
	}
	parts := make([]string, len(cols))
	for i, col := range cols {
		if col.Name == "" {
			return "", fmt.Errorf("index column %d has no name", i)
		}
		if col.SubPart > 0 {
			return "", fmt.Errorf("index column %s: prefix sub-parts are not supported on postgres", col.Name)
		}
		if strings.HasPrefix(col.Name, "(") {
			parts[i] = col.Name
			continue
		}
		parts[i] = sqlgen.QuoteIdentPg(col.Name)
	}
	return strings.Join(parts, ", "), nil
}

// foreignKeyFragmentPg renders one inline CONSTRAINT ... FOREIGN KEY.
// The referenced table arrives bare or schema-qualified; each
// dot-separated segment is double-quoted independently.
func foreignKeyFragmentPg(fk driver.ForeignKey) (string, error) {
	if len(fk.Columns) == 0 || len(fk.RefColumns) == 0 {
		return "", fmt.Errorf("foreign key %s needs columns on both sides", fk.Name)
	}
	if fk.RefTable == "" {
		return "", fmt.Errorf("foreign key %s has no referenced table", fk.Name)
	}
	if len(fk.Columns) != len(fk.RefColumns) {
		return "", fmt.Errorf("foreign key %s column counts differ", fk.Name)
	}
	for _, name := range append(slices.Clone(fk.Columns), fk.RefColumns...) {
		if name == "" {
			return "", fmt.Errorf("foreign key %s has an empty column", fk.Name)
		}
	}
	for _, action := range []*string{&fk.OnDelete, &fk.OnUpdate} {
		*action = strings.ToUpper(strings.TrimSpace(*action))
		switch *action {
		case "", "NO ACTION", "RESTRICT", "CASCADE", "SET NULL", "SET DEFAULT":
		default:
			return "", fmt.Errorf("foreign key %s has invalid action %q", fk.Name, *action)
		}
	}

	cols := make([]string, len(fk.Columns))
	for i, c := range fk.Columns {
		cols[i] = sqlgen.QuoteIdentPg(c)
	}
	refs := make([]string, len(fk.RefColumns))
	for i, c := range fk.RefColumns {
		refs[i] = sqlgen.QuoteIdentPg(c)
	}

	var sb strings.Builder
	if fk.Name != "" {
		sb.WriteString("CONSTRAINT " + sqlgen.QuoteIdentPg(fk.Name) + " ")
	}
	sb.WriteString("FOREIGN KEY (" + strings.Join(cols, ", ") + ") REFERENCES " +
		quoteForeignRefPg(fk) + " (" + strings.Join(refs, ", ") + ")")
	if fk.OnDelete != "" {
		sb.WriteString(" ON DELETE " + fk.OnDelete)
	}
	if fk.OnUpdate != "" {
		sb.WriteString(" ON UPDATE " + fk.OnUpdate)
	}
	return sb.String(), nil
}

func quoteForeignRefPg(fk driver.ForeignKey) string {
	if fk.RefSchema == "" {
		return sqlgen.QuoteIdentPg(fk.RefTable)
	}
	return sqlgen.QuoteIdentPg(fk.RefSchema) + "." + sqlgen.QuoteIdentPg(fk.RefTable)
}

// buildAlterIndexes renders the statements of one index diff — drops
// first (current indexes absent or changed), then adds — or nil when
// current and desired agree. The PRIMARY is matched by type on both
// sides; every other index by exact name, since all names are quoted.
func buildAlterIndexes(schema, table string, current, desired []driver.Index) ([]string, error) {
	qual, err := qualify(schema, table)
	if err != nil {
		return nil, fmt.Errorf("save indexes: %w", err)
	}

	var desiredPrimary *driver.Index
	seen := make(map[string]struct{}, len(desired))
	byName := make(map[string]driver.Index, len(desired))
	for i := range desired {
		idx := desired[i]
		switch strings.ToUpper(idx.Type) {
		case "PRIMARY", "UNIQUE", "INDEX":
		case "FULLTEXT", "SPATIAL":
			return nil, fmt.Errorf("index %d (%s): %s indexes are not supported on postgres",
				i+1, idx.Name, strings.ToUpper(idx.Type))
		default:
			return nil, fmt.Errorf("index %d (%s): unknown type %q", i+1, idx.Name, idx.Type)
		}
		if _, err := indexColumnsFragmentPg(idx.Columns); err != nil {
			return nil, fmt.Errorf("index %d (%s): %w", i+1, idx.Name, err)
		}
		if strings.EqualFold(idx.Type, "PRIMARY") {
			if desiredPrimary != nil {
				return nil, fmt.Errorf("index %d: more than one PRIMARY index", i+1)
			}
			desiredPrimary = &desired[i]
			continue
		}
		if idx.Name == "" {
			return nil, fmt.Errorf("index %d: %w: index name", i+1, sqlgen.ErrEmptyIdentifier)
		}
		key := idx.Name
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("index %d: duplicate index name %q", i+1, idx.Name)
		}
		seen[key] = struct{}{}
		byName[key] = idx
	}

	var currentPrimary *driver.Index
	currentByName := make(map[string]driver.Index, len(current))
	for i := range current {
		if current[i].Type == "PRIMARY" {
			currentPrimary = &current[i]
			continue
		}
		currentByName[current[i].Name] = current[i]
	}

	var stmts []string
	// Drops first.
	for i := range current {
		cur := current[i]
		if cur.Type == "PRIMARY" {
			if desiredPrimary != nil && indexEqualPg(cur, *desiredPrimary) {
				continue
			}
			stmts = append(stmts, "ALTER TABLE "+qual+" DROP CONSTRAINT "+sqlgen.QuoteIdentPg(cur.Name))
			continue
		}
		if want, ok := byName[cur.Name]; ok && indexEqualPg(cur, want) {
			continue
		}
		if cur.Constraint != "" {
			stmts = append(stmts, "ALTER TABLE "+qual+" DROP CONSTRAINT "+sqlgen.QuoteIdentPg(cur.Constraint))
		} else {
			stmts = append(stmts, "DROP INDEX "+sqlgen.QuoteIdentPg(schema)+"."+sqlgen.QuoteIdentPg(cur.Name))
		}
	}
	// Then adds.
	if desiredPrimary != nil && (currentPrimary == nil || !indexEqualPg(*currentPrimary, *desiredPrimary)) {
		cols, err := indexColumnsFragmentPg(desiredPrimary.Columns)
		if err != nil {
			return nil, fmt.Errorf("index (PRIMARY): %w", err)
		}
		stmts = append(stmts, "ALTER TABLE "+qual+" ADD PRIMARY KEY ("+cols+")")
	}
	for _, want := range desired {
		if strings.EqualFold(want.Type, "PRIMARY") {
			continue
		}
		if cur, ok := currentByName[want.Name]; ok && indexEqualPg(cur, want) {
			continue
		}
		cols, err := indexColumnsFragmentPg(want.Columns)
		if err != nil {
			return nil, fmt.Errorf("index %s: %w", want.Name, err)
		}
		create := "CREATE INDEX "
		if strings.EqualFold(want.Type, "UNIQUE") {
			create = "CREATE UNIQUE INDEX "
		}
		stmts = append(stmts, create+sqlgen.QuoteIdentPg(want.Name)+" ON "+qual+" ("+cols+")")
	}
	return stmts, nil
}

// indexEqualPg reports whether two index definitions are the same
// index: same type (case-insensitively) and the same column list in
// order.
func indexEqualPg(a, b driver.Index) bool {
	if !strings.EqualFold(a.Type, b.Type) || len(a.Columns) != len(b.Columns) {
		return false
	}
	for i := range a.Columns {
		if a.Columns[i].Name != b.Columns[i].Name {
			return false
		}
	}
	return true
}

// sqlQuoteStringPg renders a SQL string literal with PostgreSQL's
// standard_conforming_strings semantics: only the single quote is
// special (doubled to ”); a backslash is an ordinary character and is
// not escaped.
func sqlQuoteStringPg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
