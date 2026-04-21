package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
)

type metadataPolicyTestDiscovery struct {
	plugins     map[string]*core.Plugin
	metadataIDs []string
}

func (d metadataPolicyTestDiscovery) GetPluginIDs() []string { return nil }

func (d metadataPolicyTestDiscovery) GetPlugin(pluginID string) (*core.Plugin, bool) {
	plugin, ok := d.plugins[pluginID]
	return plugin, ok
}

func (d metadataPolicyTestDiscovery) GetPluginIDsProviding(method string) []string {
	if method == metadataGameLookupMethod {
		return append([]string(nil), d.metadataIDs...)
	}
	return nil
}

func TestRunScanContinuesAfterMetadataProviderFailureAndReportsDegradedStatus(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)
	bus := events.New()
	defer bus.Close()

	sub := bus.Subscribe()
	defer bus.Unsubscribe(sub)

	caller := &mockCaller{
		callFn: func(pluginID, method string, params any) (any, error) {
			switch method {
			case sourceGamesListMethod:
				return map[string]any{
					"games": []map[string]any{{
						"external_id": "epic-1",
						"title":       "Control",
						"platform":    "windows_pc",
					}},
				}, nil
			case metadataGameLookupMethod:
				return nil, fmt.Errorf("metadata provider offline")
			default:
				return nil, nil
			}
		},
	}

	discovery := metadataPolicyTestDiscovery{
		plugins: map[string]*core.Plugin{
			"game-source-epic": {
				Manifest: core.PluginManifest{ID: "game-source-epic", Provides: []string{sourceGamesListMethod}},
			},
			"metadata-steam": {
				Manifest: core.PluginManifest{ID: "metadata-steam", Provides: []string{metadataGameLookupMethod}},
			},
		},
		metadataIDs: []string{"metadata-steam"},
	}

	repo := manualReviewTestIntegrationRepo{items: []*core.Integration{
		{ID: "epic-1", PluginID: "game-source-epic", Label: "Epic", IntegrationType: "source", ConfigJSON: `{}`},
		{ID: "metadata-steam-1", PluginID: "metadata-steam", Label: "Steam Metadata", IntegrationType: "metadata", ConfigJSON: `{}`},
	}}

	orchestrator := NewOrchestrator(caller, discovery, repo, store, &eventTestMediaDownloadQueue{}, eventTestLogger{})
	orchestrator.SetEventBus(bus)

	games, err := orchestrator.RunScan(ctx, nil)
	if err != nil {
		t.Fatalf("RunScan returned error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("games = %d, want 1", len(games))
	}

	reports, err := store.GetScanReports(ctx, 1)
	if err != nil {
		t.Fatalf("GetScanReports: %v", err)
	}
	if len(reports) != 1 || len(reports[0].Results) != 1 {
		t.Fatalf("unexpected reports payload: %+v", reports)
	}
	if reports[0].Results[0].Error == "" {
		t.Fatal("expected degraded metadata failure to be reported on the integration result")
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-sub:
			if ev.Type != "scan_metadata_finished" {
				continue
			}
			var payload map[string]any
			if err := json.Unmarshal(ev.Data, &payload); err != nil {
				t.Fatalf("unmarshal metadata finished payload: %v", err)
			}
			if payload["status"] != "degraded" {
				t.Fatalf("metadata finished status = %v, want degraded", payload["status"])
			}
			if payload["error_count"] != float64(2) {
				t.Fatalf("metadata finished error_count = %v, want 2", payload["error_count"])
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for degraded scan_metadata_finished event")
		}
	}
}

