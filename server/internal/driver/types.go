// Package driver defines the database driver contract and the registry
// that maps driver IDs to connect functions.
package driver

import (
	"context"

	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// ConnConfig holds the parameters for establishing a database connection.
type ConnConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
}

// DriverInfo describes the server a driver instance is connected to.
//
//nolint:revive // name is fixed by the driver contract
type DriverInfo struct {
	Name    string `json:"name"`
	Flavor  string `json:"flavor"`
	Version string `json:"version"`
	User    string `json:"user"`
	Host    string `json:"host"`
}

// Caps declares which optional database features the connected server
// supports. Values are filled in by driver implementations.
type Caps struct {
	Views             bool `json:"views"`
	Triggers          bool `json:"triggers"`
	Routines          bool `json:"routines"`
	Procedures        bool `json:"procedures"`
	Events            bool `json:"events"`
	Partitions        bool `json:"partitions"`
	Sequences         bool `json:"sequences"`
	Schemas           bool `json:"schemas"`
	Users             bool `json:"users"`
	Monitor           bool `json:"monitor"`
	MoveCopy          bool `json:"moveCopy"`
	Comments          bool `json:"comments"`
	Upsert            bool `json:"upsert"`
	DescendingIndexes bool `json:"descendingIndexes"`
}

// DatabaseInfo describes one database on the server.
type DatabaseInfo struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	Collation string `json:"collation"`
}

// TableInfo describes one table or view in a database.
type TableInfo struct {
	Name      string `json:"name"`
	Rows      int64  `json:"rows"`
	SizeBytes int64  `json:"sizeBytes"`
	Engine    string `json:"engine"`
	Comment   string `json:"comment"`
	IsView    bool   `json:"isView"`
}

// DefaultValue is the default value of a column. IsSQLExpression marks
// defaults that are SQL expressions (CURRENT_TIMESTAMP and similar)
// rather than literals; the web layer renders them unquoted.
type DefaultValue struct {
	Value           string `json:"value"`
	IsSQLExpression bool   `json:"isSqlExpression,omitempty"` // CURRENT_TIMESTAMP, NULL-литерал в MySQL-смысле и т.п.
}

// ColumnDef describes one column of a table.
type ColumnDef struct {
	Name          string        `json:"name"`
	OldName       string        `json:"oldName,omitempty"` // чтение = Name
	Type          string        `json:"type"`              // базовый: "int", "varchar", ...
	Length        string        `json:"length,omitempty"`  // из COLUMN_TYPE: "(11)" без скобок → "11"; enum/set — ""
	Unsigned      bool          `json:"unsigned,omitempty"`
	Nullable      bool          `json:"nullable"`
	Default       *DefaultValue `json:"default,omitempty"` // nil = нет DEFAULT
	AutoIncrement bool          `json:"autoIncrement,omitempty"`
	OnUpdate      string        `json:"onUpdate,omitempty"`
	Generated     string        `json:"generated,omitempty"` // "" | "VIRTUAL" | "STORED"
	// The following values are used verbatim as SQL by the save path:
	// OnUpdate, the Default.Value of an IsSQLExpression default (incl.
	// the expression of a Generated column) and Generated's expression.
	Collation  string   `json:"collation,omitempty"`
	Comment    string   `json:"comment,omitempty"`
	EnumValues []string `json:"enumValues,omitempty"` // парсинг COLUMN_TYPE enum('a','b') с экранированием ''
	Position   string   `json:"position,omitempty"`   // чтение = "" (позиция в ORDINAL_ORDER)
}

// IndexColumn is one column of an index. SubPart is the prefixed length
// in characters, 0 when the full column is indexed.
type IndexColumn struct {
	Name    string `json:"name"`
	SubPart int    `json:"subPart,omitempty"`
}

// Index describes one index of a table. Type is one of PRIMARY, UNIQUE,
// INDEX, FULLTEXT, SPATIAL.
type Index struct {
	Name    string        `json:"name"`
	Type    string        `json:"type"`
	Columns []IndexColumn `json:"columns"`
}

