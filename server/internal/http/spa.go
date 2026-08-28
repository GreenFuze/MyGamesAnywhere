package http

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	frontenddist "github.com/GreenFuze/MyGamesAnywhere/server/frontend"
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
			setSPAAssetCacheHeader(w, p)
			fs.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}

func spaFallbackFSHandler(root fs.FS) http.Handler {
	fsrv := http.FileServer(http.FS(root))

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
		requestPath := strings.TrimPrefix(p, "/")
		if requestPath != "" && requestPath != "." {
			fi, err := fs.Stat(root, requestPath)
			if err == nil && !fi.IsDir() {
				setSPAAssetCacheHeader(w, requestPath)
				fsrv.ServeHTTP(w, r)
				return
			}
		}

		index, err := root.Open("index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer index.Close()

		readSeeker, ok := index.(io.ReadSeeker)
		if !ok {
			http.Error(w, "embedded spa asset is not seekable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, "index.html", time.Time{}, readSeeker)
	})
}

func setSPAAssetCacheHeader(w http.ResponseWriter, requestPath string) {
	lower := strings.ReplaceAll(strings.ToLower(strings.TrimLeft(requestPath, "/\\")), "\\", "/")
	if strings.HasPrefix(lower, "assets/") {
		// Vite assets contain a content hash, so they are safe to retain forever.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	// Public branding and runtime configuration keep stable names.
	w.Header().Set("Cache-Control", "no-cache")
}

// MountSPA registers a catch-all route for the built Vite app (after /api and /health).
func MountSPA(r chi.Router, rootDir string) {
	if rootDir != "" {
		index := filepath.Join(filepath.Clean(rootDir), "index.html")
		if st, err := os.Stat(index); err == nil && !st.IsDir() {
			r.Handle("/*", spaFallbackHandler(rootDir))
			return
		}
	}

	embeddedRoot, err := fs.Sub(frontenddist.Files, "dist")
	if err != nil {
		return
	}
	if _, err := fs.Stat(embeddedRoot, "index.html"); err != nil {
		return
	}
	r.Handle("/*", spaFallbackFSHandler(embeddedRoot))
}