func TestRunMetadataRefreshFailsFastOnProviderError(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)
	persistMetadataRefreshTestBatch(t, ctx, store, "source-1", "scan:refresh-1")

	caller := &mockCaller{
		callFn: func(pluginID, method string, params any) (any, error) {
			if method == metadataGameLookupMethod {
				return nil, fmt.Errorf("provider unavailable")
			}
			return nil, nil
		},
	}

	discovery := metadataPolicyTestDiscovery{
		plugins: map[string]*core.Plugin{
			"game-source-epic": {
				Manifest: core.PluginManifest{ID: "game-source-epic", Provides: []string{sourceGamesListMethod}},
			},
			"metadata-steam": {
				Manifest: core.PluginManifest{ID: "metadata-steam", Provides: []string{metadataGameLookupMethod}},
			},
		},
		metadataIDs: []string{"metadata-steam"},
	}
	repo := manualReviewTestIntegrationRepo{items: []*core.Integration{
		{ID: "source-1", PluginID: "game-source-epic", Label: "Epic", IntegrationType: "source", ConfigJSON: `{}`},
		{ID: "metadata-steam-1", PluginID: "metadata-steam", Label: "Steam Metadata", IntegrationType: "metadata", ConfigJSON: `{}`},
	}}

	orchestrator := NewOrchestrator(caller, discovery, repo, store, &eventTestMediaDownloadQueue{}, eventTestLogger{})
	_, err := orchestrator.RunMetadataRefresh(ctx, nil)
	if !errors.Is(err, core.ErrMetadataProvidersUnavailable) {
		t.Fatalf("error = %v, want %v", err, core.ErrMetadataProvidersUnavailable)
	}
}

func TestRefreshGameMetadataFailsFastOnProviderError(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)
	persistMetadataRefreshTestBatch(t, ctx, store, "source-1", "scan:refresh-1")

	canonicalGames, err := store.GetCanonicalGames(ctx)
	if err != nil {
		t.Fatalf("GetCanonicalGames: %v", err)
	}
	if len(canonicalGames) != 1 {
		t.Fatalf("canonical games = %d, want 1", len(canonicalGames))
	}

	caller := &mockCaller{
		callFn: func(pluginID, method string, params any) (any, error) {
			if method == metadataGameLookupMethod {
				return nil, fmt.Errorf("provider unavailable")
			}
			return nil, nil
		},
	}

	discovery := metadataPolicyTestDiscovery{
		plugins: map[string]*core.Plugin{
			"game-source-epic": {
				Manifest: core.PluginManifest{ID: "game-source-epic", Provides: []string{sourceGamesListMethod}},
			},
			"metadata-steam": {
				Manifest: core.PluginManifest{ID: "metadata-steam", Provides: []string{metadataGameLookupMethod}},
			},
		},
		metadataIDs: []string{"metadata-steam"},
	}
	repo := manualReviewTestIntegrationRepo{items: []*core.Integration{
		{ID: "source-1", PluginID: "game-source-epic", Label: "Epic", IntegrationType: "source", ConfigJSON: `{}`},
		{ID: "metadata-steam-1", PluginID: "metadata-steam", Label: "Steam Metadata", IntegrationType: "metadata", ConfigJSON: `{}`},
	}}

	orchestrator := NewOrchestrator(caller, discovery, repo, store, &eventTestMediaDownloadQueue{}, eventTestLogger{})
	_, err = orchestrator.RefreshGameMetadata(ctx, canonicalGames[0].ID)
	if !errors.Is(err, core.ErrMetadataProvidersUnavailable) {
		t.Fatalf("error = %v, want %v", err, core.ErrMetadataProvidersUnavailable)
	}
}

