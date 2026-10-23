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

// Driver is a connection-bound database driver instance.
type Driver interface {
	Info() DriverInfo
	Caps() Caps
	ListDatabases(ctx context.Context) ([]DatabaseInfo, error)
	ListTables(ctx context.Context, db string) ([]TableInfo, error)
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
