package scan

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/pkg/titlematch"
)

const metadataGameLookupMethod = "metadata.game.lookup"
const metadataLookupChunkSize = 10
const maxMetadataSourceConcurrency = 4
const MetadataLookupIntentIdentify = "identify"
const MetadataLookupIntentManualSearch = "manual_search"

// MetadataSource identifies a metadata plugin and its config.
// Sources are ordered by priority: index 0 is highest priority.
type MetadataSource struct {
	IntegrationID string
	Label         string
	PluginID      string
	Config        map[string]any
}

// Plugin request/response types.

type metadataLookupRequest struct {
	Games []metadataGameQuery `json:"games"`
}

type metadataGameQuery struct {
	Index        int    `json:"index"`
	Title        string `json:"title"`
	Platform     string `json:"platform"`
	RootPath     string `json:"root_path"`
	GroupKind    string `json:"group_kind"`
	LookupIntent string `json:"lookup_intent,omitempty"`
}

type metadataLookupResponse struct {
	Results []metadataMatch `json:"results"`
}

type MetadataLookupQuery = metadataGameQuery
type MetadataLookupMatch = metadataMatch
type MetadataLookupMediaItem = ipcMediaItem

type MetadataLookupSourceResult struct {
	Source  MetadataSource
	Matches []MetadataLookupMatch
	Error   error
}

type ipcMediaItem struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type ipcCompletionTime struct {
	MainStory     float64 `json:"main_story,omitempty"`
	MainExtra     float64 `json:"main_extra,omitempty"`
	Completionist float64 `json:"completionist,omitempty"`
}

type metadataMatch struct {
	Index        int    `json:"index"`
	Title        string `json:"title,omitempty"`
	Platform     string `json:"platform,omitempty"`
	Kind         string `json:"kind,omitempty"`
	ParentGameID string `json:"parent_game_id,omitempty"`
	ExternalID   string `json:"external_id"`
	URL          string `json:"url,omitempty"`

	Description    string             `json:"description,omitempty"`
	ReleaseDate    string             `json:"release_date,omitempty"`
	Genres         []string           `json:"genres,omitempty"`
	Developer      string             `json:"developer,omitempty"`
	Publisher      string             `json:"publisher,omitempty"`
	Media          []ipcMediaItem     `json:"media,omitempty"`
	Rating         float64            `json:"rating,omitempty"`
	MaxPlayers     int                `json:"max_players,omitempty"`
	CompletionTime *ipcCompletionTime `json:"completion_time,omitempty"`
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
	caller    PluginCaller
	logger    core.Logger
	publisher ScanEventFunc
}

type MetadataFailurePolicy struct {
	Name   string
	Strict bool
}

type MetadataProviderFailure struct {
	IntegrationID string
	Label         string
	PluginID      string
	Phase         string
	Error         string
}

type MetadataExecutionSummary struct {
	mu               sync.Mutex
	ProviderFailures []MetadataProviderFailure
	Identified       int
	Unidentified     int
}

func (s *MetadataExecutionSummary) Degraded() bool {
	return s != nil && len(s.ProviderFailures) > 0
}

func (s *MetadataExecutionSummary) Error() string {
	if s == nil || len(s.ProviderFailures) == 0 {
		return ""
	}
	failures := make([]string, 0, len(s.ProviderFailures))
	for _, failure := range s.ProviderFailures {
		provider := strings.TrimSpace(failure.Label)
		if provider == "" {
			provider = strings.TrimSpace(failure.PluginID)
		}
		if provider == "" {
			provider = "metadata provider"
		}
		failures = append(failures, fmt.Sprintf("%s %s failed: %s", provider, failure.Phase, failure.Error))
	}
	return strings.Join(failures, "; ")
}

func (s *MetadataExecutionSummary) recordFailure(src MetadataSource, phase string, err error) {
	if s == nil || err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ProviderFailures = append(s.ProviderFailures, MetadataProviderFailure{
		IntegrationID: src.IntegrationID,
		Label:         src.Label,
		PluginID:      src.PluginID,
		Phase:         phase,
		Error:         err.Error(),
	})
}

func (s *MetadataExecutionSummary) setCounts(games []*core.Game) {
	if s == nil {
		return
	}
	identified, unidentified := 0, 0
	for _, g := range games {
		if g.Status == "identified" {
			identified++
			continue
		}
		unidentified++
	}
	s.Identified = identified
	s.Unidentified = unidentified
}

