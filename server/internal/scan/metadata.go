package scan

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const metadataGameLookupMethod = "metadata.game.lookup"

// MetadataSource identifies a metadata plugin and its config.
// Sources are ordered by priority: index 0 is highest priority.
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

type ipcMediaItem struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type metadataMatch struct {
	Index        int    `json:"index"`
	Title        string `json:"title,omitempty"`
	Platform     string `json:"platform,omitempty"`
	Kind         string `json:"kind,omitempty"`
	ParentGameID string `json:"parent_game_id,omitempty"`
	ExternalID   string `json:"external_id"`
	URL          string `json:"url,omitempty"`

	Description string         `json:"description,omitempty"`
	ReleaseDate string         `json:"release_date,omitempty"`
	Genres      []string       `json:"genres,omitempty"`
	Developer   string         `json:"developer,omitempty"`
	Publisher   string         `json:"publisher,omitempty"`
	Media       []ipcMediaItem `json:"media,omitempty"`
	Rating      float64        `json:"rating,omitempty"`
	MaxPlayers  int            `json:"max_players,omitempty"`
}

// MetadataResolver coordinates metadata enrichment across plugins using
// a three-phase approach:
//
//	Phase 1 (Identify): Run all resolvers on raw scan data, collect matches.
//	Phase 2 (Consensus): Group matches by normalized title, majority vote
//	                      picks the canonical title, priority breaks ties.
//	Phase 3 (Fill):      Re-query resolvers that missed or were outvoted
//	                      using the consensus canonical title.
type MetadataResolver struct {
	caller PluginCaller
	logger core.Logger
}

func NewMetadataResolver(caller PluginCaller, logger core.Logger) *MetadataResolver {
	return &MetadataResolver{caller: caller, logger: logger}
}

// Enrich runs the three-phase enrichment pipeline on the provided games.
func (r *MetadataResolver) Enrich(ctx context.Context, games []*core.Game, sources []MetadataSource) {
	if len(games) == 0 || len(sources) == 0 {
		return
	}

	r.logger.Info("metadata: starting enrichment", "games", len(games), "sources", len(sources))

	// Phase 1: call every resolver with raw scan titles.
	r.identify(ctx, games, sources)

	// Phase 2: consensus vote + compute unified fields.
	r.consensus(games, sources)

	// Phase 3: re-query missed/outvoted resolvers with the consensus title.
	r.fill(ctx, games, sources)

	r.logSummary(games)
}

// ── Phase 1: Identify ───────────────────────────────────────────────

func (r *MetadataResolver) identify(ctx context.Context, games []*core.Game, sources []MetadataSource) {
	for _, src := range sources {
		matched := r.callPluginIdentify(ctx, src, games)
		r.logger.Info("metadata phase 1", "plugin", src.PluginID, "matched", matched, "total", len(games))
	}
}

func (r *MetadataResolver) callPluginIdentify(ctx context.Context, src MetadataSource, games []*core.Game) int {
	queries := make([]metadataGameQuery, len(games))
	for i, g := range games {
		queries[i] = metadataGameQuery{
			Index:     i,
			Title:     g.RawTitle,
			Platform:  string(g.Platform),
			RootPath:  g.RootPath,
			GroupKind: string(g.GroupKind),
		}
	}

	params := map[string]any{
		"games":  queries,
		"config": src.Config,
	}

	var resp metadataLookupResponse
	if err := r.caller.Call(ctx, src.PluginID, metadataGameLookupMethod, params, &resp); err != nil {
		r.logger.Error("metadata plugin call failed", fmt.Errorf("%s: %w", src.PluginID, err))
		return 0
	}

	matched := 0
	for _, m := range resp.Results {
		if m.Index < 0 || m.Index >= len(games) {
			continue
		}
		matched++
		games[m.Index].ResolverMatches = append(games[m.Index].ResolverMatches, matchToResolver(src.PluginID, m))
	}
	return matched
}

// ── Phase 2: Consensus ──────────────────────────────────────────────

func (r *MetadataResolver) consensus(games []*core.Game, sources []MetadataSource) {
	identified, unidentified := 0, 0
	for _, g := range games {
		if len(g.ResolverMatches) == 0 {
			g.Status = "unidentified"
			unidentified++
			continue
		}

		runConsensus(g, sources)
		identified++
	}
	r.logger.Info("metadata phase 2", "identified", identified, "unidentified", unidentified)
}

