package scan

import (
	"context"
	"fmt"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const metadataGameLookupMethod = "metadata.game.lookup"

// MetadataSource identifies a metadata plugin and its config.
type MetadataSource struct {
	PluginID string
	Config   map[string]any
}

// Plugin request/response types.

type metadataLookupRequest struct {
	Games []metadataGameQuery `json:"games"`
}

type metadataGameQuery struct {
	Index     int    `json:"index"`
	Title     string `json:"title"`
	Platform  string `json:"platform"`
	RootPath  string `json:"root_path"`
	GroupKind string `json:"group_kind"`
}

type metadataLookupResponse struct {
	Results []metadataMatch `json:"results"`
}

type metadataMatch struct {
	Index        int    `json:"index"`
	Title        string `json:"title,omitempty"`
	Platform     string `json:"platform,omitempty"`
	Kind         string `json:"kind,omitempty"`
	ParentGameID string `json:"parent_game_id,omitempty"`
	ExternalID   string `json:"external_id"`
	URL          string `json:"url,omitempty"`
}

// MetadataResolver calls metadata plugins in order and enriches games.
//
// Enrichment rules:
//   - All matches always append their ExternalID.
//   - First match (across all plugins) sets Title, Platform, Kind, ParentGameID.
//   - Subsequent matches only fill fields that are still at defaults.
type MetadataResolver struct {
	caller PluginCaller
	logger core.Logger
}

func NewMetadataResolver(caller PluginCaller, logger core.Logger) *MetadataResolver {
	return &MetadataResolver{caller: caller, logger: logger}
}

// Enrich calls each metadata source in order and enriches games in-place.
func (r *MetadataResolver) Enrich(ctx context.Context, games []*core.Game, sources []MetadataSource) {
	if len(games) == 0 || len(sources) == 0 {
		return
	}

	// Track which games have had their title set by a metadata plugin.
	// Title is special: the scanner always provides a raw title, so we
	// need to know whether a metadata plugin has already replaced it.
	titleEnriched := make(map[int]bool, len(games))

	for _, src := range sources {
		matched := r.callPlugin(ctx, src, games, titleEnriched)
		r.logger.Info("metadata enrichment", "plugin", src.PluginID, "matched", matched, "total", len(games))
	}
}

func (r *MetadataResolver) callPlugin(ctx context.Context, src MetadataSource, games []*core.Game, titleEnriched map[int]bool) int {
	req := buildLookupRequest(games)
	if len(req.Games) == 0 {
		return 0
	}

	params := map[string]any{
		"games":  req.Games,
		"config": src.Config,
	}

	var resp metadataLookupResponse
	if err := r.caller.Call(ctx, src.PluginID, metadataGameLookupMethod, params, &resp); err != nil {
		r.logger.Error("metadata plugin call failed", fmt.Errorf("%s: %w", src.PluginID, err))
		return 0
	}

	return applyResults(games, resp.Results, src.PluginID, titleEnriched)
}

func buildLookupRequest(games []*core.Game) metadataLookupRequest {
	queries := make([]metadataGameQuery, 0, len(games))
	for i, g := range games {
		queries = append(queries, metadataGameQuery{
			Index:     i,
			Title:     g.Title,
			Platform:  string(g.Platform),
			RootPath:  g.RootPath,
			GroupKind: string(g.GroupKind),
		})
	}
	return metadataLookupRequest{Games: queries}
}

func applyResults(games []*core.Game, results []metadataMatch, pluginID string, titleEnriched map[int]bool) int {
	matched := 0
	for _, m := range results {
		if m.Index < 0 || m.Index >= len(games) {
			continue
		}
		g := games[m.Index]
		matched++

		// Always append the external ID.
		if m.ExternalID != "" {
			g.ExternalIDs = append(g.ExternalIDs, core.ExternalID{
				Source:     pluginID,
				ExternalID: m.ExternalID,
				URL:        m.URL,
			})
		}

		// First match sets title; subsequent matches leave it alone.
		if m.Title != "" && !titleEnriched[m.Index] {
			g.Title = m.Title
			titleEnriched[m.Index] = true
		}

		// Fill remaining fields only if still at defaults.
		if m.Platform != "" && g.Platform == core.PlatformUnknown {
			g.Platform = core.Platform(m.Platform)
		}
		if m.Kind != "" && g.Kind == core.GameKindBaseGame {
			g.Kind = core.GameKind(m.Kind)
		}
		if m.ParentGameID != "" && g.ParentGameID == "" {
			g.ParentGameID = m.ParentGameID
		}
	}
	return matched
}
