package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

type fakeCacheService struct {
	job              *core.SourceCacheJobStatus
	entries          []*core.SourceCacheEntry
	prepareImmediate bool
	prepareErr       error
	resolvedPath     string
}

func (f *fakeCacheService) DescribeSourceGame(context.Context, core.Platform, *core.SourceGame) []core.SourceDeliveryProfile {
	return nil
}
func (f *fakeCacheService) Prepare(context.Context, core.SourceCachePrepareRequest, core.Platform, *core.SourceGame) (*core.SourceCacheJobStatus, bool, error) {
	return f.job, f.prepareImmediate, f.prepareErr
}
func (f *fakeCacheService) GetJob(context.Context, string) (*core.SourceCacheJobStatus, error) {
	return f.job, nil
}
func (f *fakeCacheService) ListJobs(context.Context, int) ([]*core.SourceCacheJobStatus, error) {
	if f.job == nil {
		return nil, nil
	}
	return []*core.SourceCacheJobStatus{f.job}, nil
}
func (f *fakeCacheService) ListEntries(context.Context) ([]*core.SourceCacheEntry, error) {
	return f.entries, nil
}
func (f *fakeCacheService) DeleteEntry(context.Context, string) error { return nil }
func (f *fakeCacheService) ClearEntries(context.Context) error        { return nil }
func (f *fakeCacheService) ResolveCachedFile(context.Context, string, string, string) (*core.SourceCacheEntry, *core.SourceCacheEntryFile, string, error) {
	return nil, nil, f.resolvedPath, nil
}

func TestCacheControllerPrepareGameCacheReturnsAcceptedJob(t *testing.T) {
	store := &fakeGameStore{
		game: &core.CanonicalGame{
			ID:       "game-1",
			Title:    "Drive Game",
			Platform: core.PlatformGBA,
			SourceGames: []*core.SourceGame{
				{
					ID:        "source-1",
					Platform:  core.PlatformGBA,
					GroupKind: core.GroupKindSelfContained,
					Status:    "found",
				},
			},
		},
	}
	job := &core.SourceCacheJobStatus{
		JobID:        "job-1",
		SourceGameID: "source-1",
		Profile:      core.BrowserProfileEmulatorJS,
		Status:       "queued",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	ctrl := NewCacheController(store, nil, &fakeCacheService{job: job}, noopLogger{})
	router := chi.NewRouter()
	router.Post("/api/games/{id}/cache/prepare", ctrl.PrepareGameCache)

	req := httptest.NewRequest(http.MethodPost, "/api/games/game-1/cache/prepare", strings.NewReader(`{"source_game_id":"source-1","profile":"browser.emulatorjs"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var body CachePrepareResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Accepted || body.Immediate || body.Job == nil || body.Job.JobID != "job-1" {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestCacheControllerListEntriesIncludesIntegrationLabel(t *testing.T) {
	ctrl := NewCacheController(
		&fakeGameStore{},
		&fakeControllerIntegrationRepo{
			byID: map[string]*core.Integration{
				"integration-1": {ID: "integration-1", Label: "Google Drive"},
			},
		},
		&fakeCacheService{
			entries: []*core.SourceCacheEntry{
				{
					ID:            "entry-1",
					SourceGameID:  "source-1",
					IntegrationID: "integration-1",
					PluginID:      "game-source-google-drive",
					Profile:       core.BrowserProfileEmulatorJS,
					Mode:          "materialized",
					Status:        "ready",
					CreatedAt:     time.Now().UTC(),
					UpdatedAt:     time.Now().UTC(),
				},
			},
		},
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Get("/api/cache/entries", ctrl.ListEntries)

	req := httptest.NewRequest(http.MethodGet, "/api/cache/entries", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		Entries []CacheEntryDTO `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Entries) != 1 || body.Entries[0].IntegrationLabel != "Google Drive" {
		t.Fatalf("unexpected entries: %+v", body.Entries)
	}
}
