package mysql

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// typeRe and lengthRe whitelist the pieces of a column definition that
// arrive as raw strings from the client. The type must be a bare SQL
// word, the length a numeric pair — anything else (comments, parens,
// semicolons) is rejected instead of concatenated into the DDL.
var (
	typeRe   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	lengthRe = regexp.MustCompile(`^\d+(,\d+)?$`)
	collRe   = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
)

// SaveTable applies the desired table state: an empty table name creates
// the table (NewName is its name, indexes and foreign keys go inline),
// a non-empty one alters it — rename first, then column operations, then
// table options. Index and foreign key changes on an existing table are
// handled by SaveIndexes (Task 5) and are intentionally ignored here.
// Every executed statement is numbered: errors are wrapped as
// "step %d".
func (d *MysqlDriver) SaveTable(ctx context.Context, db, table string, desired driver.TableDef) error {
	if db == "" {
		return fmt.Errorf("save table: database name is empty")
	}

	if table == "" {
		stmt, err := buildCreateTable(desired)
		if err != nil {
			return fmt.Errorf("save table: %w", err)
		}
		if _, err := d.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("save table %s: step 1: %w", db, err)
		}
		return nil
	}

	stmts, err := buildAlterTable(table, desired)
	if err != nil {
		return fmt.Errorf("save table %s.%s: %w", db, table, err)
	}
	for i, stmt := range stmts {
		if _, err := d.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("save table %s.%s: step %d: %w", db, table, i+1, err)
		}
	}
	return nil
}

// buildCreateTable renders the CREATE TABLE statement of the desired
// state: columns in order, PRIMARY/UNIQUE/KEY indexes and foreign keys
// inline, table options as the trailing clause. DDL needs no bound
// arguments: identifiers are backtick-quoted and every string literal is
// SQL-escaped from structured values.
func buildCreateTable(d driver.TableDef) (string, error) {
	if d.NewName == "" {
		return "", fmt.Errorf("create table: table name is empty")
	}
	if len(d.Columns) == 0 {
		return "", fmt.Errorf("create table %s: at least one column is required", d.NewName)
	}

	parts := make([]string, 0, len(d.Columns)+len(d.Indexes)+len(d.ForeignKeys))
	for _, c := range d.Columns {
		frag, err := columnFragment(c)
		if err != nil {
			return "", fmt.Errorf("create table %s: %w", d.NewName, err)
		}
		parts = append(parts, frag)
	}
	for _, idx := range d.Indexes {
		frag, err := indexFragment(idx)
		if err != nil {
			return "", fmt.Errorf("create table %s: %w", d.NewName, err)
		}
		parts = append(parts, frag)
	}
	for _, fk := range d.ForeignKeys {
		frag, err := foreignKeyFragment(fk)
		if err != nil {
			return "", fmt.Errorf("create table %s: %w", d.NewName, err)
		}
		parts = append(parts, frag)
	}

	options, err := createTableOptions(d.Options)
	if err != nil {
		return "", fmt.Errorf("create table %s: %w", d.NewName, err)
	}

	return "CREATE TABLE " + sqlgen.QuoteIdent(d.NewName) + " (\n  " +
		strings.Join(parts, ",\n  ") + "\n)" + options, nil
}

