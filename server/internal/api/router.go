package api

import (
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/hitslab/db-manager/server/internal/api/ratelimit"
	"github.com/hitslab/db-manager/server/internal/config"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
)

// apiServer carries the dependencies every handler needs.
type apiServer struct {
	cfg      config.Config
	sessions *session.Store
	drivers  *driver.Registry
	limiter  *ratelimit.Limiter
}

// NewRouter wires the /api routes with their middleware. Middleware order:
// SecurityHeaders, then the login rate limit (only on POST /api/session),
// then the CSRF header check, then session auth on the non-public routes.
// webFS is the embedded web content rooted at its index.html; every
// non-/api path is served by the SPA fallback (see SPAHandler).
func NewRouter(cfg config.Config, sessions *session.Store, drivers *driver.Registry, webFS fs.FS) http.Handler {
	s := &apiServer{
		cfg:      cfg,
		sessions: sessions,
		drivers:  drivers,
		limiter:  ratelimit.NewLimiter(cfg.LoginRateLimit.Count, cfg.LoginRateLimit.Per, time.Now),
	}

	r := chi.NewRouter()
	// SecurityHeaders is the outermost middleware: it covers /api and
	// the root-level handlers (fallback, NotFound, MethodNotAllowed), so
	// every response carries the baseline security headers.
	r.Use(SecurityHeaders)
	r.MethodNotAllowed(methodNotAllowed)

	r.Route("/api", func(api chi.Router) {
		// Public routes: driver list for the login screen, and login
		// itself. Login is the only rate-limited route; its limiter
		// runs before the CSRF check.
		api.Get("/drivers", s.handleDrivers)
		api.With(s.loginRateLimit, RequireCSRFHeader).Post("/session", s.handleLogin)

		// Everything else requires an authenticated session.
		api.Group(func(authed chi.Router) {
			authed.Use(RequireCSRFHeader, Auth(sessions), requestSchema)
			authed.Get("/session", s.handleGetSession)
			authed.Delete("/session", s.handleLogout)
			authed.Get("/databases", s.handleDatabases)
			s.registerDatabaseRoutes(authed)
			s.registerAdminRoutes(authed)
			s.registerObjectRoutes(authed)
			s.registerForeignKeyRoutes(authed)

			// Console: raw SQL execution, cancel and EXPLAIN. Every
			// route is a POST and already covered by the group's
			// RequireCSRFHeader.
			authed.Post("/databases/{db}/query", s.handleQuery)
			authed.Post("/query/{queryID}/cancel", s.handleCancel)
			authed.Post("/query", s.handleExplain)

			// Dump export and data import.
			authed.Post("/databases/{db}/export", s.handleExport)
			authed.Post("/databases/{db}/import", s.handleImport)

			// Schema and sequence capabilities (PostgreSQL); drivers
			// without them answer driver_not_supported.
			authed.Get("/schemas", s.handleSchemasList)
			authed.Get("/sequences", s.handleSequencesList)
			authed.Post("/sequences", s.handleSequenceCreate)
			authed.Route("/sequences/{name}", func(seq chi.Router) {
				seq.Delete("/", s.handleSequenceDrop)
				seq.Patch("/", s.handleSequenceUpdate)
			})

			// Tables and rows of one database. Every non-GET route is
			// already covered by the group's RequireCSRFHeader.
			authed.Route("/databases/{db}/tables", func(tables chi.Router) {
				tables.Get("/", s.handleTablesList)
				tables.Post("/", s.handleBulkTables)
				tables.Route("/{table}", func(t chi.Router) {
					t.Get("/", s.handleTableDef)
					t.Put("/", s.handleSaveTable)
					t.Delete("/", s.handleDropTable)
					t.Post("/truncate", s.handleTruncateTable)
					t.Post("/indexes", s.handleSaveIndexes)
					t.Get("/rows", s.handleRowsSelect)
					t.Post("/rows", s.handleRowInsert)
					t.Post("/upsert", s.handleRowsUpsert)
					t.Post("/copy-rows", s.handleRowsCopy)
					t.Patch("/rows", s.handleRowUpdate)
					t.Delete("/rows", s.handleRowDelete)
					t.Get("/pk", s.handlePrimaryKey)
					t.Get("/create-sql", s.handleTableSQL)
				})
			})
		})

		api.NotFound(notFound)
		api.MethodNotAllowed(methodNotAllowed)
	})

	// Everything outside /api is the SPA: existing assets, with the index
	// as fallback for / and deep links. With a nil webFS the SPA is
	// disabled and unknown paths answer with the JSON envelope.
	r.NotFound(SPAHandler(webFS).ServeHTTP)

	return r
}

// notFound answers with the JSON not_found envelope; it covers unknown
// /api paths and non-/api paths when no web FS is mounted.
func notFound(w http.ResponseWriter, _ *http.Request) {
	WriteErr(w, http.StatusNotFound, ErrNotFound())
}

// methodNotAllowed answers 405 with the JSON envelope so every API error
// response keeps the unified shape (chi's default is plain text).
func methodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	WriteErr(w, http.StatusMethodNotAllowed, ErrBadRequest("method not allowed"))
}

// loginRateLimit rejects the request when its client IP has exhausted its
// failed-login allowance. It is mounted only on POST /api/session.
func (s *apiServer) loginRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.limiter.Allow(clientIP(r)) {
			WriteErr(w, http.StatusTooManyRequests, ErrRateLimited())
			return
		}
		next.ServeHTTP(w, r)
	})
}
