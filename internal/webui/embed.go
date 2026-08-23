// Package webui serves the compiled Dispatch browser application embedded in
// the box-dispatch executable.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// dist is produced by `bun run build` in web/ and committed so a normal Go
// build produces a self-contained executable.
//
//go:embed all:dist
var dist embed.FS

// Handler serves the compiled application and falls back to index.html for
// client-side routes.
func Handler() http.Handler {
	assets, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	index, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		assetPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if assetPath == "." || assetPath == "" {
			assetPath = "index.html"
		}
		if _, err := fs.Stat(assets, assetPath); err == nil {
			files.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if r.Method == http.MethodGet {
			_, _ = w.Write(index)
		}
	})
}