// buildAlterTable renders the ALTER TABLE statements of the desired
// state, in execution order: rename (step 1, if NewName differs), the
// column operations in the order the columns are listed, and finally the
// table options update. Later statements address the renamed table.
func buildAlterTable(table string, d driver.TableDef) ([]string, error) {
	if table == "" {
		return nil, fmt.Errorf("alter table: table name is empty")
	}

	var stmts []string
	current := table
	if d.NewName != "" && d.NewName != table {
		stmts = append(stmts, "ALTER TABLE "+sqlgen.QuoteIdent(table)+" RENAME TO "+sqlgen.QuoteIdent(d.NewName))
		current = d.NewName
	}

	for i, c := range d.Columns {
		switch {
		case c.OldName == "" && c.Type != "": // ADD
			frag, err := columnFragment(c)
			if err != nil {
				return nil, fmt.Errorf("alter table %s: column %d: %w", table, i+1, err)
			}
			placement := ""
			switch {
			case strings.EqualFold(c.Position, "FIRST"):
				placement = " FIRST"
			case c.Position != "":
				placement = " AFTER " + sqlgen.QuoteIdent(c.Position)
			}
			stmts = append(stmts, "ALTER TABLE "+sqlgen.QuoteIdent(current)+" ADD COLUMN "+frag+placement)
		case c.OldName != "" && c.Type == "": // DROP
			if c.Name == "" {
				return nil, fmt.Errorf("alter table %s: column %d is marked for drop but has no name", table, i+1)
			}
			stmts = append(stmts, "ALTER TABLE "+sqlgen.QuoteIdent(current)+" DROP COLUMN "+sqlgen.QuoteIdent(c.Name))
		case c.OldName != "" && c.OldName != c.Name: // CHANGE
			frag, err := columnFragment(c)
			if err != nil {
				return nil, fmt.Errorf("alter table %s: column %d: %w", table, i+1, err)
			}
			stmts = append(stmts, "ALTER TABLE "+sqlgen.QuoteIdent(current)+
				" CHANGE COLUMN "+sqlgen.QuoteIdent(c.OldName)+" "+frag)
		case c.OldName != "": // MODIFY (OldName == Name)
			frag, err := columnFragment(c)
			if err != nil {
				return nil, fmt.Errorf("alter table %s: column %d: %w", table, i+1, err)
			}
			stmts = append(stmts, "ALTER TABLE "+sqlgen.QuoteIdent(current)+" MODIFY COLUMN "+frag)
		default:
			return nil, fmt.Errorf("alter table %s: column %d (%q) has neither an old name nor a type", table, i+1, c.Name)
		}
	}

	opts, err := alterTableOptions(d.Options)
	if err != nil {
		return nil, fmt.Errorf("alter table %s: %w", table, err)
	}
	if len(opts) > 0 {
		stmts = append(stmts, "ALTER TABLE "+sqlgen.QuoteIdent(current)+" "+strings.Join(opts, " "))
	}
	return stmts, nil
}

// columnFragment renders one column definition. ADD placement
// (FIRST/AFTER) is handled by buildAlterTable; for MODIFY/CHANGE MySQL
// re-creates the column in place.
func columnFragment(c driver.ColumnDef) (string, error) {
	if c.Name == "" {
		return "", fmt.Errorf("column definition has no name")
	}
	typePart, err := columnTypeFragment(c)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(sqlgen.QuoteIdent(c.Name))
	sb.WriteString(" ")
	sb.WriteString(typePart)

	if c.Generated != "" {
		// A generated column's expression travels in Default.Value;
		// per MySQL grammar NULL/NOT NULL follows the AS clause.
		if c.Default == nil || c.Default.Value == "" {
			return "", fmt.Errorf("generated column %s requires an expression", c.Name)
		}
		sb.WriteString(" AS (" + c.Default.Value + ")")
		if c.Generated == "VIRTUAL" || c.Generated == "STORED" {
			sb.WriteString(" " + c.Generated)
		}
		if c.Nullable {
			sb.WriteString(" NULL")
		} else {
			sb.WriteString(" NOT NULL")
		}
	} else {
		if c.Unsigned {
			sb.WriteString(" unsigned")
		}
		if c.Nullable {
			sb.WriteString(" NULL")
		} else {
			sb.WriteString(" NOT NULL")
		}
		if c.Default != nil {
			switch {
			case c.Default.IsSQLExpression:
				sb.WriteString(" DEFAULT " + c.Default.Value)
			default:
				sb.WriteString(" DEFAULT " + sqlQuoteString(c.Default.Value))
			}
		}
		if c.AutoIncrement {
			sb.WriteString(" AUTO_INCREMENT")
		}
		if c.OnUpdate != "" {
			sb.WriteString(" ON UPDATE " + c.OnUpdate)
		}
	}

	if c.Collation != "" {
		if !collRe.MatchString(c.Collation) {
			return "", fmt.Errorf("column %s: invalid collation %q", c.Name, c.Collation)
		}
		sb.WriteString(" COLLATE " + sqlgen.QuoteIdent(c.Collation))
	}
	if c.Comment != "" {
		sb.WriteString(" COMMENT " + sqlQuoteString(c.Comment))
	}
	return sb.String(), nil
}

