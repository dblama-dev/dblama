package api

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fkTestDriver struct {
	*stubDriver
	keys   []driver.ForeignKey
	schema string
}

func (d *fkTestDriver) SaveForeignKeys(ctx context.Context, _, _ string, keys []driver.ForeignKey) error {
	d.keys = keys
	d.schema = driver.SchemaFromContext(ctx)
	return nil
}
func TestForeignKeysValidationAndSchema(t *testing.T) {
	d := &fkTestDriver{stubDriver: &stubDriver{}}
	s := &apiServer{}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := session.NewContext(req.Context(), &session.Session{Driver: d})
			ctx = driver.WithSchema(ctx, "Odd Schema")
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	s.registerForeignKeyRoutes(r)
	request := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("POST", "/databases/test/tables/child/foreign-keys", strings.NewReader(body)))
		return rec
	}
	for _, body := range []string{"null", `[{"name":"fk","columns":["a"],"refTable":"parent","refColumns":["id"],"onDelete":"CASCADE; DROP TABLE t"}]`} {
		rec := request(body)
		if rec.Code != 400 {
			t.Fatalf("%s: %d", body, rec.Code)
		}
	}
	rec := request(`[{"name":"fk","columns":["a","b"],"refTable":"parent","refColumns":["x","y"]}]`)
	if rec.Code != 204 || len(d.keys) != 1 || d.schema != "Odd Schema" {
		t.Fatalf("%d keys=%#v schema=%s", rec.Code, d.keys, d.schema)
	}
	rec = request(`[]`)
	if rec.Code != 204 || len(d.keys) != 0 {
		t.Fatalf("clear %d %#v", rec.Code, d.keys)
	}
}
