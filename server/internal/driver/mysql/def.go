package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// defTableQuery reads the storage attributes of one table from its own
// TABLES row; the absence of that row means the table does not exist.
const defTableQuery = `
SELECT COALESCE(ENGINE, ''),
       COALESCE(TABLE_COLLATION, ''),
       COALESCE(TABLE_COMMENT, ''),
       AUTO_INCREMENT
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`

// defColumnsQuery reads the columns of one table in ordinal order.
// GENERATION_EXPRESSION carries the expression of generated columns,
// whose COLUMN_DEFAULT is always NULL.
const defColumnsQuery = `
SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, EXTRA,
       COLLATION_NAME, COLUMN_COMMENT, GENERATION_EXPRESSION
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
ORDER BY ORDINAL_POSITION`

// defIndexesQuery reads the index parts of one table. The ordering by
// INDEX_NAME, SEQ_IN_INDEX groups each index into consecutive rows.
const defIndexesQuery = `
SELECT INDEX_NAME, NON_UNIQUE, COLUMN_NAME, INDEX_TYPE, COALESCE(SUB_PART, 0)
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
ORDER BY INDEX_NAME, SEQ_IN_INDEX`

// defForeignKeysQuery reads the foreign keys of one table, joining the
// per-column KEY_COLUMN_USAGE rows with the per-constraint rules.
const defForeignKeysQuery = `
SELECT kcu.CONSTRAINT_NAME, kcu.COLUMN_NAME,
       kcu.REFERENCED_TABLE_SCHEMA, kcu.REFERENCED_TABLE_NAME, kcu.REFERENCED_COLUMN_NAME,
       rc.UPDATE_RULE, rc.DELETE_RULE
FROM information_schema.KEY_COLUMN_USAGE kcu
JOIN information_schema.REFERENTIAL_CONSTRAINTS rc
  ON rc.CONSTRAINT_SCHEMA = kcu.CONSTRAINT_SCHEMA
 AND rc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
WHERE kcu.TABLE_SCHEMA = ? AND kcu.TABLE_NAME = ?
  AND kcu.REFERENCED_TABLE_NAME IS NOT NULL
ORDER BY kcu.CONSTRAINT_NAME, kcu.ORDINAL_POSITION`

// TableDef reads the full structure of one table: columns, indexes,
// foreign keys and table options. A missing table yields an error
// containing "not found".
func (d *MysqlDriver) TableDef(ctx context.Context, db, table string) (driver.TableDef, error) {
	if db == "" {
		return driver.TableDef{}, fmt.Errorf("read table definition: database name is empty")
	}

	options, err := d.tableOptions(ctx, db, table)
	if err != nil {
		return driver.TableDef{}, err
	}

	columns, err := d.tableColumns(ctx, db, table)
	if err != nil {
		return driver.TableDef{}, err
	}
	indexes, err := d.tableIndexes(ctx, db, table)
	if err != nil {
		return driver.TableDef{}, err
	}
	fks, err := d.tableForeignKeys(ctx, db, table)
	if err != nil {
		return driver.TableDef{}, err
	}

	return driver.TableDef{
		NewName:     table,
		Columns:     columns,
		Indexes:     indexes,
		ForeignKeys: fks,
		Options:     options,
	}, nil
}

// tableOptions reads the table's own TABLES row. No row means the table
// does not exist.
func (d *MysqlDriver) tableOptions(ctx context.Context, db, table string) (driver.TableOptions, error) {
	var (
		engine    sql.NullString
		collation sql.NullString
		comment   sql.NullString
		autoInc   sql.NullInt64
	)
	err := d.db.QueryRowContext(ctx, defTableQuery, db, table).Scan(&engine, &collation, &comment, &autoInc)
	if errors.Is(err, sql.ErrNoRows) {
		return driver.TableOptions{}, fmt.Errorf("table %q not found", table)
	}
	if err != nil {
		return driver.TableOptions{}, fmt.Errorf("query options of %s.%s: %w", db, table, err)
	}
	return driver.TableOptions{
		Engine:        engine.String,
		Collation:     collation.String,
		Comment:       comment.String,
		AutoIncrement: autoInc.Int64,
	}, nil
}

