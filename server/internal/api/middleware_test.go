package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// fakeDriver is a driver.Driver stub that lets tests create real sessions
// in the store (same pattern as the session package tests).
type fakeDriver struct{}

var _ driver.Driver = (*fakeDriver)(nil)

func (f *fakeDriver) Info() driver.DriverInfo { return driver.DriverInfo{} }
func (f *fakeDriver) Caps() driver.Caps       { return driver.Caps{} }
func (f *fakeDriver) ListDatabases(context.Context) ([]driver.DatabaseInfo, error) {
	return nil, nil
}

func (f *fakeDriver) ListTables(context.Context, string) ([]driver.TableInfo, error) {
	return nil, nil
}

func (f *fakeDriver) TableDef(context.Context, string, string) (driver.TableDef, error) {
	return driver.TableDef{}, nil
}

func (f *fakeDriver) ShowCreateTable(context.Context, string, string) (string, error) {
	return "", nil
}

func (f *fakeDriver) SaveTable(context.Context, string, string, driver.TableDef) error {
	return nil
}

func (f *fakeDriver) SaveIndexes(context.Context, string, string, []driver.Index) error {
	return nil
}

func (f *fakeDriver) TruncateTables(context.Context, string, []string) error {
	return nil
}

func (f *fakeDriver) DropTables(context.Context, string, []string) error {
	return nil
}

func (f *fakeDriver) DropViews(context.Context, string, []string) error {
	return nil
}

func (f *fakeDriver) SelectRows(context.Context, string, string, sqlgen.SelectQuery) (driver.Page, error) {
	return driver.Page{}, nil
}

func (f *fakeDriver) PrimaryKey(context.Context, string, string) ([]string, error) {
	return nil, nil
}

func (f *fakeDriver) InsertRow(context.Context, string, string, driver.RowValues) error {
	return nil
}

func (f *fakeDriver) UpdateRow(context.Context, string, string, driver.PK, driver.RowValues) error {
	return nil
}

func (f *fakeDriver) DeleteRows(context.Context, string, string, []driver.PK) error {
	return nil
}

func (f *fakeDriver) UpsertRows(context.Context, string, string, []driver.RowValues) error {
	return nil
}

func (f *fakeDriver) ExecSQL(context.Context, string, string, string, driver.ExecOpts) ([]driver.Result, error) {
	return nil, nil
}

func (f *fakeDriver) Export(context.Context, driver.ExportRequest, io.Writer) error {
	return nil
}

func (f *fakeDriver) ImportSQL(context.Context, string, io.Reader, func(done, total int64)) error {
	return nil
}

func (f *fakeDriver) ImportCSV(context.Context, string, string, io.Reader, driver.CSVImportOpts, func(done, total int64)) error {
	return nil
}

func (f *fakeDriver) CancelQuery(context.Context, string) error {
	return nil
}

func (f *fakeDriver) LastInsertID(context.Context) (int64, error) {
	return 0, nil
}

func (f *fakeDriver) Close() error { return nil }

// errCode extracts the error code from an api error envelope body.
func errCode(t *testing.T, body []byte) string {
	t.Helper()
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("unmarshal body %q: %v", body, err)
	}
	return e.Error.Code
}

func TestSecurityHeaders_Set(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rr := httptest.NewRecorder()
	SecurityHeaders(next).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := rr.Header().Get(header); got != want {
			t.Errorf("header %q = %q, want %q", header, got, want)
		}
	}
}

func TestAuthMiddleware_RejectsUnknownSession(t *testing.T) {
	store := session.NewStore(time.Minute, 100, time.Now)
	handler := Auth(store)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("next handler called without a valid session")
	}))

	noCookie := httptest.NewRequest(http.MethodGet, "/", nil)
	invalidCookie := httptest.NewRequest(http.MethodGet, "/", nil)
	invalidCookie.AddCookie(&http.Cookie{Name: "dbm_session", Value: "not-a-real-id"})

	for name, req := range map[string]*http.Request{
		"no cookie":      noCookie,
		"invalid cookie": invalidCookie,
	} {
		t.Run(name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
			}
			if code := errCode(t, rr.Body.Bytes()); code != "auth_required" {
				t.Errorf("error code = %q, want %q", code, "auth_required")
			}
		})
	}
}

func TestAuthMiddleware_PassesSession(t *testing.T) {
	store := session.NewStore(time.Minute, 100, time.Now)
	created := store.Create(&fakeDriver{}, "appdb")

	var got *session.Session
	handler := Auth(store)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var ok bool
		got, ok = session.FromContext(r.Context())
		if !ok {
			t.Error("FromContext(r.Context()) ok = false inside handler")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "dbm_session", Value: created.ID})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got != created {
		t.Errorf("session from context = %v, want the created session", got)
	}
}

func TestCSRFMiddleware_RejectsMissingHeader(t *testing.T) {
	handler := RequireCSRFHeader(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("next handler called for request without CSRF header")
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/", nil))

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
	if code := errCode(t, rr.Body.Bytes()); code != "bad_request" {
		t.Errorf("error code = %q, want %q", code, "bad_request")
	}
}

func TestCSRFMiddleware_AllowsWithHeader(t *testing.T) {
	handler := RequireCSRFHeader(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("post with header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
		}
	})

	// GET and HEAD are safe methods and must pass without the header.
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method+" without header", func(t *testing.T) {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest(method, "/", nil))
			if rr.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
			}
		})
	}
}
