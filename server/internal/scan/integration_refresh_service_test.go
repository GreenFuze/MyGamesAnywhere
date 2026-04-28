package scan

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type integrationRefreshTestPluginHost struct {
	lookupResults map[string]metadataLookupResponse
}

func (h integrationRefreshTestPluginHost) Call(_ context.Context, pluginID string, method string, _ any, result any) error {
	if method != metadataGameLookupMethod {
		return nil
	}
	payload := h.lookupResults[pluginID]
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}

func (h integrationRefreshTestPluginHost) GetPluginIDsProviding(method string) []string {
	if method != metadataGameLookupMethod {
		return nil
	}
	var ids []string
	for pluginID := range h.lookupResults {
		ids = append(ids, pluginID)
	}
	return ids
}

func (h integrationRefreshTestPluginHost) GetPlugin(pluginID string) (*core.Plugin, bool) {
	return &core.Plugin{
		Manifest: core.PluginManifest{
			ID:       pluginID,
			Provides: []string{metadataGameLookupMethod},
		},
	}, true
}

func TestRefreshSourceGameForMetadataProviderPreservesOtherMatches(t *testing.T) {
	service := &IntegrationRefreshService{
		pluginHost: integrationRefreshTestPluginHost{
			lookupResults: map[string]metadataLookupResponse{
				"retroachievements": {
					Results: []metadataMatch{{
						Index:      0,
						Title:      "Altered Beast",
						ExternalID: "ra-42",
						Media: []ipcMediaItem{{
							Type: "cover",
							URL:  "https://example.com/ra-cover.png",
						}},
					}},
				},
			},
		},
		logger: testLogger{},
	}

	sourceGame := &core.SourceGame{
		ID:        "sg-1",
		RawTitle:  "Altered Beast",
		Platform:  core.PlatformGenesis,
		GroupKind: core.GroupKindSelfContained,
		ResolverMatches: []core.ResolverMatch{
			{PluginID: "metadata-launchbox", Title: "Altered Beast", ExternalID: "lb-1"},
			{PluginID: "retroachievements", Title: "Old RA", ExternalID: "ra-old"},
		},
		Media: []core.MediaRef{
			{Type: core.MediaTypeCover, URL: "https://example.com/lb-cover.png", Source: "metadata-launchbox"},
			{Type: core.MediaTypeCover, URL: "https://example.com/ra-old.png", Source: "retroachievements"},
		},
	}

	refreshed, warning, err := service.refreshSourceGameForMetadataProvider(
		context.Background(),
		sourceGame,
		MetadataSource{PluginID: "retroachievements"},
		[]MetadataSource{{PluginID: "metadata-launchbox"}, {PluginID: "retroachievements"}},
	)
	if err != nil {
		t.Fatalf("refreshSourceGameForMetadataProvider: %v", err)
	}
	if warning != "" {
		t.Fatalf("warning = %q, want empty", warning)
	}

	if len(refreshed.ResolverMatches) != 2 {
		t.Fatalf("resolver match count = %d, want 2", len(refreshed.ResolverMatches))
	}
	if refreshed.ResolverMatches[0].PluginID != "metadata-launchbox" && refreshed.ResolverMatches[1].PluginID != "metadata-launchbox" {
		t.Fatalf("expected launchbox match to be preserved: %+v", refreshed.ResolverMatches)
	}
	foundRA := false
	for _, match := range refreshed.ResolverMatches {
		if match.PluginID == "retroachievements" {
			foundRA = true
			if match.ExternalID != "ra-42" {
				t.Fatalf("refreshed RA external_id = %q, want ra-42", match.ExternalID)
			}
		}
	}
	if !foundRA {
		t.Fatal("expected refreshed RetroAchievements match")
	}
	if len(refreshed.Media) != 2 {
		t.Fatalf("media count = %d, want 2", len(refreshed.Media))
	}
}