// runConsensus groups a game's resolver matches by normalized title,
// picks the majority group (priority breaks ties), marks losers as
// outvoted, and applies the winning fields to the game.
func runConsensus(g *core.Game, sources []MetadataSource) {
	groups := map[string][]int{}
	for i, m := range g.ResolverMatches {
		key := normalizeForConsensus(m.Title)
		groups[key] = append(groups[key], i)
	}

	winnerKey := pickWinner(groups, g.ResolverMatches, sources)

	for i, m := range g.ResolverMatches {
		key := normalizeForConsensus(m.Title)
		if key != winnerKey {
			g.ResolverMatches[i].Outvoted = true
		}
	}

	applyUnifiedFields(g, sources)
	g.Status = "identified"
}

// pickWinner returns the normalized-title key with the most votes.
// Ties are broken by the highest-priority resolver in each group.
func pickWinner(groups map[string][]int, matches []core.ResolverMatch, sources []MetadataSource) string {
	var winnerKey string
	winnerVotes := -1
	winnerBestPriority := len(sources) + 1

	for key, indices := range groups {
		votes := len(indices)
		bestPri := len(sources) + 1
		for _, idx := range indices {
			if pri := sourcePriority(matches[idx].PluginID, sources); pri < bestPri {
				bestPri = pri
			}
		}
		if votes > winnerVotes || (votes == winnerVotes && bestPri < winnerBestPriority) {
			winnerKey = key
			winnerVotes = votes
			winnerBestPriority = bestPri
		}
	}
	return winnerKey
}

// applyUnifiedFields sets the game's Title, Platform, Kind, ParentGameID,
// and ExternalIDs from the non-outvoted resolver matches, respecting
// source priority order.
func applyUnifiedFields(g *core.Game, sources []MetadataSource) {
	type ranked struct {
		match    core.ResolverMatch
		priority int
	}
	var winners []ranked
	for _, m := range g.ResolverMatches {
		if m.Outvoted {
			continue
		}
		winners = append(winners, ranked{m, sourcePriority(m.PluginID, sources)})
	}
	sort.Slice(winners, func(i, j int) bool {
		return winners[i].priority < winners[j].priority
	})

	titleSet := false
	for _, w := range winners {
		m := w.match
		if m.Title != "" && !titleSet {
			g.Title = m.Title
			titleSet = true
		}
		if m.Platform != "" && g.Platform == core.PlatformUnknown {
			g.Platform = core.Platform(m.Platform)
		}
		if m.Kind != "" && g.Kind == core.GameKindBaseGame {
			g.Kind = core.GameKind(m.Kind)
		}
		if m.ParentGameID != "" && g.ParentGameID == "" {
			g.ParentGameID = m.ParentGameID
		}
		if m.Description != "" && g.Description == "" {
			g.Description = m.Description
		}
		if m.ReleaseDate != "" && g.ReleaseDate == "" {
			g.ReleaseDate = m.ReleaseDate
		}
		if len(m.Genres) > 0 && len(g.Genres) == 0 {
			g.Genres = m.Genres
		}
		if m.Developer != "" && g.Developer == "" {
			g.Developer = m.Developer
		}
		if m.Publisher != "" && g.Publisher == "" {
			g.Publisher = m.Publisher
		}
		g.Media = append(g.Media, m.Media...)
		if m.Rating > 0 && g.Rating == 0 {
			g.Rating = m.Rating
		}
		if m.MaxPlayers > 0 && g.MaxPlayers == 0 {
			g.MaxPlayers = m.MaxPlayers
		}
	}

	g.ExternalIDs = nil
	for _, m := range g.ResolverMatches {
		if m.Outvoted || m.ExternalID == "" {
			continue
		}
		g.ExternalIDs = append(g.ExternalIDs, core.ExternalID{
			Source:     m.PluginID,
			ExternalID: m.ExternalID,
			URL:        m.URL,
		})
	}
}

func sourcePriority(pluginID string, sources []MetadataSource) int {
	for i, s := range sources {
		if s.PluginID == pluginID {
			return i
		}
	}
	return len(sources)
}

