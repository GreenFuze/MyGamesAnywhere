package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

func TestReviewControllerListCandidatesReturnsSummaries(t *testing.T) {
	controller := NewReviewController(
		&fakeIntegrationRepo{items: []*core.Integration{
			{ID: "metadata-1", Label: "IGDB", IntegrationType: "metadata"},
			{ID: "source-1", Label: "Steam Library", IntegrationType: "source"},
		}},
		&fakePluginHost{},
		&fakeGameStore{
			manualReviewCandidates: []*core.ManualReviewCandidate{{
				ID:                 "scan:review-1",
				CanonicalGameID:    "canon-1",
				CurrentTitle:       "Mystery Game",
				RawTitle:           "Mystery Game",
				Platform:           core.PlatformUnknown,
				Kind:               core.GameKindBaseGame,
				GroupKind:          core.GroupKindUnknown,
				IntegrationID:      "source-1",
				PluginID:           "game-source-steam",
				ExternalID:         "source-1-game",
				RootPath:           "C:/Games/Mystery",
				Status:             "found",
				ReviewState:        core.ManualReviewStatePending,
				FileCount:          2,
				ResolverMatchCount: 0,
				ReviewReasons:      []string{"no_metadata_matches", "unknown_platform"},
				CreatedAt:          time.Unix(1710000000, 0).UTC(),
			}},
		},
		&fakeManualReviewService{},
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Get("/api/review-candidates", controller.ListCandidates)

	req := httptest.NewRequest(http.MethodGet, "/api/review-candidates?limit=20", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var items []ManualReviewCandidateSummaryDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].IntegrationLabel != "Steam Library" {
		t.Fatalf("integration_label = %q, want %q", items[0].IntegrationLabel, "Steam Library")
	}
	if items[0].ID != "scan:review-1" {
		t.Fatalf("id = %q, want %q", items[0].ID, "scan:review-1")
	}
	if items[0].ReviewState != string(core.ManualReviewStatePending) {
		t.Fatalf("review_state = %q, want %q", items[0].ReviewState, core.ManualReviewStatePending)
	}
}

func TestReviewControllerGetCandidateReturnsDetail(t *testing.T) {
	controller := NewReviewController(
		&fakeIntegrationRepo{items: []*core.Integration{
			{ID: "source-1", Label: "Steam Library", IntegrationType: "source"},
		}},
		&fakePluginHost{},
		&fakeGameStore{
			manualReviewByID: map[string]*core.ManualReviewCandidate{
				"scan:review-1": {
					ID:            "scan:review-1",
					CurrentTitle:  "Mystery Game",
					RawTitle:      "Mystery Game",
					Platform:      core.PlatformUnknown,
					Kind:          core.GameKindBaseGame,
					GroupKind:     core.GroupKindUnknown,
					IntegrationID: "source-1",
					PluginID:      "game-source-steam",
					ExternalID:    "source-1-game",
					Status:        "found",
					ReviewState:   core.ManualReviewStateMatched,
					Files: []core.GameFile{{
						GameID:   "scan:review-1",
						Path:     "C:/Games/Mystery/game.exe",
						FileName: "game.exe",
						Role:     core.GameFileRoleRoot,
						FileKind: "exe",
						Size:     1024,
					}},
					ResolverMatches: []core.ResolverMatch{{
						PluginID:   "metadata-igdb",
						ExternalID: "igdb-1",
					}},
					CreatedAt: time.Unix(1710000000, 0).UTC(),
				},
			},
		},
		&fakeManualReviewService{},
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Get("/api/review-candidates/{id}", controller.GetCandidate)

	req := httptest.NewRequest(http.MethodGet, "/api/review-candidates/scan%3Areview-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var item ManualReviewCandidateDetailDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(item.Files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(item.Files))
	}
	if len(item.ResolverMatches) != 1 {
		t.Fatalf("len(resolver_matches) = %d, want 1", len(item.ResolverMatches))
	}
	if item.IntegrationLabel != "Steam Library" {
		t.Fatalf("integration_label = %q, want %q", item.IntegrationLabel, "Steam Library")
	}
	if item.ReviewState != string(core.ManualReviewStateMatched) {
		t.Fatalf("review_state = %q, want %q", item.ReviewState, core.ManualReviewStateMatched)
	}
}

func TestReviewControllerSearchCandidateDefaultsQueryAndKeepsProviderFailures(t *testing.T) {
	controller := NewReviewController(
		&fakeIntegrationRepo{items: []*core.Integration{
			{ID: "metadata-ok", Label: "IGDB", PluginID: "metadata-igdb", IntegrationType: "metadata", ConfigJSON: `{"api_key":"x"}`},
			{ID: "metadata-fail", Label: "SteamGridDB", PluginID: "metadata-steamgriddb", IntegrationType: "metadata", ConfigJSON: `{"api_key":"y"}`},
			{ID: "source-1", Label: "Steam Library", PluginID: "game-source-steam", IntegrationType: "source"},
		}},
		&fakePluginHost{
			provides: map[string][]string{
				reviewMetadataLookupMethod: {"metadata-igdb", "metadata-steamgriddb"},
			},
			metadataResults: map[string]reviewMetadataLookupResponse{
				"metadata-igdb": {
					Results: []reviewMetadataMatch{{
						Title:       "Mystery Game",
						Platform:    string(core.PlatformWindowsPC),
						Kind:        string(core.GameKindBaseGame),
						ExternalID:  "igdb-1",
						URL:         "https://example.com/igdb-1",
						Description: "A match from IGDB",
						Genres:      []string{"Action"},
						Media: []reviewMetadataMedia{{
							Type: "cover",
							URL:  "https://example.com/cover.png",
						}},
					}},
				},
			},
			metadataCallError: map[string]error{
				"metadata-steamgriddb": errors.New("provider offline"),
			},
		},
		&fakeGameStore{
			manualReviewByID: map[string]*core.ManualReviewCandidate{
				"scan:review-1": {
					ID:            "scan:review-1",
					CurrentTitle:  "Mystery Game",
					RawTitle:      "Mystery Game Raw",
					Platform:      core.PlatformWindowsPC,
					Kind:          core.GameKindBaseGame,
					GroupKind:     core.GroupKindSelfContained,
					IntegrationID: "source-1",
					PluginID:      "game-source-steam",
					ExternalID:    "source-1-game",
					Status:        "found",
					ReviewState:   core.ManualReviewStatePending,
					CreatedAt:     time.Unix(1710000000, 0).UTC(),
				},
			},
		},
		&fakeManualReviewService{},
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Post("/api/review-candidates/{id}/search", controller.SearchCandidate)

	req := httptest.NewRequest(http.MethodPost, "/api/review-candidates/scan%3Areview-1/search", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp ManualReviewSearchResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Query != "Mystery Game" {
		t.Fatalf("query = %q, want %q", resp.Query, "Mystery Game")
	}
	if len(resp.Providers) != 2 {
		t.Fatalf("len(providers) = %d, want 2", len(resp.Providers))
	}
	if len(resp.Results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(resp.Results))
	}
	if resp.Results[0].ImageURL != "https://example.com/cover.png" {
		t.Fatalf("image_url = %q, want cover url", resp.Results[0].ImageURL)
	}
	if resp.Providers[0].Status != "success" {
		t.Fatalf("providers[0].status = %q, want %q", resp.Providers[0].Status, "success")
	}
	if resp.Providers[1].Status != "error" {
		t.Fatalf("providers[1].status = %q, want %q", resp.Providers[1].Status, "error")
	}
}

func TestReviewControllerApplyCandidateReturnsUpdatedDetail(t *testing.T) {
	store := &fakeGameStore{
		manualReviewByID: map[string]*core.ManualReviewCandidate{
			"scan:review-1": {
				ID:            "scan:review-1",
				CurrentTitle:  "Mystery Game",
				RawTitle:      "Mystery Game",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				IntegrationID: "source-1",
				PluginID:      "game-source-steam",
				ExternalID:    "source-1-game",
				Status:        "found",
				ReviewState:   core.ManualReviewStateMatched,
				CreatedAt:     time.Unix(1710000000, 0).UTC(),
				ResolverMatches: []core.ResolverMatch{{
					PluginID:        "metadata-igdb",
					ExternalID:      "igdb-1",
					Title:           "Mystery Game",
					ManualSelection: true,
				}},
			},
		},
	}
	manualReviewSvc := &fakeManualReviewService{}
	controller := NewReviewController(
		&fakeIntegrationRepo{items: []*core.Integration{{ID: "source-1", Label: "Steam Library", IntegrationType: "source"}}},
		&fakePluginHost{},
		store,
		manualReviewSvc,
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Post("/api/review-candidates/{id}/apply", controller.ApplyCandidate)

	req := httptest.NewRequest(http.MethodPost, "/api/review-candidates/scan%3Areview-1/apply", strings.NewReader(`{
		"provider_integration_id":"metadata-1",
		"provider_plugin_id":"metadata-igdb",
		"title":"Mystery Game",
		"external_id":"igdb-1"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if manualReviewSvc.appliedCandidateID != "scan:review-1" {
		t.Fatalf("applied candidate id = %q, want %q", manualReviewSvc.appliedCandidateID, "scan:review-1")
	}
	if manualReviewSvc.appliedSelection.ProviderPluginID != "metadata-igdb" {
		t.Fatalf("provider plugin id = %q, want %q", manualReviewSvc.appliedSelection.ProviderPluginID, "metadata-igdb")
	}

	var item ManualReviewCandidateDetailDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if item.ReviewState != string(core.ManualReviewStateMatched) {
		t.Fatalf("review_state = %q, want %q", item.ReviewState, core.ManualReviewStateMatched)
	}
}

func TestReviewControllerApplyCandidateMapsValidationErrors(t *testing.T) {
	controller := NewReviewController(
		&fakeIntegrationRepo{},
		&fakePluginHost{},
		&fakeGameStore{},
		&fakeManualReviewService{applyErr: core.ErrManualReviewSelectionInvalid},
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Post("/api/review-candidates/{id}/apply", controller.ApplyCandidate)

	req := httptest.NewRequest(http.MethodPost, "/api/review-candidates/scan%3Areview-1/apply", strings.NewReader(`{
		"provider_plugin_id":"metadata-igdb",
		"external_id":"igdb-1"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestReviewControllerMarkCandidateNotAGameReturnsUpdatedDetail(t *testing.T) {
	store := &fakeGameStore{
		manualReviewByID: map[string]*core.ManualReviewCandidate{
			"scan:review-1": {
				ID:            "scan:review-1",
				CurrentTitle:  "Mystery Game",
				RawTitle:      "Mystery Game",
				Platform:      core.PlatformUnknown,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindUnknown,
				IntegrationID: "source-1",
				PluginID:      "game-source-steam",
				ExternalID:    "source-1-game",
				Status:        "found",
				ReviewState:   core.ManualReviewStatePending,
				CreatedAt:     time.Unix(1710000000, 0).UTC(),
			},
		},
	}
	controller := NewReviewController(
		&fakeIntegrationRepo{items: []*core.Integration{{ID: "source-1", Label: "Steam Library", IntegrationType: "source"}}},
		&fakePluginHost{},
		store,
		&fakeManualReviewService{},
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Post("/api/review-candidates/{id}/not-a-game", controller.MarkCandidateNotAGame)

	req := httptest.NewRequest(http.MethodPost, "/api/review-candidates/scan%3Areview-1/not-a-game", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var item ManualReviewCandidateDetailDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if item.ReviewState != string(core.ManualReviewStateNotAGame) {
		t.Fatalf("review_state = %q, want %q", item.ReviewState, core.ManualReviewStateNotAGame)
	}
}

func TestReviewControllerUnarchiveCandidateReturnsUpdatedDetail(t *testing.T) {
	store := &fakeGameStore{
		manualReviewByID: map[string]*core.ManualReviewCandidate{
			"scan:review-1": {
				ID:            "scan:review-1",
				CurrentTitle:  "Mystery Game",
				RawTitle:      "Mystery Game",
				Platform:      core.PlatformUnknown,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindUnknown,
				IntegrationID: "source-1",
				PluginID:      "game-source-steam",
				ExternalID:    "source-1-game",
				Status:        "found",
				ReviewState:   core.ManualReviewStateNotAGame,
				CreatedAt:     time.Unix(1710000000, 0).UTC(),
			},
		},
	}
	controller := NewReviewController(
		&fakeIntegrationRepo{items: []*core.Integration{{ID: "source-1", Label: "Steam Library", IntegrationType: "source"}}},
		&fakePluginHost{},
		store,
		&fakeManualReviewService{},
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Post("/api/review-candidates/{id}/unarchive", controller.UnarchiveCandidate)

	req := httptest.NewRequest(http.MethodPost, "/api/review-candidates/scan%3Areview-1/unarchive", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var item ManualReviewCandidateDetailDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if item.ReviewState != string(core.ManualReviewStatePending) {
		t.Fatalf("review_state = %q, want %q", item.ReviewState, core.ManualReviewStatePending)
	}
}

type fakeManualReviewService struct {
	appliedCandidateID string
	appliedSelection   core.ManualReviewSelection
	applyErr           error
}

func (f *fakeManualReviewService) Apply(_ context.Context, candidateID string, selection core.ManualReviewSelection) error {
	f.appliedCandidateID = candidateID
	f.appliedSelection = selection
	return f.applyErr
}

type fakeIntegrationRepo struct {
	items []*core.Integration
}

func (f *fakeIntegrationRepo) Create(context.Context, *core.Integration) error {
	panic("unexpected call")
}
func (f *fakeIntegrationRepo) Update(context.Context, *core.Integration) error {
	panic("unexpected call")
}
func (f *fakeIntegrationRepo) Delete(context.Context, string) error { panic("unexpected call") }
func (f *fakeIntegrationRepo) List(context.Context) ([]*core.Integration, error) {
	return f.items, nil
}
func (f *fakeIntegrationRepo) GetByID(_ context.Context, id string) (*core.Integration, error) {
	for _, item := range f.items {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, nil
}
func (f *fakeIntegrationRepo) ListByPluginID(_ context.Context, pluginID string) ([]*core.Integration, error) {
	var out []*core.Integration
	for _, item := range f.items {
		if item != nil && item.PluginID == pluginID {
			out = append(out, item)
		}
	}
	return out, nil
}