// ForeignKey describes one foreign key of a table. OnDelete and OnUpdate
// hold the referential actions (CASCADE, SET NULL, NO ACTION, RESTRICT).
type ForeignKey struct {
	Name       string   `json:"name"`
	Columns    []string `json:"columns"`
	RefTable   string   `json:"refTable"`
	RefColumns []string `json:"refColumns"`
	OnDelete   string   `json:"onDelete,omitempty"` // CASCADE|SET NULL|NO ACTION|RESTRICT
	OnUpdate   string   `json:"onUpdate,omitempty"`
}

// TableOptions holds the storage attributes of a table.
type TableOptions struct {
	Engine        string `json:"engine,omitempty"`
	Collation     string `json:"collation,omitempty"`
	Comment       string `json:"comment,omitempty"`
	AutoIncrement int64  `json:"autoIncrement,omitempty"`
}

// TableDef is the full structure of one table: its columns, indexes,
// foreign keys and options. NewName carries the table's current name on
// read so SaveTable can detect renames.
type TableDef struct {
	NewName     string       `json:"newName,omitempty"` // чтение = имя таблицы; SaveTable сравнивает
	Columns     []ColumnDef  `json:"columns"`
	Indexes     []Index      `json:"indexes"`
	ForeignKeys []ForeignKey `json:"foreignKeys"`
	Options     TableOptions `json:"options"`
}

// SelectQuery is the structured browsing query; the alias keeps the
// driver contract in one package while the SQL generation stays in
// sqlgen.
type SelectQuery = sqlgen.SelectQuery

// ColumnMeta describes one column of a browsed result: its name, its
// base type as the driver labels it (e.g. "int", "varchar") and its
// nullability.
type ColumnMeta struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

// Page is one page of browsed rows. Rows holds JSON-compatible values:
// NULL → nil, binary values → hex string with the "0x" prefix
// (spec §4.5), timestamps → RFC3339 string, the rest as
// database/sql delivers them (int64, float64, string).
type Page struct {
	Columns   []ColumnMeta `json:"columns"`
	Rows      [][]any      `json:"rows"`
	TotalRows int64        `json:"totalRows"`
}

// RowValues holds one row's column values for a write operation; the
// keys are column names. JSON null arrives as nil and is written as SQL
// NULL, an empty string stays an empty string — the two are never
// conflated. Values are the plain Go values encoding/json produces
// (string, float64, bool, nil, ...): the "0x.."-prefixed hex strings a
// read page uses for binary columns are NOT reinterpreted here, writes
// take them literally as text.
type RowValues map[string]any

// PK identifies one row for UpdateRow and DeleteRows: column name →
// exact value. Typically the primary key columns; for a table without
// one the API layer passes all columns (its decision, not the
// driver's). An empty PK is rejected by the drivers so no unqualified
// full-table write can be issued.
type PK map[string]any

// ExecOpts tunes one ExecSQL call. The zero value enables the defaults:
// MaxRows 0 → 1000, TimeoutSec 0 → 60.
type ExecOpts struct {
	// MaxRows caps the rows buffered per statement's result; a result
	// with more rows is cut at the limit and flagged Truncated.
	MaxRows int
	// TimeoutSec bounds the whole batch execution; 0 → 60.
	TimeoutSec int
}

// Result is one statement's outcome inside an ExecSQL batch. A
// row-returning statement fills Columns/Rows (Truncated when the row
// limit cut it); a non-row statement fills AffectedRows/LastInsertID.
// Seconds is the statement's own execution time.
type Result struct {
	Columns      []ColumnMeta `json:"columns"`
	Rows         [][]any      `json:"rows"`
	AffectedRows int64        `json:"affectedRows,omitempty"`
	LastInsertID int64        `json:"lastInsertId,omitempty"`
	Truncated    bool         `json:"truncated,omitempty"`
	Seconds      float64      `json:"seconds"`
}

