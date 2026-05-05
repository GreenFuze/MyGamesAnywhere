package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if svc.missingAssetID != 42 {
		t.Fatalf("missingAssetID = %d, want 42", svc.missingAssetID)
	}
}
