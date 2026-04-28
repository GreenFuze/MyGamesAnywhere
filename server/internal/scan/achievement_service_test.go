package scan

import (
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestBuildAchievementQueryCandidatesReturnsMultipleSourceBackedSets(t *testing.T) {
	game := &core.CanonicalGame{
		ID: "game-altered-beast",
		SourceGames: []*core.SourceGame{
			{
				ID:         "source-arcade",
				RawTitle:   "Altered Beast (set 8) (8751 317-0078)",
				Platform:   core.PlatformArcade,
				PluginID:   "game-source-mame",
				Status:     "found",
				ExternalID: "mame-altbeast",
				ResolverMatches: []core.ResolverMatch{{
					PluginID:   "retroachievements",
					Title:      "Altered Beast",
					ExternalID: "11975",
				}},
			},
			{
				ID:         "source-genesis",
				RawTitle:   "Altered Beast",
				Platform:   core.PlatformGenesis,
				PluginID:   "game-source-smb",
				Status:     "found",
				ExternalID: "genesis-altbeast",
				ResolverMatches: []core.ResolverMatch{{
					PluginID:   "retroachievements",
					Title:      "Altered Beast",
					ExternalID: "24",
				}},
			},
		},
	}

	candidates := BuildAchievementQueryCandidates(game, []string{"retroachievements"})
	got := candidates["retroachievements"]
	if len(got) != 2 {
		t.Fatalf("candidate count = %d, want 2: %+v", len(got), got)
	}
	bySource := map[string]string{}
	for _, candidate := range got {
		bySource[candidate.SourceGameID] = candidate.ExternalGameID
	}
	if bySource["source-arcade"] != "11975" || bySource["source-genesis"] != "24" {
		t.Fatalf("candidates by source = %+v, want arcade 11975 and genesis 24", bySource)
	}
}

func TestBuildAchievementQueryCandidatesSkipsOutvotedResolverMatches(t *testing.T) {
	game := &core.CanonicalGame{
		ID: "game-final-fantasy",
		SourceGames: []*core.SourceGame{{
			ID:       "source-1",
			RawTitle: "Final Fantasy",
			Platform: core.PlatformWindowsPC,
			PluginID: "game-source-smb",
			Status:   "found",
			ResolverMatches: []core.ResolverMatch{{
				PluginID:   "retroachievements",
				Title:      "Final Fantasy 2.0",
				ExternalID: "wrong",
				Outvoted:   true,
			}},
		}},
	}

	candidates := BuildAchievementQueryCandidates(game, []string{"retroachievements"})
	if got := candidates["retroachievements"]; len(got) != 0 {
		t.Fatalf("outvoted candidates = %+v, want none", got)
	}
}
