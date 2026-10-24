package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitslab/db-manager/server/internal/config"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// stubDriver is a configurable driver.Driver for router tests. Unlike the
// bare fakeDriver in middleware_test.go it reports fixed info, caps and
// databases, counts Close calls and records the calls of every tables and
// rows method together with its arguments.
type stubDriver struct {
	info      driver.DriverInfo
	caps      driver.Caps
	databases []driver.DatabaseInfo
	listErr   error
	closed    int

	// ListTables / TableDef configuration and recordings.
	tables         []driver.TableInfo
	tablesErr      error
	gotListTables  []string
	tableDef       driver.TableDef
	tableDefErr    error
	gotTableDef    []identCall
	saveTableCalls []saveTableCall
	saveTableErr   error
	// Indexes, truncate, drop.
	saveIndexesCalls []saveIndexesCall
	saveIndexesErr   error
	truncateCalls    []tablesOpCall
	truncateErr      error
	dropTablesCalls  []tablesOpCall
	dropTablesErr    error
	dropViewsCalls   []tablesOpCall
	dropViewsErr     error
	// Row browsing and row writes.
	selectCalls  []selectCall
	selectPage   driver.Page
	selectErr    error
	pkCalls      []identCall
	pkColumns    []string
	pkErr        error
	insertCalls  []insertRowCall
	insertErr    error
	lastInsertID int64
	updateCalls  []updateRowCall
	updateErr    error
	deleteCalls  []deleteRowsCall
	deleteErr    error
}

// identCall records a driver call addressed by (db, table).
type identCall struct {
	db, table string
}

// saveTableCall records one SaveTable invocation.
type saveTableCall struct {
	db, table string
	desired   driver.TableDef
}

// saveIndexesCall records one SaveIndexes invocation.
type saveIndexesCall struct {
	db, table string
	indexes   []driver.Index
}

// tablesOpCall records one multi-table operation (truncate / drop).
type tablesOpCall struct {
	db     string
	tables []string
}

// selectCall records one SelectRows invocation with its parsed query.
type selectCall struct {
	db, table string
	q         sqlgen.SelectQuery
}

// insertRowCall records one InsertRow invocation.
type insertRowCall struct {
	db, table string
	vals      driver.RowValues
}

// updateRowCall records one UpdateRow invocation.
type updateRowCall struct {
	db, table string
	where     driver.PK
	vals      driver.RowValues
}

