package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
)

type databaseOpsDriver struct {
	*stubDriver
	calls []string
	err   error
}

func (d *databaseOpsDriver) CreateDatabase(_ context.Context, n, c string) error {
	d.calls = append(d.calls, "create:"+n+":"+c)
	return d.err
}
func (d *databaseOpsDriver) DropDatabase(_ context.Context, n string) error {
	d.calls = append(d.calls, "drop:"+n)
	return d.err
}
func (d *databaseOpsDriver) RenameDatabase(_ context.Context, s, n string) error {
	d.calls = append(d.calls, "rename:"+s+":"+n)
	return d.err
}
func (d *databaseOpsDriver) MoveTables(_ context.Context, s string, t []string, n string) error {
	d.calls = append(d.calls, "move:"+s+":"+strings.Join(t, ",")+":"+n)
	return d.err
}
func (d *databaseOpsDriver) CopyTables(_ context.Context, s string, t []string, n string) error {
	d.calls = append(d.calls, "copy:"+s+":"+strings.Join(t, ",")+":"+n)
	return d.err
}
func (d *databaseOpsDriver) Metadata(context.Context) (driver.Metadata, error) {
	return driver.Metadata{}, d.err
}

func databaseOpsRequest(d driver.Driver, method, path, body string) *httptest.ResponseRecorder {
	s := &apiServer{}
	r := chi.NewRouter()
	s.registerDatabaseRoutes(r)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req = req.WithContext(session.NewContext(req.Context(), &session.Session{Driver: d}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
func TestDatabaseOperationsHandlers(t *testing.T) {
	d := &databaseOpsDriver{stubDriver: &stubDriver{caps: driver.Caps{MoveCopy: true}}}
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/databases", `{"name":"new db","collation":"utf8mb4_unicode_ci"}`},
		{"PATCH", "/databases/old%20db", `{"name":"next"}`},
		{"DELETE", "/databases/next", ``},
		{"POST", "/databases/source/table-transfer", `{"action":"copy","tables":["a","b"],"targetDB":"dest"}`},
		{"POST", "/databases/source/table-transfer", `{"action":"move","tables":["a"],"targetDB":"dest"}`},
	} {
		w := databaseOpsRequest(d, c.method, c.path, c.body)
		if w.Code != 204 {
			t.Fatalf("%s %s: %d %s", c.method, c.path, w.Code, w.Body.String())
		}
	}
	want := []string{"create:new db:utf8mb4_unicode_ci", "rename:old db:next", "drop:next", "copy:source:a,b:dest", "move:source:a:dest"}
	if !reflect.DeepEqual(d.calls, want) {
		t.Fatalf("calls: %v", d.calls)
	}
	w := databaseOpsRequest(d, "GET", "/metadata", "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"columnTypes", "collations", "operators"} {
		if _, ok := body[key].([]any); !ok {
			t.Fatalf("%s not array: %s", key, w.Body.String())
		}
	}
}
func TestDatabaseOperationsRejectInvalidAndUnsupported(t *testing.T) {
	for _, c := range []struct{ path, body string }{
		{"/databases", `{"name":" "}`}, {"/databases", "{"}, {"/databases", `{"name":"ok"} {}`},
		{"/databases/a/table-transfer", `{"action":"drop","tables":["x"],"targetDB":"b"}`},
		{"/databases/a/table-transfer", `{"action":"move","tables":[],"targetDB":"b"}`},
		{"/databases/a/table-transfer", `{"action":"copy","tables":["x",""],"targetDB":"b"}`},
		{"/databases/a/table-transfer", `{"action":"copy","tables":["x"],"targetDB":"a"}`},
	} {
		d := &databaseOpsDriver{stubDriver: &stubDriver{caps: driver.Caps{MoveCopy: true}}}
		w := databaseOpsRequest(d, "POST", c.path, c.body)
		if w.Code != 400 || len(d.calls) > 0 {
			t.Fatalf("invalid request %s => %d calls %v", c.body, w.Code, d.calls)
		}
	}
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/databases", `{"name":"x"}`}, {"GET", "/metadata", ""},
		{"POST", "/databases/a/table-transfer", `{"action":"copy","tables":["x"],"targetDB":"b"}`},
	} {
		w := databaseOpsRequest(&stubDriver{}, c.method, c.path, c.body)
		if w.Code != 404 || !strings.Contains(w.Body.String(), "driver_not_supported") {
			t.Fatalf("unsupported => %d %s", w.Code, w.Body.String())
		}
	}
	// A type assertion alone cannot enable a capability the driver disabled.
	d := &databaseOpsDriver{stubDriver: &stubDriver{}}
	w := databaseOpsRequest(d, "POST", "/databases/a/table-transfer", `{"action":"copy","tables":["x"],"targetDB":"b"}`)
	if w.Code != 404 || len(d.calls) > 0 {
		t.Fatal("MoveCopy capability ignored")
	}
	d = &databaseOpsDriver{stubDriver: &stubDriver{}, err: errors.New("permission denied")}
	w = databaseOpsRequest(d, "POST", "/databases", `{"name":"x"}`)
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "permission denied") {
		t.Fatalf("driver error: %d %s", w.Code, w.Body.String())
	}
}
