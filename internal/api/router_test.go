package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dblama-dev/dblama/internal/config"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/session"
	"github.com/dblama-dev/dblama/internal/sqlgen"
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
	// Schema and sequence capability configuration and recordings. A nil
	// schemaAware/sequenceMgr models a driver without the capability.
	schemaAware     bool
	gotSchemas      []string
	schemaErr       error
	setSchemaErr    error
	schemaCalls     []string
	sequenceMgr     bool
	sequences       []driver.SequenceInfo
	seqErr          error
	seqCalls        []string
	dropTablesCalls []tablesOpCall
	dropTablesErr   error
	dropViewsCalls  []tablesOpCall
	dropViewsErr    error
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
	// Console: raw SQL execution and cancel.
	execCalls   []execCall
	execResults []driver.Result
	execErr     error
	cancelCalls []string
	cancelErr   error
	// Transfer: dump export and data import.
	exportCalls    []exportCall
	exportErr      error
	exportOutput   string
	importSQLCalls []importSQLCall
	importSQLErr   error
	importCSVCalls []importCSVCall
	importCSVErr   error
	// progSteps are the progress callbacks the fake import methods emit
	// (in order) before returning.
	progSteps []progStep
}

// execCall records one ExecSQL invocation with its parsed options.
type execCall struct {
	db, query, queryID string
	opts               driver.ExecOpts
}

// exportCall records one Export invocation: the request and the bytes the
// fake wrote (or would have written).
type exportCall struct {
	db     string
	req    driver.ExportRequest
	output string
}

// importSQLCall records one ImportSQL invocation: the target database and
// the raw bytes read from the input stream.
type importSQLCall struct {
	db    string
	input string
}

// importCSVCall records one ImportCSV invocation.
type importCSVCall struct {
	db, table, input string
	opts             driver.CSVImportOpts
}

