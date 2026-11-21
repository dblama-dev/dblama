//go:build integration

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/dblama-dev/dblama/internal/config"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/driver/mysql"
	"github.com/dblama-dev/dblama/internal/session"
)

// apiTestDB is the dedicated schema every table test runs against. It is
// created and dropped directly on the container, while the session logs
// in without a default database — every API call names the schema in its
// path, so the round trips also cover cross-schema writes.
const apiTestDB = "dblama_p2_api"

// startIntegrationAPI boots the router over a live container connection
// (mysql and mariadb driver IDs on the real mysql.Connect) and logs in
// WITHOUT a default database. It returns the server's base URL, the
// server's HTTP client and the session cookie value.
func startIntegrationAPI(t *testing.T, dbCfg driver.ConnConfig) (baseURL string, client *http.Client, cookie string) {
	t.Helper()

	reg := driver.NewRegistry()
	reg.Register(driver.DriverMeta{ID: "mysql", Name: "MySQL", DefaultPort: "3306"}, mysql.Connect)
	reg.Register(driver.DriverMeta{ID: "mariadb", Name: "MariaDB", DefaultPort: "3306"}, mysql.Connect)

	store := session.NewStore(time.Minute, 100, time.Now)
	h := NewRouter(config.Config{LoginRateLimit: config.Rate{Count: 10, Per: time.Minute}}, store, reg, nil)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	login := fmt.Sprintf(`{"driver":"mysql","host":%q,"port":%q,"user":"root","password":"dblama-test"}`,
		dbCfg.Host, dbCfg.Port)
	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session", login, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	return srv.URL, srv.Client(), sessionCookie(t, resp).Value
}

// execAdminQuery runs one statement directly on the container as root,
// outside the API — used to create and drop the test schema.
func execAdminQuery(t *testing.T, dbCfg driver.ConnConfig, query string) {
	t.Helper()
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%s)/", dbCfg.User, dbCfg.Password, dbCfg.Host, dbCfg.Port))
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(context.Background(), query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// findColumn returns the column def with the given name.
func findColumn(def driver.TableDef, name string) (driver.ColumnDef, bool) {
	for _, c := range def.Columns {
		if c.Name == name {
			return c, true
		}
	}
	return driver.ColumnDef{}, false
}

// findIndex returns the index def with the given name.
func findIndex(def driver.TableDef, name string) (driver.Index, bool) {
	for _, idx := range def.Indexes {
		if idx.Name == name {
			return idx, true
		}
	}
	return driver.Index{}, false
}

// getTableDef fetches and decodes the full table def over the API.
func getTableDef(t *testing.T, client *http.Client, base, cookie, db, table string) driver.TableDef {
	t.Helper()
	resp := apiRequest(t, client, http.MethodGet,
		base+"/api/databases/"+db+"/tables/"+table, "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET table %s status = %d, want %d", table, resp.StatusCode, http.StatusOK)
	}
	var def driver.TableDef
	if err := json.NewDecoder(resp.Body).Decode(&def); err != nil {
		t.Fatalf("decode table def: %v", err)
	}
	return def
}

// getRowsPage fetches one page of rows over the API and asserts 200.
func getRowsPage(t *testing.T, client *http.Client, base, cookie, db, table, query string) driver.Page {
	t.Helper()
	resp := apiRequest(t, client, http.MethodGet,
		base+"/api/databases/"+db+"/tables/"+table+"/rows"+query, "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET rows%s status = %d, want %d", query, resp.StatusCode, http.StatusOK)
	}
	var page driver.Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode rows page: %v", err)
	}
	return page
}