func TestRefreshSourceGameForMetadataProviderRemovesStaleProviderMatchOnNoMatch(t *testing.T) {
	service := &IntegrationRefreshService{
		pluginHost: integrationRefreshTestPluginHost{
			lookupResults: map[string]metadataLookupResponse{
				"retroachievements": {Results: nil},
			},
		},
		logger: testLogger{},
	}

	sourceGame := &core.SourceGame{
		ID:        "sg-1",
		RawTitle:  "Altered Beast",
		Platform:  core.PlatformGenesis,
		GroupKind: core.GroupKindSelfContained,
		ResolverMatches: []core.ResolverMatch{
			{PluginID: "metadata-launchbox", Title: "Altered Beast", ExternalID: "lb-1"},
			{PluginID: "retroachievements", Title: "Old RA", ExternalID: "ra-old"},
		},
		Media: []core.MediaRef{
			{Type: core.MediaTypeCover, URL: "https://example.com/lb-cover.png", Source: "metadata-launchbox"},
			{Type: core.MediaTypeCover, URL: "https://example.com/ra-old.png", Source: "retroachievements"},
		},
	}

	refreshed, warning, err := service.refreshSourceGameForMetadataProvider(
		context.Background(),
		sourceGame,
		MetadataSource{PluginID: "retroachievements"},
		[]MetadataSource{{PluginID: "metadata-launchbox"}, {PluginID: "retroachievements"}},
	)
	if err != nil {
		t.Fatalf("refreshSourceGameForMetadataProvider: %v", err)
	}
	if warning != "" {
		t.Fatalf("warning = %q, want empty", warning)
	}

	if len(refreshed.ResolverMatches) != 1 {
		t.Fatalf("resolver match count = %d, want 1", len(refreshed.ResolverMatches))
	}
	if refreshed.ResolverMatches[0].PluginID != "metadata-launchbox" {
		t.Fatalf("remaining match plugin = %q, want metadata-launchbox", refreshed.ResolverMatches[0].PluginID)
	}
	if len(refreshed.Media) != 1 || refreshed.Media[0].Source != "metadata-launchbox" {
		t.Fatalf("expected only preserved launchbox media, got %+v", refreshed.Media)
	}
}

