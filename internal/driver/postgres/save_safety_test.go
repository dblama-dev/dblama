package postgres

import (
	"context"
	"database/sql"
	dbdriver "database/sql/driver"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/dblama-dev/dblama/internal/driver"
)

func TestCreateRejectsDuplicateIndexesAndUnsafeForeignKeyActions(t *testing.T) {
	base := driver.TableDef{NewName: "t", Columns: []driver.ColumnDef{{Name: "id", Type: "int"}}}
	for _, indexes := range [][]driver.Index{
		{{Name: "same", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "id"}}}, {Name: "same", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "id"}}}},
		{{Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}}, {Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}}},
	} {
		base.Indexes = indexes
		if _, err := buildCreateTable("public", base); err == nil {
			t.Fatal("duplicate indexes accepted")
		}
	}
	for _, fk := range []driver.ForeignKey{
		{Columns: []string{"id"}, RefTable: "p", RefColumns: []string{"id"}, OnDelete: "CASCADE; DROP TABLE important"},
		{Columns: []string{"id"}, RefTable: "p", RefColumns: []string{"id"}, OnUpdate: "SET DEFAULT; SELECT 1"},
		{Columns: []string{"id", ""}, RefTable: "p", RefColumns: []string{"id", "x"}},
		{Columns: []string{"id"}, RefTable: "p", RefColumns: []string{"id", "x"}},
	} {
		if _, err := foreignKeyFragmentPg(fk); err == nil {
			t.Fatalf("unsafe foreign key accepted: %+v", fk)
		}
	}
}

type panicDDLDriver struct{ rolledBack *atomic.Bool }

func (d panicDDLDriver) Open(string) (dbdriver.Conn, error) { return panicDDLConn(d), nil }

type panicDDLConnector struct{ rolledBack *atomic.Bool }

func (c panicDDLConnector) Connect(context.Context) (dbdriver.Conn, error) {
	return panicDDLConn(c), nil
}
func (c panicDDLConnector) Driver() dbdriver.Driver { return panicDDLDriver(c) }

type panicDDLConn struct{ rolledBack *atomic.Bool }

func (c panicDDLConn) Prepare(string) (dbdriver.Stmt, error) { return nil, errors.New("not used") }
func (c panicDDLConn) Close() error                          { return nil }
func (c panicDDLConn) Begin() (dbdriver.Tx, error)           { return panicDDLTx(c), nil }
func (c panicDDLConn) ExecContext(context.Context, string, []dbdriver.NamedValue) (dbdriver.Result, error) {
	panic("DDL driver panic")
}

type panicDDLTx struct{ rolledBack *atomic.Bool }

func (t panicDDLTx) Commit() error   { return nil }
func (t panicDDLTx) Rollback() error { t.rolledBack.Store(true); return nil }
func TestExecDDLRollsBackOnPanic(t *testing.T) {
	var rollback atomic.Bool
	db := sql.OpenDB(panicDDLConnector{&rollback})
	defer func() { _ = db.Close() }()
	d := &PostgresDriver{db: db}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()
		_ = d.execDDL(context.Background(), []string{"CREATE TABLE t(id int)"})
	}()
	if !rollback.Load() {
		t.Fatal("transaction retained after panic")
	}
}
