package driver

import (
	"context"
	"reflect"
	"strings"
	"testing"
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
