//go:build integration

package postgres

import (
	"context"
	"github.com/hitslab/db-manager/server/internal/driver"
	"testing"
)

func TestAdminMonitor(t *testing.T) {
	d := connectOK(t, startDB(t)).(*PostgresDriver)
	ctx := context.Background()
	processes, err := d.Processes(ctx)
	if err != nil || len(processes) == 0 {
		t.Fatalf("processes=%v err=%v", processes, err)
	}
	conn, err := d.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var id int64
	if err := conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := d.KillProcess(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := d.KillProcess(ctx, 0); err == nil {
		t.Fatal("invalid pid accepted")
	}
	for _, fn := range []func(context.Context) ([]driver.ServerValue, error){d.ServerStatus, d.ServerVariables} {
		v, err := fn(ctx)
		if err != nil || len(v) == 0 {
			t.Fatalf("metrics=%v err=%v", v, err)
		}
	}
}
