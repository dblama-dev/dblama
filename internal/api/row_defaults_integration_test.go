//go:build integration

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dblama-dev/dblama/internal/config"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/driver/mysql"
	"github.com/dblama-dev/dblama/internal/driver/postgres"
	"github.com/dblama-dev/dblama/internal/session"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Tests the actual JSON handler, driver SQL, defaults and LastInsertID together.
// Every engine runs in a fresh disposable container created by this test.
func TestAPI_DefaultOnlyRowInsert(t *testing.T) {
	for _, engine := range []string{"mysql", "mariadb", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			if testing.Short() {
				t.Skip("integration test")
			}
			ctx := context.Background()
			var cfg driver.ConnConfig
			connect := mysql.Connect
			identity := "BIGINT AUTO_INCREMENT PRIMARY KEY"
			if engine == "postgres" {
				ctr, err := tcpostgres.Run(ctx, "postgres:17", tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("dblama-test"), tcpostgres.WithDatabase("postgres"), tcpostgres.BasicWaitStrategies())
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = ctr.Terminate(ctx) })
				dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
				if err != nil {
					t.Fatal(err)
				}
				address, err := url.Parse(dsn)
				if err != nil {
					t.Fatal(err)
				}
				password, _ := address.User.Password()
				cfg = driver.ConnConfig{Host: address.Hostname(), Port: address.Port(), User: address.User.Username(), Password: password, Database: "postgres"}
				connect = postgres.Connect
				identity = "BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY"
			} else {
				image := "mysql:8.4"
				if engine == "mariadb" {
					image = "mariadb:11.4"
				}
				cfg = startIntegrationDB(t, image)
				execAdminQuery(t, cfg, "CREATE DATABASE default_insert")
				cfg.Database = "default_insert"
			}
			setup, err := connect(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = setup.Close() })
			ddl := fmt.Sprintf("CREATE TABLE default_rows (id %s, status VARCHAR(30) NOT NULL DEFAULT 'active', created TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, note TEXT, base_value INTEGER NOT NULL DEFAULT 7, generated_value INTEGER GENERATED ALWAYS AS (base_value * 2) STORED); CREATE TABLE identity_only (id %s); CREATE TABLE no_identity (status VARCHAR(30) NOT NULL DEFAULT 'active')", identity, identity)
			if err := setup.ImportSQL(ctx, cfg.Database, strings.NewReader(ddl), nil); err != nil {
				t.Fatal(err)
			}

			registry := driver.NewRegistry()
			registry.Register(driver.DriverMeta{ID: engine, Name: engine}, connect)
			store := session.NewStore(time.Minute, 100, time.Now)
			srv := httptest.NewServer(NewRouter(config.Config{LoginRateLimit: config.Rate{Count: 10, Per: time.Minute}}, store, registry, nil))
			t.Cleanup(srv.Close)
			login, err := json.Marshal(map[string]string{"driver": engine, "host": cfg.Host, "port": cfg.Port, "user": cfg.User, "password": cfg.Password, "database": cfg.Database})
			if err != nil {
				t.Fatal(err)
			}
			response := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session", string(login), "")
			if response.StatusCode != http.StatusNoContent {
				t.Fatalf("login status %d", response.StatusCode)
			}
			cookie := sessionCookie(t, response).Value
			firstID := insertRow(t, srv.Client(), srv.URL, cookie, cfg.Database, "default_rows", `{}`)
			if firstID != 1 {
				t.Fatalf("default insert id %d, want 1", firstID)
			}
			secondID := insertRow(t, srv.Client(), srv.URL, cookie, cfg.Database, "default_rows", `{"status":"","note":null}`)
			if secondID != 2 {
				t.Fatalf("explicit empty/null insert id %d, want 2", secondID)
			}
			page := getRowsPage(t, srv.Client(), srv.URL, cookie, cfg.Database, "default_rows", "?order=id:asc")
			if len(page.Rows) != 2 {
				t.Fatalf("rows %#v", page.Rows)
			}
			column := func(name string) int {
				for i, col := range page.Columns {
					if col.Name == name {
						return i
					}
				}
				t.Fatalf("missing column %s", name)
				return -1
			}
			for i, row := range page.Rows {
				wantStatus := "active"
				if i == 1 {
					wantStatus = ""
				}
				if row[column("status")] != wantStatus || row[column("note")] != nil || fmt.Sprint(row[column("base_value")]) != "7" || fmt.Sprint(row[column("generated_value")]) != "14" || row[column("created")] == nil {
					t.Fatalf("default/empty/null values row %d: %#v", i, row)
				}
			}
			if id := insertRow(t, srv.Client(), srv.URL, cookie, cfg.Database, "identity_only", `{}`); id != 1 {
				t.Fatalf("identity-only id %d", id)
			}
			if id := insertRow(t, srv.Client(), srv.URL, cookie, cfg.Database, "no_identity", `{}`); id != 0 {
				t.Fatalf("non-identity id %d", id)
			}
			noID := getRowsPage(t, srv.Client(), srv.URL, cookie, cfg.Database, "no_identity", "")
			if len(noID.Rows) != 1 || noID.Rows[0][0] != "active" {
				t.Fatalf("non-identity default %#v", noID.Rows)
			}
		})
	}
}
