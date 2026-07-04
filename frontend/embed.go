// Package frontend embeds the compiled SvelteKit UI so the Go binary can
// serve it without a Node runtime. Run `pnpm build` here (or `make build` at
// the repo root) to populate build/ before compiling; without it the binary
// still builds but serves a "frontend not built" notice instead of the UI.
package frontend

import (
	"io/fs"
	"net/http"
	"strings"

	"embed"
)

//go:embed all:build
var buildFS embed.FS

// Handler serves the embedded UI: real files by path, index.html for
// everything else so client-side routes survive refreshes and deep links.
func Handler() http.Handler {
	dist, err := fs.Sub(buildFS, "build")
	if err != nil {
		panic(err) // "build" is the embed root above; cannot fail
	}

	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "frontend not built: run `pnpm build` in frontend/ and rebuild the binary", http.StatusNotImplemented)
		})
	}

	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" && path != "index.html" {
			if info, err := fs.Stat(dist, path); err == nil && !info.IsDir() {
				// Vite fingerprints everything under _app/immutable, so it
				// can be cached forever.
				if strings.HasPrefix(path, "_app/immutable/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}
