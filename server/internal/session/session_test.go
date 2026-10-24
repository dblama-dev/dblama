package session

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// fakeDriver is a driver.Driver that records Close calls.
type fakeDriver struct {
	closes atomic.Int64
}

var _ driver.Driver = (*fakeDriver)(nil)

func (f *fakeDriver) Info() driver.DriverInfo { return driver.DriverInfo{} }
func (f *fakeDriver) Caps() driver.Caps       { return driver.Caps{} }
func (f *fakeDriver) ListDatabases(context.Context) ([]driver.DatabaseInfo, error) {
	return nil, nil
}

func (f *fakeDriver) ListTables(context.Context, string) ([]driver.TableInfo, error) {
	return nil, nil
}

func (f *fakeDriver) TableDef(context.Context, string, string) (driver.TableDef, error) {
	return driver.TableDef{}, nil
}

func (f *fakeDriver) ShowCreateTable(context.Context, string, string) (string, error) {
	return "", nil
}

func (f *fakeDriver) SaveTable(context.Context, string, string, driver.TableDef) error {
	return nil
}

func (f *fakeDriver) SaveIndexes(context.Context, string, string, []driver.Index) error {
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

func (f *fakeDriver) SelectRows(context.Context, string, string, sqlgen.SelectQuery) (driver.Page, error) {
	return driver.Page{}, nil
}

func (f *fakeDriver) PrimaryKey(context.Context, string, string) ([]string, error) {
	return nil, nil
}

func (f *fakeDriver) InsertRow(context.Context, string, string, driver.RowValues) error {
	return nil
}

func (f *fakeDriver) UpdateRow(context.Context, string, string, driver.PK, driver.RowValues) error {
	return nil
}

func (f *fakeDriver) DeleteRows(context.Context, string, string, []driver.PK) error {
	return nil
}

func (f *fakeDriver) UpsertRows(context.Context, string, string, []driver.RowValues) error {
	return nil
}

func (f *fakeDriver) ExecSQL(context.Context, string, string, string, driver.ExecOpts) ([]driver.Result, error) {
	return nil, nil
}

func (f *fakeDriver) Export(context.Context, driver.ExportRequest, io.Writer) error {
	return nil
}

func (f *fakeDriver) CancelQuery(context.Context, string) error {
	return nil
}

func (f *fakeDriver) LastInsertID(context.Context) (int64, error) {
	return 0, nil
}

func (f *fakeDriver) Close() error {
	f.closes.Add(1)
	return nil
}

// fakeClock is a controllable time source.
type fakeClock struct {
	mu  sync.Mutex
	cur time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cur
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cur = c.cur.Add(d)
}

func TestStore_CreateGet(t *testing.T) {
	clk := &fakeClock{cur: time.Unix(0, 0)}
	s := NewStore(time.Minute, 1000, clk.Now)

	d := &fakeDriver{}
	sess := s.Create(d, "appdb")

	if len(sess.ID) != 64 {
		t.Errorf("ID length = %d, want 64", len(sess.ID))
	}
	for _, r := range sess.ID {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("ID %q contains non-hex character %q", sess.ID, r)
		}
	}
	if sess.Driver != d {
		t.Error("sess.Driver is not the driver passed to Create")
	}
	if sess.Database != "appdb" {
		t.Errorf("sess.Database = %q, want %q", sess.Database, "appdb")
	}

	got, ok := s.Get(sess.ID)
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got != sess {
		t.Error("Get() returned a different session pointer")
	}
	if got.Database != "appdb" {
		t.Errorf("got.Database = %q, want %q", got.Database, "appdb")
	}
}

func TestStore_Create_EmptyDatabase(t *testing.T) {
	s := NewStore(time.Minute, 1000, time.Now)

	sess := s.Create(&fakeDriver{}, "")
	if sess.Database != "" {
		t.Errorf("sess.Database = %q, want empty", sess.Database)
	}
	if _, ok := s.Get(sess.ID); !ok {
		t.Error("Get() ok = false, want true")
	}
}

func TestStore_Get_Unknown(t *testing.T) {
	s := NewStore(time.Minute, 1000, time.Now)

	if _, ok := s.Get("does-not-exist"); ok {
		t.Error("Get(unknown id) ok = true, want false")
	}
}

func TestStore_TTLExpiry(t *testing.T) {
	clk := &fakeClock{cur: time.Unix(0, 0)}
	ttl := time.Minute
	s := NewStore(ttl, 1000, clk.Now)

	d := &fakeDriver{}
	sess := s.Create(d, "appdb")
	clk.Advance(ttl + time.Second)

	if _, ok := s.Get(sess.ID); ok {
		t.Error("Get(expired id) ok = true, want false")
	}
	if n := d.closes.Load(); n != 1 {
		t.Errorf("driver Close called %d times after expired Get, want 1", n)
	}
}