// progStep is one progress(done, total) callback the fake emits.
type progStep struct {
	done, total int64
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

func (d *stubDriver) ListTables(ctx context.Context, db string) ([]driver.TableInfo, error) {
	d.gotListTables = append(d.gotListTables, db)
	if schema := driver.SchemaFromContext(ctx); schema != "" {
		d.schemaCalls = append(d.schemaCalls, schema)
	}
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

func (d *stubDriver) ShowCreateTable(context.Context, string, string) (string, error) {
	return "", nil
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

// emitProg replays the configured progress steps.
func (d *stubDriver) emitProg(progress func(done, total int64)) {
	for _, s := range d.progSteps {
		progress(s.done, s.total)
	}
}

func (d *stubDriver) ExecSQL(_ context.Context, db, query, queryID string, opts driver.ExecOpts) ([]driver.Result, error) {
	d.execCalls = append(d.execCalls, execCall{db, query, queryID, opts})
	if d.execErr != nil {
		return nil, d.execErr
	}
	return d.execResults, nil
}

func (d *stubDriver) Export(_ context.Context, db string, req driver.ExportRequest, w io.Writer) error {
	d.exportCalls = append(d.exportCalls, exportCall{db: db, req: req, output: d.exportOutput})
	if d.exportErr != nil {
		return d.exportErr
	}
	_, err := io.WriteString(w, d.exportOutput)
	return err
}

func (d *stubDriver) ImportSQL(_ context.Context, db string, r io.Reader, progress func(done, total int64)) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	d.importSQLCalls = append(d.importSQLCalls, importSQLCall{db: db, input: string(b)})
	d.emitProg(progress)
	return d.importSQLErr
}

func (d *stubDriver) ImportCSV(_ context.Context, db, table string, r io.Reader, opts driver.CSVImportOpts, progress func(done, total int64)) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	d.importCSVCalls = append(d.importCSVCalls, importCSVCall{db: db, table: table, input: string(b), opts: opts})
	d.emitProg(progress)
	return d.importCSVErr
}

func (d *stubDriver) CancelQuery(_ context.Context, queryID string) error {
	d.cancelCalls = append(d.cancelCalls, queryID)
	return d.cancelErr
}

func (d *stubDriver) LastInsertID(context.Context) (int64, error) {
	return d.lastInsertID, nil
}

func (d *stubDriver) SetSchema(schema string) error {
	if d.setSchemaErr != nil {
		return d.setSchemaErr
	}
	if !d.schemaAware {
		return errCapabilityMissing
	}
	d.schemaCalls = append(d.schemaCalls, schema)
	return nil
}

func (d *stubDriver) CurrentSchema() string {
	if n := len(d.schemaCalls); n > 0 {
		return d.schemaCalls[n-1]
	}
	return "public"
}

func (d *stubDriver) ListSchemas(context.Context) ([]string, error) {
	if !d.schemaAware {
		return nil, errCapabilityMissing
	}
	return d.gotSchemas, d.schemaErr
}

func (d *stubDriver) ListSequences(_ context.Context, schema string) ([]driver.SequenceInfo, error) {
	if !d.sequenceMgr {
		return nil, errCapabilityMissing
	}
	d.seqCalls = append(d.seqCalls, "list:"+schema)
	return d.sequences, d.seqErr
}

func (d *stubDriver) CreateSequence(_ context.Context, name string, _ int64) error {
	if !d.sequenceMgr {
		return errCapabilityMissing
	}
	d.seqCalls = append(d.seqCalls, "create:"+name)
	return d.seqErr
}

func (d *stubDriver) DropSequence(_ context.Context, name string) error {
	if !d.sequenceMgr {
		return errCapabilityMissing
	}
	d.seqCalls = append(d.seqCalls, "drop:"+name)
	return d.seqErr
}

func (d *stubDriver) AlterSequence(_ context.Context, name string, _, _ *int64) error {
	if !d.sequenceMgr {
		return errCapabilityMissing
	}
	d.seqCalls = append(d.seqCalls, "alter:"+name)
	return d.seqErr
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
// dblama_session cookie.
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

// sessionCookie returns the dblama_session cookie set by a login response.
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

// loginSessionDB logs in with database db selected and returns the
// session cookie value — the flow of a user who picked a database on the
// login screen (required by the dump endpoints).
func loginSessionDB(t *testing.T, srv *httptest.Server, db string) string {
	t.Helper()
	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session",
		loginBody("dbhost", "3307", db), "")
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

// --- Console endpoints: query / cancel / explain ---

func TestQueryHandler(t *testing.T) {
	fake := &stubDriver{
		info: driver.DriverInfo{Name: "FakeDB"},
		execResults: []driver.Result{
			{
				Columns: []driver.ColumnMeta{{Name: "1", Type: "int", Nullable: false}},
				Rows:    [][]any{{int64(1)}},
				Seconds: 0.01,
			},
			{AffectedRows: 2, Seconds: 0.02},
		},
	}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	body := `{"sql":"SELECT 1; SELECT 2","maxRows":500,"timeoutSec":30,"queryID":"q-1"}`
	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/query", body, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST query status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var got struct {
		Results []driver.Result `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode query body: %v", err)
	}
	if len(got.Results) != 2 {
		t.Fatalf("results = %+v, want 2 entries", got.Results)
	}
	if got.Results[0].Columns[0].Name != "1" || len(got.Results[0].Rows) != 1 || got.Results[0].Rows[0][0] != float64(1) {
		t.Errorf("result[0] = %+v, want the driver's row result", got.Results[0])
	}
	if got.Results[1].AffectedRows != 2 {
		t.Errorf("result[1].affectedRows = %d, want 2", got.Results[1].AffectedRows)
	}

	if len(fake.execCalls) != 1 {
		t.Fatalf("ExecSQL called %d times, want 1", len(fake.execCalls))
	}
	call := fake.execCalls[0]
	if call.db != "appdb" || call.query != "SELECT 1; SELECT 2" || call.queryID != "q-1" {
		t.Errorf("ExecSQL args = %s/%s/%s, want appdb / raw sql / q-1", call.db, call.query, call.queryID)
	}
	if call.opts.MaxRows != 500 || call.opts.TimeoutSec != 30 {
		t.Errorf("ExecSQL opts = %+v, want MaxRows 500 TimeoutSec 30", call.opts)
	}

	t.Run("empty results marshal as empty array", func(t *testing.T) {
		fake.execResults = nil
		resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/query",
			`{"sql":"SET @x=1","queryID":"q-2"}`, cookie)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST query status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		raw, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(raw), `"results":[]`) {
			t.Errorf("body = %s, want it to contain %q", raw, `"results":[]`)
		}
	})
}

func TestQueryHandler_Validation(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	tests := []struct {
		name, body string
	}{
		{"empty sql", `{"sql":"","queryID":"q-1"}`},
		{"whitespace sql", `{"sql":"   ","queryID":"q-1"}`},
		{"missing queryID", `{"sql":"SELECT 1"}`},
		{"maxRows over the cap", `{"sql":"SELECT 1","queryID":"q-1","maxRows":100001}`},
		{"timeoutSec over the cap", `{"sql":"SELECT 1","queryID":"q-1","timeoutSec":3601}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/query", tc.body, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("POST query %s status = %d, want %d", tc.body, resp.StatusCode, http.StatusBadRequest)
			}
			b, _ := io.ReadAll(resp.Body)
			if code := errCode(t, b); code != "bad_request" {
				t.Errorf("error code = %q, want %q", code, "bad_request")
			}
		})
	}
	if len(fake.execCalls) != 0 {
		t.Errorf("ExecSQL called %d times on invalid bodies, want 0", len(fake.execCalls))
	}

	// The caps themselves are accepted and passed through.
	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/query",
		`{"sql":"SELECT 1","queryID":"q-1","maxRows":100000,"timeoutSec":3600}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST query at the caps status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if call := fake.execCalls[0]; call.opts.MaxRows != 100000 || call.opts.TimeoutSec != 3600 {
		t.Errorf("ExecSQL opts = %+v, want MaxRows 100000 TimeoutSec 3600", call.opts)
	}
}

func TestCancelHandler(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/query/q-9/cancel", "", cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST cancel status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if len(fake.cancelCalls) != 1 || fake.cancelCalls[0] != "q-9" {
		t.Errorf("CancelQuery ids = %v, want [q-9]", fake.cancelCalls)
	}

	// Cancel is idempotent: even a driver-level failure (a query that
	// already finished) still answers 204.
	fake.cancelErr = errors.New("query not found")
	resp = apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/query/q-9/cancel", "", cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("POST cancel with driver error status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestExplainHandler(t *testing.T) {
	fake := &stubDriver{
		info: driver.DriverInfo{Name: "FakeDB"},
		execResults: []driver.Result{
			{Columns: []driver.ColumnMeta{{Name: "id", Type: "select_type"}}, Rows: [][]any{{int64(1), "SIMPLE"}}},
		},
	}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/query",
		`{"db":"appdb","sql":"SELECT * FROM users"}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST explain status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if len(fake.execCalls) != 1 {
		t.Fatalf("ExecSQL called %d times, want 1", len(fake.execCalls))
	}
	call := fake.execCalls[0]
	if call.query != "EXPLAIN SELECT * FROM users" {
		t.Errorf("ExecSQL query = %q, want the EXPLAIN-prefixed statement", call.query)
	}
	if call.db != "appdb" {
		t.Errorf("ExecSQL db = %q, want %q", call.db, "appdb")
	}
	if call.opts.TimeoutSec != 60 {
		t.Errorf("ExecSQL timeoutSec = %d, want 60", call.opts.TimeoutSec)
	}
	if call.queryID == "" {
		t.Error("ExecSQL queryID is empty, want a generated id")
	}
	var got struct {
		Result *driver.Result `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode explain body: %v", err)
	}
	if got.Result == nil || len(got.Result.Rows) != 1 {
		t.Errorf("explain result = %+v, want the driver's plan row", got.Result)
	}

	t.Run("empty sql", func(t *testing.T) {
		resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/query",
			`{"db":"appdb","sql":""}`, cookie)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("POST explain empty sql status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("multi-statement sql is rejected without executing", func(t *testing.T) {
		// A trailing statement must never fire because the user pressed
		// EXPLAIN: ExecSQL would run every statement of the batch.
		fake.execCalls = nil
		resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/query",
			`{"db":"appdb","sql":"SELECT 1; UPDATE t SET x=1"}`, cookie)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("POST explain two statements status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
		b, _ := io.ReadAll(resp.Body)
		if code := errCode(t, b); code != "bad_request" {
			t.Errorf("error code = %q, want %q", code, "bad_request")
		}
		if !strings.Contains(string(b), "explain accepts a single statement") {
			t.Errorf("error body = %s, want the single-statement message", b)
		}
		if len(fake.execCalls) != 0 {
			t.Errorf("ExecSQL called %d times for a multi-statement explain, want 0", len(fake.execCalls))
		}
	})

	t.Run("single statement is trimmed and prefixed", func(t *testing.T) {
		fake.execCalls = nil
		resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/query",
			`{"db":"appdb","sql":"  SELECT * FROM users;  "}`, cookie)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST explain trailing semicolon status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if len(fake.execCalls) != 1 {
			t.Fatalf("ExecSQL called %d times, want 1", len(fake.execCalls))
		}
		if got := fake.execCalls[0].query; got != "EXPLAIN SELECT * FROM users" {
			t.Errorf("ExecSQL query = %q, want the EXPLAIN-prefixed trimmed statement", got)
		}
	})
}

// --- Transfer endpoints: export / import ---

func TestExportHandler_Validation(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSessionDB(t, srv, "appdb")

	tests := []struct {
		name, body string
	}{
		{"unknown format", `{"format":"xlsx"}`},
		{"missing format", `{}`},
		{"csv without table", `{"format":"csv","tables":[]}`},
		{"csv with two tables", `{"format":"csv","tables":["a","b"]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/export", tc.body, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("POST export %s status = %d, want %d", tc.body, resp.StatusCode, http.StatusBadRequest)
			}
			b, _ := io.ReadAll(resp.Body)
			if code := errCode(t, b); code != "bad_request" {
				t.Errorf("error code = %q, want %q", code, "bad_request")
			}
			// Validation happens before any byte is streamed: the error
			// response is the JSON envelope, not an attachment.
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("error Content-Type = %q, want application/json", ct)
			}
		})
	}
	// The driver is never called for an invalid export body.
	if len(fake.exportCalls) != 0 {
		t.Errorf("Export called %d times on invalid bodies, want 0", len(fake.exportCalls))
	}
}

