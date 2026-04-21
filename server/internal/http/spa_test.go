package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMountSPAFallsBackToEmbeddedDist(t *testing.T) {
	router := chi.NewRouter()
	MountSPA(router, filepath.Join(t.TempDir(), "missing"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<!doctype html>") && !strings.Contains(strings.ToLower(rec.Body.String()), "<html") {
		t.Fatalf("expected embedded index.html body, got %q", rec.Body.String())
	}
}

func TestMountSPAPrefersFilesystemOverride(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html><body>override index</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte("console.log('override asset')"), 0o644); err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	MountSPA(router, root)

	assetReq := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	assetRec := httptest.NewRecorder()
	router.ServeHTTP(assetRec, assetReq)
	if assetRec.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want %d", assetRec.Code, http.StatusOK)
	}
	if body := strings.TrimSpace(assetRec.Body.String()); body != "console.log('override asset')" {
		t.Fatalf("asset body = %q, want filesystem override asset", body)
	}

	indexReq := httptest.NewRequest(http.MethodGet, "/non-existent", nil)
	indexRec := httptest.NewRecorder()
	router.ServeHTTP(indexRec, indexReq)
	if indexRec.Code != http.StatusOK {
		t.Fatalf("index status = %d, want %d", indexRec.Code, http.StatusOK)
	}
	if body := strings.TrimSpace(indexRec.Body.String()); body != "<html><body>override index</body></html>" {
		t.Fatalf("index body = %q, want filesystem override index", body)
	}
}
