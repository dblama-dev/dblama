package driver

import "context"

// SchemaAware exposes schema metadata and a default for direct driver callers.
// HTTP requests must pass their selection through WithSchema rather than
// mutate this shared default; requests in one session can run concurrently.
type SchemaAware interface {
	// SetSchema makes schema the current schema of this driver instance.
	// An empty schema name is an error; the current selection is left
	// unchanged.
	SetSchema(schema string) error
	// CurrentSchema returns the current schema ("public" on PostgreSQL
	// until the first SetSchema).
	CurrentSchema() string
	// ListSchemas returns the user-visible schema names of the login
	// database (system schemas excluded).
	ListSchemas(ctx context.Context) ([]string, error)
}

type schemaContextKey struct{}

// WithSchema binds a selection to a single operation. Empty means public.
func WithSchema(ctx context.Context, schema string) context.Context {
	if schema == "" {
		schema = "public"
	}
	return context.WithValue(ctx, schemaContextKey{}, schema)
}

// SchemaFromContext returns the request selection, or empty for direct callers.
func SchemaFromContext(ctx context.Context) string {
	schema, _ := ctx.Value(schemaContextKey{}).(string)
	return schema
}
