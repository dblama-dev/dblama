package api

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type adminStub struct {
	*stubDriver
	driver.UserManager
	driver.MonitorManager
	account  driver.UserAccount
	password string
	killed   int64
	disabled bool
	grant    *driver.GrantChange
}

func (d *adminStub) ListUsers(context.Context) ([]driver.UserInfo, error) {
	return []driver.UserInfo{{UserAccount: driver.UserAccount{User: "alice", Host: "%"}}}, nil
}
func (d *adminStub) CreateUser(_ context.Context, a driver.UserAccount, p string) error {
	d.account = a
	d.password = p
	return nil
}
func (d *adminStub) SetPassword(_ context.Context, a driver.UserAccount, p string) error {
	d.account = a
	return errors.New("bad password " + p)
}
func (d *adminStub) KillProcess(_ context.Context, id int64) error { d.killed = id; return nil }
func (d *adminStub) Processes(context.Context) ([]driver.ProcessInfo, error) {
	return []driver.ProcessInfo{{ID: 7, User: "alice"}}, nil
}
func adminRequest(t *testing.T, d driver.Driver, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	s := &apiServer{}
	s.registerAdminRoutes(router)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req = req.WithContext(session.NewContext(req.Context(), &session.Session{Driver: d}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
func TestAdminCapabilityAndValidation(t *testing.T) {
	for _, path := range []string{"/users", "/processes", "/variables", "/status"} {
		w := adminRequest(t, &stubDriver{}, "GET", path, "")
		if w.Code != 404 || !strings.Contains(w.Body.String(), "driver_not_supported") {
			t.Fatalf("%s=%d %s", path, w.Code, w.Body.String())
		}
	}
	d := &adminStub{stubDriver: &stubDriver{}}
	w := adminRequest(t, d, "POST", "/users", `{"user":"a'b","host":"%","password":"secret"}`)
	if w.Code != 201 || d.account.User != "a'b" || d.password != "secret" {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	w = adminRequest(t, d, "PUT", "/users/alice/password?host=%25", `{"password":"secret-marker"}`)
	if strings.Contains(w.Body.String(), "secret-marker") || w.Code != 500 {
		t.Fatalf("password leaked: %s", w.Body.String())
	}
	for _, id := range []string{"0", "-1", "1;DROP", "99999999999999999999999"} {
		w = adminRequest(t, d, "POST", "/processes/"+id+"/kill", "")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid id accepted %s: %d", id, w.Code)
		}
	}
	w = adminRequest(t, d, "POST", "/processes/7/kill", "")
	if w.Code != 204 || d.killed != 7 {
		t.Fatalf("kill=%d id=%d", w.Code, d.killed)
	}
	w = adminRequest(t, d, "GET", "/users", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "alice") {
		t.Fatalf("list=%d %s", w.Code, w.Body.String())
	}
}

func (d *adminStub) PrivilegeCatalog() []driver.PrivilegeInfo {
	return []driver.PrivilegeInfo{{Name: "SELECT", Levels: []string{"global", "database", "table"}}}
}
func TestAdminRejectsUnsafeGrantPayloads(t *testing.T) {
	d := &adminStub{stubDriver: &stubDriver{}}
	for _, body := range []string{
		`{"scope":{"level":"global"},"privileges":["SELECT; DROP USER root"]}`,
		`{"scope":{"level":"table","database":"app"},"privileges":["SELECT"]}`,
		`{"scope":{"level":"column","database":"app","table":"t"},"privileges":["SELECT"]}`,
		`{"scope":{"level":"global","database":"app"},"privileges":["SELECT"]}`,
		`{"scope":{"level":"global"},"privileges":[]}`,
	} {
		w := adminRequest(t, d, "POST", "/users/alice/grants?host=%25", body)
		if w.Code != 400 {
			t.Fatalf("unsafe grant accepted: %s %d", body, w.Code)
		}
	}
	for _, body := range []string{`{"queriesPerHour":-1}`, `{"queriesPerHour":4294967296}`} {
		w := adminRequest(t, d, "PUT", "/users/alice/limits?host=%25", body)
		if w.Code != 400 {
			t.Fatalf("invalid limits accepted: %s %d", body, w.Code)
		}
	}
}

func TestAdminDisabledCapability(t *testing.T) {
	d := &adminStub{stubDriver: &stubDriver{}, disabled: true}
	for _, path := range []string{"/users", "/processes"} {
		w := adminRequest(t, d, "GET", path, "")
		if w.Code != 404 || !strings.Contains(w.Body.String(), "driver_not_supported") {
			t.Fatalf("disabled capability accepted: %s %d", path, w.Code)
		}
	}
}

func (d *adminStub) Caps() driver.Caps { return driver.Caps{Users: !d.disabled, Monitor: !d.disabled} }

func (d *adminStub) DropUser(_ context.Context, a driver.UserAccount) error {
	d.account = a
	return nil
}
func (d *adminStub) GrantPrivileges(_ context.Context, _ driver.UserAccount, g driver.GrantChange) error {
	d.grant = &g
	return nil
}
func (d *adminStub) RevokePrivileges(_ context.Context, _ driver.UserAccount, g driver.GrantChange) error {
	d.grant = &g
	return nil
}
func TestAdminAnonymousAndGrantOptionOnly(t *testing.T) {
	d := &adminStub{stubDriver: &stubDriver{}}
	w := adminRequest(t, d, "POST", "/users", `{"user":"","host":"localhost","password":"secret"}`)
	if w.Code != 201 {
		t.Fatalf("anonymous create=%d", w.Code)
	}
	w = adminRequest(t, d, "DELETE", "/users/_anonymous?host=localhost&anonymous=true", "")
	if w.Code != 204 || d.account.User != "" {
		t.Fatalf("anonymous address=%d %#v", w.Code, d.account)
	}
	for _, method := range []string{"POST", "DELETE"} {
		w = adminRequest(t, d, method, "/users/alice/grants?host=%25", `{"scope":{"level":"global"},"privileges":[],"grantOption":true}`)
		if w.Code != 204 || d.grant == nil || !d.grant.GrantOption {
			t.Fatalf("grant-option only=%d", w.Code)
		}
	}
}