func TestStore_TTLRenewedByGet(t *testing.T) {
	clk := &fakeClock{cur: time.Unix(0, 0)}
	ttl := time.Minute
	s := NewStore(ttl, 1000, clk.Now)

	d := &fakeDriver{}
	sess := s.Create(d, "appdb")
	clk.Advance(ttl - time.Second)
	if _, ok := s.Get(sess.ID); !ok {
		t.Fatal("Get(before expiry) ok = false, want true")
	}
	clk.Advance(ttl - time.Second) // Get renewed lastUsed, still within ttl
	if _, ok := s.Get(sess.ID); !ok {
		t.Error("Get(after renewal) ok = false, want true")
	}
	if n := d.closes.Load(); n != 0 {
		t.Errorf("driver Close called %d times, want 0", n)
	}
}

func TestStore_Delete_ClosesDriver(t *testing.T) {
	s := NewStore(time.Minute, 1000, time.Now)

	d := &fakeDriver{}
	sess := s.Create(d, "appdb")

	s.Delete(sess.ID)
	if n := d.closes.Load(); n != 1 {
		t.Errorf("driver Close called %d times after Delete, want 1", n)
	}
	if _, ok := s.Get(sess.ID); ok {
		t.Error("Get(after Delete) ok = true, want false")
	}

	s.Delete(sess.ID) // deleting again must be a no-op
	if n := d.closes.Load(); n != 1 {
		t.Errorf("driver Close called %d times after second Delete, want 1", n)
	}
}

func TestStore_MaxSessions_EvictsOldestIdle(t *testing.T) {
	clk := &fakeClock{cur: time.Unix(0, 0)}
	s := NewStore(time.Hour, 2, clk.Now)

	d1 := &fakeDriver{}
	s1 := s.Create(d1, "db1")
	clk.Advance(time.Second)
	s2 := s.Create(&fakeDriver{}, "db2")
	clk.Advance(time.Second)
	s3 := s.Create(&fakeDriver{}, "db3")

	if n := d1.closes.Load(); n != 1 {
		t.Errorf("oldest session driver Close called %d times, want 1", n)
	}
	if _, ok := s.Get(s1.ID); ok {
		t.Error("Get(evicted id) ok = true, want false")
	}
	if _, ok := s.Get(s2.ID); !ok {
		t.Error("Get(second session) ok = false, want true")
	}
	if _, ok := s.Get(s3.ID); !ok {
		t.Error("Get(third session) ok = false, want true")
	}
}

func TestJanitor_Evicts(t *testing.T) {
	clk := &fakeClock{cur: time.Unix(0, 0)}
	ttl := time.Minute
	s := NewStore(ttl, 1000, clk.Now)

	d := &fakeDriver{}
	sess := s.Create(d, "appdb")
	clk.Advance(ttl + time.Second)

	stop := s.StartJanitor(time.Millisecond)
	defer stop()

	deadline := time.Now().Add(2 * time.Second)
	for d.closes.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if n := d.closes.Load(); n != 1 {
		t.Fatalf("driver Close called %d times by janitor, want 1", n)
	}
	if _, ok := s.Get(sess.ID); ok {
		t.Error("Get(after janitor eviction) ok = true, want false")
	}

	stop() // second call must be safe (no panic, no double-sweep issues)
}

func TestJanitor_NonPositiveIntervalDoesNotPanic(t *testing.T) {
	// interval <= 0 must not reach time.NewTicker (which panics); the
	// janitor is skipped and stop stays safe to call.
	s := NewStore(time.Minute, 100, time.Now)

	stop := s.StartJanitor(0)
	stop()
	stop() // still safe

	// A broken implementation would spawn the janitor goroutine and panic
	// there, killing the process; this pause gives such a goroutine time
	// to be scheduled so the panic surfaces instead of racing test exit.
	time.Sleep(50 * time.Millisecond)

	// With the janitor skipped the store must remain fully usable.
	d := &fakeDriver{}
	sess := s.Create(d, "appdb")
	if got, ok := s.Get(sess.ID); !ok || got != sess {
		t.Errorf("Get after Create ok = %v, want true with the same session", ok)
	}
}

func TestSessionContext_RoundTrip(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Error("FromContext(empty ctx) ok = true, want false")
	}

	sess := &Session{ID: "abc", Database: "appdb"}
	ctx := NewContext(context.Background(), sess)

	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext(ctx with session) ok = false, want true")
	}
	if got != sess {
		t.Error("FromContext returned a different session pointer")
	}
}