func NewMetadataResolver(caller PluginCaller, logger core.Logger) *MetadataResolver {
	return &MetadataResolver{caller: caller, logger: logger}
}

// SetScanEventPublisher sets an optional callback for SSE / scan progress events (nil disables).
func (r *MetadataResolver) SetScanEventPublisher(fn ScanEventFunc) {
	r.publisher = fn
}

func (r *MetadataResolver) emitScanEvent(ctx context.Context, integrationID, typ string, fields map[string]any) {
	if r.publisher == nil {
		return
	}
	m := make(map[string]any, len(fields)+1)
	for k, v := range fields {
		m[k] = v
	}
	m["integration_id"] = integrationID
	r.publisher(ctx, typ, m)
}

func metadataSourceFields(src MetadataSource) map[string]any {
	return map[string]any{
		"metadata_integration_id": src.IntegrationID,
		"metadata_label":          src.Label,
		"plugin_id":               src.PluginID,
	}
}

// Enrich runs the three-phase enrichment pipeline on the provided games.
func (r *MetadataResolver) Enrich(ctx context.Context, integrationID string, games []*core.Game, sources []MetadataSource) {
	_, _ = r.EnrichWithPolicy(ctx, integrationID, games, sources, MetadataFailurePolicy{Name: "tolerant"})
}

func (r *MetadataResolver) EnrichStrict(ctx context.Context, integrationID string, games []*core.Game, sources []MetadataSource) error {
	_, err := r.EnrichWithPolicy(ctx, integrationID, games, sources, MetadataFailurePolicy{Name: "strict", Strict: true})
	return err
}

func (r *MetadataResolver) EnrichWithPolicy(
	ctx context.Context,
	integrationID string,
	games []*core.Game,
	sources []MetadataSource,
	policy MetadataFailurePolicy,
) (*MetadataExecutionSummary, error) {
	summary := &MetadataExecutionSummary{}
	if len(games) == 0 || len(sources) == 0 {
		summary.setCounts(games)
		return summary, nil
	}

	r.logger.Info("metadata: starting enrichment", "games", len(games), "sources", len(sources), "policy", policy.Name, "strict", policy.Strict)

	if err := r.identifyWithPolicy(ctx, integrationID, games, sources, policy, summary); err != nil {
		summary.setCounts(games)
		return summary, err
	}
	r.consensus(ctx, integrationID, games, sources)
	if err := r.fillWithPolicy(ctx, integrationID, games, sources, policy, summary); err != nil {
		summary.setCounts(games)
		return summary, err
	}

	summary.setCounts(games)
	r.logSummary(games)
	r.emitMetadataFinished(ctx, integrationID, games, summary)
	return summary, nil
}

func (r *MetadataResolver) FillWithPolicy(
	ctx context.Context,
	integrationID string,
	games []*core.Game,
	sources []MetadataSource,
	policy MetadataFailurePolicy,
) (*MetadataExecutionSummary, error) {
	summary := &MetadataExecutionSummary{}
	if len(games) == 0 || len(sources) == 0 {
		summary.setCounts(games)
		return summary, nil
	}
	if err := r.fillWithPolicy(ctx, integrationID, games, sources, policy, summary); err != nil {
		summary.setCounts(games)
		return summary, err
	}
	summary.setCounts(games)
	return summary, nil
}

// ── Phase 1: Identify ───────────────────────────────────────────────

func (r *MetadataResolver) identify(ctx context.Context, integrationID string, games []*core.Game, sources []MetadataSource) {
	_ = r.identifyWithPolicy(ctx, integrationID, games, sources, MetadataFailurePolicy{Name: "tolerant"}, nil)
}

func (r *MetadataResolver) identifyStrict(ctx context.Context, integrationID string, games []*core.Game, sources []MetadataSource) error {
	return r.identifyWithPolicy(ctx, integrationID, games, sources, MetadataFailurePolicy{Name: "strict", Strict: true}, nil)
}

func (r *MetadataResolver) identifyWithPolicy(
	ctx context.Context,
	integrationID string,
	games []*core.Game,
	sources []MetadataSource,
	policy MetadataFailurePolicy,
	summary *MetadataExecutionSummary,
) error {
	r.emitScanEvent(ctx, integrationID, "scan_metadata_phase", map[string]any{"phase": "identify"})
	var matchesMu sync.Mutex
	return runMetadataSourcesConcurrently(ctx, sources, func(runCtx context.Context, src MetadataSource) error {
		matched, err := r.callPluginIdentify(runCtx, integrationID, src, games, &matchesMu)
		if err != nil {
			summary.recordFailure(src, "identify", err)
			if policy.Strict {
				return fmt.Errorf("%w: %s identify failed: %v", core.ErrMetadataProvidersUnavailable, src.PluginID, err)
			}
			return nil
		}
		r.logger.Info("metadata phase 1", "plugin", src.PluginID, "matched", matched, "total", len(games))
		return nil
	})
}

