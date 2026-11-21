// Package dblama exposes the frontend assets embedded in the executable.
package dblama

import "embed"

// WebFS contains the built Vue application under frontend/dist.
//
//go:embed all:frontend/dist
var WebFS embed.FS