// isEnumOrSetType reports whether the base type word is enum or set; the
// values then live in ColumnDef.EnumValues, not in Length.
func isEnumOrSetType(baseType string) bool {
	switch strings.ToLower(baseType) {
	case "enum", "set":
		return true
	default:
		return false
	}
}

// columnTypeFragment renders the data type: enum and set from their
// quoted value list, everything else as type with the numeric length
// suffix. Both the type word and the length are whitelisted so no raw
// client string can reach the DDL text.
func columnTypeFragment(c driver.ColumnDef) (string, error) {
	if !typeRe.MatchString(c.Type) {
		return "", fmt.Errorf("column %s: invalid type %q", c.Name, c.Type)
	}
	if isEnumOrSetType(c.Type) {
		if len(c.EnumValues) == 0 {
			return "", fmt.Errorf("column %s: %s requires values", c.Name, c.Type)
		}
		quoted := make([]string, len(c.EnumValues))
		for i, v := range c.EnumValues {
			quoted[i] = sqlQuoteString(v)
		}
		return c.Type + "(" + strings.Join(quoted, ",") + ")", nil
	}
	if c.Length != "" {
		if !lengthRe.MatchString(c.Length) {
			return "", fmt.Errorf("column %s: invalid length %q", c.Name, c.Length)
		}
		return c.Type + "(" + c.Length + ")", nil
	}
	return c.Type, nil
}

// indexFragment renders one inline index of a CREATE TABLE.
func indexFragment(idx driver.Index) (string, error) {
	cols, err := indexColumnsFragment(idx.Columns)
	if err != nil {
		return "", err
	}
	switch idx.Type {
	case "PRIMARY":
		return "PRIMARY KEY (" + cols + ")", nil
	case "UNIQUE":
		if idx.Name == "" {
			return "", fmt.Errorf("unique index without a name")
		}
		return "UNIQUE KEY " + sqlgen.QuoteIdent(idx.Name) + " (" + cols + ")", nil
	case "FULLTEXT":
		return "FULLTEXT KEY " + sqlgen.QuoteIdent(idx.Name) + " (" + cols + ")", nil
	case "SPATIAL":
		return "SPATIAL KEY " + sqlgen.QuoteIdent(idx.Name) + " (" + cols + ")", nil
	default: // INDEX
		return "KEY " + sqlgen.QuoteIdent(idx.Name) + " (" + cols + ")", nil
	}
}

// indexColumnsFragment renders the parenthesised column list of an
// index, honouring the sub-part prefix length.
func indexColumnsFragment(cols []driver.IndexColumn) (string, error) {
	if len(cols) == 0 {
		return "", fmt.Errorf("index without columns")
	}
	parts := make([]string, len(cols))
	for i, col := range cols {
		if col.Name == "" {
			return "", fmt.Errorf("index column %d has no name", i)
		}
		parts[i] = sqlgen.QuoteIdent(col.Name)
		if col.SubPart > 0 {
			parts[i] += "(" + fmt.Sprint(col.SubPart) + ")"
		}
	}
	return strings.Join(parts, ", "), nil
}