// deleteRowsCall records one DeleteRows invocation.
type deleteRowsCall struct {
	db, table string
	where     []driver.PK
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

func (d *stubDriver) ListTables(_ context.Context, db string) ([]driver.TableInfo, error) {
	d.gotListTables = append(d.gotListTables, db)
	if d.tablesErr != nil {
		return nil, d.tablesErr
	}
	return d.tables, nil
}

func (d *stubDriver) TableDef(_ context.Context, db, table string) (driver.TableDef, error) {
	d.gotTableDef = append(d.gotTableDef, identCall{db, table})
	if d.tableDefErr != nil {
		return driver.TableDef{}, d.tableDefErr
	}
	return d.tableDef, nil
}

func (d *stubDriver) SaveTable(_ context.Context, db, table string, desired driver.TableDef) error {
	d.saveTableCalls = append(d.saveTableCalls, saveTableCall{db, table, desired})
	return d.saveTableErr
}

func (d *stubDriver) SaveIndexes(_ context.Context, db, table string, indexes []driver.Index) error {
	d.saveIndexesCalls = append(d.saveIndexesCalls, saveIndexesCall{db, table, indexes})
	return d.saveIndexesErr
}

func (d *stubDriver) TruncateTables(_ context.Context, db string, tables []string) error {
	d.truncateCalls = append(d.truncateCalls, tablesOpCall{db, tables})
	return d.truncateErr
}

func (d *stubDriver) DropTables(_ context.Context, db string, tables []string) error {
	d.dropTablesCalls = append(d.dropTablesCalls, tablesOpCall{db, tables})
	return d.dropTablesErr
}

func (d *stubDriver) DropViews(_ context.Context, db string, views []string) error {
	d.dropViewsCalls = append(d.dropViewsCalls, tablesOpCall{db, views})
	return d.dropViewsErr
}

func (d *stubDriver) SelectRows(_ context.Context, db, table string, q sqlgen.SelectQuery) (driver.Page, error) {
	d.selectCalls = append(d.selectCalls, selectCall{db, table, q})
	if d.selectErr != nil {
		return driver.Page{}, d.selectErr
	}
	return d.selectPage, nil
}

func (d *stubDriver) PrimaryKey(_ context.Context, db, table string) ([]string, error) {
	d.pkCalls = append(d.pkCalls, identCall{db, table})
	if d.pkErr != nil {
		return nil, d.pkErr
	}
	return d.pkColumns, nil
}

func (d *stubDriver) InsertRow(_ context.Context, db, table string, vals driver.RowValues) error {
	d.insertCalls = append(d.insertCalls, insertRowCall{db, table, vals})
	return d.insertErr
}

func (d *stubDriver) UpdateRow(_ context.Context, db, table string, where driver.PK, vals driver.RowValues) error {
	d.updateCalls = append(d.updateCalls, updateRowCall{db, table, where, vals})
	return d.updateErr
}

func (d *stubDriver) DeleteRows(_ context.Context, db, table string, where []driver.PK) error {
	d.deleteCalls = append(d.deleteCalls, deleteRowsCall{db, table, where})
	return d.deleteErr
}

func (d *stubDriver) UpsertRows(context.Context, string, string, []driver.RowValues) error {
	return nil
}

func (d *stubDriver) ExecSQL(context.Context, string, string, string, driver.ExecOpts) ([]driver.Result, error) {
	return nil, nil
}

func (d *stubDriver) CancelQuery(context.Context, string) error {
	return nil
}

func (d *stubDriver) LastInsertID(context.Context) (int64, error) {
	return d.lastInsertID, nil
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

// loginSession performs a login against srv and returns the session
// cookie value for authenticated follow-up requests.
func loginSession(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session",
		loginBody("dbhost", "3307", ""), "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	return sessionCookie(t, resp).Value
}

func TestTablesListHandler(t *testing.T) {
	t.Run("requires auth", func(t *testing.T) {
		h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, &stubDriver{}))
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)

		resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases/appdb/tables", "", "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("GET tables without session status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})

	t.Run("lists tables and passes db", func(t *testing.T) {
		fake := &stubDriver{
			info: driver.DriverInfo{Name: "FakeDB"},
			tables: []driver.TableInfo{
				{Name: "users", Rows: 3, Engine: "InnoDB"},
				{Name: "v_users", IsView: true},
			},
		}
		h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		cookie := loginSession(t, srv)

		resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases/appdb/tables", "", cookie)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET tables status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		var got struct {
			Tables []driver.TableInfo `json:"tables"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode tables body: %v", err)
		}
		if len(got.Tables) != 2 || got.Tables[1] != fake.tables[1] {
			t.Errorf("tables = %+v, want %+v", got.Tables, fake.tables)
		}
		if len(fake.gotListTables) != 1 || fake.gotListTables[0] != "appdb" {
			t.Errorf("ListTables dbs = %v, want [appdb]", fake.gotListTables)
		}
	})

	t.Run("empty list marshals as empty array", func(t *testing.T) {
		h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, &stubDriver{}))
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		cookie := loginSession(t, srv)

		resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases/appdb/tables", "", cookie)
		raw, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(raw), `"tables":[]`) {
			t.Errorf("body = %s, want it to contain %q", raw, `"tables":[]`)
		}
	})

	t.Run("driver error", func(t *testing.T) {
		fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}, tablesErr: errors.New("list tables: boom")}
		h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		cookie := loginSession(t, srv)

		resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases/appdb/tables", "", cookie)
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("GET tables status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
		}
		body, _ := io.ReadAll(resp.Body)
		if code := errCode(t, body); code != "driver_error" {
			t.Errorf("error code = %q, want %q", code, "driver_error")
		}
	})
}

func TestTableDefHandler(t *testing.T) {
	wantDef := driver.TableDef{
		NewName: "users",
		Columns: []driver.ColumnDef{{Name: "id", Type: "int", Nullable: false}},
	}
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}, tableDef: wantDef}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases/appdb/tables/users", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET table status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var def driver.TableDef
	if err := json.NewDecoder(resp.Body).Decode(&def); err != nil {
		t.Fatalf("decode table body: %v", err)
	}
	if len(def.Columns) != 1 || def.Columns[0].Name != "id" || def.Columns[0].Type != "int" || def.Columns[0].Nullable {
		t.Errorf("table def = %+v, want %+v", def, wantDef)
	}
	if len(fake.gotTableDef) != 1 || fake.gotTableDef[0] != (identCall{"appdb", "users"}) {
		t.Errorf("TableDef calls = %+v, want one for appdb.users", fake.gotTableDef)
	}
}

func TestSaveTableHandler_New(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	body := `{"newName":"users","columns":[{"name":"id","type":"int","nullable":false}]}`
	resp := apiRequest(t, srv.Client(), http.MethodPut, srv.URL+"/api/databases/appdb/tables/_new", body, cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT _new status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if len(fake.saveTableCalls) != 1 {
		t.Fatalf("SaveTable called %d times, want 1", len(fake.saveTableCalls))
	}
	call := fake.saveTableCalls[0]
	if call.db != "appdb" {
		t.Errorf("SaveTable db = %q, want %q", call.db, "appdb")
	}
	if call.table != "" {
		t.Errorf("SaveTable table = %q, want empty for a create", call.table)
	}
	if call.desired.NewName != "users" || len(call.desired.Columns) != 1 || call.desired.Columns[0].Name != "id" {
		t.Errorf("SaveTable desired = %+v, want the decoded body", call.desired)
	}
}

func TestSaveTableHandler_Existing(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	// An alter of the existing "users" table, optionally renaming it.
	body := `{"newName":"people","columns":[{"name":"id","type":"int","nullable":false}]}`
	resp := apiRequest(t, srv.Client(), http.MethodPut, srv.URL+"/api/databases/appdb/tables/users", body, cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT users status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if len(fake.saveTableCalls) != 1 {
		t.Fatalf("SaveTable called %d times, want 1", len(fake.saveTableCalls))
	}
	call := fake.saveTableCalls[0]
	if call.table != "users" {
		t.Errorf("SaveTable table = %q, want %q (the existing name)", call.table, "users")
	}
	if call.desired.NewName != "people" {
		t.Errorf("SaveTable desired.NewName = %q, want %q", call.desired.NewName, "people")
	}

	t.Run("malformed body", func(t *testing.T) {
		resp := apiRequest(t, srv.Client(), http.MethodPut, srv.URL+"/api/databases/appdb/tables/users", `{oops`, cookie)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("PUT malformed status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
		body, _ := io.ReadAll(resp.Body)
		if code := errCode(t, body); code != "bad_request" {
			t.Errorf("error code = %q, want %q", code, "bad_request")
		}
	})
}

func TestDropTableHandler(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	// Default: drop a table.
	resp := apiRequest(t, srv.Client(), http.MethodDelete, srv.URL+"/api/databases/appdb/tables/users", "", cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE users status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if len(fake.dropTablesCalls) != 1 || fake.dropTablesCalls[0].db != "appdb" || len(fake.dropTablesCalls[0].tables) != 1 || fake.dropTablesCalls[0].tables[0] != "users" {
		t.Errorf("DropTables calls = %+v, want one for [users]", fake.dropTablesCalls)
	}
	if len(fake.dropViewsCalls) != 0 {
		t.Errorf("DropViews called %d times without isView, want 0", len(fake.dropViewsCalls))
	}

	// isView=true: drop a view instead.
	resp = apiRequest(t, srv.Client(), http.MethodDelete, srv.URL+"/api/databases/appdb/tables/v_users?isView=true", "", cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE v_users status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if len(fake.dropViewsCalls) != 1 || fake.dropViewsCalls[0].db != "appdb" || len(fake.dropViewsCalls[0].tables) != 1 || fake.dropViewsCalls[0].tables[0] != "v_users" {
		t.Errorf("DropViews calls = %+v, want one for [v_users]", fake.dropViewsCalls)
	}
	if len(fake.dropTablesCalls) != 1 {
		t.Errorf("DropTables called again with isView=true, want still 1 call, got %d", len(fake.dropTablesCalls))
	}
}

func TestTruncateHandler(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/tables/users/truncate", "", cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST truncate status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if len(fake.truncateCalls) != 1 || fake.truncateCalls[0].db != "appdb" || len(fake.truncateCalls[0].tables) != 1 || fake.truncateCalls[0].tables[0] != "users" {
		t.Errorf("TruncateTables calls = %+v, want one for [users]", fake.truncateCalls)
	}
}

func TestSaveIndexesHandler(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	body := `[{"name":"PRIMARY","type":"PRIMARY","columns":[{"name":"id"}]},{"name":"email_idx","type":"UNIQUE","columns":[{"name":"email"}]}]`
	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/tables/users/indexes", body, cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST indexes status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if len(fake.saveIndexesCalls) != 1 {
		t.Fatalf("SaveIndexes called %d times, want 1", len(fake.saveIndexesCalls))
	}
	call := fake.saveIndexesCalls[0]
	if call.db != "appdb" || call.table != "users" {
		t.Errorf("SaveIndexes target = %s.%s, want appdb.users", call.db, call.table)
	}
	if len(call.indexes) != 2 || call.indexes[0].Name != "PRIMARY" || call.indexes[1].Type != "UNIQUE" {
		t.Errorf("SaveIndexes desired = %+v, want the decoded body", call.indexes)
	}

	t.Run("malformed body", func(t *testing.T) {
		resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/tables/users/indexes", `{`, cookie)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("POST malformed indexes status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
		body, _ := io.ReadAll(resp.Body)
		if code := errCode(t, body); code != "bad_request" {
			t.Errorf("error code = %q, want %q", code, "bad_request")
		}
	})
}

func TestRowsHandler_QueryParsing(t *testing.T) {
	fake := &stubDriver{
		info: driver.DriverInfo{Name: "FakeDB"},
		selectPage: driver.Page{
			Columns:   []driver.ColumnMeta{{Name: "id", Type: "int", Nullable: false}},
			Rows:      [][]any{{int64(1)}},
			TotalRows: 1,
		},
	}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	// order plus two filters; the second filter's value contains a colon,
	// which must stay part of the value (split on the first two ':' only).
	url := srv.URL + "/api/databases/appdb/tables/users/rows" +
		"?limit=100&offset=5&order=name:desc" +
		"&filter=age:>=:30" +
		"&filter=created:>:2024-01-01T00:00:00Z"
	resp := apiRequest(t, srv.Client(), http.MethodGet, url, "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET rows status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if len(fake.selectCalls) != 1 {
		t.Fatalf("SelectRows called %d times, want 1", len(fake.selectCalls))
	}
	call := fake.selectCalls[0]
	if call.db != "appdb" || call.table != "users" {
		t.Errorf("SelectRows target = %s.%s, want appdb.users", call.db, call.table)
	}
	wantQ := sqlgen.SelectQuery{
		Limit:  100,
		Offset: 5,
		Sorts:  []sqlgen.Sort{{Column: "name", Desc: true}},
		Filters: []sqlgen.Filter{
			{Column: "age", Operator: ">=", Value: "30"},
			{Column: "created", Operator: ">", Value: "2024-01-01T00:00:00Z"},
		},
	}
	if len(call.q.Sorts) != 1 || call.q.Sorts[0] != wantQ.Sorts[0] {
		t.Errorf("sorts = %+v, want %+v", call.q.Sorts, wantQ.Sorts)
	}
	if len(call.q.Filters) != 2 {
		t.Fatalf("filters = %+v, want %d entries", call.q.Filters, 2)
	}
	for i, want := range wantQ.Filters {
		if call.q.Filters[i] != want {
			t.Errorf("filter[%d] = %+v, want %+v", i, call.q.Filters[i], want)
		}
	}
	if call.q.Limit != 100 || call.q.Offset != 5 {
		t.Errorf("limit/offset = %d/%d, want 100/5", call.q.Limit, call.q.Offset)
	}

	// The page body must round-trip the driver's page.
	var page driver.Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode page body: %v", err)
	}
	if page.TotalRows != 1 || len(page.Rows) != 1 || len(page.Columns) != 1 {
		t.Errorf("page = %+v, want the driver's page", page)
	}

	t.Run("defaults", func(t *testing.T) {
		resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases/appdb/tables/users/rows", "", cookie)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET rows status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		call := fake.selectCalls[len(fake.selectCalls)-1]
		if call.q.Limit != 50 || call.q.Offset != 0 {
			t.Errorf("default limit/offset = %d/%d, want 50/0", call.q.Limit, call.q.Offset)
		}
		if len(call.q.Sorts) != 0 || len(call.q.Filters) != 0 {
			t.Errorf("default sorts/filters = %+v/%+v, want none", call.q.Sorts, call.q.Filters)
		}
	})

	t.Run("nil rows marshal as empty array", func(t *testing.T) {
		fake.selectPage = driver.Page{TotalRows: 0}
		resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases/appdb/tables/users/rows", "", cookie)
		raw, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(raw), `"rows":[]`) {
			t.Errorf("body = %s, want it to contain %q", raw, `"rows":[]`)
		}
	})
}

func TestRowsHandler_BadQueryParams(t *testing.T) {
	tests := []struct {
		name, query string
	}{
		{"unknown operator", "filter=age:!>:30"},
		{"filter missing value part", "filter=age:>="},
		{"filter missing operator part", "filter=age"},
		{"filter with empty column", "filter=:>:30"},
		{"invalid order suffix", "order=name:ascending"},
		{"order without suffix", "order=name"},
		{"limit zero", "limit=0"},
		{"limit negative", "limit=-5"},
		{"limit not a number", "limit=abc"},
		{"offset negative", "offset=-1"},
		{"offset not a number", "offset=xyz"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
			h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
			srv := httptest.NewServer(h)
			t.Cleanup(srv.Close)
			cookie := loginSession(t, srv)

			resp := apiRequest(t, srv.Client(), http.MethodGet,
				srv.URL+"/api/databases/appdb/tables/users/rows?"+tc.query, "", cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("GET rows %s status = %d, want %d", tc.query, resp.StatusCode, http.StatusBadRequest)
			}
			body, _ := io.ReadAll(resp.Body)
			if code := errCode(t, body); code != "bad_request" {
				t.Errorf("error code = %q, want %q", code, "bad_request")
			}
			if len(fake.selectCalls) != 0 {
				t.Errorf("SelectRows called %d times on a bad query, want 0", len(fake.selectCalls))
			}
		})
	}
}

func TestRowsHandler_LimitCap(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases/appdb/tables/users/rows?limit=1001", "", cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET rows limit=1001 status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	body, _ := io.ReadAll(resp.Body)
	if code := errCode(t, body); code != "bad_request" {
		t.Errorf("error code = %q, want %q", code, "bad_request")
	}

	// The cap itself is allowed.
	resp = apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases/appdb/tables/users/rows?limit=1000", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET rows limit=1000 status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if call := fake.selectCalls[len(fake.selectCalls)-1]; call.q.Limit != 1000 {
		t.Errorf("limit = %d, want 1000", call.q.Limit)
	}
}

func TestRowsHandler_InsertReturnsLastInsertId(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}, lastInsertID: 42}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/tables/users/rows",
		`{"name":"ada","age":36}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST rows status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if len(fake.insertCalls) != 1 {
		t.Fatalf("InsertRow called %d times, want 1", len(fake.insertCalls))
	}
	call := fake.insertCalls[0]
	if call.db != "appdb" || call.table != "users" {
		t.Errorf("InsertRow target = %s.%s, want appdb.users", call.db, call.table)
	}
	if call.vals["name"] != "ada" || call.vals["age"] != float64(36) {
		t.Errorf("InsertRow vals = %+v, want the decoded body", call.vals)
	}

	raw, _ := io.ReadAll(resp.Body)
	var got struct {
		LastInsertID int64 `json:"lastInsertId"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal body %q: %v", raw, err)
	}
	if got.LastInsertID != 42 {
		t.Errorf("lastInsertId = %d, want 42", got.LastInsertID)
	}

	t.Run("insert driver error", func(t *testing.T) {
		fake.insertErr = errors.New("insert: boom")
		defer func() { fake.insertErr = nil }()
		resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/tables/users/rows", `{"name":"x"}`, cookie)
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("POST rows status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
		}
		body, _ := io.ReadAll(resp.Body)
		if code := errCode(t, body); code != "driver_error" {
			t.Errorf("error code = %q, want %q", code, "driver_error")
		}
	})
}

func TestRowsHandler_Patch(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	resp := apiRequest(t, srv.Client(), http.MethodPatch, srv.URL+"/api/databases/appdb/tables/users/rows",
		`{"where":{"id":1},"values":{"name":"grace"}}`, cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PATCH rows status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if len(fake.updateCalls) != 1 {
		t.Fatalf("UpdateRow called %d times, want 1", len(fake.updateCalls))
	}
	call := fake.updateCalls[0]
	if call.db != "appdb" || call.table != "users" {
		t.Errorf("UpdateRow target = %s.%s, want appdb.users", call.db, call.table)
	}
	if call.where["id"] != float64(1) || call.vals["name"] != "grace" {
		t.Errorf("UpdateRow where/vals = %+v/%+v, want the decoded body", call.where, call.vals)
	}
}

func TestRowsHandler_Delete(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	resp := apiRequest(t, srv.Client(), http.MethodDelete, srv.URL+"/api/databases/appdb/tables/users/rows",
		`{"where":[{"id":1},{"id":2}]}`, cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE rows status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if len(fake.deleteCalls) != 1 {
		t.Fatalf("DeleteRows called %d times, want 1", len(fake.deleteCalls))
	}
	call := fake.deleteCalls[0]
	if call.db != "appdb" || call.table != "users" {
		t.Errorf("DeleteRows target = %s.%s, want appdb.users", call.db, call.table)
	}
	if len(call.where) != 2 || call.where[0]["id"] != float64(1) || call.where[1]["id"] != float64(2) {
		t.Errorf("DeleteRows where = %+v, want the decoded body", call.where)
	}
}

func TestBulkTablesHandler(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	t.Run("truncate", func(t *testing.T) {
		resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/tables",
			`{"action":"truncate","tables":["a","b"]}`, cookie)
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("POST tables truncate status = %d, want %d", resp.StatusCode, http.StatusNoContent)
		}
		if len(fake.truncateCalls) != 1 || fake.truncateCalls[0].db != "appdb" || len(fake.truncateCalls[0].tables) != 2 {
			t.Errorf("TruncateTables calls = %+v, want one for [a b]", fake.truncateCalls)
		}
	})

	t.Run("drop", func(t *testing.T) {
		resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/tables",
			`{"action":"drop","tables":["a","b"]}`, cookie)
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("POST tables drop status = %d, want %d", resp.StatusCode, http.StatusNoContent)
		}
		if len(fake.dropTablesCalls) != 1 || fake.dropTablesCalls[0].db != "appdb" || len(fake.dropTablesCalls[0].tables) != 2 {
			t.Errorf("DropTables calls = %+v, want one for [a b]", fake.dropTablesCalls)
		}
	})

	t.Run("invalid action", func(t *testing.T) {
		resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/tables",
			`{"action":"rename","tables":["a"]}`, cookie)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("POST tables rename status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
		body, _ := io.ReadAll(resp.Body)
		if code := errCode(t, body); code != "bad_request" {
			t.Errorf("error code = %q, want %q", code, "bad_request")
		}
	})
}

func TestPkHandler(t *testing.T) {
	t.Run("returns key columns", func(t *testing.T) {
		fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}, pkColumns: []string{"id", "tenant"}}
		h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		cookie := loginSession(t, srv)

		resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases/appdb/tables/users/pk", "", cookie)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET pk status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		var got struct {
			Columns []string `json:"columns"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode pk body: %v", err)
		}
		if len(got.Columns) != 2 || got.Columns[0] != "id" || got.Columns[1] != "tenant" {
			t.Errorf("pk columns = %v, want [id tenant]", got.Columns)
		}
		if len(fake.pkCalls) != 1 || fake.pkCalls[0] != (identCall{"appdb", "users"}) {
			t.Errorf("PrimaryKey calls = %+v, want one for appdb.users", fake.pkCalls)
		}
	})

	t.Run("no primary key marshals as empty array", func(t *testing.T) {
		fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}, pkColumns: nil}
		h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		cookie := loginSession(t, srv)

		resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases/appdb/tables/users/pk", "", cookie)
		raw, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(raw), `"columns":[]`) {
			t.Errorf("body = %s, want it to contain %q", raw, `"columns":[]`)
		}
	})
}

func TestRowsHandler_DriverValidationErrMapsTo400(t *testing.T) {
	// A driver error that wraps sqlgen.ErrEmptyIdentifier is a client
	// problem (e.g. an empty db or table name) and maps to bad_request,
	// not to driver_error.
	fake := &stubDriver{
		info:      driver.DriverInfo{Name: "FakeDB"},
		selectErr: fmt.Errorf("build select: %w", sqlgen.ErrEmptyIdentifier),
	}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/databases/appdb/tables/users/rows", "", cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET rows with validation error status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	body, _ := io.ReadAll(resp.Body)
	if code := errCode(t, body); code != "bad_request" {
		t.Errorf("error code = %q, want %q", code, "bad_request")
	}
}
