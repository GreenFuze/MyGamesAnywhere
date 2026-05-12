package http

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/pkg/titlematch"
)

const (
	duplicateModeLoose  = "loose"
	duplicateModeStrict = "strict"
)

type DuplicateGamesResponse struct {
	Mode   string                  `json:"mode"`
	Groups []DuplicateGameGroupDTO `json:"groups"`
}

type DuplicateGameGroupDTO struct {
	ID                  string                   `json:"id"`
	Mode                string                   `json:"mode"`
	RepresentativeTitle string                   `json:"representative_title"`
	NormalizedTitle     string                   `json:"normalized_title"`
	CanonicalIDs        []string                 `json:"canonical_ids"`
	Sources             []DuplicateGameSourceDTO `json:"sources"`
}

type DuplicateGameSourceDTO struct {
	CanonicalGameID       string              `json:"canonical_game_id"`
	CanonicalTitle        string              `json:"canonical_title"`
	Source                SourceGameDetailDTO `json:"source"`
	FileCount             int                 `json:"file_count"`
	TotalSize             int64               `json:"total_size"`
	Cached                bool                `json:"cached"`
	CacheStatuses         []string            `json:"cache_statuses,omitempty"`
	HasCachedAchievements bool                `json:"has_cached_achievements,omitempty"`
}

type duplicateCandidate struct {
	key             string
	normalizedTitle string
	displayTitle    string
	source          DuplicateGameSourceDTO
}

func (c *GameController) DuplicateGames(w http.ResponseWriter, r *http.Request) {
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = duplicateModeLoose
	}
	if mode != duplicateModeLoose && mode != duplicateModeStrict {
		http.Error(w, "mode must be loose or strict", http.StatusBadRequest)
		return
	}

	records, err := c.gameStore.GetDuplicateGameSourceRecords(r.Context())
	if err != nil {
		c.logger.Error("duplicate games", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cacheBySource, err := c.duplicateCacheStatuses(r.Context())
	if err != nil {
		c.logger.Error("duplicate games cache entries", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	labels := c.loadIntegrationLabels(r.Context())
	groups := c.buildDuplicateGameGroups(r.Context(), mode, records, labels, cacheBySource)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(DuplicateGamesResponse{Mode: mode, Groups: groups})
}

func (c *GameController) duplicateCacheStatuses(ctx context.Context) (map[string][]string, error) {
	out := map[string][]string{}
	if c == nil || c.cacheSvc == nil {
		return out, nil
	}
	entries, err := c.cacheSvc.ListEntries(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry == nil || strings.TrimSpace(entry.SourceGameID) == "" {
			continue
		}
		status := strings.TrimSpace(entry.Status)
		if status == "" {
			status = "cached"
		}
		out[entry.SourceGameID] = appendUniqueString(out[entry.SourceGameID], status)
	}
	return out, nil
}

func (c *GameController) buildDuplicateGameGroups(
	ctx context.Context,
	mode string,
	records []core.DuplicateGameSourceRecord,
	integrationLabels map[string]string,
	cacheBySource map[string][]string,
) []DuplicateGameGroupDTO {
	buckets := map[string][]duplicateCandidate{}
	for _, record := range records {
		sourceGame := record.SourceGame
		if sourceGame == nil || sourceGame.Status != "found" {
			continue
		}
		title := duplicateRecordTitle(record)
		normalized := titlematch.NormalizeLookupTitle(title)
		if normalized == "" {
			continue
		}
		key := duplicateGroupKey(mode, record.CanonicalGameID, normalized, sourceGame)
		if key == "" {
			continue
		}
		detail, _, _ := c.sourceGameToDetailDTO(ctx, sourceGame, sourceGame.Platform, supportsBrowserPlayPlatform(sourceGame.Platform), integrationLabels)
		cacheStatuses := append([]string(nil), cacheBySource[sourceGame.ID]...)
		sort.Strings(cacheStatuses)
		buckets[key] = append(buckets[key], duplicateCandidate{
			key:             key,
			normalizedTitle: normalized,
			displayTitle:    title,
			source: DuplicateGameSourceDTO{
				CanonicalGameID:       record.CanonicalGameID,
				CanonicalTitle:        strings.TrimSpace(record.CanonicalTitle),
				Source:                detail,
				FileCount:             record.FileCount,
				TotalSize:             record.TotalSize,
				Cached:                len(cacheStatuses) > 0,
				CacheStatuses:         cacheStatuses,
				HasCachedAchievements: record.HasCachedAchievements,
			},
		})
	}

	groups := make([]DuplicateGameGroupDTO, 0)
	for key, candidates := range buckets {
		if len(candidates) < 2 {
			continue
		}
		sort.Slice(candidates, func(i, j int) bool {
			left := candidates[i].source
			right := candidates[j].source
			if left.CanonicalTitle != right.CanonicalTitle {
				return left.CanonicalTitle < right.CanonicalTitle
			}
			if left.Source.IntegrationLabel != right.Source.IntegrationLabel {
				return left.Source.IntegrationLabel < right.Source.IntegrationLabel
			}
			return left.Source.ID < right.Source.ID
		})

		sources := make([]DuplicateGameSourceDTO, 0, len(candidates))
		canonicalIDs := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			sources = append(sources, candidate.source)
			canonicalIDs = appendUniqueString(canonicalIDs, candidate.source.CanonicalGameID)
		}
		sort.Strings(canonicalIDs)

		groups = append(groups, DuplicateGameGroupDTO{
			ID:                  duplicateGroupID(mode, key),
			Mode:                mode,
			RepresentativeTitle: duplicateRepresentativeTitle(candidates),
			NormalizedTitle:     candidates[0].normalizedTitle,
			CanonicalIDs:        canonicalIDs,
			Sources:             sources,
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		if len(groups[i].Sources) != len(groups[j].Sources) {
			return len(groups[i].Sources) > len(groups[j].Sources)
		}
		if groups[i].RepresentativeTitle != groups[j].RepresentativeTitle {
			return groups[i].RepresentativeTitle < groups[j].RepresentativeTitle
		}
		return groups[i].ID < groups[j].ID
	})
	return groups
}

func duplicateGroupKey(mode, canonicalID, normalizedTitle string, sourceGame *core.SourceGame) string {
	if sourceGame == nil || normalizedTitle == "" {
		return ""
	}
	if mode == duplicateModeStrict {
		return strings.Join([]string{
			mode,
			canonicalID,
			normalizedTitle,
			string(sourceGame.Platform),
			string(sourceGame.Kind),
			string(sourceGame.GroupKind),
		}, "|")
	}
	return duplicateModeLoose + "|" + normalizedTitle
}

func duplicateGroupID(mode, key string) string {
	sum := sha1.Sum([]byte(key))
	return mode + ":" + hex.EncodeToString(sum[:8])
}

func duplicateRecordTitle(record core.DuplicateGameSourceRecord) string {
	if cleaned := titlematch.CleanDisplayTitle(record.CanonicalTitle); cleaned != "" {
		return cleaned
	}
	if record.SourceGame != nil {
		if cleaned := titlematch.CleanDisplayTitle(record.SourceGame.RawTitle); cleaned != "" {
			return cleaned
		}
		return strings.TrimSpace(record.SourceGame.ExternalID)
	}
	return ""
}

func duplicateRepresentativeTitle(candidates []duplicateCandidate) string {
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.displayTitle) != "" {
			return strings.TrimSpace(candidate.displayTitle)
		}
	}
	if len(candidates) > 0 {
		return candidates[0].normalizedTitle
	}
	return ""
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