// tableList fetches GET tables over the API and returns the table names.
func tableList(t *testing.T, client *http.Client, base, cookie, db string) []string {
	t.Helper()
	resp := apiRequest(t, client, http.MethodGet,
		base+"/api/databases/"+db+"/tables", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET tables status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var got struct {
		Tables []driver.TableInfo `json:"tables"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode tables body: %v", err)
	}
	names := make([]string, 0, len(got.Tables))
	for _, tbl := range got.Tables {
		names = append(names, tbl.Name)
	}
	return names
}

// insertRow posts one row and returns the reported lastInsertId.
func insertRow(t *testing.T, client *http.Client, base, cookie, db, table, body string) int64 {
	t.Helper()
	resp := apiRequest(t, client, http.MethodPost,
		base+"/api/databases/"+db+"/tables/"+table+"/rows", body, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST rows into %s status = %d, want %d", table, resp.StatusCode, http.StatusOK)
	}
	var got struct {
		LastInsertID int64 `json:"lastInsertId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode insert response: %v", err)
	}
	return got.LastInsertID
}

// assertNoContent runs a mutating request and asserts 204.
func assertNoContent(t *testing.T, client *http.Client, method, url, body, cookie string) {
	t.Helper()
	resp := apiRequest(t, client, method, url, body, cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("%s %s status = %d, want %d", method, url, resp.StatusCode, http.StatusNoContent)
	}
}

