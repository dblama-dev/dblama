package driver

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

type fakeDriver struct {
	info DriverInfo
}

var _ Driver = (*fakeDriver)(nil)

func (f *fakeDriver) Info() DriverInfo { return f.info }
func (f *fakeDriver) Caps() Caps       { return Caps{} }

func (f *fakeDriver) ListDatabases(_ context.Context) ([]DatabaseInfo, error) {
	return []DatabaseInfo{}, nil
}

func (f *fakeDriver) ListTables(context.Context, string) ([]TableInfo, error) {
	return nil, nil
}

func (f *fakeDriver) TableDef(context.Context, string, string) (TableDef, error) {
	return TableDef{}, nil
}

func (f *fakeDriver) ShowCreateTable(context.Context, string, string) (string, error) {
	return "", nil
}

func (f *fakeDriver) SaveTable(context.Context, string, string, TableDef) error {
	return nil
}

func (f *fakeDriver) SaveIndexes(context.Context, string, string, []Index) error {
	return nil
}

func (f *fakeDriver) TruncateTables(context.Context, string, []string) error {
	return nil
}

func (f *fakeDriver) DropTables(context.Context, string, []string) error {
	return nil
}

func (f *fakeDriver) DropViews(context.Context, string, []string) error {
	return nil
}

func (f *fakeDriver) SelectRows(context.Context, string, string, sqlgen.SelectQuery) (Page, error) {
	return Page{}, nil
}

func (f *fakeDriver) PrimaryKey(context.Context, string, string) ([]string, error) {
	return nil, nil
}

func (f *fakeDriver) InsertRow(context.Context, string, string, RowValues) error {
	return nil
}

func (f *fakeDriver) UpdateRow(context.Context, string, string, PK, RowValues) error {
	return nil
}

func (f *fakeDriver) DeleteRows(context.Context, string, string, []PK) error {
	return nil
}

func (f *fakeDriver) UpsertRows(context.Context, string, string, []RowValues) error {
	return nil
}

func (f *fakeDriver) LastInsertID(context.Context) (int64, error) {
	return 0, nil
}

func (f *fakeDriver) ExecSQL(context.Context, string, string, string, ExecOpts) ([]Result, error) {
	return nil, nil
}

func (f *fakeDriver) Export(context.Context, ExportRequest, io.Writer) error {
	return nil
}

func (f *fakeDriver) ImportSQL(context.Context, string, io.Reader, func(done, total int64)) error {
	return nil
}

func (f *fakeDriver) ImportCSV(context.Context, string, string, io.Reader, CSVImportOpts, func(done, total int64)) error {
	return nil
}

func (f *fakeDriver) CancelQuery(context.Context, string) error {
	return nil
}

func (f *fakeDriver) Close() error { return nil }

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	mariadb := DriverMeta{ID: "mariadb", Name: "MariaDB", DefaultPort: "3306"}
	mysql := DriverMeta{ID: "mysql", Name: "MySQL", DefaultPort: "3306"}
	r.Register(mysql, func(context.Context, ConnConfig) (Driver, error) { return nil, nil })
	r.Register(mariadb, func(context.Context, ConnConfig) (Driver, error) { return nil, nil })

	got := r.List()
	want := []DriverMeta{mariadb, mysql} // sorted by ID
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List() = %v, want %v", got, want)
	}
}

func TestRegistry_Connect_UnknownID(t *testing.T) {
	r := NewRegistry()

	_, err := r.Connect(context.Background(), "postgres", ConnConfig{})
	if err == nil {
		t.Fatal("Connect(unknown id) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown driver") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "unknown driver")
	}
}

func TestRegistry_Connect_Delegates(t *testing.T) {
	r := NewRegistry()
	want := &fakeDriver{info: DriverInfo{Name: "MySQL"}}
	var gotCfg ConnConfig
	r.Register(DriverMeta{ID: "mysql"}, func(_ context.Context, cfg ConnConfig) (Driver, error) {
		gotCfg = cfg
		return want, nil
	})

	inCfg := ConnConfig{Host: "localhost", Port: "3306", User: "root", Password: "secret", Database: "test"}
	got, err := r.Connect(context.Background(), "mysql", inCfg)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if got != want {
		t.Errorf("Connect() returned a different driver instance, want the one from the ConnectFunc")
	}
	if !reflect.DeepEqual(gotCfg, inCfg) {
		t.Errorf("cfg passed to ConnectFunc = %v, want %v", gotCfg, inCfg)
	}
}
