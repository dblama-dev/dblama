package api

import (
	"context"
	"net/http"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/session"
)

// SessionCookieName is the name of the cookie carrying the session ID.
const SessionCookieName = "dblama_session"

// csrfHeader must be present (non-empty) on state-changing requests.
const csrfHeader = "X-Requested-With"

// SecurityHeaders sets the baseline browser security response headers on
// every response before delegating to next.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func databaseDriver(ctx context.Context, current driver.Driver, database string) (driver.Driver, func(), error) {
	if connector, ok := current.(driver.DatabaseConnector); ok {
		return connector.ForDatabase(ctx, database)
	}
	return current, func() {}, nil
}

// Keep database selection local to the request so tabs using one session can
// work on different databases at the same time.
func requestDatabase(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, _ := session.FromContext(r.Context())
		name := pathParam(r, "db")
		d, release, err := databaseDriver(r.Context(), sess.Driver, name)
		if err != nil {
			writeDriverErr(w, err)
			return
		}
		defer release()
		scoped := *sess
		scoped.Driver = d
		scoped.Database = name
		next.ServeHTTP(w, r.WithContext(session.NewContext(r.Context(), &scoped)))
	})
}

// Auth returns middleware that requires a valid dblama_session cookie. When
// the cookie is missing or the session is unknown or expired, the request
// is answered with a 401 auth_required envelope. Otherwise the session is
// stored in the request context (see session.NewContext) and the request
// proceeds.
func Auth(sessions *session.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(SessionCookieName)
			if err != nil {
				WriteErr(w, http.StatusUnauthorized, ErrAuthRequired())
				return
			}
			sess, ok := sessions.Get(c.Value)
			if !ok {
				WriteErr(w, http.StatusUnauthorized, ErrAuthRequired())
				return
			}
			next.ServeHTTP(w, r.WithContext(session.NewContext(r.Context(), sess)))
		})
	}
}

// RequireCSRFHeader rejects state-changing requests (anything other than
// GET and HEAD) that do not carry a non-empty X-Requested-With header with
// a 403 bad_request envelope. The frontend sets this custom header on its
// requests, which cross-site form posts cannot do.
func RequireCSRFHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get(csrfHeader) == "" {
			WriteErr(w, http.StatusForbidden, ErrBadRequest("missing "+csrfHeader+" header"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
