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
		nil,
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
		nil,
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
		nil,
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

func TestReviewControllerSearchCandidatePreservesMetadataSourceOrder(t *testing.T) {
	controller := NewReviewController(
		&fakeIntegrationRepo{items: []*core.Integration{
			{ID: "metadata-z", Label: "Zeta", PluginID: "metadata-z", IntegrationType: "metadata", ConfigJSON: `{}`},
			{ID: "metadata-a", Label: "Alpha", PluginID: "metadata-a", IntegrationType: "metadata", ConfigJSON: `{}`},
		}},
		&fakePluginHost{
			provides: map[string][]string{
				reviewMetadataLookupMethod: {"metadata-z", "metadata-a"},
			},
			metadataResults: map[string]reviewMetadataLookupResponse{
				"metadata-z": {Results: []reviewMetadataMatch{{Title: "Mystery Game", ExternalID: "z-1"}}},
				"metadata-a": {Results: []reviewMetadataMatch{{Title: "Mystery Game", ExternalID: "a-1"}}},
			},
		},
		&fakeGameStore{
			manualReviewByID: map[string]*core.ManualReviewCandidate{
				"scan:review-order": {
					ID:            "scan:review-order",
					CurrentTitle:  "Mystery Game",
					RawTitle:      "Mystery Game",
					Platform:      core.PlatformUnknown,
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
		nil,
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Post("/api/review-candidates/{id}/search", controller.SearchCandidate)

	req := httptest.NewRequest(http.MethodPost, "/api/review-candidates/scan%3Areview-order/search", strings.NewReader(`{}`))
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
	if len(resp.Providers) != 2 {
		t.Fatalf("len(providers) = %d, want 2", len(resp.Providers))
	}
	if resp.Providers[0].IntegrationID != "metadata-z" || resp.Providers[1].IntegrationID != "metadata-a" {
		t.Fatalf("provider order = %+v, want repo/source order metadata-z then metadata-a", resp.Providers)
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
		nil,
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
		nil,
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

func TestReviewControllerRedetectCandidateReturnsResultAndUpdatedDetail(t *testing.T) {
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
					PluginID:   "metadata-igdb",
					ExternalID: "igdb-1",
					Title:      "Mystery Game",
				}},
			},
		},
	}
	manualReviewSvc := &fakeManualReviewService{
		redetectResult: &core.ManualReviewRedetectResult{
			CandidateID:   "scan:review-1",
			Status:        core.ManualReviewRedetectStatusMatched,
			MatchCount:    1,
			ProviderCount: 1,
		},
	}
	controller := NewReviewController(
		&fakeIntegrationRepo{items: []*core.Integration{{ID: "source-1", Label: "Steam Library", IntegrationType: "source"}}},
		&fakePluginHost{},
		store,
		manualReviewSvc,
		nil,
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Post("/api/review-candidates/{id}/redetect", controller.RedetectCandidate)

	req := httptest.NewRequest(http.MethodPost, "/api/review-candidates/scan%3Areview-1/redetect", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if manualReviewSvc.redetectedCandidateID != "scan:review-1" {
		t.Fatalf("redetected candidate id = %q, want %q", manualReviewSvc.redetectedCandidateID, "scan:review-1")
	}

	var resp ManualReviewRedetectResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Result.Status != core.ManualReviewRedetectStatusMatched {
		t.Fatalf("result.status = %q, want %q", resp.Result.Status, core.ManualReviewRedetectStatusMatched)
	}
	if resp.Candidate.ReviewState != string(core.ManualReviewStateMatched) {
		t.Fatalf("candidate.review_state = %q, want %q", resp.Candidate.ReviewState, core.ManualReviewStateMatched)
	}
}

func TestReviewControllerRedetectCandidateMapsValidationErrors(t *testing.T) {
	controller := NewReviewController(
		&fakeIntegrationRepo{},
		&fakePluginHost{},
		&fakeGameStore{},
		&fakeManualReviewService{redetectErr: core.ErrManualReviewCandidateNotEligible},
		nil,
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Post("/api/review-candidates/{id}/redetect", controller.RedetectCandidate)

	req := httptest.NewRequest(http.MethodPost, "/api/review-candidates/scan%3Areview-1/redetect", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestReviewControllerRedetectActiveReturnsBatchResult(t *testing.T) {
	controller := NewReviewController(
		&fakeIntegrationRepo{},
		&fakePluginHost{},
		&fakeGameStore{},
		&fakeManualReviewService{redetectBatchResult: &core.ManualReviewRedetectBatchResult{
			Attempted:    2,
			Matched:      1,
			Unidentified: 1,
			Results: []core.ManualReviewRedetectResult{{
				CandidateID: "scan:review-1",
				Status:      core.ManualReviewRedetectStatusMatched,
			}},
		}},
		nil,
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Post("/api/review-candidates/redetect", controller.RedetectActive)

	req := httptest.NewRequest(http.MethodPost, "/api/review-candidates/redetect", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp core.ManualReviewRedetectBatchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Attempted != 2 || resp.Matched != 1 || resp.Unidentified != 1 {
		t.Fatalf("batch = %+v, want attempted 2 matched 1 unidentified 1", resp)
	}
}

func TestReviewControllerRedetectActiveReturnsFailFastBatchError(t *testing.T) {
	controller := NewReviewController(
		&fakeIntegrationRepo{},
		&fakePluginHost{},
		&fakeGameStore{},
		&fakeManualReviewService{
			redetectBatchResult: &core.ManualReviewRedetectBatchResult{
				Attempted:         2,
				Matched:           1,
				FailedCandidateID: "scan:review-2",
				Results:           []core.ManualReviewRedetectResult{},
			},
			redetectBatchErr: core.ErrMetadataProvidersUnavailable,
		},
		nil,
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Post("/api/review-candidates/redetect", controller.RedetectActive)

	req := httptest.NewRequest(http.MethodPost, "/api/review-candidates/redetect", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var resp core.ManualReviewRedetectBatchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.FailedCandidateID != "scan:review-2" || strings.TrimSpace(resp.Error) == "" {
		t.Fatalf("failure response = %+v, want failed candidate and error", resp)
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
		nil,
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
		nil,
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

func TestReviewControllerDeleteCandidateFilesReturnsDeletedCandidate(t *testing.T) {
	deleteSvc := &fakeGameDeletionService{
		result: &core.DeleteSourceGameResult{
			DeletedSourceGameID: "scan:review-1",
			CanonicalExists:     false,
		},
	}
	controller := NewReviewController(
		&fakeIntegrationRepo{},
		&fakePluginHost{},
		&fakeGameStore{},
		&fakeManualReviewService{},
		deleteSvc,
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Delete("/api/review-candidates/{id}/files", controller.DeleteCandidateFiles)

	req := httptest.NewRequest(http.MethodDelete, "/api/review-candidates/scan%3Areview-1/files", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if deleteSvc.reviewCandidateID != "scan:review-1" {
		t.Fatalf("review candidate id = %q, want scan:review-1", deleteSvc.reviewCandidateID)
	}
	var resp ManualReviewDeleteCandidateFilesResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.DeletedCandidateID != "scan:review-1" || resp.CanonicalExists {
		t.Fatalf("response = %+v, want deleted candidate without canonical", resp)
	}
}

func TestReviewControllerDeleteCandidateFilesRejectsIneligibleCandidate(t *testing.T) {
	controller := NewReviewController(
		&fakeIntegrationRepo{},
		&fakePluginHost{},
		&fakeGameStore{},
		&fakeManualReviewService{},
		&fakeGameDeletionService{err: core.ErrSourceGameDeleteNotEligible},
		noopLogger{},
	)

	router := chi.NewRouter()
	router.Delete("/api/review-candidates/{id}/files", controller.DeleteCandidateFiles)

	req := httptest.NewRequest(http.MethodDelete, "/api/review-candidates/scan%3Areview-1/files", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

type fakeManualReviewService struct {
	appliedCandidateID    string
	appliedSelection      core.ManualReviewSelection
	applyErr              error
	redetectedCandidateID string
	redetectResult        *core.ManualReviewRedetectResult
	redetectErr           error
	redetectBatchResult   *core.ManualReviewRedetectBatchResult
	redetectBatchErr      error
}

func (f *fakeManualReviewService) Apply(_ context.Context, candidateID string, selection core.ManualReviewSelection) error {
	f.appliedCandidateID = candidateID
	f.appliedSelection = selection
	return f.applyErr
}

func (f *fakeManualReviewService) Redetect(_ context.Context, candidateID string) (*core.ManualReviewRedetectResult, error) {
	f.redetectedCandidateID = candidateID
	return f.redetectResult, f.redetectErr
}

func (f *fakeManualReviewService) RedetectActive(context.Context) (*core.ManualReviewRedetectBatchResult, error) {
	return f.redetectBatchResult, f.redetectBatchErr
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
