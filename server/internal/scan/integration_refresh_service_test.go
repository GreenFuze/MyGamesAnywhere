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
		ID:       "sg-1",
		RawTitle: "Altered Beast",
		Platform: core.PlatformGenesis,
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
		ID:       "sg-1",
		RawTitle: "Altered Beast",
		Platform: core.PlatformGenesis,
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
