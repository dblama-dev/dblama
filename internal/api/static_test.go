package api

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/dblama-dev/dblama/internal/config"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/session"
)

// indexHTML mirrors frontend/dist/index.html: the placeholder the SPA build
// will replace.
const indexHTML = "<!doctype html>\n<html><body>build the frontend</body></html>\n"

// webTestFS returns a fake web FS shaped like fs.Sub(WebFS, "frontend/dist"): the
// index at its root plus one bundled asset.
func webTestFS() fs.FS {
	return fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte(indexHTML)},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('app')\n")},
	}
}

// newSPARouter builds a full router like newTestRouter but with the web FS
// mounted, exercising the real SPA fallback wiring.
func newSPARouter(t *testing.T) http.Handler {
	t.Helper()
	reg := driver.NewRegistry()
	reg.Register(driver.DriverMeta{ID: "fakedb", Name: "FakeDB", DefaultPort: "3307"},
		stubConnectOK(nil, &stubDriver{}))
	store := session.NewStore(time.Minute, 100, time.Now)
	return NewRouter(config.Config{LoginRateLimit: testRate}, store, reg, webTestFS())
}

func TestSPA_ServesIndex(t *testing.T) {
	h := newSPARouter(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("GET / Content-Type = %q, want %q", ct, "text/html; charset=utf-8")
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<!doctype html>") {
		t.Errorf("GET / body = %q, want it to contain <!doctype html>", body)
	}

	// SecurityHeaders is the outermost middleware, so the SPA fallback
	// response must carry the baseline headers too.
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := resp.Header.Get(header); got != want {
			t.Errorf("GET /: header %q = %q, want %q", header, got, want)
		}
	}
}

func TestSPA_DeepLink(t *testing.T) {
	h := newSPARouter(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// Unknown non-/api paths serve the index so the frontend router can
	// resolve them client-side.
	resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/server", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /server status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("GET /server Content-Type = %q, want %q", ct, "text/html; charset=utf-8")
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<!doctype html>") {
		t.Errorf("GET /server body = %q, want the index page", body)
	}
}

func TestSPA_ServesAssets(t *testing.T) {
	h := newSPARouter(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// Files that exist in the web FS are served as-is with the proper
	// Content-Type, not replaced by the index.
	resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/assets/app.js", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /assets/app.js status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("GET /assets/app.js Content-Type = %q, want text/javascript", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "console.log") {
		t.Errorf("GET /assets/app.js body = %q, want the asset contents", body)
	}
}

func TestAPI_UnknownPath_JSON404(t *testing.T) {
	h := newSPARouter(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// The SPA fallback must not swallow unknown /api paths: they keep the
	// JSON not_found envelope, never index.html.
	resp := apiRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/nope", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/nope status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("GET /api/nope Content-Type = %q, want application/json", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if code := errCode(t, body); code != "not_found" {
		t.Errorf("GET /api/nope error code = %q, want %q", code, "not_found")
	}
	if strings.Contains(string(body), "<!doctype html>") {
		t.Errorf("GET /api/nope returned the SPA index, want the JSON envelope")
	}
}
