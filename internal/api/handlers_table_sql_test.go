package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/dblama-dev/dblama/internal/driver"
)

type tableSQLDriver struct {
	*stubDriver
	sql     string
	err     error
	calls   []identCall
	schemas []string
}

func (d *tableSQLDriver) ShowCreateTable(ctx context.Context, db, table string) (string, error) {
	d.calls = append(d.calls, identCall{db, table})
	d.schemas = append(d.schemas, driver.SchemaFromContext(ctx))
	return d.sql, d.err
}

func TestTableSQLHandler(t *testing.T) {
	fake := &tableSQLDriver{stubDriver: &stubDriver{}, sql: "CREATE TABLE `Mixed%Case` (`id` int)"}
	_, client, base := loginStubServer(t, fake)
	resp, err := client.Get(base + "/api/databases/" + url.PathEscape("app db") + "/tables/" + url.PathEscape("Mixed%Case") + "/create-sql")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.SQL != fake.sql {
		t.Fatalf("SQL = %q", body.SQL)
	}
	if len(fake.calls) != 1 || fake.calls[0] != (identCall{"app db", "Mixed%Case"}) {
		t.Fatalf("calls = %#v", fake.calls)
	}
}
func TestTableSQLHandler_Schema(t *testing.T) {
	fake := &tableSQLDriver{stubDriver: &stubDriver{schemaAware: true}, sql: "CREATE TABLE test (id int)"}
	_, client, base := loginStubServer(t, fake)
	resp, err := client.Get(base + "/api/databases/appdb/tables/test/create-sql?schema=Odd%20Schema")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(fake.schemas) != 1 || fake.schemas[0] != "Odd Schema" {
		t.Fatalf("schemas = %v", fake.schemas)
	}
	if len(fake.schemaCalls) != 0 {
		t.Fatalf("mutable schema changed: %v", fake.schemaCalls)
	}
}
func TestTableSQLHandler_Error(t *testing.T) {
	fake := &tableSQLDriver{stubDriver: &stubDriver{}, err: errors.New("permission denied")}
	_, client, base := loginStubServer(t, fake)
	resp, err := client.Get(base + "/api/databases/appdb/tables/test/create-sql")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 500 || body.Error.Code != "driver_error" || body.Error.Message != "permission denied" {
		t.Fatalf("status %d: %+v", resp.StatusCode, body)
	}
}
func TestTableSQLHandler_Unauthenticated(t *testing.T) {
	fake := &tableSQLDriver{stubDriver: &stubDriver{}}
	srv, _, base := loginStubServer(t, fake)
	resp, err := srv.Client().Get(base + "/api/databases/appdb/tables/test/create-sql")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 401 || len(fake.calls) != 0 {
		t.Fatalf("status %d, calls %v", resp.StatusCode, fake.calls)
	}
}
