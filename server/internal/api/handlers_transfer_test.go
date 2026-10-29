package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hitslab/db-manager/server/internal/config"
	"github.com/hitslab/db-manager/server/internal/driver"
)

func TestTransferExportSelectedMySQLDatabase(t *testing.T) {
	for _, login := range []string{"", "login_db"} {
		t.Run(login, func(t *testing.T) {
			fake := &stubDriver{info: driver.DriverInfo{Name: "MySQL", Flavor: "mysql"}, exportOutput: "selected dump"}
			srv := httptest.NewServer(newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake)))
			defer srv.Close()
			cookie := loginSessionDB(t, srv, login)
			resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/selected/export", `{"format":"sql","mode":"data"}`, cookie)
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != 200 {
				t.Fatalf("status %d: %s", resp.StatusCode, body)
			}
			if len(fake.exportCalls) != 1 || fake.exportCalls[0].db != "selected" {
				t.Fatalf("calls: %+v", fake.exportCalls)
			}
		})
	}
}
func TestTransferExportRejectInvalidMode(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "MySQL", Flavor: "mysql"}}
	srv := httptest.NewServer(newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake)))
	defer srv.Close()
	cookie := loginSessionDB(t, srv, "appdb")
	resp := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/export", `{"format":"sql","mode":"bad"}`, cookie)
	defer resp.Body.Close()
	if resp.StatusCode != 400 || len(fake.exportCalls) != 0 {
		t.Fatalf("status %d, calls %d", resp.StatusCode, len(fake.exportCalls))
	}
}

func TestTransferCSVColumnsMultipart(t *testing.T) {
	for _, tc := range []struct {
		name, mapping string
		status        int
	}{
		{"mapped", `["name","","id"]`, 200},
		{"not array", `{"name":"id"}`, 400},
		{"null", `null`, 400},
		{"invalid element", `[1]`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &stubDriver{info: driver.DriverInfo{Name: "MySQL", Flavor: "mysql"}}
			srv := httptest.NewServer(newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake)))
			defer srv.Close()
			cookie := loginSessionDB(t, srv, "appdb")
			resp := multipartImport(t, srv, cookie, map[string]string{"type": "csv", "table": "items", "columns": tc.mapping}, []byte("rows.csv"), []byte("name,ignored,1\n"))
			defer resp.Body.Close()
			io.ReadAll(resp.Body)
			if resp.StatusCode != tc.status {
				t.Fatalf("status %d", resp.StatusCode)
			}
			if tc.status == 400 {
				if len(fake.importCSVCalls) != 0 {
					t.Fatal("invalid mapping reached importer")
				}
				return
			}
			if len(fake.importCSVCalls) != 1 {
				t.Fatalf("calls %d", len(fake.importCSVCalls))
			}
			got := fake.importCSVCalls[0].opts.Columns
			if len(got) != 3 || got[0] != "name" || got[1] != "" || got[2] != "id" {
				t.Fatalf("mapping %v", got)
			}
		})
	}
}

func TestTransferCSVEncodingValidation(t *testing.T) {
	fake := &stubDriver{info: driver.DriverInfo{Name: "MySQL", Flavor: "mysql"}}
	srv := httptest.NewServer(newTestRouter(t, config.Config{LoginRateLimit: testRate}, stubConnectOK(nil, fake)))
	defer srv.Close()
	cookie := loginSessionDB(t, srv, "appdb")
	response := apiRequest(t, srv.Client(), http.MethodPost, srv.URL+"/api/databases/appdb/export", `{"format":"csv","tables":["t"],"csvEncoding":"unknown"}`, cookie)
	io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 400 || len(fake.exportCalls) != 0 {
		t.Fatalf("invalid encoding export status=%d calls=%d", response.StatusCode, len(fake.exportCalls))
	}
	for _, encoding := range []string{"literal", "escaped", "unknown"} {
		resp := multipartImport(t, srv, cookie, map[string]string{"type": "csv", "table": "t", "columns": `["value"]`, "csvEncoding": encoding}, []byte("rows.csv"), []byte("value\n"))
		io.ReadAll(resp.Body)
		resp.Body.Close()
		if encoding == "unknown" {
			if resp.StatusCode != 400 {
				t.Fatalf("invalid import encoding status=%d", resp.StatusCode)
			}
			continue
		}
		if resp.StatusCode != 200 || fake.importCSVCalls[len(fake.importCSVCalls)-1].opts.CSVEncoding != encoding {
			t.Fatalf("encoding %q status=%d calls=%+v", encoding, resp.StatusCode, fake.importCSVCalls)
		}
	}
	if len(fake.importCSVCalls) != 2 {
		t.Fatalf("invalid encoding reached importer: %+v", fake.importCSVCalls)
	}
}