func (r *MetadataResolver) callPluginIdentify(
	ctx context.Context,
	integrationID string,
	src MetadataSource,
	games []*core.Game,
	matchesMu *sync.Mutex,
) (int, error) {
	queries := make([]metadataGameQuery, len(games))
	for i, g := range games {
		queries[i] = metadataGameQuery{
			Index:        i,
			Title:        g.RawTitle,
			Platform:     string(g.Platform),
			RootPath:     g.RootPath,
			GroupKind:    string(g.GroupKind),
			LookupIntent: MetadataLookupIntentIdentify,
		}
	}

	batchSize := len(games)
	fields := metadataSourceFields(src)
	fields["phase"] = "identify"
	fields["batch_size"] = batchSize
	r.emitScanEvent(ctx, integrationID, "scan_metadata_plugin_started", fields)

	matches, err := lookupMetadataSource(ctx, r.caller, src, queries, func(chunkEnd int) {
		fields := metadataSourceFields(src)
		fields["phase"] = "identify"
		fields["game_index"] = chunkEnd
		fields["game_count"] = batchSize
		fields["game_title"] = queries[chunkEnd-1].Title
		r.emitScanEvent(ctx, integrationID, "scan_metadata_game_progress", fields)
	})
	if err != nil {
		r.logger.Error("metadata plugin call failed", fmt.Errorf("%s: %w", src.PluginID, err))
		fields := metadataSourceFields(src)
		fields["phase"] = "identify"
		fields["error"] = err.Error()
		r.emitScanEvent(ctx, integrationID, "scan_metadata_plugin_error", fields)
		return 0, err
	}

	for _, m := range matches {
		if m.Index < 0 || m.Index >= len(games) {
			continue
		}
		if matchesMu != nil {
			matchesMu.Lock()
		}
		games[m.Index].ResolverMatches = append(games[m.Index].ResolverMatches, matchToResolver(src.PluginID, m))
		if matchesMu != nil {
			matchesMu.Unlock()
		}
	}

	fields = metadataSourceFields(src)
	fields["phase"] = "identify"
	fields["matched"] = len(matches)
	fields["total"] = batchSize
	r.emitScanEvent(ctx, integrationID, "scan_metadata_plugin_complete", fields)
	return len(matches), nil
}

func LookupMetadataSources(
	ctx context.Context,
	caller PluginCaller,
	sources []MetadataSource,
	queries []MetadataLookupQuery,
) []MetadataLookupSourceResult {
	if len(sources) == 0 {
		return nil
	}

	type indexedResult struct {
		index  int
		result MetadataLookupSourceResult
	}
	results := make([]MetadataLookupSourceResult, len(sources))
	resultCh := make(chan indexedResult, len(sources))
	sem := make(chan struct{}, metadataSourceConcurrency(len(sources)))
	var wg sync.WaitGroup

	for i, src := range sources {
		index := i
		source := src
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				resultCh <- indexedResult{index: index, result: MetadataLookupSourceResult{Source: source, Error: ctx.Err()}}
				return
			}
			defer func() { <-sem }()

			matches, err := lookupMetadataSource(ctx, caller, source, queries, nil)
			resultCh <- indexedResult{index: index, result: MetadataLookupSourceResult{
				Source:  source,
				Matches: matches,
				Error:   err,
			}}
		}()
	}

	wg.Wait()
	close(resultCh)
	for item := range resultCh {
		results[item.index] = item.result
	}
	return results
}

func lookupMetadataSource(
	ctx context.Context,
	caller PluginCaller,
	src MetadataSource,
	queries []metadataGameQuery,
	onChunkComplete func(chunkEnd int),
) ([]metadataMatch, error) {
	if len(queries) == 0 {
		return nil, nil
	}

	matches := make([]metadataMatch, 0, len(queries))
	for chunkStart := 0; chunkStart < len(queries); chunkStart += metadataLookupChunkSize {
		chunkEnd := chunkStart + metadataLookupChunkSize
		if chunkEnd > len(queries) {
			chunkEnd = len(queries)
		}

		params := map[string]any{
			"games":  queries[chunkStart:chunkEnd],
			"config": src.Config,
		}

		var resp metadataLookupResponse
		if err := caller.Call(ctx, src.PluginID, metadataGameLookupMethod, params, &resp); err != nil {
			return matches, err
		}
		matches = append(matches, resp.Results...)
		if onChunkComplete != nil {
			onChunkComplete(chunkEnd)
		}
	}
	return matches, nil
}

