//go:build integration

package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hitslab/db-manager/server/internal/config"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/driver/mysql"
	"github.com/hitslab/db-manager/server/internal/session"
)

// The three schemas of the transfer round trip: the dump source, the
// database the SQL dump is imported into and the database the CSV import
// lands in. All are created and dropped directly on the container.
const (
	transferSrcDB = "dbmanager_p3_src"
	transferSQLDB = "dbmanager_p3_sql"
	transferCSVDB = "dbmanager_p3_csv"
)

// startTransferAPI boots the router over a live container connection and
// logs in WITH a default database. Unlike the table tests (whose session
// logs in without one) the dump endpoints need it: Export pins the dump
// to the driver's own login database, so the export source is the
// database selected at login — the flow of a user who picked a database
// on the login screen. Every other endpoint still names the schema in
// its path, so the import into the *other* two databases keeps covering
// cross-schema addressing.
func startTransferAPI(t *testing.T, dbCfg driver.ConnConfig, database string) (baseURL string, client *http.Client, cookie string) {
	t.Helper()

	reg := driver.NewRegistry()
	reg.Register(driver.DriverMeta{ID: "mysql", Name: "MySQL", DefaultPort: "3306"}, mysql.Connect)
	reg.Register(driver.DriverMeta{ID: "mariadb", Name: "MariaDB", DefaultPort: "3306"}, mysql.Connect)

	store := session.NewStore(time.Minute, 100, time.Now)
	h := NewRouter(config.Config{LoginRateLimit: config.Rate{Count: 10, Per: time.Minute}}, store, reg, nil)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	login := fmt.Sprintf(`{"driver":"mysql","host":%q,"port":%q,"user":"root","password":"dbmanager-test","database":%q}`,
		dbCfg.Host, dbCfg.Port, database)
	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session", login, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	return srv.URL, srv.Client(), sessionCookie(t, resp).Value
}

// buildMultipart renders a multipart form body: the fields first (the
// import handler streams parts in order and stops at the file part), then
// the file. Returns the body and the Content-Type header value.
func buildMultipart(t *testing.T, fields map[string]string, filename string, file []byte) (body, contentType string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, k := range slices.Sorted(maps.Keys(fields)) {
		if err := mw.WriteField(k, fields[k]); err != nil {
			t.Fatalf("write multipart field %s: %v", k, err)
		}
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create multipart file part: %v", err)
	}
	if _, err := fw.Write(file); err != nil {
		t.Fatalf("write multipart file part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return buf.String(), mw.FormDataContentType()
}

// postMultipart sends a multipart POST with an explicit Content-Type
// (apiRequest only handles plain bodies) and the CSRF header.
func postMultipart(t *testing.T, client *http.Client, url, contentType, body, cookieValue string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build multipart request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if cookieValue != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: cookieValue})
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST multipart %s: %v", url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// readImportStream consumes the NDJSON progress stream of an import: it
// asserts no record carries an error and that the stream ends with the
// final {"ok":true} line, and returns all records.
func readImportStream(t *testing.T, resp *http.Response) []importProgressLine {
	t.Helper()
	var lines []importProgressLine
	rd := bufio.NewReader(resp.Body)
	for {
		line, rerr := rd.ReadString('\n')
		if line != "" {
			var rec importProgressLine
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				t.Fatalf("decode import progress line %q: %v", line, err)
			}
			lines = append(lines, rec)
			if rec.Error != "" {
				t.Fatalf("import stream reported an error: %s", rec.Error)
			}
			if rec.OK {
				return lines
			}
		}
		if rerr != nil {
			t.Fatalf("import stream ended without a final ok line: %v", rerr)
		}
	}
}

// createProductsTable PUTs the products table definition (120-row batch
// table) into db over the tables API.
func createProductsTable(t *testing.T, client *http.Client, base, cookie, db string) {
	t.Helper()
	createBody := `{
		"newName": "products",
		"columns": [
			{"name": "id", "type": "int", "unsigned": true, "nullable": false, "autoIncrement": true},
			{"name": "name", "type": "varchar", "length": "64", "nullable": false},
			{"name": "kind", "type": "enum", "nullable": true, "enumValues": ["a", "b"]},
			{"name": "price", "type": "decimal", "length": "10,2", "nullable": true}
		],
		"indexes": [{"name": "PRIMARY", "type": "PRIMARY", "columns": [{"name": "id"}]}]
	}`
	assertNoContent(t, client, http.MethodPut, base+"/api/databases/"+db+"/tables/_new", createBody, cookie)
}

