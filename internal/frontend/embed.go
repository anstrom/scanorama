// Package frontend provides the embedded frontend filesystem for production builds.
// The dist/ subdirectory is populated by running `make frontend` (npm run build).
// If only the placeholder index.html is present, the server will display a
// "frontend not built" page — run `make frontend` or `make build-all` first.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var files embed.FS

// FS returns the embedded frontend filesystem rooted at the dist/ subdirectory.
// The returned fs.FS is suitable for passing to api.WithFrontend.
func FS() fs.FS {
	sub, err := fs.Sub(files, "dist")
	if err != nil {
		// This can only happen if the embed directive above is broken.
		panic("frontend: embedded dist directory missing; run 'make frontend' before 'go build'")
	}
	return sub
}