// ── Phase 3: Fill ───────────────────────────────────────────────────

func (r *MetadataResolver) fill(ctx context.Context, games []*core.Game, sources []MetadataSource) {
	for _, src := range sources {
		type fillEntry struct {
			gameIdx int
		}
		var entries []fillEntry

		for i, g := range games {
			if g.Status != "identified" {
				continue
			}
			if hasGoodMatch(g, src.PluginID) {
				continue
			}
			entries = append(entries, fillEntry{gameIdx: i})
		}

		if len(entries) == 0 {
			continue
		}

		queries := make([]metadataGameQuery, len(entries))
		for j, e := range entries {
			g := games[e.gameIdx]
			queries[j] = metadataGameQuery{
				Index:     j,
				Title:     g.Title,
				Platform:  string(g.Platform),
				RootPath:  g.RootPath,
				GroupKind: string(g.GroupKind),
			}
		}

		params := map[string]any{
			"games":  queries,
			"config": src.Config,
		}

		var resp metadataLookupResponse
		if err := r.caller.Call(ctx, src.PluginID, metadataGameLookupMethod, params, &resp); err != nil {
			r.logger.Error("metadata fill call failed", fmt.Errorf("%s: %w", src.PluginID, err))
			continue
		}

		filled := 0
		for _, m := range resp.Results {
			if m.Index < 0 || m.Index >= len(entries) {
				continue
			}
			g := games[entries[m.Index].gameIdx]
			filled++

			g.ResolverMatches = append(g.ResolverMatches, matchToResolver(src.PluginID, m))
			if m.ExternalID != "" {
				g.ExternalIDs = append(g.ExternalIDs, core.ExternalID{
					Source:     src.PluginID,
					ExternalID: m.ExternalID,
					URL:        m.URL,
				})
			}
		}
		r.logger.Info("metadata phase 3", "plugin", src.PluginID, "filled", filled, "candidates", len(entries))
	}
}

func hasGoodMatch(g *core.Game, pluginID string) bool {
	for _, m := range g.ResolverMatches {
		if m.PluginID == pluginID && !m.Outvoted {
			return true
		}
	}
	return false
}

// ── Match mapping ───────────────────────────────────────────────────

func matchToResolver(pluginID string, m metadataMatch) core.ResolverMatch {
	media := make([]core.MediaItem, 0, len(m.Media))
	for _, mi := range m.Media {
		media = append(media, core.MediaItem{
			Type:     core.MediaType(mi.Type),
			URL:      mi.URL,
			Width:    mi.Width,
			Height:   mi.Height,
			MimeType: mi.MimeType,
			Source:   pluginID,
		})
	}
	return core.ResolverMatch{
		PluginID:     pluginID,
		Title:        m.Title,
		Platform:     m.Platform,
		Kind:         m.Kind,
		ParentGameID: m.ParentGameID,
		ExternalID:   m.ExternalID,
		URL:          m.URL,
		Description:  m.Description,
		ReleaseDate:  m.ReleaseDate,
		Genres:       m.Genres,
		Developer:    m.Developer,
		Publisher:    m.Publisher,
		Media:        media,
		Rating:       m.Rating,
		MaxPlayers:   m.MaxPlayers,
	}
}

// ── Helpers ─────────────────────────────────────────────────────────

var (
	consensusNonAlphaNum = regexp.MustCompile(`[^a-z0-9\s]+`)
	consensusMultiSpace  = regexp.MustCompile(`\s{2,}`)
)

func normalizeForConsensus(s string) string {
	s = strings.ToLower(s)
	s = consensusNonAlphaNum.ReplaceAllString(s, " ")
	s = consensusMultiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func (r *MetadataResolver) logSummary(games []*core.Game) {
	identified, unidentified := 0, 0
	totalExtIDs := 0
	for _, g := range games {
		if g.Status == "identified" {
			identified++
		} else {
			unidentified++
		}
		totalExtIDs += len(g.ExternalIDs)
	}
	r.logger.Info("metadata: enrichment complete",
		"identified", identified,
		"unidentified", unidentified,
		"total_external_ids", totalExtIDs,
	)
}
