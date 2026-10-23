package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitslab/db-manager/server/internal/config"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
)

// stubDriver is a configurable driver.Driver for router tests. Unlike the
// bare fakeDriver in middleware_test.go it reports fixed info, caps and
// databases and counts Close calls.
type stubDriver struct {
	info      driver.DriverInfo
	caps      driver.Caps
	databases []driver.DatabaseInfo
	listErr   error
	closed    int
}

var _ driver.Driver = (*stubDriver)(nil)

func (d *stubDriver) Info() driver.DriverInfo { return d.info }
func (d *stubDriver) Caps() driver.Caps       { return d.caps }

func (d *stubDriver) ListDatabases(context.Context) ([]driver.DatabaseInfo, error) {
	if d.listErr != nil {
		return nil, d.listErr
	}
	return d.databases, nil
}

func (d *stubDriver) ListTables(context.Context, string) ([]driver.TableInfo, error) {
	return nil, nil
}

func (d *stubDriver) TableDef(context.Context, string, string) (driver.TableDef, error) {
	return driver.TableDef{}, nil
}

func (d *stubDriver) Close() error {
	d.closed++
	return nil
}

// testRate is the login rate limit used unless a test overrides it: 10
// failures per minute, matching the config default.
var testRate = config.Rate{Count: 10, Per: time.Minute}

// newTestRouter builds a router over a registry with the single fake
// driver "fakedb" (default port 3307). connect is the registered
// ConnectFunc; every received ConnConfig is appended to *got (when
// non-nil).
func newTestRouter(t *testing.T, cfg config.Config, connect driver.ConnectFunc) http.Handler {
	t.Helper()
	reg := driver.NewRegistry()
	reg.Register(driver.DriverMeta{ID: "fakedb", Name: "FakeDB", DefaultPort: "3307"}, connect)
	store := session.NewStore(time.Minute, 100, time.Now)
	return NewRouter(cfg, store, reg, nil)
}

// stubConnectOK returns a ConnectFunc that records cfg into got and
// returns d.
func stubConnectOK(got *[]driver.ConnConfig, d *stubDriver) driver.ConnectFunc {
	return func(_ context.Context, cfg driver.ConnConfig) (driver.Driver, error) {
		if got != nil {
			*got = append(*got, cfg)
		}
		return d, nil
	}
}

// stubConnectFail returns a ConnectFunc that records cfg into got and
// fails with err.
func stubConnectFail(got *[]driver.ConnConfig, err error) driver.ConnectFunc {
	return func(_ context.Context, cfg driver.ConnConfig) (driver.Driver, error) {
		if got != nil {
			*got = append(*got, cfg)
		}
		return nil, err
	}
}

// apiRequest sends a request to the router-mounted httptest server.
// Mutating methods always carry the X-Requested-With CSRF header, as the
// real frontend does; cookieValue, when non-empty, is sent as the
// dbm_session cookie.
func apiRequest(t *testing.T, client *http.Client, method, url, body, cookieValue string) *http.Response {
	t.Helper()

	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
	}
	if cookieValue != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: cookieValue})
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// sessionCookie returns the dbm_session cookie set by a login response.
func sessionCookie(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == SessionCookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie in response", SessionCookieName)
	return nil
}

// testLoginPassword is the password every loginBody sends. The
// wrong-password handler test asserts it never leaks into an error
// response.
const testLoginPassword = "secret"

