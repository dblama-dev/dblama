package postgres

import (
	"context"
	"fmt"
	"github.com/dblama-dev/dblama/internal/driver"
)

var _ driver.MonitorManager = (*PostgresDriver)(nil)

// Processes lists server connections and their current queries.
func (d *PostgresDriver) Processes(ctx context.Context) ([]driver.ProcessInfo, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT pid, COALESCE(usename,''), COALESCE(client_addr::text,''), COALESCE(datname,''), backend_type, CASE WHEN state='active' THEN GREATEST(0,EXTRACT(EPOCH FROM clock_timestamp()-query_start))::bigint ELSE 0 END, COALESCE(state,''), COALESCE(query,'') FROM pg_stat_activity ORDER BY pid`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []driver.ProcessInfo{}
	for rows.Next() {
		var p driver.ProcessInfo
		if err := rows.Scan(&p.ID, &p.User, &p.Host, &p.Database, &p.Command, &p.Seconds, &p.State, &p.Query); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// KillProcess terminates a server connection identified by its positive ID.
func (d *PostgresDriver) KillProcess(ctx context.Context, id int64) error {
	if id <= 0 || id > 2147483647 {
		return fmt.Errorf("invalid process id")
	}
	var killed bool
	if err := d.db.QueryRowContext(ctx, "SELECT pg_terminate_backend($1)", id).Scan(&killed); err != nil {
		return err
	}
	if !killed {
		return fmt.Errorf("process not found or cannot be terminated")
	}
	return nil
}

// ServerStatus reads global server status counters.
func (d *PostgresDriver) ServerStatus(ctx context.Context) ([]driver.ServerValue, error) {
	return d.monitorValues(ctx, `SELECT key, COALESCE(value,'') FROM pg_stat_database CROSS JOIN LATERAL jsonb_each_text(to_jsonb(pg_stat_database)) AS item(key,value) WHERE datname=current_database() ORDER BY key`)
}

// ServerVariables reads global server variables without modifying them.
func (d *PostgresDriver) ServerVariables(ctx context.Context) ([]driver.ServerValue, error) {
	return d.monitorValues(ctx, "SELECT name, setting || CASE WHEN unit IS NULL THEN '' ELSE ' ' || unit END FROM pg_settings ORDER BY name")
}
func (d *PostgresDriver) monitorValues(ctx context.Context, query string) ([]driver.ServerValue, error) {
	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []driver.ServerValue{}
	for rows.Next() {
		var v driver.ServerValue
		if err := rows.Scan(&v.Name, &v.Value); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
