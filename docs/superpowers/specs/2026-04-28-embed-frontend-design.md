# Embed Frontend into Release Binary — Design Spec

## Goal

Bake the compiled React frontend into the Go release binary so that a single binary serves both the API and the web UI, without changing the source repository layout.

## Context

The frontend lives in `frontend/` and is built separately by Vite. Today, production deployments require distributing the binary **and** the built `frontend/dist/` directory out-of-band. This spec eliminates that second artifact.

## Approach

Use Go's `//go:embed` directive to embed the Vite build output into the binary. GoReleaser's `before.hooks` build the frontend and copy the output into `internal/frontend/dist/` before `go build` runs. Local development is unchanged: developers continue using the Vite dev server on port 5173.

## Architecture

### Source layout (unchanged)

```
frontend/          — React/TypeScript source
  src/
  vite.config.ts
  package.json
internal/
  frontend/
    dist/          — populated by GoReleaser before build; stub committed for local builds
      index.html   — minimal stub (keeps //go:embed from failing locally)
    embed.go       — declares //go:embed all:dist
    handler.go     — http.Handler that serves embedded SPA
```

### `.gitignore` addition

```
internal/frontend/dist/assets/
```

The stub `index.html` stays committed. Generated asset files (`assets/*.js`, `assets/*.css`) are gitignored — they are produced by the GoReleaser hook and are never in source control.

### `internal/frontend/embed.go`

```go
package frontend

import "embed"

//go:embed all:dist
var FS embed.FS
```

`all:` prefix ensures hidden files are included. `dist/` must contain at least one file at compile time — the committed stub satisfies this.

### `internal/frontend/handler.go`

Returns an `http.Handler` that:
1. Tries to open the exact request path from `FS`.
2. If found (asset), serves it directly with `http.FileServerFS`.
3. If not found, serves `dist/index.html` (React Router handles client-side routing).

```go
package frontend

import (
    "io/fs"
    "net/http"
)

func Handler() http.Handler {
    sub, err := fs.Sub(FS, "dist")
    if err != nil {
        panic("frontend: embed misconfigured: " + err.Error())
    }
    fileServer := http.FileServerFS(sub)
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, err := sub.(fs.StatFS).Stat(r.URL.Path[1:])
        if err != nil {
            // Unknown path → serve index.html for React Router
            http.ServeFileFS(w, r, sub, "index.html")
            return
        }
        fileServer.ServeHTTP(w, r)
    })
}
```

> **SPA routing invariant:** any path that doesn't match a known asset falls back to `index.html`. This is required because React Router manages client-side navigation — the server must not return 404 for deep links.

### `internal/api/routes.go` changes

Remove the existing root redirect:
```go
// REMOVE:
s.router.HandleFunc("/", s.redirectToAPI).Methods("GET")
```

Add the SPA catchall as the **last** registered route (gorilla/mux tries routes in registration order):
```go
s.router.PathPrefix("/").Handler(frontend.Handler())
```

The `/api/v1/` and `/swagger/` prefixes are registered before this line and continue to take priority.

### `.goreleaser.yml` changes

Add `npm ci && npm run build` to `before.hooks`, followed by a `cp` that places the build output where the embed directive expects it:

```yaml
before:
  hooks:
    - go mod tidy
    - bash -c "cd frontend && npm ci && npm run build"
    - bash -c "cp -r frontend/dist/. internal/frontend/dist/"
```

The `cp -r frontend/dist/.` (trailing dot) copies the directory contents, not a nested `dist/dist/`.

### `.github/workflows/release.yml` changes

Add a Node.js setup step before the GoReleaser step:

```yaml
- uses: actions/setup-node@v4
  with:
    node-version: '22'
    cache: 'npm'
    cache-dependency-path: frontend/package-lock.json
```

## Stub `index.html`

Replace the stale committed `internal/frontend/dist/index.html` with a minimal placeholder that makes it obvious the binary was not built with the frontend:

```html
<!doctype html>
<html><body>
<p>Frontend not embedded. Run a release build or use the Vite dev server.</p>
</body></html>
```

## Out of scope

- Hot-reload or watch mode for the embedded frontend — developers always use `make dev` with Vite
- Serving the embedded frontend in local `go build` output — local builds retain the stub
- Cache-control headers or ETags — standard `http.FileServerFS` headers are sufficient

## Testing plan (manual, post-implementation)

1. Run `goreleaser build --single-target --skip=validate` locally (requires Node 22).
2. Start the resulting binary with `./dist/scanorama_linux_amd64/scanorama serve`.
3. Open `http://localhost:8080/` — confirm the React UI loads.
4. Navigate to a deep link (e.g. `/hosts/some-id`) — confirm the page loads via React Router, not a 404.
5. Confirm API routes still respond: `curl http://localhost:8080/api/v1/hosts`.
6. Confirm Swagger UI still loads at `/docs`.
