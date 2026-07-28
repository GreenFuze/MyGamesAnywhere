package gamesvc

import (
	"sort"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type ContentRelationshipState string

const (
	ContentRelationshipStateNone      ContentRelationshipState = ""
	ContentRelationshipStateKnown     ContentRelationshipState = "known"
	ContentRelationshipStateUnlinked  ContentRelationshipState = "unlinked"
	ContentRelationshipStateAmbiguous ContentRelationshipState = "ambiguous"
)

type ContentRelationshipProjection struct {
	Parent *core.CanonicalGame
	AddOns []*core.CanonicalGame
	State  ContentRelationshipState
}

// ContentRelationshipProjector builds a read-only relationship view from
// provider-scoped external IDs. It deliberately refuses to guess when the
// available metadata points to more than one canonical game.
type ContentRelationshipProjector struct{}

func NewContentRelationshipProjector() *ContentRelationshipProjector {
	return &ContentRelationshipProjector{}
}

func (p *ContentRelationshipProjector) Project(targetID string, games []*core.CanonicalGame) ContentRelationshipProjection {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return ContentRelationshipProjection{}
	}

	byID := make(map[string]*core.CanonicalGame, len(games))
	parentsByExternalID := make(map[string]map[string]*core.CanonicalGame)
	for _, game := range games {
		if game == nil || strings.TrimSpace(game.ID) == "" {
			continue
		}
		byID[game.ID] = game
		for _, source := range game.SourceGames {
			if source == nil || source.Status != "found" {
				continue
			}
			for _, match := range activeRelationshipMatches(source.ResolverMatches) {
				p.addExternalIDCandidate(parentsByExternalID, match.PluginID, match.ExternalID, game)
			}
		}
	}

	target := byID[targetID]
	if target == nil {
		return ContentRelationshipProjection{}
	}

	parentByChildID := make(map[string]*core.CanonicalGame)
	stateByChildID := make(map[string]ContentRelationshipState)
	for _, game := range games {
		if game == nil || !isSupplementalContent(game.Kind) {
			continue
		}
		parent, state := p.resolveParent(game, parentsByExternalID)
		stateByChildID[game.ID] = state
		if parent != nil {
			parentByChildID[game.ID] = parent
		}
	}

	projection := ContentRelationshipProjection{}
	if isSupplementalContent(target.Kind) {
		projection.Parent = parentByChildID[target.ID]
		projection.State = stateByChildID[target.ID]
	}
	for childID, parent := range parentByChildID {
		if parent.ID == target.ID && childID != target.ID {
			projection.AddOns = append(projection.AddOns, byID[childID])
		}
	}
	sort.SliceStable(projection.AddOns, func(i, j int) bool {
		left, right := projection.AddOns[i], projection.AddOns[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if !strings.EqualFold(left.Title, right.Title) {
			return strings.ToLower(left.Title) < strings.ToLower(right.Title)
		}
		return left.ID < right.ID
	})
	return projection
}

func (p *ContentRelationshipProjector) resolveParent(
	game *core.CanonicalGame,
	parentsByExternalID map[string]map[string]*core.CanonicalGame,
) (*core.CanonicalGame, ContentRelationshipState) {
	candidates := make(map[string]*core.CanonicalGame)
	hasReference := false
	for _, source := range game.SourceGames {
		if source == nil || source.Status != "found" {
			continue
		}
		for _, match := range activeRelationshipMatches(source.ResolverMatches) {
			parentID := strings.TrimSpace(match.ParentGameID)
			if parentID == "" {
				continue
			}
			hasReference = true
			for id, candidate := range parentsByExternalID[externalIDKey(match.PluginID, parentID)] {
				if id != game.ID {
					candidates[id] = candidate
				}
			}
		}
	}
	if len(candidates) == 1 {
		for _, candidate := range candidates {
			return candidate, ContentRelationshipStateKnown
		}
	}
	if len(candidates) > 1 {
		return nil, ContentRelationshipStateAmbiguous
	}
	if hasReference {
		return nil, ContentRelationshipStateUnlinked
	}
	return nil, ContentRelationshipStateUnlinked
}

func (p *ContentRelationshipProjector) addExternalIDCandidate(
	index map[string]map[string]*core.CanonicalGame,
	provider string,
	externalID string,
	game *core.CanonicalGame,
) {
	key := externalIDKey(provider, externalID)
	if key == "" || game == nil {
		return
	}
	if index[key] == nil {
		index[key] = make(map[string]*core.CanonicalGame)
	}
	index[key][game.ID] = game
}

func activeRelationshipMatches(matches []core.ResolverMatch) []core.ResolverMatch {
	manual := make([]core.ResolverMatch, 0)
	automatic := make([]core.ResolverMatch, 0)
	for _, match := range matches {
		if match.Outvoted {
			continue
		}
		automatic = append(automatic, match)
		if match.ManualSelection {
			manual = append(manual, match)
		}
	}
	if len(manual) > 0 {
		return manual
	}
	return automatic
}

func externalIDKey(provider string, externalID string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	externalID = strings.TrimSpace(externalID)
	if provider == "" || externalID == "" {
		return ""
	}
	return provider + "\x00" + externalID
}

func isSupplementalContent(kind core.GameKind) bool {
	switch kind {
	case core.GameKindAddon, core.GameKindDLC, core.GameKindPatch, core.GameKindExpansion, core.GameKindExtras:
		return true
	default:
		return false
	}
}