func TestExportHandler_StreamHeaders(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		gzip     bool
		wantCT   string
		wantName string
	}{
		{"sql dump", "sql", false, "application/sql", "dump.sql"},
		{"csv dump", "csv", false, "text/csv", "dump.csv"},
		{"semicolon csv dump", "csv_semicolon", false, "text/csv", "dump.csv"},
		{"tsv dump", "tsv", false, "text/tab-separated-values", "dump.tsv"},
		{"gzipped sql dump", "sql", true, "application/sql", "dump.sql.gz"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}, exportOutput: "DUMP-DATA"}
			h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
			srv := httptest.NewServer(h)
			t.Cleanup(srv.Close)
			cookie := loginSessionDB(t, srv, "appdb")

			body := fmt.Sprintf(`{"format":%q,"gzip":%t,"tables":%s}`, tc.format, tc.gzip, tablesFor(tc.format))
			resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/export", body, cookie)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("POST export status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			if ct := resp.Header.Get("Content-Type"); ct != tc.wantCT {
				t.Errorf("Content-Type = %q, want %q", ct, tc.wantCT)
			}
			wantCD := `attachment; filename="` + tc.wantName + `"`
			if cd := resp.Header.Get("Content-Disposition"); cd != wantCD {
				t.Errorf("Content-Disposition = %q, want %q", cd, wantCD)
			}
			raw, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(raw), "DUMP-DATA") {
				t.Errorf("streamed body = %q, want the fake driver's output", raw)
			}
			if len(fake.exportCalls) != 1 {
				t.Fatalf("Export called %d times, want 1", len(fake.exportCalls))
			}
			if fake.exportCalls[0].req.Format != tc.format || fake.exportCalls[0].req.Gzip != tc.gzip {
				t.Errorf("Export request = %+v, want the decoded body", fake.exportCalls[0].req)
			}
		})
	}
}

