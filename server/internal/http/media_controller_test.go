package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

type fakeMediaDownloadService struct {
	status         *core.MediaDownloadStatus
	missingAssetID int
}

func (f *fakeMediaDownloadService) Start(context.Context) error          { return nil }
func (f *fakeMediaDownloadService) EnqueuePending(context.Context) error { return nil }
func (f *fakeMediaDownloadService) Status(context.Context) (*core.MediaDownloadStatus, error) {
	if f.status != nil {
		return f.status, nil
	}
	return &core.MediaDownloadStatus{}, nil
}
func (f *fakeMediaDownloadService) RetryFailed(context.Context) (*core.MediaDownloadStatus, error) {
	return f.Status(context.Background())
}
func (f *fakeMediaDownloadService) ClearCache(context.Context) (*core.MediaDownloadStatus, error) {
	return f.Status(context.Background())
}
func (f *fakeMediaDownloadService) MarkLocalFileMissing(_ context.Context, assetID int) error {
	f.missingAssetID = assetID
	return nil
}

func TestMediaControllerQueueStatusReturnsServiceStatus(t *testing.T) {
	svc := &fakeMediaDownloadService{status: &core.MediaDownloadStatus{
		ItemsLeft:   3,
		Downloading: 1,
		Queued:      2,
		Total:       10,
		Current: []core.MediaDownloadActiveItem{{
			AssetID: 99,
			URL:     "https://example.test/current.png",
		}},
		RecentErrors: []core.MediaDownloadErrorItem{{
			AssetID: 7,
			URL:     "https://example.test/broken.png",
			Error:   "timeout",
		}},
	}}
	ctrl := NewMediaController(&fakeGameStore{}, staticConfig{}, noopLogger{}, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/media/queue/status", nil)
	rec := httptest.NewRecorder()
	ctrl.QueueStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body core.MediaDownloadStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ItemsLeft != 3 || body.Downloading != 1 || body.Queued != 2 {
		t.Fatalf("unexpected body: %+v", body)
	}
	if len(body.Current) != 1 || body.Current[0].URL != "https://example.test/current.png" {
		t.Fatalf("unexpected current downloads: %+v", body.Current)
	}
	if len(body.RecentErrors) != 1 || body.RecentErrors[0].URL != "https://example.test/broken.png" {
		t.Fatalf("unexpected recent errors: %+v", body.RecentErrors)
	}
}

func TestMediaControllerMissingLocalFileClearsStateAndQueuesRetry(t *testing.T) {
	mediaRoot := t.TempDir()
	svc := &fakeMediaDownloadService{}
	store := &fakeGameStore{mediaAsset: &core.MediaAsset{
		ID:        42,
		URL:       "https://example.test/missing.png",
		LocalPath: filepath.ToSlash(filepath.Join("assets", "missing.png")),
	}}
	ctrl := NewMediaController(store, staticConfig{values: map[string]string{"MEDIA_ROOT": mediaRoot}}, noopLogger{}, svc)
	router := chi.NewRouter()
	router.Get("/api/media/{assetID}", ctrl.ServeMedia)

	req := httptest.NewRequest(http.MethodGet, "/api/media/42", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}
	if location := rec.Header().Get("Location"); location != "https://example.test/missing.png" {
		t.Fatalf("Location = %q", location)
	}
	if policy := rec.Header().Get("Referrer-Policy"); policy != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q", policy)
	}
	if svc.missingAssetID != 42 {
		t.Fatalf("missingAssetID = %d, want 42", svc.missingAssetID)
	}
}

func TestMediaControllerDoesNotRedirectMissingFileToUnsafeURL(t *testing.T) {
	mediaRoot := t.TempDir()
	svc := &fakeMediaDownloadService{}
	store := &fakeGameStore{mediaAsset: &core.MediaAsset{
		ID:        43,
		URL:       "file:///private/art.png",
		LocalPath: filepath.ToSlash(filepath.Join("assets", "missing.png")),
	}}
	ctrl := NewMediaController(store, staticConfig{values: map[string]string{"MEDIA_ROOT": mediaRoot}}, noopLogger{}, svc)
	router := chi.NewRouter()
	router.Get("/api/media/{assetID}", ctrl.ServeMedia)

	req := httptest.NewRequest(http.MethodGet, "/api/media/43", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("unsafe redirect Location = %q", location)
	}
	if svc.missingAssetID != 43 {
		t.Fatalf("missingAssetID = %d, want 43", svc.missingAssetID)
	}
}

