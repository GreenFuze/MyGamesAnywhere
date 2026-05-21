package scan

import (
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestInstallerAddOnClassifierClassifiesStrongPackedWindowsInstallers(t *testing.T) {
	tests := []struct {
		name string
		want core.GameKind
	}{
		{"setup_lego_dc_super-villains_batman_the_animated_series_level_pack_1.0_(64bit)_(57222)", core.GameKindDLC},
		{"setup_lego_dc_super-villains_dc_movies_character_pack_1.0_(64bit)_(57222)", core.GameKindDLC},
		{"setup_lego_dc_super-villains_shazam_movie_level_pack_1__2_1.0_(64bit)_(57222)", core.GameKindDLC},
		{"setup_lego_batman_3_beyond_gotham_season_pass_1.6_(37309)", core.GameKindDLC},
		{"setup_strategy_game_expansion_pack_2.0_(12345)", core.GameKindExpansion},
	}

	classifier := NewInstallerAddOnClassifier()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			game := packedWindowsInstaller(tt.name)
			classifier.ClassifyAll([]*core.Game{game})
			if game.Kind != tt.want {
				t.Fatalf("kind = %q, want %q", game.Kind, tt.want)
			}
			if !classifier.ShouldAutoArchive(game) {
				t.Fatalf("ShouldAutoArchive = false, want true")
			}
		})
	}
}

func TestInstallerAddOnClassifierIgnoresBaseAndGenericPackNames(t *testing.T) {
	tests := []string{
		"setup_lego_dc_super-villains_1.0_(64bit)_(57222)",
		"setup_space_pack_adventure_1.0_(12345)",
	}

	classifier := NewInstallerAddOnClassifier()
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			game := packedWindowsInstaller(name)
			classifier.ClassifyAll([]*core.Game{game})
			if game.Kind != core.GameKindBaseGame {
				t.Fatalf("kind = %q, want %q", game.Kind, core.GameKindBaseGame)
			}
			if classifier.ShouldAutoArchive(game) {
				t.Fatalf("ShouldAutoArchive = true, want false")
			}
		})
	}
}

func TestInstallerAddOnClassifierRequiresPackedWindowsInstallerEvidence(t *testing.T) {
	game := packedWindowsInstaller("setup_lego_dc_super-villains_character_pack_1.0_(64bit)_(57222)")
	game.GroupKind = core.GroupKindSelfContained

	NewInstallerAddOnClassifier().ClassifyAll([]*core.Game{game})

	if game.Kind != core.GameKindBaseGame {
		t.Fatalf("kind = %q, want %q", game.Kind, core.GameKindBaseGame)
	}
}

func TestGamesToScanBatchArchivesDetectedInstallerAddOns(t *testing.T) {
	addOn := packedWindowsInstaller("setup_lego_dc_super-villains_character_pack_1.0_(64bit)_(57222)")
	NewInstallerAddOnClassifier().ClassifyAll([]*core.Game{addOn})
	base := packedWindowsInstaller("setup_lego_dc_super-villains_1.0_(64bit)_(57222)")

	batch := gamesToScanBatch("integration-1", "game-source-smb", []*core.Game{addOn, base})

	if got := batch.SourceGames[0].ReviewState; got != core.ManualReviewStateNotAGame {
		t.Fatalf("add-on review state = %q, want %q", got, core.ManualReviewStateNotAGame)
	}
	if got := batch.SourceGames[1].ReviewState; got != "" {
		t.Fatalf("base review state = %q, want empty", got)
	}
}

func packedWindowsInstaller(name string) *core.Game {
	return &core.Game{
		ID:        "scan:" + name,
		Title:     name,
		RawTitle:  name,
		Platform:  core.PlatformWindowsPC,
		Kind:      core.GameKindBaseGame,
		GroupKind: core.GroupKindPacked,
		RootPath:  "Installers",
		Status:    "found",
		Files:     []core.GameFile{{FileName: name + ".exe", Path: "Installers/" + name + ".exe", FileKind: "exe"}},
	}
}
