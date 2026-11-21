//go:build integration

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqldriver "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/dblama-dev/dblama/internal/config"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/driver/mysql"
	"github.com/dblama-dev/dblama/internal/session"
)

// integrationImages is the phase 1 matrix: image → expected session flavor.
var integrationImages = []struct {
	image  string
	flavor string
}{
	{"mysql:8.4", mysql.FlavorMySQL},
	{"mariadb:11.4", mysql.FlavorMariaDB},
}

// startIntegrationDB boots image as a testcontainer and returns the root
// ConnConfig (password dblama-test). Same pattern as the mysql package
// integration tests; skips in -short mode.
func startIntegrationDB(t *testing.T, image string) driver.ConnConfig {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	ctx := context.Background()
	ctr, err := tcmysql.Run(ctx, image,
		tcmysql.WithUsername("root"),
		tcmysql.WithPassword("dblama-test"),
		testcontainers.WithWaitStrategy(wait.ForLog("port: 3306")),
	)
	if err != nil {
		t.Fatalf("start %s container: %v", image, err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	dsn, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("container connection string: %v", err)
	}
	parsed, err := sqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse container connection string: %v", err)
	}
	host, port, err := net.SplitHostPort(parsed.Addr)
	if err != nil {
		t.Fatalf("split container address: %v", err)
	}
	return driver.ConnConfig{Host: host, Port: port, User: parsed.User, Password: parsed.Passwd}
}

// TestRouterIntegration_LoginFlow drives the whole session lifecycle over
// HTTP against a real container: login → GET session → GET databases →
// logout → GET session 401.
func TestRouterIntegration_LoginFlow(t *testing.T) {
	for _, tc := range integrationImages {
		t.Run(tc.image, func(t *testing.T) {
			dbCfg := startIntegrationDB(t, tc.image)

			// Registry mirrors the production shape: both driver IDs share
			// one mysql.Connect (wired fully in main.go by Task 8).
			reg := driver.NewRegistry()
			reg.Register(driver.DriverMeta{ID: "mysql", Name: "MySQL", DefaultPort: "3306"}, mysql.Connect)
			reg.Register(driver.DriverMeta{ID: "mariadb", Name: "MariaDB", DefaultPort: "3306"}, mysql.Connect)

			store := session.NewStore(time.Minute, 100, time.Now)
			h := NewRouter(config.Config{LoginRateLimit: config.Rate{Count: 10, Per: time.Minute}}, store, reg, nil)
			srv := httptest.NewServer(h)
			t.Cleanup(srv.Close)

			login := fmt.Sprintf(`{"driver":"mysql","host":%q,"port":%q,"user":"root","password":"dblama-test"}`,
				dbCfg.Host, dbCfg.Port)

			// Login.
			resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/session", login, "")
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusNoContent)
			}
			cookie := sessionCookie(t, resp).Value

			// Session info reports the server flavor.
			resp = apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/session", "", cookie)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET session status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			var info struct {
				Driver driver.DriverInfo `json:"driver"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
				t.Fatalf("decode session body: %v", err)
			}
			if info.Driver.Flavor != tc.flavor {
				t.Errorf("session flavor = %q, want %q", info.Driver.Flavor, tc.flavor)
			}

			// Database list contains the mysql system schema.
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
			found := false
			for _, db := range got.Databases {
				if db.Name == "mysql" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("databases list %v does not contain %q", got.Databases, "mysql")
			}

			// Logout, then the session is gone.
			resp = apiRequest(t, srv.Client(), http.MethodDelete, srv.URL+"/api/session", "", cookie)
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("DELETE session status = %d, want %d", resp.StatusCode, http.StatusNoContent)
			}
			resp = apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/session", "", cookie)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("GET session after logout status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
			}
		})
	}
}