// serveMediaFixture writes one real file under a temp MEDIA_ROOT and returns a
// router that serves it, so these assertions come from http.ServeContent rather
// than from a stub.
func serveMediaFixture(t *testing.T, asset *core.MediaAsset, body []byte) chi.Router {
	t.Helper()
	mediaRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mediaRoot, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mediaRoot, "assets", "cover.png"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	asset.LocalPath = filepath.ToSlash(filepath.Join("assets", "cover.png"))
	controller := NewMediaController(
		&fakeGameStore{mediaAsset: asset},
		staticConfig{values: map[string]string{"MEDIA_ROOT": mediaRoot}},
		noopLogger{}, &fakeMediaDownloadService{},
	)
	router := chi.NewRouter()
	router.Get("/api/media/{assetID}", controller.ServeMedia)
	router.Head("/api/media/{assetID}", controller.ServeMedia)
	return router
}

func TestMediaRevalidatesWithAStrongETagAndIsNotPubliclyCacheable(t *testing.T) {
	// A frontend holds thousands of covers and revalidates them on every
	// refresh. Without a validator every refresh was a full redownload of
	// byte-identical files.
	asset := &core.MediaAsset{ID: 7, MimeType: "image/png", Hash: "e3b0c44298fc1c149afbf4c8996fb924"}
	router := serveMediaFixture(t, asset, []byte("cover-bytes"))

	request := httptest.NewRequest(http.MethodGet, "/api/media/7", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "cover-bytes" {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}

	etag := recorder.Header().Get("ETag")
	if etag != `"e3b0c44298fc1c149afbf4c8996fb924"` {
		t.Fatalf("ETag = %q, want the stored content hash", etag)
	}
	// Serving one profile's artwork out of a shared cache to another profile is
	// the failure this guards; the asset is only reachable with authorization.
	if cacheControl := recorder.Header().Get("Cache-Control"); !strings.HasPrefix(cacheControl, "private") {
		t.Fatalf("Cache-Control = %q, want private", cacheControl)
	}

	conditional := httptest.NewRequest(http.MethodGet, "/api/media/7", nil)
	conditional.Header.Set("If-None-Match", etag)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, conditional)
	if recorder.Code != http.StatusNotModified || recorder.Body.Len() != 0 {
		t.Fatalf("conditional GET status=%d body=%d bytes, want 304 and no body", recorder.Code, recorder.Body.Len())
	}

	stale := httptest.NewRequest(http.MethodGet, "/api/media/7", nil)
	stale.Header.Set("If-None-Match", `"some-other-revision"`)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, stale)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "cover-bytes" {
		t.Fatalf("a client holding a different revision must be resent the file, got %d", recorder.Code)
	}
}

func TestMediaWithoutAStoredHashStillOffersAWeakValidator(t *testing.T) {
	// Rows whose download never recorded a checksum must not be left with no
	// validator at all; one missing hash should not cost a client its artwork.
	asset := &core.MediaAsset{ID: 8, MimeType: "image/png"}
	router := serveMediaFixture(t, asset, []byte("no-hash-bytes"))

	request := httptest.NewRequest(http.MethodGet, "/api/media/8", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	etag := recorder.Header().Get("ETag")
	if !strings.HasPrefix(etag, `W/"`) {
		t.Fatalf("ETag = %q, want a weak validator when no hash is stored", etag)
	}

	conditional := httptest.NewRequest(http.MethodGet, "/api/media/8", nil)
	conditional.Header.Set("If-None-Match", etag)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, conditional)
	if recorder.Code != http.StatusNotModified {
		t.Fatalf("weak validator did not revalidate: status=%d", recorder.Code)
	}
}

func TestMediaRangeAndHeadStillWorkAlongsideTheValidator(t *testing.T) {
	// Adding an ETag changes how http.ServeContent evaluates preconditions, so
	// the partial and metadata paths are re-checked rather than assumed intact.
	asset := &core.MediaAsset{ID: 9, MimeType: "image/png", Hash: "abc123"}
	router := serveMediaFixture(t, asset, []byte("abcdefghij"))

	partial := httptest.NewRequest(http.MethodGet, "/api/media/9", nil)
	partial.Header.Set("Range", "bytes=2-5")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, partial)
	if recorder.Code != http.StatusPartialContent || recorder.Body.String() != "cdef" {
		t.Fatalf("range status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("Accept-Ranges missing: %v", recorder.Header())
	}

	head := httptest.NewRequest(http.MethodHead, "/api/media/9", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, head)
	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("head status=%d body=%d bytes", recorder.Code, recorder.Body.Len())
	}
	if recorder.Header().Get("ETag") != `"abc123"` {
		t.Fatalf("HEAD must carry the validator too: %v", recorder.Header())
	}
}
