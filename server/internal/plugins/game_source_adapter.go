package plugins

import (
	"context"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// PluginGameSourceAdapter adapts a plugin that provides source.library.list to core.GameSourceProvider.
type PluginGameSourceAdapter struct {
	pluginHost PluginHost
	pluginID   string
}

// NewPluginGameSourceAdapter returns a GameSourceProvider that calls the given plugin's source.library.list with the provided config.
func NewPluginGameSourceAdapter(pluginHost PluginHost, pluginID string) core.GameSourceProvider {
	return &PluginGameSourceAdapter{pluginHost: pluginHost, pluginID: pluginID}
}

// LibraryListResult is the JSON shape returned by a plugin's source.library.list.
type LibraryListResult struct {
	Games []struct {
		SourceGameKey string         `json:"source_game_key"`
		DisplayName   string         `json:"display_name"`
		ProviderIDs   map[string]any `json:"provider_ids"`
		SourcePayload any            `json:"source_payload"`
	} `json:"games"`
}

// ListGames calls the plugin's source.library.list with config and returns core.GameEntry slice.
func (a *PluginGameSourceAdapter) ListGames(ctx context.Context, config map[string]any) ([]core.GameEntry, error) {
	var result LibraryListResult
	if err := a.pluginHost.Call(ctx, a.pluginID, "source.library.list", config, &result); err != nil {
		return nil, err
	}
	entries := make([]core.GameEntry, 0, len(result.Games))
	for _, g := range result.Games {
		entries = append(entries, core.GameEntry{
			SourceGameKey: g.SourceGameKey,
			DisplayName:   g.DisplayName,
			ProviderIDs:   g.ProviderIDs,
			SourcePayload: g.SourcePayload,
		})
	}
	return entries, nil
}
