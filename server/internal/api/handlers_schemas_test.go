package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hitslab/db-manager/server/internal/config"
	"github.com/hitslab/db-manager/server/internal/sqlgen"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// loginStubServer starts a router over the stub driver and logs a client
// in (database appdb); it returns the server, an authorized client and
// the base URL.
func loginStubServer(t *testing.T, fake driver.Driver) (*httptest.Server, *http.Client, string) {
	t.Helper()
	h := newTestRouter(t, config.Config{LoginRateLimit: testRate}, func(_ context.Context, _ driver.ConnConfig) (driver.Driver, error) {
		return fake, nil
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session",
		loginBody("dbhost", "3307", "appdb"), "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login status = %d, want 204", resp.StatusCode)
	}
	cookie := sessionCookie(t, resp)
	client := &http.Client{}
	t.Cleanup(func() { _ = resp.Body.Close() })
	// Wrap the cookie into the client transport so every request carries it.
	client.Transport = cookieTransport{cookie: cookie, base: srv.Client().Transport}
	return srv, client, srv.URL
}

type cookieTransport struct {
	cookie *http.Cookie
	base   http.RoundTripper
}

func (c cookieTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.AddCookie(c.cookie)
	return c.base.RoundTrip(clone)
}

func TestSchemasList_Handler(t *testing.T) {
	fake := &stubDriver{
		info:        driver.DriverInfo{Name: "pg", Flavor: "postgres", Version: "17"},
		schemaAware: true,
		gotSchemas:  []string{"public", "odd schema"},
	}
	_, client, base := loginStubServer(t, fake)

	resp, err := client.Get(base + "/api/schemas")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Schemas []string `json:"schemas"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Schemas) != 2 || body.Schemas[1] != "odd schema" {
		t.Errorf("schemas = %v, want [public odd schema]", body.Schemas)
	}
}

func TestSchemaParam_PassedToDriver(t *testing.T) {
	fake := &stubDriver{
		info:        driver.DriverInfo{Name: "pg", Flavor: "postgres", Version: "17"},
		schemaAware: true,
		tables:      []driver.TableInfo{{Name: "t1"}},
	}
	_, client, base := loginStubServer(t, fake)

	resp, err := client.Get(base + "/api/databases/appdb/tables?schema=odd%20schema")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(fake.schemaCalls) != 1 || fake.schemaCalls[0] != "odd schema" {
		t.Errorf("SetSchema calls = %v, want [odd schema]", fake.schemaCalls)
	}
}

func TestSchemaParam_NonSchemaDriverIgnored(t *testing.T) {
	wrapped := &noSchemaStub{d: &stubDriver{
		info:   driver.DriverInfo{Name: "my", Flavor: "mysql", Version: "8.4"},
		tables: []driver.TableInfo{{Name: "t1"}},
	}}
	_, client, base := loginStubServer(t, wrapped)

	resp, err := client.Get(base + "/api/databases/appdb/tables?schema=whatever")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(wrapped.d.schemaCalls) != 0 {
		t.Errorf("SetSchema called on non-schema driver: %v", wrapped.d.schemaCalls)
	}
}

func TestSequences_CRUD(t *testing.T) {
	fake := &stubDriver{
		info:        driver.DriverInfo{Name: "pg", Flavor: "postgres", Version: "17"},
		sequenceMgr: true,
		sequences:   []driver.SequenceInfo{{Name: "s1", LastValue: 10, Increment: 1}},
	}
	_, client, base := loginStubServer(t, fake)

	resp, err := client.Get(base + "/api/sequences")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var listBody struct {
		Sequences []driver.SequenceInfo `json:"sequences"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	_ = resp.Body.Close()
	if len(listBody.Sequences) != 1 || listBody.Sequences[0].Name != "s1" {
		t.Errorf("sequences = %+v", listBody.Sequences)
	}
	if len(fake.seqCalls) != 1 || fake.seqCalls[0] != "list:public" {
		t.Errorf("seqCalls = %v, want [list:public]", fake.seqCalls)
	}

	resp2, err := client.Get(base + "/api/sequences?schema=odd")
	if err != nil {
		t.Fatalf("list schema: %v", err)
	}
	_ = resp2.Body.Close()
	if len(fake.seqCalls) != 2 || fake.seqCalls[1] != "list:odd" {
		t.Errorf("seqCalls = %v, want [... list:odd]", fake.seqCalls)
	}

	resp3 := apiRequest(t, client, http.MethodPost, base+"/api/sequences",
		`{"name":"s2","increment":5}`, "")
	if resp3.StatusCode != http.StatusCreated {
		t.Errorf("create status = %d, want 201", resp3.StatusCode)
	}
	_ = resp3.Body.Close()

	resp4 := apiRequest(t, client, http.MethodPatch, base+"/api/sequences/s1",
		`{"restart":100}`, "")
	if resp4.StatusCode != http.StatusNoContent {
		t.Errorf("alter status = %d, want 204", resp4.StatusCode)
	}
	_ = resp4.Body.Close()

	resp5 := apiRequest(t, client, http.MethodDelete, base+"/api/sequences/s2", "", "")
	if resp5.StatusCode != http.StatusNoContent {
		t.Errorf("drop status = %d, want 204", resp5.StatusCode)
	}
	_ = resp5.Body.Close()

	want := []string{"list:public", "list:odd", "create:s2", "alter:s1", "drop:s2"}
	if len(fake.seqCalls) != len(want) {
		t.Fatalf("seqCalls = %v, want %v", fake.seqCalls, want)
	}
	for i, w := range want {
		if fake.seqCalls[i] != w {
			t.Errorf("seqCalls[%d] = %q, want %q", i, fake.seqCalls[i], w)
		}
	}
}

