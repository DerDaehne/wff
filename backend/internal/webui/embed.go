// Package webui serves the SvelteKit frontend's static build, embedded into
// the backend binary so a single process (and container) serves both the
// API and the PWA — no separate nginx/static-file service (#573).
//
// dist/ is a git-tracked placeholder by default (see dist/index.html); the
// real build output is copied over it before compiling, either by
// `nix build .#backend` (see flake.nix) or manually via
// `cp -r ../frontend/build/* internal/webui/dist/` for local testing.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded static build, falling back to index.html for
// any path that isn't a real file — the SPA's own client-side router
// resolves those (see adapter-static's fallback in the frontend).
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // dist/ is always embedded at compile time; this can't fail at runtime
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if len(path) > 0 && path[0] == '/' {
			path = path[1:]
		}
		if path == "" {
			path = "."
		}
		if info, err := fs.Stat(sub, path); err != nil || info.IsDir() {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
