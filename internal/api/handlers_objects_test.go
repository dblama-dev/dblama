package api

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/session"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type objectTestDriver struct {
	*stubDriver
	schema, source string
}

func (d *objectTestDriver) ListViews(ctx context.Context, _ string) ([]driver.ObjectInfo, error) {
	d.schema = driver.SchemaFromContext(ctx)
	return []driver.ObjectInfo{{Name: "view", Kind: "VIEW", Source: "CREATE VIEW view AS SELECT 1"}}, nil
}
func (d *objectTestDriver) AlterView(_ context.Context, _, _ string, def driver.ObjectDef) error {
	d.source = def.Source
	if def.Source == "wrong" {
		return driver.ErrObjectValidation
	}
	return nil
}
func (d *objectTestDriver) DropView(context.Context, string, string) error { return nil }
func objectRequest(t *testing.T, d driver.Driver, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	s := &apiServer{}
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := session.NewContext(r.Context(), &session.Session{Driver: d})
			ctx = driver.WithSchema(ctx, "Odd Schema")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	s.registerObjectRoutes(router)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}
func TestObjectsCapabilityAndPayload(t *testing.T) {
	unsupported := objectRequest(t, &stubDriver{}, "GET", "/databases/test/views", "")
	if unsupported.Code != 404 || !strings.Contains(unsupported.Body.String(), "driver_not_supported") {
		t.Fatal(unsupported.Code, unsupported.Body.String())
	}
	d := &objectTestDriver{stubDriver: &stubDriver{caps: driver.Caps{Views: true}}}
	listed := objectRequest(t, d, "GET", "/databases/test/views", "")
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), "CREATE VIEW") || d.schema != "Odd Schema" {
		t.Fatal(listed.Code, listed.Body.String(), d.schema)
	}
	for _, body := range []string{`{`, `{}`, `{"source":"wrong"}`} {
		r := objectRequest(t, d, "PUT", "/databases/test/views/view", body)
		if r.Code != 400 {
			t.Fatalf("%s: %d %s", body, r.Code, r.Body.String())
		}
	}
	saved := objectRequest(t, d, "PUT", "/databases/test/views/view", `{"source":"CREATE VIEW view AS SELECT 1"}`)
	if saved.Code != 204 {
		t.Fatal(saved.Code, saved.Body.String())
	}
	// A driver implementing an extension still needs to advertise the capability.
	d.caps.Views = false
	denied := objectRequest(t, d, "GET", "/databases/test/views", "")
	if denied.Code != 404 {
		t.Fatal(denied.Code)
	}
}
