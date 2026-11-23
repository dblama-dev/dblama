package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/session"
)

// loginRequest is the POST /api/session body.
type loginRequest struct {
	Driver   string `json:"driver"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

// sessionInfo is the GET /api/session body; Database is null when the
// login did not select a database.
type sessionInfo struct {
	Driver   driver.DriverInfo `json:"driver"`
	Caps     driver.Caps       `json:"caps"`
	Database *string           `json:"database"`
}

// handleLogin connects to the requested server and creates a session.
func (s *apiServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	// Cap the body at 1 MiB: login requests are tiny, and an unbounded
	// decode lets a client exhaust server memory. An oversized body fails
	// the decode with *http.MaxBytesError and is answered as bad_request.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("malformed JSON body"))
		return
	}

	meta, ok := s.driverMeta(req.Driver)
	if !ok {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest(fmt.Sprintf("unknown driver %q", req.Driver)))
		return
	}

	port := req.Port
	if port == "" {
		port = meta.DefaultPort
	}
	if port == "" {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("missing port"))
		return
	}
	if !s.isTrustedServer(req.Host, port) {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("server "+req.Host+":"+port+" is not in TRUSTED_SERVERS"))
		return
	}

	drv, err := s.drivers.Connect(r.Context(), req.Driver, driver.ConnConfig{
		Host:     req.Host,
		Port:     port,
		User:     req.User,
		Password: req.Password,
		Database: req.Database,
	})
	if err != nil {
		s.limiter.Hit(clientIP(r))
		WriteErr(w, http.StatusUnauthorized, ErrAuthFailed(err.Error()))
		return
	}

	sess := s.sessions.Create(drv, req.Database)
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sess.ID,
		HttpOnly: true,
		Secure:   s.cfg.SessionCookieSecure,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})
	w.WriteHeader(http.StatusNoContent)
}

// handleGetSession reports the connected server, its caps and the current
// database.
func (s *apiServer) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())

	info := sessionInfo{Driver: sess.Driver.Info(), Caps: sess.Driver.Caps()}
	if sess.Database != "" {
		db := sess.Database
		info.Database = &db
	}
	WriteJSON(w, http.StatusOK, info)
}

// handleLogout deletes the session and closes its driver connection.
func (s *apiServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookieName); err == nil {
		s.sessions.Delete(c.Value)
	}
	w.WriteHeader(http.StatusNoContent)
}

// driverMeta returns the meta registered under id.
func (s *apiServer) driverMeta(id string) (driver.DriverMeta, bool) {
	for _, m := range s.drivers.List() {
		if m.ID == id {
			return m, true
		}
	}
	return driver.DriverMeta{}, false
}

// isTrustedServer reports whether host:port is an allowed connection
// target. An empty TRUSTED_SERVERS disables the check. Entries are
// compared after trimming, and empty entries (e.g. from a trailing comma)
// never match; the comparison uses the final port, so a request with an
// omitted port is checked against the driver's default.
func (s *apiServer) isTrustedServer(host, port string) bool {
	if len(s.cfg.TrustedServers) == 0 {
		return true
	}
	target := host + ":" + port
	for _, entry := range s.cfg.TrustedServers {
		if strings.TrimSpace(entry) == target {
			return true
		}
	}
	return false
}

// clientIP returns the host part of r.RemoteAddr, or the address as-is
// when it has no port.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