func TestSequences_CreateValidation(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "pg", Flavor: "postgres"}, sequenceMgr: true}
	_, client, base := loginStubServer(t, fake)

	resp := apiRequest(t, client, http.MethodPost, base+"/api/sequences", `{"name":""}`, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty name status = %d, want 400", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestSequences_NotSupported(t *testing.T) {
	wrapped := &noSchemaStub{d: &stubDriver{
		info:   driver.DriverInfo{Name: "my", Flavor: "mysql", Version: "8.4"},
		tables: []driver.TableInfo{{Name: "t1"}},
	}}
	_, client, base := loginStubServer(t, wrapped)

	resp, err := client.Get(base + "/api/sequences")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	var body struct {
		Error struct{ Code string }
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "driver_not_supported" {
		t.Errorf("code = %q, want driver_not_supported", body.Error.Code)
	}
}

// errCapabilityMissing models a driver without the capability: the
// non-capable stub wrappers below make the handler type-assertions fail
// the way real MySQL drivers do.
var errCapabilityMissing = errors.New("capability missing")

// noSchemaStub wraps a stubDriver WITHOUT the SchemaAware/SequenceManager
// methods, so `driver.(driver.SchemaAware)` fails on it exactly like a
// MySQL driver in production.
type noSchemaStub struct{ d *stubDriver }

func (n noSchemaStub) Info() driver.DriverInfo                             { return n.d.Info() }
func (n noSchemaStub) Caps() driver.Caps                                   { return n.d.Caps() }
func (n noSchemaStub) ListDatabases(c context.Context) ([]driver.DatabaseInfo, error) {
	return n.d.ListDatabases(c)
}
func (n noSchemaStub) ListTables(c context.Context, db string) ([]driver.TableInfo, error) {
	return n.d.ListTables(c, db)
}
func (n noSchemaStub) TableDef(c context.Context, db, t string) (driver.TableDef, error) {
	return n.d.TableDef(c, db, t)
}
func (n noSchemaStub) ShowCreateTable(c context.Context, db, t string) (string, error) {
	return n.d.ShowCreateTable(c, db, t)
}
func (n noSchemaStub) SaveTable(c context.Context, db, t string, def driver.TableDef) error {
	return n.d.SaveTable(c, db, t, def)
}
func (n noSchemaStub) SaveIndexes(c context.Context, db, t string, i []driver.Index) error {
	return n.d.SaveIndexes(c, db, t, i)
}
func (n noSchemaStub) TruncateTables(c context.Context, db string, ts []string) error {
	return n.d.TruncateTables(c, db, ts)
}
func (n noSchemaStub) DropTables(c context.Context, db string, ts []string) error {
	return n.d.DropTables(c, db, ts)
}
func (n noSchemaStub) DropViews(c context.Context, db string, vs []string) error {
	return n.d.DropViews(c, db, vs)
}
func (n noSchemaStub) SelectRows(c context.Context, db, t string, q sqlgen.SelectQuery) (driver.Page, error) {
	return n.d.SelectRows(c, db, t, q)
}
func (n noSchemaStub) PrimaryKey(c context.Context, db, t string) ([]string, error) {
	return n.d.PrimaryKey(c, db, t)
}
func (n noSchemaStub) InsertRow(c context.Context, db, t string, v driver.RowValues) error {
	return n.d.InsertRow(c, db, t, v)
}
func (n noSchemaStub) UpdateRow(c context.Context, db, t string, w driver.PK, v driver.RowValues) error {
	return n.d.UpdateRow(c, db, t, w, v)
}
func (n noSchemaStub) DeleteRows(c context.Context, db, t string, w []driver.PK) error {
	return n.d.DeleteRows(c, db, t, w)
}
func (n noSchemaStub) UpsertRows(c context.Context, db, t string, r []driver.RowValues) error {
	return n.d.UpsertRows(c, db, t, r)
}
func (n noSchemaStub) LastInsertID(c context.Context) (int64, error) {
	return n.d.LastInsertID(c)
}
func (n noSchemaStub) ExecSQL(c context.Context, db, q, id string, o driver.ExecOpts) ([]driver.Result, error) {
	return n.d.ExecSQL(c, db, q, id, o)
}
func (n noSchemaStub) CancelQuery(c context.Context, id string) error {
	return n.d.CancelQuery(c, id)
}
func (n noSchemaStub) Export(c context.Context, db string, req driver.ExportRequest, w io.Writer) error {
	return n.d.Export(c, db, req, w)
}
func (n noSchemaStub) ImportSQL(c context.Context, db string, r io.Reader, p func(int64, int64)) error {
	return n.d.ImportSQL(c, db, r, p)
}
func (n noSchemaStub) ImportCSV(c context.Context, db, t string, r io.Reader, o driver.CSVImportOpts, p func(int64, int64)) error {
	return n.d.ImportCSV(c, db, t, r, o, p)
}
func (n noSchemaStub) Close() error { return n.d.Close() }
