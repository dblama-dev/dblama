// Package driver defines the database driver contract and the registry
// that maps driver IDs to connect functions.
package driver

import "context"

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