// tableColumns reads and parses the columns of one table in ordinal
// order.
func (d *MysqlDriver) tableColumns(ctx context.Context, db, table string) ([]driver.ColumnDef, error) {
	rows, err := d.db.QueryContext(ctx, defColumnsQuery, db, table)
	if err != nil {
		return nil, fmt.Errorf("query columns of %s.%s: %w", db, table, err)
	}
	defer func() { _ = rows.Close() }()

	var columns []driver.ColumnDef
	for rows.Next() {
		var (
			name      string
			colType   string
			nullable  string
			defRaw    sql.NullString
			extra     sql.NullString
			collation sql.NullString
			comment   sql.NullString
			genExpr   sql.NullString
		)
		if err := rows.Scan(&name, &colType, &nullable, &defRaw, &extra, &collation, &comment, &genExpr); err != nil {
			return nil, fmt.Errorf("scan column row of %s.%s: %w", db, table, err)
		}
		base, length, unsigned, enumVals := parseColumnType(colType)
		generated := generatedKind(extra.String)
		var def *driver.DefaultValue
		if generated != "" {
			// A generated column has no COLUMN_DEFAULT; its expression
			// travels in Default.Value (the save path renders it as
			// AS (expr)).
			def = &driver.DefaultValue{Value: normalizeGenExpr(genExpr.String), IsSQLExpression: true}
		} else {
			def = parseColumnDefault(defRaw, extra.String)
		}
		columns = append(columns, driver.ColumnDef{
			Name:          name,
			OldName:       name, // чтение = Name; SaveTable различает ADD (OldName == "") и MODIFY/CHANGE
			Type:          base,
			Length:        length,
			Unsigned:      unsigned,
			Nullable:      nullable == "YES",
			Default:       def,
			AutoIncrement: strings.Contains(extra.String, "auto_increment"),
			OnUpdate:      onUpdateExpr(extra.String),
			Generated:     generated,
			Collation:     collation.String,
			Comment:       comment.String,
			EnumValues:    enumVals,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate columns of %s.%s: %w", db, table, err)
	}
	return columns, nil
}

// tableIndexes reads the indexes of one table, grouping the STATISTICS
// rows by index name in STATISTICS order.
func (d *MysqlDriver) tableIndexes(ctx context.Context, db, table string) ([]driver.Index, error) {
	rows, err := d.db.QueryContext(ctx, defIndexesQuery, db, table)
	if err != nil {
		return nil, fmt.Errorf("query indexes of %s.%s: %w", db, table, err)
	}
	defer func() { _ = rows.Close() }()

	var indexes []driver.Index
	for rows.Next() {
		var (
			name      string
			nonUnique int
			colName   string
			idxType   string
			subPart   int
		)
		if err := rows.Scan(&name, &nonUnique, &colName, &idxType, &subPart); err != nil {
			return nil, fmt.Errorf("scan index row of %s.%s: %w", db, table, err)
		}
		if len(indexes) == 0 || indexes[len(indexes)-1].Name != name {
			indexes = append(indexes, driver.Index{Name: name, Type: indexKind(name, nonUnique, idxType)})
		}
		last := &indexes[len(indexes)-1]
		last.Columns = append(last.Columns, driver.IndexColumn{Name: colName, SubPart: subPart})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexes of %s.%s: %w", db, table, err)
	}
	return indexes, nil
}

// tableForeignKeys reads the foreign keys declared on one table, ordered
// by constraint name and column position.
func (d *MysqlDriver) tableForeignKeys(ctx context.Context, db, table string) ([]driver.ForeignKey, error) {
	rows, err := d.db.QueryContext(ctx, defForeignKeysQuery, db, table)
	if err != nil {
		return nil, fmt.Errorf("query foreign keys of %s.%s: %w", db, table, err)
	}
	defer func() { _ = rows.Close() }()

	var fks []driver.ForeignKey
	for rows.Next() {
		var (
			name       string
			colName    string
			refSchema  sql.NullString
			refTable   string
			refColName string
			updateRule sql.NullString
			deleteRule sql.NullString
		)
		if err := rows.Scan(&name, &colName, &refSchema, &refTable, &refColName, &updateRule, &deleteRule); err != nil {
			return nil, fmt.Errorf("scan foreign key row of %s.%s: %w", db, table, err)
		}
		if len(fks) == 0 || fks[len(fks)-1].Name != name {
			fks = append(fks, driver.ForeignKey{
				Name:     name,
				RefTable: refTable,
				RefSchema: func() string {
					if refSchema.Valid && refSchema.String != db {
						return refSchema.String
					}
					return ""
				}(),
				OnUpdate: updateRule.String,
				OnDelete: deleteRule.String,
			})
		}
		last := &fks[len(fks)-1]
		last.Columns = append(last.Columns, colName)
		last.RefColumns = append(last.RefColumns, refColName)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate foreign keys of %s.%s: %w", db, table, err)
	}
	return fks, nil
}

// parseColumnType splits a COLUMN_TYPE value such as "int unsigned",
// "varchar(64)", "decimal(10,2)" or "enum('a','b')" into its base type,
// the content of the parenthesised suffix (empty for enum and set,
// whose values are returned instead) and the unsigned flag.
func parseColumnType(colType string) (base, length string, unsigned bool, enumVals []string) {
	base = colType
	if i := strings.IndexAny(base, "( "); i >= 0 {
		base = base[:i]
	}
	if isEnumOrSet(colType) {
		// An enum/set value may itself contain the word "unsigned"
		// (enum('unsigned')); the substring scan would report a false
		// positive, so the flag is skipped for these types.
		return base, "", false, enumValues(colType)
	}
	unsigned = strings.Contains(colType, "unsigned")
	if i := strings.IndexByte(colType, '('); i >= 0 {
		if j := strings.LastIndexByte(colType, ')'); j > i {
			length = colType[i+1 : j]
		}
	}
	return base, length, unsigned, nil
}

// isEnumOrSet reports whether colType is an enum(...) or set(...).
func isEnumOrSet(colType string) bool {
	lower := strings.ToLower(colType)
	return strings.HasPrefix(lower, "enum(") || strings.HasPrefix(lower, "set(")
}

// enumValues parses the quoted value list of an enum(...) or set(...)
// COLUMN_TYPE, unescaping doubled quotes.
func enumValues(colType string) []string {
	open := strings.IndexByte(colType, '(')
	closing := strings.LastIndexByte(colType, ')')
	if open < 0 || closing <= open+1 {
		return nil
	}
	parts := splitUnquotedCommas(colType[open+1 : closing])
	vals := make([]string, 0, len(parts))
	for _, part := range parts {
		vals = append(vals, unquoteSQLString(strings.TrimSpace(part)))
	}
	return vals
}

// splitUnquotedCommas splits s on commas that are outside single-quoted
// strings; doubled quotes inside a value are kept verbatim.
func splitUnquotedCommas(s string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' && inQuote && i+1 < len(s) && s[i+1] == '\'':
			cur.WriteString("''")
			i++
		case c == '\'':
			inQuote = !inQuote
			cur.WriteByte(c)
		case c == ',' && !inQuote:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	return append(parts, cur.String())
}

// unquoteSQLString removes the surrounding quotes of a SQL string
// literal and unescapes the escapes the servers use when reporting
// literals back (COLUMN_DEFAULT of MariaDB, COLUMN_TYPE of enum/set):
// doubled quotes (”), backslash-escaped quotes (\') and doubled
// backslashes (\\). The scan is left-to-right so sequences like \\\'
// resolve in one pass.
func unquoteSQLString(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		s = s[1 : len(s)-1]
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) && (s[i+1] == '\\' || s[i+1] == '\'') {
			b.WriteByte(s[i+1])
			i++
			continue
		}
		if c == '\'' && i+1 < len(s) && s[i+1] == '\'' {
			b.WriteByte('\'')
			i++
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// sqlDefaultRe matches defaults that are SQL expressions rather than
// literals: CURRENT_TIMESTAMP (with optional precision), NOW(...) and
// UNIX_TIMESTAMP(...). The match is case-insensitive because MariaDB
// reports lowercase "current_timestamp()".
var sqlDefaultRe = regexp.MustCompile(`(?i)^(current_timestamp|now\(|unix_timestamp\()`)

// parseColumnDefault converts a raw COLUMN_DEFAULT value into a
// DefaultValue; a missing default means no DefaultValue. MySQL 8
// reports a missing default as SQL NULL while MariaDB reports the
// literal string "NULL" (a real string default would come back quoted).
// MariaDB reports literal string defaults with the quotes preserved
// ("'x'") while MySQL 8 strips them ("x"); the quotes are removed when
// present so both flavors yield "x". Defaults reported by MySQL 8 with
// the DEFAULT_GENERATED extra that are not quoted literals are
// expressions.
func parseColumnDefault(raw sql.NullString, extra string) *driver.DefaultValue {
	if !raw.Valid || raw.String == "NULL" {
		return nil
	}
	value := raw.String
	isExpr := sqlDefaultRe.MatchString(value)
	if !isExpr && strings.Contains(extra, "DEFAULT_GENERATED") && !isQuotedLiteral(value) {
		isExpr = true
	}
	if !isExpr && isQuotedLiteral(value) {
		value = unquoteSQLString(value)
	}
	return &driver.DefaultValue{Value: value, IsSQLExpression: isExpr}
}

// genIntrocerRe matches the charset introcer MySQL prints before string
// literals inside GENERATION_EXPRESSION (e.g. _utf8mb4'!').
var genIntrocerRe = regexp.MustCompile(`_[A-Za-z0-9_]+(')`)

// normalizeGenExpr rewrites MySQL's printed form of a generation
// expression to standard SQL: string literals are printed as
// _utf8mb4\'text\' (backslash-quoted, with charset introcer) and become
// 'text'. Literal backslashes stay doubled so the expression re-parses
// verbatim. MariaDB already reports the standard form, which passes
// through unchanged — both flavors then yield the same value.
func normalizeGenExpr(expr string) string {
	var b strings.Builder
	for i := 0; i < len(expr); i++ {
		if expr[i] == '\\' && i+1 < len(expr) && expr[i+1] == '\'' {
			b.WriteByte('\'')
			i++
			continue
		}
		b.WriteByte(expr[i])
	}
	return genIntrocerRe.ReplaceAllString(b.String(), "$1")
}

// isQuotedLiteral reports whether s looks like a quoted string literal.
func isQuotedLiteral(s string) bool {
	return len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\''
}

// onUpdateExpr extracts the expression from an EXTRA value such as
// "DEFAULT_GENERATED on update CURRENT_TIMESTAMP", returning "" when the
// column has no ON UPDATE clause.
func onUpdateExpr(extra string) string {
	const marker = "on update "
	if i := strings.Index(extra, marker); i >= 0 {
		return extra[i+len(marker):]
	}
	return ""
}

// generatedKind maps the generated-column marker in EXTRA ("VIRTUAL
// GENERATED" / "STORED GENERATED") to "VIRTUAL" or "STORED"; "" when
// the column is not generated. The plain DEFAULT_GENERATED extra of
// expression defaults must not match.
func generatedKind(extra string) string {
	switch {
	case strings.Contains(extra, "STORED GENERATED"):
		return "STORED"
	case strings.Contains(extra, "VIRTUAL GENERATED"):
		return "VIRTUAL"
	default:
		return ""
	}
}

// indexKind classifies one index from its STATISTICS rows: the PRIMARY
// name wins, then the physical index type, then uniqueness.
func indexKind(name string, nonUnique int, indexType string) string {
	switch {
	case name == "PRIMARY":
		return "PRIMARY"
	case indexType == "FULLTEXT":
		return "FULLTEXT"
	case indexType == "SPATIAL":
		return "SPATIAL"
	case nonUnique == 0:
		return "UNIQUE"
	default:
		return "INDEX"
	}
}