// tablesFor returns the Tables JSON for an export body: csv/tsv dumps name
// exactly one table, sql dumps the whole database.
func tablesFor(format string) string {
	if format == "sql" {
		return "[]"
	}
	return `["users"]`
}

func TestExportHandler_DriverErrorBeforeBytes(t *testing.T) {
	// An export rejected during preflight must be a JSON error rather than
	// an apparently successful empty attachment.
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}, exportErr: errors.New("dump: boom")}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSessionDB(t, srv, "appdb")

	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/export",
		`{"format":"sql","tables":[]}`, cookie)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("POST export with preflight error status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	if got := resp.Header.Get("Content-Disposition"); got != "" {
		t.Errorf("preflight error has attachment header %q", got)
	}
	if len(fake.exportCalls) != 1 {
		t.Errorf("Export called %d times, want 1", len(fake.exportCalls))
	}
}

type exportPartialFailureDriver struct{ *stubDriver }

func (d *exportPartialFailureDriver) Export(_ context.Context, _ string, _ driver.ExportRequest, out io.Writer) error {
	_, _ = io.WriteString(out, "partial SQL dump")
	return errors.New("source connection lost")
}

func TestExportHandler_MidstreamErrorAbortsDownload(t *testing.T) {
	fake := &exportPartialFailureDriver{stubDriver: &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, func(context.Context, driver.ConnConfig) (driver.Driver, error) { return fake, nil })
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSessionDB(t, srv, "appdb")

	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/export",
		`{"format":"sql","tables":[]}`, cookie)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want 200", resp.StatusCode)
	}
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Fatal("partial export completed without a transport error")
	}
}

