package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// all: is required, not stylistic — before `npm run build` has ever run,
// web/dist contains only the tracked .gitkeep (a dot-file), and a plain
// (non-"all:") embed pattern excludes dot/underscore-prefixed files. With
// nothing else to match, `//go:embed dist` would fail to compile with
// "pattern dist: no matching files found". `all:dist` always has at least
// .gitkeep to embed, so this package always compiles — go build/vet never
// depends on the frontend having been built first.
//
//go:embed all:dist
var distFS embed.FS

// Handler serves the built SPA (the contents of web/dist, produced by
// `npm run build`) from the embedded filesystem. Any request path that
// doesn't match a real file falls back to index.html, so client-side
// routes (e.g. /channels/5) resolve to the SPA instead of 404ing.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}
