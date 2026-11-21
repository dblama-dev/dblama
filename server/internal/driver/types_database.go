package driver

import "context"

// DatabaseConnector is implemented by drivers that need a physical connection
// to the requested database. The caller releases it when the request finishes.
type DatabaseConnector interface {
	ForDatabase(ctx context.Context, name string) (Driver, func(), error)
}

// DatabaseManager is optional so established Driver implementations remain valid.
type DatabaseManager interface {
	CreateDatabase(context.Context, string, string) error
	DropDatabase(context.Context, string) error
	RenameDatabase(context.Context, string, string) error
}

// TableTransfer is implemented only where the server supports cross-database DDL.
type TableTransfer interface {
	MoveTables(context.Context, string, []string, string) error
	CopyTables(context.Context, string, []string, string) error
}

// TypeInfo describes a column type offered by the structure editor.
type TypeInfo struct {
	Name      string `json:"name"`
	Category  string `json:"category"`
	HasLength bool   `json:"hasLength,omitempty"`
}

// Collation describes a server collation or database locale.
type Collation struct {
	Name    string `json:"name"`
	Charset string `json:"charset,omitempty"`
	// Database distinguishes database locales from column-only collations on PG.
	Database bool `json:"database"`
}

// Operator describes a supported browsing filter and its value arity.
type Operator struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Arity int    `json:"arity"`
}

// Metadata contains catalogs used by database and structure editors.
type Metadata struct {
	ColumnTypes []TypeInfo  `json:"columnTypes"`
	Collations  []Collation `json:"collations"`
	Operators   []Operator  `json:"operators"`
}

// MetadataProvider supplies server-specific editor catalogs.
type MetadataProvider interface {
	Metadata(context.Context) (Metadata, error)
}
