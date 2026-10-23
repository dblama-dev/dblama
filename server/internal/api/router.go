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
// webFS serves the SPA fallback and is wired in a later task; it is
// accepted and ignored for now.
func NewRouter(cfg config.Config, sessions *session.Store, drivers *driver.Registry, webFS fs.FS) http.Handler {
	s := &apiServer{
		cfg:      cfg,
		sessions: sessions,
		drivers:  drivers,
		limiter:  ratelimit.NewLimiter(cfg.LoginRateLimit.Count, cfg.LoginRateLimit.Per, time.Now),
	}
	_ = webFS

	r := chi.NewRouter()
	r.Route("/api", func(api chi.Router) {
		// Public routes: driver list for the login screen, and login
		// itself. Login is the only rate-limited route; its limiter
		// runs before the CSRF check.
		api.Get("/drivers", s.handleDrivers)
		api.With(s.loginRateLimit, RequireCSRFHeader).Post("/session", s.handleLogin)

		// Everything else requires an authenticated session.
		api.Group(func(authed chi.Router) {
			authed.Use(RequireCSRFHeader, Auth(sessions))
			authed.Get("/session", s.handleGetSession)
			authed.Delete("/session", s.handleLogout)
			authed.Get("/databases", s.handleDatabases)
		})

		api.NotFound(notFound)
	})
	r.NotFound(notFound)

	return r
}

// notFound answers with the JSON not_found envelope; it covers unknown
// /api paths (until the SPA fallback takes over the rest in a later task).
func notFound(w http.ResponseWriter, _ *http.Request) {
	WriteErr(w, http.StatusNotFound, ErrNotFound())
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
