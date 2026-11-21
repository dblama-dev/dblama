package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/dblama-dev/dblama/internal/driver"
	"strconv"
)

var _ driver.MonitorManager = (*MysqlDriver)(nil)

// Processes lists server connections and their current queries.
func (d *MysqlDriver) Processes(ctx context.Context) ([]driver.ProcessInfo, error) {
	rows, err := d.db.QueryContext(ctx, "SHOW FULL PROCESSLIST")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	// MariaDB may append progress columns; scan known positions and discard extras.
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	processes := []driver.ProcessInfo{}
	for rows.Next() {
		var p driver.ProcessInfo
		var db, state, query sql.NullString
		var seconds sql.NullInt64
		dest := []any{&p.ID, &p.User, &p.Host, &db, &p.Command, &seconds, &state, &query}
		for len(dest) < len(cols) {
			dest = append(dest, new(any))
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		p.Database = db.String
		p.State = state.String
		p.Query = query.String
		p.Seconds = seconds.Int64
		processes = append(processes, p)
	}
	return processes, rows.Err()
}

// KillProcess terminates a server connection identified by its positive ID.
func (d *MysqlDriver) KillProcess(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("process id must be positive")
	}
	_, err := d.db.ExecContext(ctx, "KILL CONNECTION "+strconv.FormatInt(id, 10))
	return err
}

// ServerStatus reads global server status counters.
func (d *MysqlDriver) ServerStatus(ctx context.Context) ([]driver.ServerValue, error) {
	return d.serverValues(ctx, "SHOW GLOBAL STATUS")
}

// ServerVariables reads global server variables without modifying them.
func (d *MysqlDriver) ServerVariables(ctx context.Context) ([]driver.ServerValue, error) {
	return d.serverValues(ctx, "SHOW GLOBAL VARIABLES")
}
func (d *MysqlDriver) serverValues(ctx context.Context, query string) ([]driver.ServerValue, error) {
	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	values := []driver.ServerValue{}
	for rows.Next() {
		var v driver.ServerValue
		if err := rows.Scan(&v.Name, &v.Value); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}
