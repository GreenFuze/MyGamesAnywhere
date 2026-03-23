package http

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// spaFallbackHandler serves static files from rootDir and falls back to index.html for SPA routes.
func spaFallbackHandler(rootDir string) http.Handler {
	rootDir = filepath.Clean(rootDir)
	index := filepath.Join(rootDir, "index.html")
	fs := http.FileServer(http.Dir(rootDir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := filepath.Clean(r.URL.Path)
		if strings.Contains(p, "..") {
			http.NotFound(w, r)
			return
		}
		full := filepath.Join(rootDir, strings.TrimPrefix(p, "/"))
		fi, err := os.Stat(full)
		if err == nil && !fi.IsDir() {
			// Avoid long-lived stale branding assets when developers swap public/* (browser cache).
			if strings.HasSuffix(strings.ToLower(p), ".png") ||
				strings.HasSuffix(strings.ToLower(p), ".ico") ||
				strings.HasSuffix(strings.ToLower(p), ".svg") {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, index)
	})
}

// MountSPA registers a catch-all route for the built Vite app (after /api and /health).
func MountSPA(r chi.Router, rootDir string) {
	if rootDir == "" {
		return
	}
	index := filepath.Join(filepath.Clean(rootDir), "index.html")
	if st, err := os.Stat(index); err != nil || st.IsDir() {
		return
	}
	r.Handle("/*", spaFallbackHandler(rootDir))
}