// multipartImport sends a POST /api/databases/appdb/import with the given
// form fields (sent before the file part) and the named file.
func multipartImport(t *testing.T, srv *httptest.Server, cookie string, fields map[string]string, fileName, fileContent []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("write field %s: %v", k, err)
		}
	}
	if fileName != nil {
		fw, err := mw.CreateFormFile("file", string(fileName))
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := fw.Write(fileContent); err != nil {
			t.Fatalf("write file content: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/databases/appdb/import", &buf)
	if err != nil {
		t.Fatalf("build import request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: cookie})
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST import: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// ndjsonLines decodes a JSON-lines body into ordered objects.
func ndjsonLines(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("decode ndjson line from %q: %v", raw, err)
		}
		out = append(out, m)
	}
	return out
}

func TestImportHandler_Multipart(t *testing.T) {
	t.Run("sql import streams progress", func(t *testing.T) {
		fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}, progSteps: []progStep{{100, 200}, {200, 200}}}
		h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		cookie := loginSession(t, srv)

		resp := multipartImport(t, srv, cookie,
			map[string]string{"type": "sql"}, []byte("dump.sql"), []byte("INSERT INTO t VALUES (1);"))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST import status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/x-ndjson" {
			t.Errorf("Content-Type = %q, want application/x-ndjson", ct)
		}
		raw, _ := io.ReadAll(resp.Body)
		lines := ndjsonLines(t, raw)
		if len(lines) != 3 {
			t.Fatalf("progress lines = %v, want 3 (2 progress + final ok)", lines)
		}
		if lines[0]["done"] != float64(100) || lines[0]["total"] != float64(200) || lines[0]["ok"] != nil {
			t.Errorf("line[0] = %v, want {done:100,total:200}", lines[0])
		}
		if lines[1]["done"] != float64(200) || lines[1]["total"] != float64(200) {
			t.Errorf("line[1] = %v, want {done:200,total:200}", lines[1])
		}
		if lines[2]["done"] != float64(200) || lines[2]["ok"] != true || lines[2]["error"] != nil {
			t.Errorf("line[2] = %v, want the final {done:200,ok:true}", lines[2])
		}

		if len(fake.importSQLCalls) != 1 {
			t.Fatalf("ImportSQL called %d times, want 1", len(fake.importSQLCalls))
		}
		call := fake.importSQLCalls[0]
		if call.db != "appdb" || call.input != "INSERT INTO t VALUES (1);" {
			t.Errorf("ImportSQL call = %+v, want appdb with the raw file content", call)
		}
	})

	t.Run("sql import error stops the stream", func(t *testing.T) {
		fake := &stubDriver{
			info:         driver.DriverInfo{Name: "FakeDB"},
			progSteps:    []progStep{{120, 200}},
			importSQLErr: errors.New("syntax error at line 3"),
		}
		h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		cookie := loginSession(t, srv)

		resp := multipartImport(t, srv, cookie,
			map[string]string{"type": "sql"}, []byte("dump.sql"), []byte("SELECT 1;"))
		raw, _ := io.ReadAll(resp.Body)
		lines := ndjsonLines(t, raw)
		if len(lines) != 2 {
			t.Fatalf("progress lines = %v, want 2 (progress + error, then stop)", lines)
		}
		if lines[1]["done"] != float64(120) || lines[1]["error"] != "syntax error at line 3" || lines[1]["ok"] != nil {
			t.Errorf("line[1] = %v, want {done:120,error:...} without ok", lines[1])
		}
	})

	t.Run("csv import parses options", func(t *testing.T) {
		fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}, progSteps: []progStep{{2, 2}}}
		h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		cookie := loginSession(t, srv)

		resp := multipartImport(t, srv, cookie,
			map[string]string{"type": "csv", "table": "users", "separator": ";", "enclosure": "\"", "skipHeader": "true"},
			[]byte("data.csv"), []byte("name;age\nada;36\ngrace;29\n"))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST import status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		raw, _ := io.ReadAll(resp.Body)
		lines := ndjsonLines(t, raw)
		if len(lines) != 2 || lines[1]["ok"] != true || lines[1]["done"] != float64(2) {
			t.Errorf("lines = %v, want progress + final {done:2,ok:true}", lines)
		}

		if len(fake.importCSVCalls) != 1 {
			t.Fatalf("ImportCSV called %d times, want 1", len(fake.importCSVCalls))
		}
		call := fake.importCSVCalls[0]
		if call.db != "appdb" || call.table != "users" || !strings.Contains(call.input, "ada;36") {
			t.Errorf("ImportCSV call = %+v, want appdb.users with the raw file content", call)
		}
		want := driver.CSVImportOpts{Separator: ";", Enclosure: "\"", SkipHeader: true}
		if !reflect.DeepEqual(call.opts, want) {
			t.Errorf("ImportCSV opts = %+v, want %+v", call.opts, want)
		}
	})

	t.Run("gzipped sql passes through untouched", func(t *testing.T) {
		fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
		h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		cookie := loginSession(t, srv)

		// The driver decompresses gzip transparently; the API layer must
		// hand over the stream without reading or rewriting it.
		var gz bytes.Buffer
		zw := gzip.NewWriter(&gz)
		if _, err := zw.Write([]byte("INSERT INTO t VALUES (1);")); err != nil {
			t.Fatalf("gzip write: %v", err)
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("gzip close: %v", err)
		}

		resp := multipartImport(t, srv, cookie,
			map[string]string{"type": "sql"}, []byte("dump.sql.gz"), gz.Bytes())
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST import status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if len(fake.importSQLCalls) != 1 {
			t.Fatalf("ImportSQL called %d times, want 1", len(fake.importSQLCalls))
		}
		if got := fake.importSQLCalls[0].input; got != gz.String() {
			t.Errorf("ImportSQL input differs from the uploaded gzip stream")
		}
	})

	t.Run("csv without table", func(t *testing.T) {
		fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
		h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		cookie := loginSession(t, srv)

		resp := multipartImport(t, srv, cookie,
			map[string]string{"type": "csv"}, []byte("data.csv"), []byte("a,b\n"))
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("POST csv import without table status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
		if len(fake.importCSVCalls) != 0 {
			t.Errorf("ImportCSV called %d times, want 0", len(fake.importCSVCalls))
		}
	})

	t.Run("missing file part", func(t *testing.T) {
		fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
		h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		cookie := loginSession(t, srv)

		resp := multipartImport(t, srv, cookie,
			map[string]string{"type": "sql"}, nil, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("POST import without file status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
		if len(fake.importSQLCalls) != 0 {
			t.Errorf("ImportSQL called %d times, want 0", len(fake.importSQLCalls))
		}
	})
}

func TestImportHandler_InvalidType(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "FakeDB"}}
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cookie := loginSession(t, srv)

	for _, typ := range []string{"xml", ""} {
		fields := map[string]string{"type": typ}
		if typ == "" {
			delete(fields, "type")
		}
		resp := multipartImport(t, srv, cookie, fields, []byte("data.xml"), []byte("<a/>"))
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("POST import type=%q status = %d, want %d", typ, resp.StatusCode, http.StatusBadRequest)
		}
		raw, _ := io.ReadAll(resp.Body)
		if code := errCode(t, raw); code != "bad_request" {
			t.Errorf("error code = %q, want %q", code, "bad_request")
		}
	}
	if len(fake.importSQLCalls)+len(fake.importCSVCalls) != 0 {
		t.Errorf("import driver called on an invalid type, want 0 calls")
	}
}