func TestGamesForSourceRefreshPreservesOnlySourceOwnedXboxState(t *testing.T) {
	sourceGame := &core.SourceGame{
		ID:            "sg-xbox",
		IntegrationID: "integration-1",
		PluginID:      "game-source-xbox",
		ExternalID:    "xbox-final-fantasy",
		RawTitle:      "Final Fantasy",
		Platform:      core.PlatformWindowsPC,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		Status:        "found",
		ResolverMatches: []core.ResolverMatch{
			{
				PluginID:        "game-source-xbox",
				Title:           "Final Fantasy",
				ExternalID:      "xbox-final-fantasy",
				IsGamePass:      true,
				XcloudAvailable: true,
				StoreProductID:  "9NFINALFANTASY",
				XcloudURL:       "https://www.xbox.com/play/games/final-fantasy/9NFINALFANTASY",
			},
			{PluginID: "metadata-igdb", Title: "Final Fantasy 2.0", ExternalID: "igdb-stale"},
		},
		Media: []core.MediaRef{
			{Type: core.MediaTypeCover, URL: "https://example.com/xbox-cover.jpg", Source: "game-source-xbox", Width: 600, Height: 800},
			{Type: core.MediaTypeScreenshot, URL: "https://example.com/igdb-stale.png", Source: "metadata-igdb"},
		},
	}

	games, err := gamesForSourceRefresh([]*core.SourceGame{sourceGame})
	if err != nil {
		t.Fatalf("gamesForSourceRefresh: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("game count = %d, want 1", len(games))
	}
	game := games[0]
	if len(game.ResolverMatches) != 1 {
		t.Fatalf("resolver match count = %d, want 1: %+v", len(game.ResolverMatches), game.ResolverMatches)
	}
	match := game.ResolverMatches[0]
	if match.PluginID != "game-source-xbox" || !match.IsGamePass || !match.XcloudAvailable || match.StoreProductID != "9NFINALFANTASY" || match.XcloudURL == "" {
		t.Fatalf("source-owned xbox match not preserved: %+v", match)
	}
	if len(game.Media) != 1 {
		t.Fatalf("media count = %d, want 1: %+v", len(game.Media), game.Media)
	}
	media := game.Media[0]
	if media.Source != "game-source-xbox" || media.URL != "https://example.com/xbox-cover.jpg" || media.Width != 600 || media.Height != 800 {
		t.Fatalf("source-owned media not preserved: %+v", media)
	}
}

func TestRefreshedSourceGamesToBatchPreservesXboxAndClearsStaleMetadata(t *testing.T) {
	sourceGame := &core.SourceGame{
		ID:            "sg-xbox",
		IntegrationID: "integration-1",
		PluginID:      "game-source-xbox",
		ExternalID:    "xbox-final-fantasy",
		RawTitle:      "Final Fantasy",
		Platform:      core.PlatformWindowsPC,
		Kind:          core.GameKindBaseGame,
		GroupKind:     core.GroupKindSelfContained,
		Status:        "found",
		ResolverMatches: []core.ResolverMatch{
			{
				PluginID:        "game-source-xbox",
				Title:           "Final Fantasy",
				ExternalID:      "xbox-final-fantasy",
				IsGamePass:      true,
				XcloudAvailable: true,
				StoreProductID:  "9NFINALFANTASY",
				XcloudURL:       "https://www.xbox.com/play/games/final-fantasy/9NFINALFANTASY",
			},
			{PluginID: "metadata-igdb", Title: "Final Fantasy 2.0", ExternalID: "igdb-stale"},
		},
		Media: []core.MediaRef{
			{Type: core.MediaTypeCover, URL: "https://example.com/xbox-cover.jpg", Source: "game-source-xbox"},
			{Type: core.MediaTypeScreenshot, URL: "https://example.com/igdb-stale.png", Source: "metadata-igdb"},
		},
	}

	games, err := gamesForSourceRefresh([]*core.SourceGame{sourceGame})
	if err != nil {
		t.Fatalf("gamesForSourceRefresh: %v", err)
	}
	games[0].ResolverMatches = append(games[0].ResolverMatches, core.ResolverMatch{
		PluginID:   "metadata-launchbox",
		Title:      "Final Fantasy",
		ExternalID: "launchbox-final-fantasy",
		Media: []core.MediaItem{{
			Type:   core.MediaTypeScreenshot,
			URL:    "https://example.com/launchbox.png",
			Source: "metadata-launchbox",
		}},
	})

	batch, err := refreshedSourceGamesToBatch([]*core.SourceGame{sourceGame}, games)
	if err != nil {
		t.Fatalf("refreshedSourceGamesToBatch: %v", err)
	}
	matches := batch.ResolverMatches[sourceGame.ID]
	if len(matches) != 2 {
		t.Fatalf("resolver match count = %d, want 2: %+v", len(matches), matches)
	}
	for _, match := range matches {
		if match.PluginID == "metadata-igdb" {
			t.Fatalf("stale metadata match was preserved: %+v", matches)
		}
	}

	media := batch.MediaItems[sourceGame.ID]
	if len(media) != 2 {
		t.Fatalf("media count = %d, want 2: %+v", len(media), media)
	}
	foundXboxMedia := false
	foundLaunchBoxMedia := false
	for _, ref := range media {
		if ref.Source == "metadata-igdb" {
			t.Fatalf("stale metadata media was preserved: %+v", media)
		}
		if ref.Source == "game-source-xbox" && ref.URL == "https://example.com/xbox-cover.jpg" {
			foundXboxMedia = true
		}
		if ref.Source == "metadata-launchbox" && ref.URL == "https://example.com/launchbox.png" {
			foundLaunchBoxMedia = true
		}
	}
	if !foundXboxMedia || !foundLaunchBoxMedia {
		t.Fatalf("expected xbox and launchbox media, got %+v", media)
	}
}