func TestRefreshGameMetadataDoesNotDropUnrelatedDetectedGames(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)
	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "source-1",
		SourceGames: []*core.SourceGame{
			{
				ID:            "scan:refresh-keep-1",
				IntegrationID: "source-1",
				PluginID:      "game-source-epic",
				ExternalID:    "epic-1",
				RawTitle:      "Control",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			},
			{
				ID:            "scan:refresh-keep-2",
				IntegrationID: "source-1",
				PluginID:      "game-source-epic",
				ExternalID:    "epic-2",
				RawTitle:      "Alan Wake",
				Platform:      core.PlatformWindowsPC,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				Status:        "found",
			},
		},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:refresh-keep-1": {{
				PluginID:   "metadata-steam",
				Title:      "Control",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "steam-control",
			}},
			"scan:refresh-keep-2": {{
				PluginID:   "metadata-steam",
				Title:      "Alan Wake",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "steam-alan-wake",
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}

	canonicalGames, err := store.GetCanonicalGames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(canonicalGames) != 2 {
		t.Fatalf("canonical games before = %d, want 2", len(canonicalGames))
	}

	var controlID string
	for _, game := range canonicalGames {
		if game != nil && game.Title == "Control" {
			controlID = game.ID
			break
		}
	}
	if controlID == "" {
		t.Fatal("expected Control canonical id")
	}

	caller := &mockCaller{
		callFn: func(pluginID, method string, params any) (any, error) {
			if method != metadataGameLookupMethod {
				return nil, nil
			}
			request, ok := params.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("missing metadata request")
			}
			games, ok := request["games"].([]metadataGameQuery)
			if !ok || len(games) == 0 {
				return nil, fmt.Errorf("missing metadata games")
			}
			return metadataLookupResponse{
				Results: []metadataMatch{{
					Index:      0,
					Title:      games[0].Title,
					Platform:   string(core.PlatformWindowsPC),
					ExternalID: "refresh-" + games[0].Title,
				}},
			}, nil
		},
	}

	discovery := metadataPolicyTestDiscovery{
		plugins: map[string]*core.Plugin{
			"metadata-steam": {
				Manifest: core.PluginManifest{ID: "metadata-steam", Provides: []string{metadataGameLookupMethod}},
			},
		},
		metadataIDs: []string{"metadata-steam"},
	}
	repo := manualReviewTestIntegrationRepo{items: []*core.Integration{
		{ID: "source-1", PluginID: "game-source-epic", Label: "Epic", IntegrationType: "source", ConfigJSON: `{}`},
		{ID: "metadata-steam-1", PluginID: "metadata-steam", Label: "Steam Metadata", IntegrationType: "metadata", ConfigJSON: `{}`},
	}}

	orchestrator := NewOrchestrator(caller, discovery, repo, store, &eventTestMediaDownloadQueue{}, eventTestLogger{})
	if _, err := orchestrator.RefreshGameMetadata(ctx, controlID); err != nil {
		t.Fatal(err)
	}

	after, err := store.GetCanonicalGames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 2 {
		t.Fatalf("canonical games after = %d, want 2", len(after))
	}
}

func TestManualReviewApplyFailsFastWhenFillProviderFails(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)
	persistMetadataRefreshTestBatch(t, ctx, store, "source-1", "scan:manual-review-1")

	caller := &mockCaller{
		callFn: func(pluginID, method string, params any) (any, error) {
			if method != metadataGameLookupMethod {
				return nil, nil
			}
			if pluginID == "metadata-other" {
				return nil, fmt.Errorf("provider unavailable")
			}
			return metadataLookupResponse{Results: nil}, nil
		},
	}

	discovery := metadataPolicyTestDiscovery{
		plugins:     map[string]*core.Plugin{},
		metadataIDs: []string{"metadata-chosen", "metadata-other"},
	}
	repo := manualReviewTestIntegrationRepo{items: []*core.Integration{
		{ID: "metadata-chosen-1", PluginID: "metadata-chosen", Label: "Chosen Metadata", IntegrationType: "metadata", ConfigJSON: `{}`},
		{ID: "metadata-other-1", PluginID: "metadata-other", Label: "Other Metadata", IntegrationType: "metadata", ConfigJSON: `{}`},
	}}

	service := NewManualReviewService(caller, discovery, repo, store, &countingMediaDownloadQueue{}, testLogger{})
	err := service.Apply(ctx, "scan:manual-review-1", core.ManualReviewSelection{
		ProviderPluginID: "metadata-chosen",
		ExternalID:       "chosen-1",
		Title:            "Chosen Game",
		Platform:         string(core.PlatformWindowsPC),
	})
	if !errors.Is(err, core.ErrMetadataProvidersUnavailable) {
		t.Fatalf("error = %v, want %v", err, core.ErrMetadataProvidersUnavailable)
	}
}

func persistMetadataRefreshTestBatch(t *testing.T, ctx context.Context, store core.GameStore, integrationID, sourceGameID string) {
	t.Helper()

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: integrationID,
		SourceGames: []*core.SourceGame{{
			ID:            sourceGameID,
			IntegrationID: integrationID,
			PluginID:      "game-source-epic",
			ExternalID:    "epic-1",
			RawTitle:      "Control",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			RootPath:      "Games/Control",
			Status:        "found",
			Files: []core.GameFile{{
				GameID:   sourceGameID,
				Path:     "Control.exe",
				FileName: "Control.exe",
				Role:     core.GameFileRoleRoot,
				FileKind: "exe",
				Size:     4096,
			}},
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			sourceGameID: {{
				PluginID:   "metadata-steam",
				Title:      "Control",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "steam-control",
			}},
		},
		MediaItems: map[string][]core.MediaRef{},
	}); err != nil {
		t.Fatal(err)
	}
}
