package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dblama-dev/dblama/internal/config"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/session"
)

type scopeTestDriver struct {
	*databaseOpsDriver
	target   *stubDriver
	opened   []string
	released int
}

func (d *scopeTestDriver) ForDatabase(_ context.Context, db string) (driver.Driver, func(), error) {
	d.opened = append(d.opened, db)
	if db != "selected / %" {
		return nil, nil, errors.New("cannot connect to requested database")
	}
	return d.target, func() { d.released++ }, nil
}

func TestDatabaseRequestsUseSelectedConnection(t *testing.T) {
	for _, login := range []string{"", "login_db"} {
		t.Run("login="+login, func(t *testing.T) {
			root := &scopeTestDriver{
				databaseOpsDriver: &databaseOpsDriver{stubDriver: &stubDriver{info: driver.DriverInfo{Flavor: "postgres"}}},
				target: &stubDriver{
					schemaAware: true, gotSchemas: []string{"selected_schema"},
					tables:       []driver.TableInfo{{Name: "selected_table"}},
					exportOutput: "selected dump", execResults: []driver.Result{{AffectedRows: 23}},
				},
			}
			store := session.NewStore(time.Minute, 10, time.Now)
			sess := store.Create(root, login)
			router := NewRouter(config.Config{}, store, driver.NewRegistry(), nil)
			request := func(method, path, body string, status int, contains string) {
				t.Helper()
				r := httptest.NewRequest(method, "/api"+path, strings.NewReader(body))
				r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sess.ID})
				r.Header.Set(csrfHeader, "XMLHttpRequest")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code != status || !strings.Contains(w.Body.String(), contains) {
					t.Errorf("%s %s returned %d %s, want %d containing %q", method, path, w.Code, w.Body.String(), status, contains)
				}
			}
			const base = "/databases/selected%20%2F%20%25"
			request("GET", base+"/tables?schema=one", "", 200, "selected_table")
			request("GET", base+"/schemas", "", 200, "selected_schema")
			request("POST", base+"/export", `{"format":"sql"}`, 200, "selected dump")
			request("POST", "/query?schema=one", `{"db":"selected / %","sql":"SELECT 1"}`, 200, "23")
			request("POST", "/databases/denied/query", `{"sql":"SELECT 1","queryID":"abc"}`, 500, "cannot connect")
			request("PATCH", base, `{"name":"renamed"}`, 204, "")
			if len(root.opened) != 5 || root.released != 4 {
				t.Errorf("opened=%v released=%d", root.opened, root.released)
			}
			if len(root.execCalls) != 0 || len(root.calls) != 1 || root.calls[0] != "rename:selected / %:renamed" {
				t.Errorf("login queries=%v database operations=%v", root.execCalls, root.calls)
			}
			if len(root.target.schemaCalls) != 1 || root.target.schemaCalls[0] != "one" || sess.Driver != root || sess.Database != login {
				t.Errorf("request changed login scope or lost schema: schema=%v session database=%q", root.target.schemaCalls, sess.Database)
			}
		})
	}
}

type waitingDatabaseDriver struct {
	*stubDriver
	queued    chan struct{}
	available chan struct{}
}

func (d *waitingDatabaseDriver) ForDatabase(ctx context.Context, _ string) (driver.Driver, func(), error) {
	close(d.queued)
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-d.available:
		return d, func() {}, nil
	}
}

func TestCancelWhileWaitingForSelectedDatabase(t *testing.T) {
	d := &waitingDatabaseDriver{stubDriver: &stubDriver{}, queued: make(chan struct{}), available: make(chan struct{})}
	store := session.NewStore(time.Minute, 10, time.Now)
	sess := store.Create(d, "")
	router := NewRouter(config.Config{}, store, driver.NewRegistry(), nil)
	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sess.ID})
		req.Header.Set(csrfHeader, "XMLHttpRequest")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		finished <- post("/api/databases/selected/query", `{"sql":"INSERT INTO jobs DEFAULT VALUES","queryID":"queued"}`)
	}()
	select {
	case <-d.queued:
	case <-time.After(time.Second):
		t.Fatal("query did not start acquiring the database")
	}
	w := post("/api/query/queued/cancel", "")
	if w.Code != 204 {
		t.Errorf("cancel status = %d", w.Code)
	}
	// A canceled query must finish before any database slot becomes available.
	select {
	case w := <-finished:
		if w.Code == 200 || len(d.execCalls) != 0 {
			t.Errorf("canceled query reached the driver: %v", d.execCalls)
		}
	case <-time.After(time.Second):
		close(d.available)
		<-finished
		t.Fatal("canceled query was still queued for execution")
	}
}