// createTagsTable PUTs the tags table definition (special-values table)
// into db over the tables API.
func createTagsTable(t *testing.T, client *http.Client, base, cookie, db string) {
	t.Helper()
	createBody := `{
		"newName": "tags",
		"columns": [
			{"name": "id", "type": "int", "unsigned": true, "nullable": false, "autoIncrement": true},
			{"name": "label", "type": "varchar", "length": "64", "nullable": false},
			{"name": "note", "type": "text", "nullable": true}
		],
		"indexes": [{"name": "PRIMARY", "type": "PRIMARY", "columns": [{"name": "id"}]}]
	}`
	assertNoContent(t, client, http.MethodPut, base+"/api/databases/"+db+"/tables/_new", createBody, cookie)
}

// TestAPI_ExportImportRoundTrip drives the spec §11 round trip over HTTP
// against live servers: two tables with data created through the tables
// API → POST export (sql) → POST import (multipart) into a fresh database
// → structure and data of the new database match the original. It also
// covers the csv format round trip, the csv import validation error path
// and the console cancel endpoint smoke.
func TestAPI_ExportImportRoundTrip(t *testing.T) {
	for _, tc := range integrationImages {
		t.Run(tc.image, func(t *testing.T) {
			dbCfg := startIntegrationDB(t, tc.image)
			for _, db := range []string{transferSrcDB, transferSQLDB, transferCSVDB} {
				execAdminQuery(t, dbCfg, "DROP DATABASE IF EXISTS "+db)
				execAdminQuery(t, dbCfg, "CREATE DATABASE "+db)
				t.Cleanup(func() { execAdminQuery(t, dbCfg, "DROP DATABASE IF EXISTS "+db) })
			}

			base, client, cookie := startTransferAPI(t, dbCfg, transferSrcDB)

			// --- 1. Source data through the tables API. ---------------

			createProductsTable(t, client, base, cookie, transferSrcDB)
			createTagsTable(t, client, base, cookie, transferSrcDB)

			// 120 rows: the sql export batches INSERTs at 100 rows, so
			// this exercises more than one page and more than one INSERT
			// statement on the way through.
			for i := range 120 {
				kind := "a"
				if i%2 == 1 {
					kind = "b"
				}
				price := "null"
				if i%3 == 0 {
					price = "12.5"
				}
				insertRow(t, client, base, cookie, transferSrcDB, "products",
					fmt.Sprintf(`{"name": "row-%d", "kind": %q, "price": %s}`, i, kind, price))
			}

			// The special-values set: NULL vs empty string, quote,
			// backslash, double quote, newline, unicode. insertRow asserts
			// success; the ids are 1..4 in insert order.
			insertRow(t, client, base, cookie, transferSrcDB, "tags", `{"label": "plain"}`)
			insertRow(t, client, base, cookie, transferSrcDB, "tags",
				`{"label": "O'Reilly\\path", "note": "quote' back\\slash \"double\""}`)
			insertRow(t, client, base, cookie, transferSrcDB, "tags",
				"{\"label\": \"Привет 🌍\", \"note\": \"line1\\nline2\"}")
			insertRow(t, client, base, cookie, transferSrcDB, "tags", `{"label": "", "note": null}`)

			// --- 2. SQL export. ---------------------------------------

			resp := apiRequest(t, client, http.MethodPost,
				base+"/api/databases/"+transferSrcDB+"/export",
				`{"format": "sql", "dropIfExists": true, "ifNotExists": true}`, cookie)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("export status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/sql" {
				t.Errorf("export Content-Type = %q, want %q", ct, "application/sql")
			}
			if cd := resp.Header.Get("Content-Disposition"); cd != `attachment; filename="dump.sql"` {
				t.Errorf("export Content-Disposition = %q, want the dump.sql attachment", cd)
			}
			var dumpBuf bytes.Buffer
			if _, err := dumpBuf.ReadFrom(resp.Body); err != nil {
				t.Fatalf("read export body: %v", err)
			}
			dump := dumpBuf.Bytes()
			if !strings.Contains(string(dump), "CREATE TABLE") || !strings.Contains(string(dump), "INSERT INTO") {
				t.Fatalf("sql dump is missing structure or data: %d bytes", len(dump))
			}

			// --- 3. SQL import into the fresh database. ---------------

			body, contentType := buildMultipart(t, map[string]string{"type": "sql"}, "dump.sql", dump)
			resp = postMultipart(t, client, base+"/api/databases/"+transferSQLDB+"/import",
				contentType, body, cookie)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("import status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			lines := readImportStream(t, resp)
			last := lines[len(lines)-1]
			if !last.OK || last.Error != "" {
				t.Fatalf("final import line = %+v, want ok:true without error", last)
			}
			if last.Done != last.Total || last.Total == 0 {
				t.Errorf("final import progress = done %d, total %d, want done == total > 0", last.Done, last.Total)
			}
			if len(lines) < 2 {
				t.Errorf("import stream has %d lines, want progress lines before the final one", len(lines))
			}

			// --- 4. The new database matches the original. ------------

			tables := tableList(t, client, base, cookie, transferSQLDB)
			for _, want := range []string{"products", "tags"} {
				if !slices.Contains(tables, want) {
					t.Errorf("imported tables %v do not contain %q", tables, want)
				}
			}

			// Products: 120 rows imported, and the full pages are
			// value-identical between source and import (covers NULL vs
			// empty string, quoting, unicode and the numeric rendering).
			srcPage := getRowsPage(t, client, base, cookie, transferSrcDB, "products", "?limit=200&order=id:asc")
			dstPage := getRowsPage(t, client, base, cookie, transferSQLDB, "products", "?limit=200&order=id:asc")
			if srcPage.TotalRows != 120 {
				t.Fatalf("source products total = %d, want 120", srcPage.TotalRows)
			}
			if dstPage.TotalRows != srcPage.TotalRows {
				t.Errorf("imported products total = %d, want %d", dstPage.TotalRows, srcPage.TotalRows)
			}
			if !reflect.DeepEqual(srcPage.Rows, dstPage.Rows) {
				t.Errorf("imported products rows differ from the source:\nsrc[0] = %v\ndst[0] = %v",
					srcPage.Rows[0], dstPage.Rows[0])
			}
			if got := srcPage.Rows[0][1]; got != "row-0" {
				t.Errorf("first product name = %v, want row-0", got)
			}

			// Tags: the special values survive the dump verbatim.
			srcTags := getRowsPage(t, client, base, cookie, transferSrcDB, "tags", "?limit=10&order=id:asc")
			dstTags := getRowsPage(t, client, base, cookie, transferSQLDB, "tags", "?limit=10&order=id:asc")
			if srcTags.TotalRows != 4 || dstTags.TotalRows != 4 {
				t.Fatalf("tags totals: src %d, dst %d, want 4 and 4", srcTags.TotalRows, dstTags.TotalRows)
			}
			wantTags := [][]any{
				{float64(1), "plain", nil},
				{float64(2), "O'Reilly\\path", "quote' back\\slash \"double\""},
				{float64(3), "Привет 🌍", "line1\nline2"},
				{float64(4), "", nil},
			}
			if !reflect.DeepEqual(srcTags.Rows, wantTags) {
				t.Errorf("source tags rows = %v, want %v (fixture check)", srcTags.Rows, wantTags)
			}
			if !reflect.DeepEqual(dstTags.Rows, wantTags) {
				t.Errorf("imported tags rows = %v, want %v", dstTags.Rows, wantTags)
			}

			// The imported products structure matches the source's
			// (columns and indexes; options deliberately excluded — the
			// auto_increment counters may legitimately differ per image).
			srcDef := getTableDef(t, client, base, cookie, transferSrcDB, "products")
			dstDef := getTableDef(t, client, base, cookie, transferSQLDB, "products")
			if dstDef.NewName != srcDef.NewName {
				t.Errorf("imported NewName = %q, want %q", dstDef.NewName, srcDef.NewName)
			}
			if !reflect.DeepEqual(srcDef.Columns, dstDef.Columns) {
				t.Errorf("imported columns differ from the source:\nsrc = %+v\ndst = %+v", srcDef.Columns, dstDef.Columns)
			}
			if !reflect.DeepEqual(srcDef.Indexes, dstDef.Indexes) {
				t.Errorf("imported indexes differ from the source:\nsrc = %+v\ndst = %+v", srcDef.Indexes, dstDef.Indexes)
			}

			// --- 5. CSV round trip. ----------------------------------

			resp = apiRequest(t, client, http.MethodPost,
				base+"/api/databases/"+transferSrcDB+"/export",
				`{"format": "csv", "tables": ["products"]}`, cookie)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("csv export status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "text/csv" {
				t.Errorf("csv export Content-Type = %q, want %q", ct, "text/csv")
			}
			if cd := resp.Header.Get("Content-Disposition"); cd != `attachment; filename="dump.csv"` {
				t.Errorf("csv export Content-Disposition = %q, want the dump.csv attachment", cd)
			}
			var csvBuf bytes.Buffer
			if _, err := csvBuf.ReadFrom(resp.Body); err != nil {
				t.Fatalf("read csv export body: %v", err)
			}

			// ImportCSV does not create tables: pre-create products in
			// the csv target database, then import with skipHeader.
			createProductsTable(t, client, base, cookie, transferCSVDB)
			body, contentType = buildMultipart(t,
				map[string]string{"type": "csv", "table": "products", "skipHeader": "true"},
				"dump.csv", csvBuf.Bytes())
			resp = postMultipart(t, client, base+"/api/databases/"+transferCSVDB+"/import",
				contentType, body, cookie)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("csv import status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			lines = readImportStream(t, resp)
			last = lines[len(lines)-1]
			if !last.OK {
				t.Fatalf("final csv import line = %+v, want ok:true", last)
			}
			if last.Done != 120 || last.Total != 120 {
				t.Errorf("final csv import progress = done %d, total %d, want 120/120", last.Done, last.Total)
			}
			csvPage := getRowsPage(t, client, base, cookie, transferCSVDB, "products", "?limit=1")
			if csvPage.TotalRows != 120 {
				t.Errorf("csv-imported products total = %d, want 120", csvPage.TotalRows)
			}

			// Error path: a csv import without the table field is a 400
			// (validated before any byte is streamed).
			body, contentType = buildMultipart(t, map[string]string{"type": "csv"}, "dump.csv", csvBuf.Bytes())
			resp = postMultipart(t, client, base+"/api/databases/"+transferCSVDB+"/import",
				contentType, body, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("csv import without table status = %d, want %d",
					resp.StatusCode, http.StatusBadRequest)
			}

			// --- 6. Cancel smoke. ------------------------------------

			// A long SELECT on a goroutine, then cancel it: the query
			// request must return within 5s of the cancel (KILL QUERY).
			// The error text is deliberately not pinned: MySQL returns
			// the SLEEP result silently, MariaDB reports the kill — both
			// are acceptable outcomes of a cancelled console query.
			const queryID = "transfer-cancel-e2e"
			queryDone := make(chan int, 1)
			go func() {
				req, err := http.NewRequest(http.MethodPost,
					base+"/api/databases/"+transferSrcDB+"/query",
					strings.NewReader(fmt.Sprintf(`{"sql": "SELECT SLEEP(30)", "queryID": %q}`, queryID)))
				if err != nil {
					queryDone <- -1
					return
				}
				req.Header.Set("X-Requested-With", "XMLHttpRequest")
				req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: cookie})
				resp, err := client.Do(req)
				if err != nil {
					queryDone <- -1
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				queryDone <- resp.StatusCode
			}()
			time.Sleep(time.Second) // let the query start and register its queryID

			cancelResp := apiRequest(t, client, http.MethodPost,
				base+"/api/query/"+queryID+"/cancel", "", cookie)
			if cancelResp.StatusCode != http.StatusNoContent {
				t.Errorf("cancel status = %d, want %d", cancelResp.StatusCode, http.StatusNoContent)
			}
			select {
			case status := <-queryDone:
				if status < 0 {
					t.Error("cancelled query request failed at the transport level")
				}
			case <-time.After(5 * time.Second):
				t.Error("cancelled query did not return within 5s of the cancel")
			}
		})
	}
}