// Driver is a connection-bound database driver instance.
type Driver interface {
	Info() DriverInfo
	Caps() Caps
	ListDatabases(ctx context.Context) ([]DatabaseInfo, error)
	ListTables(ctx context.Context, db string) ([]TableInfo, error)
	TableDef(ctx context.Context, db, table string) (TableDef, error)
	// SaveTable applies the desired table state. An empty table name
	// creates the table (desired.NewName); a non-empty one alters it
	// (rename, columns, options — indexes via SaveIndexes).
	SaveTable(ctx context.Context, db, table string, desired TableDef) error
	// SaveIndexes applies the full desired index state of one table:
	// existing indexes not in desired are dropped (the PRIMARY via
	// DROP PRIMARY KEY, the rest via DROP INDEX), missing or changed
	// ones are added. Changed means same name with different type or
	// columns.
	SaveIndexes(ctx context.Context, db, table string, desired []Index) error
	// TruncateTables empties the named tables in order; the first
	// error aborts the rest. An empty name fails before any statement.
	TruncateTables(ctx context.Context, db string, tables []string) error
	// DropTables drops the named tables (no IF EXISTS — the caller
	// validates existence); the first error aborts the rest.
	DropTables(ctx context.Context, db string, tables []string) error
	// DropViews drops the named views (no IF EXISTS); the first error
	// aborts the rest.
	DropViews(ctx context.Context, db string, views []string) error
	// SelectRows returns one page of browsed rows of table in database
	// db. TotalRows counts the rows matching the filters, independent
	// of the page's limit and offset. An empty db or table name fails
	// before any query is issued.
	SelectRows(ctx context.Context, db, table string, q sqlgen.SelectQuery) (Page, error)
	// PrimaryKey returns the primary key column names in key order; a
	// table without a primary key yields an empty slice.
	PrimaryKey(ctx context.Context, db, table string) ([]string, error)
	// InsertRow inserts one row into table in database db. The values
	// are bound as parameters in sorted column order, so the statement
	// is deterministic for a given row. The auto_increment id generated
	// by the insert (0 when the table has none) is remembered in the
	// driver instance and read back with LastInsertID.
	InsertRow(ctx context.Context, db, table string, vals RowValues) error
	// UpdateRow updates every row whose columns match where exactly with
	// the given values. An empty vals or an empty where fails before any
	// statement is issued: the driver never runs an UPDATE without a
	// WHERE clause.
	UpdateRow(ctx context.Context, db, table string, where PK, vals RowValues) error
	// DeleteRows deletes one row per key in a single transaction —
	// either every row is deleted or none is. An empty key inside where
	// or an empty key list fails before any statement.
	DeleteRows(ctx context.Context, db, table string, where []PK) error
	// UpsertRows inserts every row as INSERT ... ON DUPLICATE KEY
	// UPDATE: a row whose unique key already exists updates the
	// conflicting row with the new values, the rest are inserted. All
	// rows must have the same column set. The auto_increment id of the
	// statement is remembered for LastInsertID.
	UpsertRows(ctx context.Context, db, table string, rows []RowValues) error
	// LastInsertID returns the auto_increment id generated by the most
	// recent InsertRow or UpsertRows executed through this driver
	// instance (0 when that write generated none). The value lives in
	// the mutex-protected driver state — driver instances are shared
	// across requests, so a per-connection LAST_INSERT_ID() would not
	// be reliable. It is driver state, not per-request state: the last
	// insert wins across concurrent requests, so an id must be read
	// immediately after the insert it belongs to.
	LastInsertID(ctx context.Context) (int64, error)
	// ExecSQL runs the (possibly multi-statement) console query against
	// database db on a dedicated connection and returns one Result per
	// statement, in order. Statements split on ';' outside strings and
	// comments (no DELIMITER support). A statement that returns rows
	// yields Columns/Rows (up to opts.MaxRows, flagged Truncated);
	// every other statement yields AffectedRows/LastInsertID — detected
	// by a pinned keyword heuristic (SELECT/SHOW/DESC/DESCRIBE/EXPLAIN/
	// WITH), so a row-returning statement outside that list comes back
	// without rows. The first failing statement aborts the rest: the
	// Results accumulated so far are returned together with the error.
	// A dedicated connection per call: db other than the login database
	// is selected via USE on that connection.
	ExecSQL(ctx context.Context, db, query string, queryID string, opts ExecOpts) ([]Result, error)
	// CancelQuery cancels the running query registered under queryID.
	// An unknown queryID (the query already finished, or never ran) is
	// not an error: nil is returned.
	CancelQuery(ctx context.Context, queryID string) error
	Close() error
}

// DriverMeta describes a driver available for connections.
//
//nolint:revive // name is fixed by the driver contract
type DriverMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DefaultPort string `json:"defaultPort"`
}

// ConnectFunc establishes a connection and returns a driver instance
// bound to it.
type ConnectFunc func(ctx context.Context, cfg ConnConfig) (Driver, error)
