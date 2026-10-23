package api

import (
	"io/fs"
	"net/http"
	"strings"
)

// SPAHandler serves the single-page web frontend from webFS, which must
// be rooted at the web content directory (index.html at its root; main
// passes fs.Sub(server.WebFS, "web")). Existing files are served with
// their proper Content-Type; every other path — / and unknown deep links
// such as /server — falls back to index.html so the frontend router can
// resolve it client-side. A nil webFS disables the SPA: unknown paths
// answer with the JSON not_found envelope instead.
func SPAHandler(webFS fs.FS) http.Handler {
	if webFS == nil {
		return http.HandlerFunc(notFound)
	}
	files := http.FileServerFS(webFS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve real files as-is; directories fall through to the index
		// so no directory listing is ever exposed.
		if p := strings.TrimPrefix(r.URL.Path, "/"); p != "" {
			if info, err := fs.Stat(webFS, p); err == nil && !info.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
		}
		http.ServeFileFS(w, r, webFS, "index.html")
	})
}
