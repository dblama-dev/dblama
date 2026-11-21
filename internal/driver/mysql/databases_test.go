package mysql

import (
	"context"
	"strings"
	"testing"
)

// A driver whose DSN still selects a removed database cannot open new pooled
// connections. Such mutations must stop before querying the connection pool.
func TestDatabaseOperationsPreserveBoundPool(t *testing.T) {
	for _, operation := range []string{"drop", "rename"} {
		t.Run(operation, func(t *testing.T) {
			d := &MysqlDriver{loginDB: "bound"}
			var err error
			if operation == "drop" {
				err = d.DropDatabase(context.Background(), "bound")
			} else {
				err = d.RenameDatabase(context.Background(), "bound", "new")
			}
			if err == nil || !strings.Contains(err.Error(), "reconnect") {
				t.Fatalf("missing reconnect guidance: %v", err)
			}
		})
	}
}