// ── Phase 2: Consensus ──────────────────────────────────────────────

func (r *MetadataResolver) consensus(ctx context.Context, integrationID string, games []*core.Game, sources []MetadataSource) {
	r.emitScanEvent(ctx, integrationID, "scan_metadata_phase", map[string]any{"phase": "consensus"})
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
	r.emitScanEvent(ctx, integrationID, "scan_metadata_consensus_complete", map[string]any{
		"identified":   identified,
		"unidentified": unidentified,
	})
}

// runConsensus groups a game's resolver matches by normalized title,
// picks the majority group (priority breaks ties), marks losers as
// outvoted, and applies the winning fields to the game.
func runConsensus(g *core.Game, sources []MetadataSource) {
	for _, match := range g.ResolverMatches {
		if !match.ManualSelection {
			continue
		}
		selectedKey := normalizeForConsensus(match.Title)
		for i, current := range g.ResolverMatches {
			currentKey := normalizeForConsensus(current.Title)
			g.ResolverMatches[i].Outvoted = selectedKey != "" && currentKey != selectedKey
		}
		applyUnifiedFields(g, sources)
		g.Status = "identified"
		return
	}

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
		if winners[i].match.ManualSelection != winners[j].match.ManualSelection {
			return winners[i].match.ManualSelection
		}
		return winners[i].priority < winners[j].priority
	})

	titleSet := false
	for _, w := range winners {
		m := w.match
		if m.Title != "" && !titleSet {
			g.Title = titlematch.CleanDisplayTitle(m.Title)
			if g.Title == "" {
				g.Title = m.Title
			}
			titleSet = true
		}
		if m.Platform != "" && g.Platform == core.PlatformUnknown {
			if normalized := core.NormalizePlatformAlias(m.Platform); normalized != core.PlatformUnknown {
				g.Platform = normalized
			}
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
		if m.CompletionTime != nil && g.CompletionTime == nil {
			g.CompletionTime = m.CompletionTime
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

func (r *MetadataResolver) fill(ctx context.Context, integrationID string, games []*core.Game, sources []MetadataSource) {
	_ = r.fillWithPolicy(ctx, integrationID, games, sources, MetadataFailurePolicy{Name: "tolerant"}, nil)
}

func (r *MetadataResolver) fillStrict(ctx context.Context, integrationID string, games []*core.Game, sources []MetadataSource) error {
	return r.fillWithPolicy(ctx, integrationID, games, sources, MetadataFailurePolicy{Name: "strict", Strict: true}, nil)
}

func (r *MetadataResolver) fillWithPolicy(
	ctx context.Context,
	integrationID string,
	games []*core.Game,
	sources []MetadataSource,
	policy MetadataFailurePolicy,
	summary *MetadataExecutionSummary,
) error {
	r.emitScanEvent(ctx, integrationID, "scan_metadata_phase", map[string]any{"phase": "fill"})
	var matchesMu sync.Mutex
	return runMetadataSourcesConcurrently(ctx, sources, func(runCtx context.Context, src MetadataSource) error {
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
			return nil
		}

		queries := make([]metadataGameQuery, len(entries))
		for j, e := range entries {
			g := games[e.gameIdx]
			queries[j] = metadataGameQuery{
				Index:        j,
				Title:        g.Title,
				Platform:     string(g.Platform),
				RootPath:     g.RootPath,
				GroupKind:    string(g.GroupKind),
				LookupIntent: MetadataLookupIntentIdentify,
			}
		}

		params := map[string]any{
			"config": src.Config,
		}

		candidates := len(entries)
		fields := metadataSourceFields(src)
		fields["phase"] = "fill"
		fields["batch_size"] = candidates
		r.emitScanEvent(runCtx, integrationID, "scan_metadata_plugin_started", fields)

		filled := 0
		for chunkStart := 0; chunkStart < len(queries); chunkStart += metadataLookupChunkSize {
			chunkEnd := chunkStart + metadataLookupChunkSize
			if chunkEnd > len(queries) {
				chunkEnd = len(queries)
			}

			params["games"] = queries[chunkStart:chunkEnd]

			var resp metadataLookupResponse
			if err := r.caller.Call(runCtx, src.PluginID, metadataGameLookupMethod, params, &resp); err != nil {
				r.logger.Error("metadata fill call failed", fmt.Errorf("%s: %w", src.PluginID, err))
				fields := metadataSourceFields(src)
				fields["phase"] = "fill"
				fields["error"] = err.Error()
				r.emitScanEvent(runCtx, integrationID, "scan_metadata_plugin_error", fields)
				summary.recordFailure(src, "fill", err)
				if policy.Strict {
					return fmt.Errorf("%w: %s fill failed: %v", core.ErrMetadataProvidersUnavailable, src.PluginID, err)
				}
				return nil
			}

			for _, m := range resp.Results {
				if m.Index < chunkStart || m.Index >= chunkEnd {
					continue
				}
				if !fillMatchMatchesConsensusTitle(queries[m.Index].Title, m.Title) {
					continue
				}
				entry := entries[m.Index]
				g := games[entry.gameIdx]
				filled++

				matchesMu.Lock()
				g.ResolverMatches = append(g.ResolverMatches, matchToResolver(src.PluginID, m))
				if m.ExternalID != "" {
					g.ExternalIDs = append(g.ExternalIDs, core.ExternalID{
						Source:     src.PluginID,
						ExternalID: m.ExternalID,
						URL:        m.URL,
					})
				}
				matchesMu.Unlock()
			}

			fields := metadataSourceFields(src)
			fields["phase"] = "fill"
			fields["game_index"] = chunkEnd
			fields["game_count"] = candidates
			fields["game_title"] = queries[chunkEnd-1].Title
			r.emitScanEvent(runCtx, integrationID, "scan_metadata_game_progress", fields)
		}

		r.logger.Info("metadata phase 3", "plugin", src.PluginID, "filled", filled, "candidates", len(entries))
		fields = metadataSourceFields(src)
		fields["phase"] = "fill"
		fields["filled"] = filled
		fields["candidates"] = candidates
		r.emitScanEvent(runCtx, integrationID, "scan_metadata_plugin_complete", fields)
		return nil
	})
}

func fillMatchMatchesConsensusTitle(consensusTitle, matchTitle string) bool {
	consensusKey := normalizeForConsensus(consensusTitle)
	matchKey := normalizeForConsensus(matchTitle)
	return consensusKey != "" && matchKey != "" && consensusKey == matchKey
}

func runMetadataSourcesConcurrently(
	ctx context.Context,
	sources []MetadataSource,
	fn func(context.Context, MetadataSource) error,
) error {
	if len(sources) == 0 {
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, metadataSourceConcurrency(len(sources)))
	errCh := make(chan error, len(sources))
	var wg sync.WaitGroup

	for _, src := range sources {
		source := src
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-runCtx.Done():
				return
			}
			defer func() { <-sem }()

			if err := fn(runCtx, source); err != nil {
				select {
				case errCh <- err:
				default:
				}
				cancel()
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func metadataSourceConcurrency(total int) int {
	if total <= 1 {
		return 1
	}
	if total < maxMetadataSourceConcurrency {
		return total
	}
	return maxMetadataSourceConcurrency
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
	rm := core.ResolverMatch{
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
	if m.CompletionTime != nil {
		rm.CompletionTime = &core.CompletionTime{
			MainStory:     m.CompletionTime.MainStory,
			MainExtra:     m.CompletionTime.MainExtra,
			Completionist: m.CompletionTime.Completionist,
			Source:        pluginID,
		}
	}
	return rm
}

// ── Helpers ─────────────────────────────────────────────────────────

func normalizeForConsensus(s string) string {
	return titlematch.NormalizeLookupTitle(s)
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

func (r *MetadataResolver) emitMetadataFinished(ctx context.Context, integrationID string, games []*core.Game, summary *MetadataExecutionSummary) {
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
	r.emitScanEvent(ctx, integrationID, "scan_metadata_finished", map[string]any{
		"identified":        identified,
		"unidentified":      unidentified,
		"external_id_count": totalExtIDs,
		"status":            metadataSummaryStatus(summary),
		"error_count":       metadataSummaryErrorCount(summary),
	})
}

func metadataSummaryStatus(summary *MetadataExecutionSummary) string {
	if summary != nil && summary.Degraded() {
		return "degraded"
	}
	return "ok"
}

func metadataSummaryErrorCount(summary *MetadataExecutionSummary) int {
	if summary == nil {
		return 0
	}
	return len(summary.ProviderFailures)
}
