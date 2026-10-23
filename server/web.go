// Package server exposes the embedded web frontend assets shipped with
// the db-manager binary.
package server

import "embed"

// WebFS holds the embedded contents of the web directory: the SPA's
// index.html and its bundled assets. It is rooted at web/; the HTTP layer
// serves the sub-FS fs.Sub(WebFS, "web").
//
//go:embed all:web
var WebFS embed.FS