// loginBody marshals a login request body; the database key is omitted
// when database is empty (an omitted key and an empty string are
// equivalent on the server side).
func loginBody(host, port, database string) string {
	m := map[string]any{
		"driver":   "fakedb",
		"host":     host,
		"port":     port,
		"user":     "root",
		"password": testLoginPassword,
	}
	if database != "" {
		m["database"] = database
	}
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestLoginHandler_Success(t *testing.T) {
	var gotCfg []driver.ConnConfig
	fake := &stubDriver{
		info: driver.DriverInfo{Name: "FakeDB", Flavor: "fake", Version: "1.2.3", User: "root", Host: "dbhost"},
		caps: driver.Caps{Views: true, Upsert: true},
	}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(&gotCfg, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// Login with an empty port: the registry default must be substituted.
	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session",
		loginBody("dbhost", "", "appdb"), "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	cookie := sessionCookie(t, resp)
	if cookie.Value == "" {
		t.Fatal("session cookie value is empty")
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" {
		t.Errorf("cookie attributes = HttpOnly:%v Secure:%v Path:%q, want all true/true,\"/\"",
			cookie.HttpOnly, cookie.Secure, cookie.Path)
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie SameSite = %v, want Strict", cookie.SameSite)
	}

	if len(gotCfg) != 1 {
		t.Fatalf("Connect called %d times, want 1", len(gotCfg))
	}
	wantCfg := driver.ConnConfig{Host: "dbhost", Port: "3307", User: "root", Password: "secret", Database: "appdb"}
	if gotCfg[0] != wantCfg {
		t.Errorf("Connect cfg = %+v, want %+v", gotCfg[0], wantCfg)
	}

	// GET /api/session returns the fake driver's info, caps and database.
	resp = apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/session", "", cookie.Value)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET session status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var info struct {
		Driver   driver.DriverInfo `json:"driver"`
		Caps     driver.Caps       `json:"caps"`
		Database *string           `json:"database"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decode session body: %v", err)
	}
	if info.Driver != fake.info {
		t.Errorf("session driver = %+v, want %+v", info.Driver, fake.info)
	}
	if info.Caps != fake.caps {
		t.Errorf("session caps = %+v, want %+v", info.Caps, fake.caps)
	}
	if info.Database == nil || *info.Database != "appdb" {
		t.Errorf("session database = %v, want \"appdb\"", info.Database)
	}

	// DELETE /api/session logs out and closes the driver.
	resp = apiRequest(t, srv.Client(), http.MethodDelete, srv.URL+"/api/session", "", cookie.Value)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE session status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if fake.closed != 1 {
		t.Errorf("driver Close calls after logout = %d, want 1", fake.closed)
	}

	// The deleted session no longer authenticates.
	resp = apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/session", "", cookie.Value)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET session after logout status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	body, _ := io.ReadAll(resp.Body)
	if code := errCode(t, body); code != "auth_required" {
		t.Errorf("error code after logout = %q, want %q", code, "auth_required")
	}
}

func TestGetSession_DatabaseOmittedIsNull(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session",
		loginBody("dbhost", "3307", ""), "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	cookie := sessionCookie(t, resp).Value

	resp = apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/session", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET session status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	raw, _ := io.ReadAll(resp.Body)
	var info struct {
		Database *string `json:"database"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatalf("decode session body %q: %v", raw, err)
	}
	if info.Database != nil {
		t.Errorf("session database = %v, want null (raw: %s)", *info.Database, raw)
	}
}

func TestLoginHandler_UnknownDriver(t *testing.T) {
	var gotCfg []driver.ConnConfig
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(&gotCfg, &stubDriver{}))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session",
		`{"driver":"nope","host":"dbhost","port":"3307","user":"root","password":"secret"}`, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	body, _ := io.ReadAll(resp.Body)
	if code := errCode(t, body); code != "bad_request" {
		t.Errorf("error code = %q, want %q", code, "bad_request")
	}
	if len(gotCfg) != 0 {
		t.Errorf("Connect called %d times for unknown driver, want 0", len(gotCfg))
	}
}

func TestLoginHandler_MalformedBody(t *testing.T) {
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, &stubDriver{}))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session", `{not json`, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	body, _ := io.ReadAll(resp.Body)
	if code := errCode(t, body); code != "bad_request" {
		t.Errorf("error code = %q, want %q", code, "bad_request")
	}
}

func TestLoginHandler_OversizedBody(t *testing.T) {
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, &stubDriver{}))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// A valid JSON login body larger than the 1 MiB cap must be rejected
	// as bad_request, not decoded (the padding below inflates the
	// password to well over the limit).
	const padding = 2 << 20
	body := `{"driver":"fakedb","host":"dbhost","port":"3307","user":"root","password":"` +
		strings.Repeat("a", padding) + `"}`

	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session", body, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized login status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	raw, _ := io.ReadAll(resp.Body)
	if code := errCode(t, raw); code != "bad_request" {
		t.Errorf("error code = %q, want %q", code, "bad_request")
	}
}

func TestLoginHandler_WrongPassword(t *testing.T) {
	connectErr := errors.New("connect to dbhost:3307 as root: invalid credentials")
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectFail(nil, connectErr))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session",
		loginBody("dbhost", "3307", ""), "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	body, _ := io.ReadAll(resp.Body)
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("unmarshal body %q: %v", body, err)
	}
	if e.Error.Code != "auth_failed" {
		t.Errorf("error code = %q, want %q", e.Error.Code, "auth_failed")
	}
	if !strings.Contains(e.Error.Message, "invalid credentials") {
		t.Errorf("error message = %q, want it to contain the driver error text", e.Error.Message)
	}
	// Anti-regression: the submitted password (loginBody uses "secret")
	// must never appear in the error response, even wrapped in detail.
	if strings.Contains(string(body), testLoginPassword) {
		t.Errorf("error body %q leaks the submitted password", body)
	}
}

func TestLoginHandler_RateLimited(t *testing.T) {
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate},
		stubConnectFail(nil, errors.New("nope")))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	const attempts = 11
	for i := 1; i <= attempts; i++ {
		resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session",
			loginBody("dbhost", "3307", ""), "")
		body, _ := io.ReadAll(resp.Body)

		if i < attempts {
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("attempt %d status = %d, want %d", i, resp.StatusCode, http.StatusUnauthorized)
			}
			continue
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("attempt %d status = %d, want %d", i, resp.StatusCode, http.StatusTooManyRequests)
		}
		if code := errCode(t, body); code != "rate_limited" {
			t.Fatalf("attempt %d error code = %q, want %q", i, code, "rate_limited")
		}
	}
}

