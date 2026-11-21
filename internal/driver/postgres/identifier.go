package postgres

import (
	"fmt"

	"github.com/dblama-dev/dblama/internal/sqlgen"
)

// qualify renders "schema"."table": every catalog and data query
// qualifies the schema in its SQL text (spec §13.5 — no SET search_path
// on the shared pool), with each part double-quoted by QuoteIdentPg so
// mixed-case and special-character names survive verbatim. An empty part
// fails before any SQL is built.
func qualify(schema, table string) (string, error) {
	if schema == "" {
		return "", fmt.Errorf("%w: schema", sqlgen.ErrEmptyIdentifier)
	}
	if table == "" {
		return "", fmt.Errorf("%w: table", sqlgen.ErrEmptyIdentifier)
	}
	return sqlgen.QuoteIdentPg(schema) + "." + sqlgen.QuoteIdentPg(table), nil
}
