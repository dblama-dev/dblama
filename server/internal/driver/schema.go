package driver

// SchemaAware is the optional driver extension for servers that organize
// tables into schemas inside one database (PostgreSQL): drivers that
// implement it keep a current-schema selection the API layer sets from
// the ?schema= parameter before calling the table/structure methods
// (spec §4.3). Handlers reach it by type assertion against the Driver
// interface — drivers without schemas (MySQL) simply do not implement it.
//
// The schema is instance state guarded by the driver's own mutex: one
// driver instance serves one API session, so one schema is current at a
// time. Implementations must qualify the current schema inside their SQL
// text (never via SET search_path on a shared pool) and reject an empty
// schema name.
type SchemaAware interface {
	// SetSchema makes schema the current schema of this driver instance.
	// An empty schema name is an error; the current selection is left
	// unchanged.
	SetSchema(schema string) error
	// CurrentSchema returns the current schema ("public" on PostgreSQL
	// until the first SetSchema).
	CurrentSchema() string
}