func TestLoginHandler_TrustedServers(t *testing.T) {
	// Entries arrive untrimmed and may be empty (e.g. a trailing comma in
	// TRUSTED_SERVERS); the allowlist comparison must tolerate both.
	cfg := config.Config{
		LoginRateLimit: testRate,
		TrustedServers: []string{"db1:3307", " db2:3307 ", ""},
	}

	tests := []struct {
		name       string
		host, port string
		want       int
	}{
		{"host not in list", "db3", "3307", http.StatusBadRequest},
		{"wrong port for listed host", "db1", "9999", http.StatusBadRequest},
		{"listed host and port", "db1", "3307", http.StatusNoContent},
		{"untrimmed entry with default port", "db2", "", http.StatusNoContent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotCfg []driver.ConnConfig
			h := newTestRouter(t, cfg, stubConnectOK(&gotCfg, &stubDriver{}))
			srv := httptest.NewServer(h)
			t.Cleanup(srv.Close)

			resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session",
				loginBody(tc.host, tc.port, ""), "")
			if resp.StatusCode != tc.want {
				t.Fatalf("login status = %d, want %d", resp.StatusCode, tc.want)
			}
			if tc.want == http.StatusBadRequest {
				body, _ := io.ReadAll(resp.Body)
				if code := errCode(t, body); code != "bad_request" {
					t.Errorf("error code = %q, want %q", code, "bad_request")
				}
			}
		})
	}
}

func TestDriversHandler(t *testing.T) {
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, &stubDriver{}))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// The endpoint is public: no session cookie is sent.
	resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/drivers", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET drivers status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var metas []driver.DriverMeta
	if err := json.NewDecoder(resp.Body).Decode(&metas); err != nil {
		t.Fatalf("decode drivers body: %v", err)
	}
	want := []driver.DriverMeta{{ID: "fakedb", Name: "FakeDB", DefaultPort: "3307"}}
	if len(metas) != 1 || metas[0] != want[0] {
		t.Errorf("drivers = %+v, want %+v", metas, want)
	}
}

func TestDatabasesHandler(t *testing.T) {
	fake := &stubDriver{
		info: driver.DriverInfo{Name: "FakeDB"},
		databases: []driver.DatabaseInfo{
			{Name: "information_schema", SizeBytes: 0, Collation: "utf8_general_ci"},
			{Name: "appdb", SizeBytes: 42, Collation: "utf8mb4_0900_ai_ci"},
		},
	}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	t.Run("requires auth", func(t *testing.T) {
		resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases", "", "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("GET databases without session status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})

	t.Run("lists databases", func(t *testing.T) {
		resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session",
			loginBody("dbhost", "3307", ""), "")
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusNoContent)
		}
		cookie := sessionCookie(t, resp).Value

		resp = apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases", "", cookie)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET databases status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		var got struct {
			Databases []driver.DatabaseInfo `json:"databases"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode databases body: %v", err)
		}
		if len(got.Databases) != 2 || got.Databases[1] != fake.databases[1] {
			t.Errorf("databases = %+v, want %+v", got.Databases, fake.databases)
		}
	})
}

func TestDatabasesHandler_DriverError(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}, listErr: errors.New("query databases: boom")}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session",
		loginBody("dbhost", "3307", ""), "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	cookie := sessionCookie(t, resp).Value

	resp = apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases", "", cookie)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("GET databases status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	body, _ := io.ReadAll(resp.Body)
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("unmarshal body %q: %v", body, err)
	}
	if e.Error.Code != "driver_error" {
		t.Errorf("error code = %q, want %q", e.Error.Code, "driver_error")
	}
	if !strings.Contains(e.Error.Message, "query databases") {
		t.Errorf("error message = %q, want it to contain the driver error text", e.Error.Message)
	}
}

func TestRouter_SecurityHeaders(t *testing.T) {
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, &stubDriver{}))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// The headers must be present on /api responses and on the root
	// fallback (404), since SecurityHeaders is the outermost middleware.
	for url, wantStatus := range map[string]int{
		srv.URL + "/api/drivers": http.StatusOK,
		srv.URL + "/api/no/such": http.StatusNotFound,
		srv.URL + "/outside/api": http.StatusNotFound,
	} {
		resp := apiRequest(t, srv.Client(), http.MethodGet, url, "", "")
		if resp.StatusCode != wantStatus {
			t.Fatalf("GET %s status = %d, want %d", url, resp.StatusCode, wantStatus)
		}
		for header, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "no-referrer",
		} {
			if got := resp.Header.Get(header); got != want {
				t.Errorf("GET %s: header %q = %q, want %q", url, header, got, want)
			}
		}
	}
}

func TestRouter_MethodNotAllowed(t *testing.T) {
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, &stubDriver{}))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// PUT on a known /api path must answer with the JSON envelope, not
	// chi's default plain-text 405.
	resp := apiRequest(t, srv.Client(), http.MethodPut, srv.URL+"/api/session", "{}", "")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("PUT /api/session status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if code := errCode(t, body); code != "bad_request" {
		t.Errorf("error code = %q, want %q", code, "bad_request")
	}
}

func TestRouter_UnknownAPIPath(t *testing.T) {
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, &stubDriver{}))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/no/such/path", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET unknown api path status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if code := errCode(t, body); code != "not_found" {
		t.Errorf("error code = %q, want %q", code, "not_found")
	}
}