// TestAPI_TableLifecycle drives the whole table lifecycle over HTTP
// against a real server: create → read def → list → pk → insert → browse
// with filters → update → delete rows → indexes → truncate → drop. The
// session logs in without a default database and every call addresses the
// dedicated apiTestDB schema, so cross-schema writes are covered too.
func TestAPI_TableLifecycle(t *testing.T) {
	for _, tc := range integrationImages {
		t.Run(tc.image, func(t *testing.T) {
			dbCfg := startIntegrationDB(t, tc.image)
			execAdminQuery(t, dbCfg, "DROP DATABASE IF EXISTS "+apiTestDB)
			execAdminQuery(t, dbCfg, "CREATE DATABASE "+apiTestDB)
			t.Cleanup(func() { execAdminQuery(t, dbCfg, "DROP DATABASE IF EXISTS "+apiTestDB) })

			base, client, cookie := startIntegrationAPI(t, dbCfg)

			tablesURL := base + "/api/databases/" + apiTestDB + "/tables"

			// Without a session cookie the table endpoints are 401.
			resp := apiRequest(t, client, http.MethodGet, tablesURL, "", "")
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("GET tables without session status = %d, want %d",
					resp.StatusCode, http.StatusUnauthorized)
			}

			// Create via PUT _new: PK inline through TableDef.Indexes.
			createBody := `{
				"newName": "products",
				"columns": [
					{"name": "id", "type": "int", "unsigned": true, "nullable": false, "autoIncrement": true},
					{"name": "name", "type": "varchar", "length": "64", "nullable": false,
					 "default": {"value": "x"}},
					{"name": "kind", "type": "enum", "nullable": true, "enumValues": ["a", "b"]},
					{"name": "price", "type": "decimal", "length": "10,2", "nullable": true}
				],
				"indexes": [{"name": "PRIMARY", "type": "PRIMARY", "columns": [{"name": "id"}]}],
				"options": {"comment": "p2 lifecycle table"}
			}`
			assertNoContent(t, client, http.MethodPut, tablesURL+"/_new", createBody, cookie)

			// The stored def matches what was asked for (columns are
			// compared image-agnostically: no length or collation, which
			// the two servers report differently).
			def := getTableDef(t, client, base, cookie, apiTestDB, "products")
			if def.NewName != "products" {
				t.Errorf("NewName = %q, want %q", def.NewName, "products")
			}
			if len(def.Columns) != 4 {
				t.Fatalf("Columns: got %d, want 4: %#v", len(def.Columns), def.Columns)
			}
			idCol, ok := findColumn(def, "id")
			if !ok || idCol.Type != "int" || !idCol.Unsigned || idCol.Nullable || !idCol.AutoIncrement {
				t.Errorf("id column = %#v, want int unsigned NOT NULL AUTO_INCREMENT", idCol)
			}
			nameCol, ok := findColumn(def, "name")
			if !ok || nameCol.Type != "varchar" || nameCol.Nullable ||
				nameCol.Default == nil || nameCol.Default.Value != "x" || nameCol.Default.IsSQLExpression {
				t.Errorf("name column = %#v, want varchar NOT NULL DEFAULT 'x'", nameCol)
			}
			kindCol, ok := findColumn(def, "kind")
			if !ok || kindCol.Type != "enum" || !kindCol.Nullable ||
				!slices.Equal(kindCol.EnumValues, []string{"a", "b"}) {
				t.Errorf("kind column = %#v, want nullable enum('a','b')", kindCol)
			}
			priceCol, ok := findColumn(def, "price")
			if !ok || priceCol.Type != "decimal" || !priceCol.Nullable {
				t.Errorf("price column = %#v, want nullable decimal(10,2)", priceCol)
			}
			pk, ok := findIndex(def, "PRIMARY")
			if !ok || pk.Type != "PRIMARY" || len(pk.Columns) != 1 || pk.Columns[0].Name != "id" {
				t.Errorf("PRIMARY index = %#v, want PRIMARY(id)", pk)
			}
			if def.Options.Comment != "p2 lifecycle table" {
				t.Errorf("Options.Comment = %q, want %q", def.Options.Comment, "p2 lifecycle table")
			}

			// The table shows up in the list.
			if !slices.Contains(tableList(t, client, base, cookie, apiTestDB), "products") {
				t.Errorf("table list does not contain %q after create: %v",
					"products", tableList(t, client, base, cookie, apiTestDB))
			}

			// The primary key is reported as [id].
			resp = apiRequest(t, client, http.MethodGet, tablesURL+"/products/pk", "", cookie)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET pk status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			var pkGot struct {
				Columns []string `json:"columns"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&pkGot); err != nil {
				t.Fatalf("decode pk body: %v", err)
			}
			if !slices.Equal(pkGot.Columns, []string{"id"}) {
				t.Errorf("pk columns = %v, want [id]", pkGot.Columns)
			}

			// Insert a row and read back the generated id.
			firstID := insertRow(t, client, base, cookie, apiTestDB, "products", `{"name": "row1", "kind": "a"}`)
			if firstID < 1 {
				t.Fatalf("lastInsertId = %d, want >= 1", firstID)
			}

			// Browse: one row with the inserted values.
			page := getRowsPage(t, client, base, cookie, apiTestDB, "products", "?limit=10&order=id:asc")
			if page.TotalRows != 1 || len(page.Rows) != 1 {
				t.Fatalf("rows page = %d rows (total %d), want 1/1", len(page.Rows), page.TotalRows)
			}
			if len(page.Columns) != 4 || page.Columns[0].Name != "id" {
				t.Fatalf("row columns = %+v, want id/name/kind/price", page.Columns)
			}
			if page.Rows[0][1] != "row1" || page.Rows[0][2] != "a" || page.Rows[0][3] != nil {
				t.Errorf("row values = %v, want [id, row1, a, NULL]", page.Rows[0])
			}

			// The LIKE filter finds the row; an unknown operator is 400.
			page = getRowsPage(t, client, base, cookie, apiTestDB, "products", "?filter=name:LIKE:row%25")
			if page.TotalRows != 1 || len(page.Rows) != 1 || page.Rows[0][1] != "row1" {
				t.Errorf("LIKE filter page = %d rows (total %d), first %v, want the row1 row",
					len(page.Rows), page.TotalRows, firstOf(page))
			}
			resp = apiRequest(t, client, http.MethodGet,
				tablesURL+"/products/rows?filter=name:!like:x", "", cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("GET rows with unknown filter op status = %d, want %d",
					resp.StatusCode, http.StatusBadRequest)
			}

			// Update the row by primary key and verify through a browse.
			assertNoContent(t, client, http.MethodPatch, tablesURL+"/products/rows",
				fmt.Sprintf(`{"where": {"id": %d}, "values": {"name": "row1-edited"}}`, firstID), cookie)
			page = getRowsPage(t, client, base, cookie, apiTestDB, "products", "?filter=name:=:row1-edited")
			if page.TotalRows != 1 || len(page.Rows) != 1 || page.Rows[0][1] != "row1-edited" {
				t.Errorf("rows after update = %d rows (total %d), first %v, want the edited row",
					len(page.Rows), page.TotalRows, firstOf(page))
			}

			// A second row is inserted; deleting the first leaves one.
			secondID := insertRow(t, client, base, cookie, apiTestDB, "products",
				`{"name": "row2", "kind": "b", "price": 9.99}`)
			if secondID < 1 {
				t.Fatalf("second lastInsertId = %d, want >= 1", secondID)
			}
			if page = getRowsPage(t, client, base, cookie, apiTestDB, "products", "?limit=10"); page.TotalRows != 2 {
				t.Fatalf("rows after second insert: total = %d, want 2", page.TotalRows)
			}
			assertNoContent(t, client, http.MethodDelete, tablesURL+"/products/rows",
				fmt.Sprintf(`{"where": [{"id": %d}]}`, firstID), cookie)
			page = getRowsPage(t, client, base, cookie, apiTestDB, "products", "?limit=10&order=id:asc")
			if page.TotalRows != 1 || len(page.Rows) != 1 || page.Rows[0][1] != "row2" {
				t.Errorf("rows after delete = %d rows (total %d), first %v, want only row2",
					len(page.Rows), page.TotalRows, firstOf(page))
			}

			// Index management: add a unique index next to the PK, then
			// drop it again by posting the desired state without it.
			assertNoContent(t, client, http.MethodPost, tablesURL+"/products/indexes",
				`[{"name": "PRIMARY", "type": "PRIMARY", "columns": [{"name": "id"}]},
				  {"name": "uq_name", "type": "UNIQUE", "columns": [{"name": "name"}]}]`, cookie)
			def = getTableDef(t, client, base, cookie, apiTestDB, "products")
			uq, ok := findIndex(def, "uq_name")
			if !ok || uq.Type != "UNIQUE" || len(uq.Columns) != 1 || uq.Columns[0].Name != "name" {
				t.Errorf("uq_name index = %#v, want UNIQUE(name)", uq)
			}
			assertNoContent(t, client, http.MethodPost, tablesURL+"/products/indexes",
				`[{"name": "PRIMARY", "type": "PRIMARY", "columns": [{"name": "id"}]}]`, cookie)
			def = getTableDef(t, client, base, cookie, apiTestDB, "products")
			if len(def.Indexes) != 1 || def.Indexes[0].Name != "PRIMARY" {
				t.Errorf("indexes after removal = %+v, want only PRIMARY", def.Indexes)
			}

			// Truncate empties the table.
			assertNoContent(t, client, http.MethodPost, tablesURL+"/products/truncate", "", cookie)
			page = getRowsPage(t, client, base, cookie, apiTestDB, "products", "?limit=10")
			if page.TotalRows != 0 || len(page.Rows) != 0 {
				t.Errorf("rows after truncate = %d rows (total %d), want 0/0", len(page.Rows), page.TotalRows)
			}

			// Drop removes the table from the list.
			assertNoContent(t, client, http.MethodDelete, tablesURL+"/products", "", cookie)
			if slices.Contains(tableList(t, client, base, cookie, apiTestDB), "products") {
				t.Errorf("table list still contains %q after drop", "products")
			}
		})
	}
}

// firstOf returns the first row of a page or nil.
func firstOf(page driver.Page) []any {
	if len(page.Rows) == 0 {
		return nil
	}
	return page.Rows[0]
}

// TestAPI_ReservedIdentifiers runs the full CRUD round trip against a
// table named "order" with the reserved-word columns "select" and
// "where" and the backtick-bearing column "a`b": every identifier is
// quoted on the way through the driver, so the API must behave exactly
// as for a plain table.
func TestAPI_ReservedIdentifiers(t *testing.T) {
	const backtickCol = "a`b"

	for _, tc := range integrationImages {
		t.Run(tc.image, func(t *testing.T) {
			dbCfg := startIntegrationDB(t, tc.image)
			execAdminQuery(t, dbCfg, "DROP DATABASE IF EXISTS "+apiTestDB)
			execAdminQuery(t, dbCfg, "CREATE DATABASE "+apiTestDB)
			t.Cleanup(func() { execAdminQuery(t, dbCfg, "DROP DATABASE IF EXISTS "+apiTestDB) })

			base, client, cookie := startIntegrationAPI(t, dbCfg)

			tablesURL := base + "/api/databases/" + apiTestDB + "/tables"

			createBody := `{
				"newName": "order",
				"columns": [
					{"name": "id", "type": "int", "unsigned": true, "nullable": false, "autoIncrement": true},
					{"name": "select", "type": "varchar", "length": "32", "nullable": false},
					{"name": "where", "type": "varchar", "length": "32", "nullable": true},
					{"name": "a` + "`" + `b", "type": "varchar", "length": "32", "nullable": true}
				],
				"indexes": [{"name": "PRIMARY", "type": "PRIMARY", "columns": [{"name": "id"}]}]
			}`
			assertNoContent(t, client, http.MethodPut, tablesURL+"/_new", createBody, cookie)

			// The def comes back with every odd identifier intact.
			def := getTableDef(t, client, base, cookie, apiTestDB, "order")
			if def.NewName != "order" {
				t.Errorf("NewName = %q, want %q", def.NewName, "order")
			}
			for _, name := range []string{"id", "select", "where", backtickCol} {
				if _, ok := findColumn(def, name); !ok {
					t.Errorf("columns %v missing %q", def.Columns, name)
				}
			}

			// Insert into the reserved-word columns.
			insertBody := `{"select": "s1", "where": "w1", "a` + "`" + `b": "x1"}`
			id := insertRow(t, client, base, cookie, apiTestDB, "order", insertBody)
			if id < 1 {
				t.Fatalf("lastInsertId = %d, want >= 1", id)
			}

			// Select and order by a reserved-word column.
			page := getRowsPage(t, client, base, cookie, apiTestDB, "order", "?limit=10&order=select:asc")
			if page.TotalRows != 1 || len(page.Rows) != 1 {
				t.Fatalf("rows page = %d rows (total %d), want 1/1", len(page.Rows), page.TotalRows)
			}
			if page.Rows[0][1] != "s1" || page.Rows[0][2] != "w1" || page.Rows[0][3] != "x1" {
				t.Errorf("row values = %v, want [id, s1, w1, x1]", page.Rows[0])
			}

			// Update and re-verify.
			assertNoContent(t, client, http.MethodPatch, tablesURL+"/order/rows",
				`{"where": {"select": "s1"}, "values": {"where": "w2", "a` + "`" + `b": "x2"}}`, cookie)
			page = getRowsPage(t, client, base, cookie, apiTestDB, "order", "?filter=select:=:s1")
			if page.TotalRows != 1 || len(page.Rows) != 1 ||
				page.Rows[0][2] != "w2" || page.Rows[0][3] != "x2" {
				t.Errorf("rows after update = %d rows (total %d), first %v, want the updated reserved-word values",
					len(page.Rows), page.TotalRows, firstOf(page))
			}

			// Drop the reserved-word table.
			assertNoContent(t, client, http.MethodDelete, tablesURL+"/order", "", cookie)
			if slices.Contains(tableList(t, client, base, cookie, apiTestDB), "order") {
				t.Errorf("table list still contains %q after drop", "order")
			}
		})
	}
}