// foreignKeyFragment renders one inline CONSTRAINT ... FOREIGN KEY.
func foreignKeyFragment(fk driver.ForeignKey) (string, error) {
	if len(fk.Columns) == 0 || len(fk.RefColumns) == 0 {
		return "", fmt.Errorf("foreign key %s needs columns on both sides", fk.Name)
	}
	if fk.RefTable == "" {
		return "", fmt.Errorf("foreign key %s has no referenced table", fk.Name)
	}
	cols := make([]string, len(fk.Columns))
	for i, c := range fk.Columns {
		cols[i] = sqlgen.QuoteIdent(c)
	}
	refs := make([]string, len(fk.RefColumns))
	for i, c := range fk.RefColumns {
		refs[i] = sqlgen.QuoteIdent(c)
	}

	sb := &strings.Builder{}
	if fk.Name != "" {
		sb.WriteString("CONSTRAINT " + sqlgen.QuoteIdent(fk.Name) + " ")
	}
	sb.WriteString("FOREIGN KEY (" + strings.Join(cols, ", ") + ") REFERENCES " +
		quoteQualifiedIdent(fk.RefTable) + " (" + strings.Join(refs, ", ") + ")")
	if fk.OnDelete != "" {
		sb.WriteString(" ON DELETE " + fk.OnDelete)
	}
	if fk.OnUpdate != "" {
		sb.WriteString(" ON UPDATE " + fk.OnUpdate)
	}
	return sb.String(), nil
}

// quoteQualifiedIdent backtick-quotes each dot-separated part of a
// possibly schema-qualified identifier.
func quoteQualifiedIdent(name string) string {
	parts := strings.Split(name, ".")
	quoted := make([]string, len(parts))
	for i, p := range parts {
		quoted[i] = sqlgen.QuoteIdent(p)
	}
	return strings.Join(quoted, ".")
}

// charsetFromCollation derives the character set of a collation name:
// the part before the first underscore ("utf8mb4_unicode_ci" →
// "utf8mb4", "binary" → "binary").
func charsetFromCollation(collation string) string {
	if charset, _, found := strings.Cut(collation, "_"); found {
		return charset
	}
	return collation
}

// createTableOptions renders the trailing table options of a CREATE
// TABLE; the returned string is empty or starts with a space.
func createTableOptions(o driver.TableOptions) (string, error) {
	var parts []string
	if o.Engine != "" {
		if !collRe.MatchString(o.Engine) {
			return "", fmt.Errorf("invalid engine %q", o.Engine)
		}
		parts = append(parts, "ENGINE="+o.Engine)
	}
	if o.AutoIncrement > 0 {
		parts = append(parts, "AUTO_INCREMENT="+fmt.Sprint(o.AutoIncrement))
	}
	if o.Collation != "" {
		if !collRe.MatchString(o.Collation) {
			return "", fmt.Errorf("invalid collation %q", o.Collation)
		}
		parts = append(parts, "DEFAULT CHARSET="+charsetFromCollation(o.Collation), "COLLATE="+o.Collation)
	}
	if o.Comment != "" {
		parts = append(parts, "COMMENT="+sqlQuoteString(o.Comment))
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " " + strings.Join(parts, " "), nil
}

// alterTableOptions renders the table option clauses of an ALTER TABLE
// (only the options the desired state actually sets).
func alterTableOptions(o driver.TableOptions) ([]string, error) {
	var parts []string
	if o.Engine != "" {
		if !collRe.MatchString(o.Engine) {
			return nil, fmt.Errorf("invalid engine %q", o.Engine)
		}
		parts = append(parts, "ENGINE="+o.Engine)
	}
	if o.AutoIncrement > 0 {
		parts = append(parts, "AUTO_INCREMENT="+fmt.Sprint(o.AutoIncrement))
	}
	if o.Comment != "" {
		parts = append(parts, "COMMENT="+sqlQuoteString(o.Comment))
	}
	if o.Collation != "" {
		if !collRe.MatchString(o.Collation) {
			return nil, fmt.Errorf("invalid collation %q", o.Collation)
		}
		parts = append(parts, "COLLATE="+o.Collation)
	}
	return parts, nil
}

// sqlQuoteString renders a SQL string literal, doubling inner single
// quotes so a value cannot terminate the literal early.
func sqlQuoteString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
