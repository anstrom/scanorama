package api

import (
	"io/fs"
	"net/http"
	"os"
	"strings"
)

// notBuiltPage is served when the frontend has not been compiled into the binary
// (i.e. index.html is absent from the embedded FS). This happens when someone
// runs `go build` directly without running `make frontend` first.
const notBuiltPage = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Scanorama — Build Required</title>
    <style>
      body {
        font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
        background: #0d1117; color: #c9d1d9;
        display: flex; align-items: center; justify-content: center;
        height: 100vh; margin: 0;
      }
      .box { border: 1px solid #30363d; padding: 2rem 2.5rem; border-radius: 6px; max-width: 500px; }
      h1  { color: #58a6ff; margin-top: 0; font-size: 1.1rem; }
      p   { color: #8b949e; line-height: 1.6; margin: 0.75rem 0; }
      code { background: #161b22; padding: 0.2rem 0.5rem; border-radius: 3px; }
      a   { color: #58a6ff; }
    </style>
  </head>
  <body>
    <div class="box">
      <h1>Frontend not built</h1>
      <p>The frontend bundle has not been compiled yet.</p>
      <p>Build the frontend and binary together:</p>
      <p><code>make build-all</code></p>
      <p>Or build only the frontend assets:</p>
      <p><code>make frontend</code></p>
      <p>The REST API and Swagger UI are available at
         <a href="/api/v1/health">/api/v1/health</a> and
         <a href="/swagger/">/swagger/</a>.</p>
    </div>
  </body>
</html>`

// cspHeader is the Content-Security-Policy applied to every response served
// by the embedded frontend handler.
//
//   - script-src 'self'          — all JS is bundled by Vite; no CDN scripts
//   - style-src 'self' 'unsafe-inline' — Recharts and some UI libraries apply
//     inline styles at runtime, so unsafe-inline is required
//   - connect-src 'self'         — covers XHR and same-origin WebSocket (ws://)
//   - img-src 'self' data:       — favicons and any base64-encoded images
//   - frame-ancestors 'none'     — prevents clickjacking via iframes
const cspHeader = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"font-src 'self'; " +
	"frame-ancestors 'none'"

// resolveFrontendFS returns the fs.FS to use for serving the frontend.
//
// Resolution order:
//  1. If frontendDir is non-empty, serve from that directory on disk
//     (the --frontend-dir escape hatch for custom builds).
//  2. Otherwise, return the embedded FS (which may be nil when no frontend
//     was compiled into the binary).
func resolveFrontendFS(embedded fs.FS, frontendDir string) fs.FS {
	if frontendDir != "" {
		return os.DirFS(frontendDir)
	}
	return embedded
}

// newSPAHandler returns an http.Handler that serves a single-page application
// from fsys with proper cache and security headers.
//
// Cache strategy:
//   - Vite-hashed assets (e.g. assets/main-BF3wh5rq.js) → immutable, 1 year
//   - index.html and anything else → no-cache
//
// SPA fallback: any request whose path does not correspond to an existing file
// in fsys is served as index.html, allowing client-side routing to take over.
// Because the app uses hash-based routing (#/path), the server almost always
// just sees "/" and the fallback is mostly a safety net.
func newSPAHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServerFS(fsys)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Security headers on every response from this handler.
		w.Header().Set("Content-Security-Policy", cspHeader)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Normalise the path to a relative FS key.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Try to open the file; fall back to index.html on any error.
		f, err := fsys.Open(path)
		if err != nil {
			spaFallback(w, r, fsys)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			spaFallback(w, r, fsys)
			return
		}

		// Immutable long-term cache for Vite's content-hashed filenames.
		if isHashedAsset(path) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// index.html must always be re-fetched so clients pick up new
			// deployments immediately.
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}

		fileServer.ServeHTTP(w, r)
	})
}

// spaFallback serves index.html as the SPA shell for any unknown path.
// If index.html is not present in the embedded FS (because the frontend was
// never compiled), it falls back to the inline notBuiltPage constant so the
// user gets a helpful message instead of a cryptic 404.
func spaFallback(w http.ResponseWriter, r *http.Request, fsys fs.FS) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	content, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		// Frontend not built — serve the inline placeholder page.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(notBuiltPage))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// isHashedAsset reports whether path looks like a Vite-generated asset with a
// content hash in the filename (e.g. assets/index-CiE4CGKV.js).
// Vite's default pattern is <name>-<8-char-hash>.<ext> inside assets/.
func isHashedAsset(path string) bool {
	if !strings.HasPrefix(path, "assets/") {
		return false
	}
	base := path[strings.LastIndex(path, "/")+1:]
	dashIdx := strings.LastIndex(base, "-")
	dotIdx := strings.LastIndex(base, ".")
	if dashIdx < 0 || dotIdx <= dashIdx {
		return false
	}
	hash := base[dashIdx+1 : dotIdx]
	return len(hash) >= 8
}
