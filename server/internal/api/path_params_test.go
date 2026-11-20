package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitslab/db-manager/server/internal/config"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
)

type pathTestDriver struct {
	*adminStub
	relationsDB string
}

func (d *pathTestDriver) ListRelationships(_ context.Context, db string) ([]driver.TableRelationship, error) {
	d.relationsDB = db
	return nil, nil
}

func TestRouterDecodesIdentifierSegmentsExactlyOnce(t *testing.T) {
	d := &pathTestDriver{adminStub: &adminStub{stubDriver: &stubDriver{}}}
	store := session.NewStore(time.Minute, 10, time.Now)
	loggedIn := store.Create(d, "app")
	router := NewRouter(config.Config{}, store, driver.NewRegistry(), nil)
	request := func(method, path, body string) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: loggedIn.ID})
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code < 200 || w.Code >= 300 {
			t.Fatalf("%s %s: status %d, %s", method, path, w.Code, w.Body.String())
		}
	}
	for _, tc := range []struct{ path, want string }{
		{"A%26B", "A&B"}, {"plus%2Bapp", "plus+app"}, {"literal%252B", "literal%2B"}, {"sale%25", "sale%"},
	} {
		request("GET", "/api/databases/"+tc.path+"/relationships", "")
		if d.relationsDB != tc.want {
			t.Errorf("relationships for %q reached %q", tc.want, d.relationsDB)
		}
	}

	// Escaping a slash in the table keeps it in one segment. A separate
	// database segment containing a literal escape must decode just once.
	request("POST", "/api/databases/literal%252B/tables/a%2Fb/rows", `{"value":"raw%2F"}`)
	call := d.insertCalls[0]
	if call.db != "literal%2B" || call.table != "a/b" || call.vals["value"] != "raw%2F" {
		t.Errorf("insert target/payload changed: %#v", call)
	}

	request("DELETE", "/api/users/a%2Bb?host=literal%252B", "")
	if d.account.User != "a+b" || d.account.Host != "literal%2B" {
		t.Errorf("wrong account: %#v", d.account)
	}
}
