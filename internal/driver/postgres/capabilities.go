package postgres

import (
	"context"
	"fmt"

	"github.com/dblama-dev/dblama/internal/driver"
)

// ListSchemas returns the user-visible schemas of the bound database
// (system schemas excluded).
func (d *PostgresDriver) ListSchemas(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT nspname FROM pg_namespace
		 WHERE nspname NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
		   AND nspname NOT LIKE 'pg_temp_%'
		 ORDER BY nspname`)
	if err != nil {
		return nil, fmt.Errorf("list schemas: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("list schemas: %w", err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list schemas: %w", err)
	}
	return out, nil
}

// ListSequences returns the sequences of one schema.
func (d *PostgresDriver) ListSequences(ctx context.Context, schema string) ([]driver.SequenceInfo, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT sequencename, COALESCE(last_value, 0), increment_by
		FROM pg_sequences WHERE schemaname = $1 ORDER BY sequencename`, schema)
	if err != nil {
		return nil, fmt.Errorf("list sequences: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []driver.SequenceInfo
	for rows.Next() {
		var s driver.SequenceInfo
		if err := rows.Scan(&s.Name, &s.LastValue, &s.Increment); err != nil {
			return nil, fmt.Errorf("list sequences: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sequences: %w", err)
	}
	return out, nil
}

// CreateSequence creates a sequence in the current schema.
func (d *PostgresDriver) CreateSequence(ctx context.Context, name string, increment int64) error {
	qual, err := qualify(d.schemaFor(ctx), name)
	if err != nil {
		return fmt.Errorf("create sequence: %w", err)
	}
	inc := ""
	if increment != 0 && increment != 1 {
		inc = fmt.Sprintf(" INCREMENT BY %d", increment)
	}
	if _, err := d.db.ExecContext(ctx, "CREATE SEQUENCE "+qual+inc); err != nil {
		return fmt.Errorf("create sequence %s: %w", qual, err)
	}
	return nil
}

// DropSequence drops a sequence in the current schema.
func (d *PostgresDriver) DropSequence(ctx context.Context, name string) error {
	qual, err := qualify(d.schemaFor(ctx), name)
	if err != nil {
		return fmt.Errorf("drop sequence: %w", err)
	}
	if _, err := d.db.ExecContext(ctx, "DROP SEQUENCE "+qual); err != nil {
		return fmt.Errorf("drop sequence %s: %w", qual, err)
	}
	return nil
}

// AlterSequence restarts a sequence and/or changes its increment.
func (d *PostgresDriver) AlterSequence(ctx context.Context, name string, restartWith *int64, increment *int64) error {
	qual, err := qualify(d.schemaFor(ctx), name)
	if err != nil {
		return fmt.Errorf("alter sequence: %w", err)
	}
	if increment != nil {
		if _, err := d.db.ExecContext(ctx, "ALTER SEQUENCE "+qual+" INCREMENT BY "+fmt.Sprint(*increment)); err != nil {
			return fmt.Errorf("alter sequence %s: %w", qual, err)
		}
	}
	if restartWith != nil {
		if _, err := d.db.ExecContext(ctx, "ALTER SEQUENCE "+qual+" RESTART WITH "+fmt.Sprint(*restartWith)); err != nil {
			return fmt.Errorf("alter sequence %s: %w", qual, err)
		}
	}
	return nil
}
