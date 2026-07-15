package http

import (
	"path/filepath"
	"testing"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestFindSupportedGogInnoPackage(t *testing.T) {
	t.Parallel()
	game := &core.CanonicalGame{
		SourceGames: []*core.SourceGame{{
			ID:   "source-1",
			Kind: core.GameKindBaseGame,
			Files: []core.GameFile{
				{Path: `setup_duke_nukem_3d_1.5_(28044).exe`, FileName: `setup_duke_nukem_3d_1.5_(28044).exe`},
				{Path: `setup_duke_nukem_3d_1.5_(28044)-1.bin`, FileName: `setup_duke_nukem_3d_1.5_(28044)-1.bin`},
			},
		}},
	}
	source, installer, companions, err := findSupportedGogInnoPackage(game, "source-1")
	if err != nil {
		t.Fatalf("findSupportedGogInnoPackage() error = %v", err)
	}
	if source == nil || installer == nil || len(companions) != 1 {
		t.Fatalf("unexpected package selection: source=%v installer=%v companions=%d", source, installer, len(companions))
	}
	if filepath.Base(installer.Path) != `setup_duke_nukem_3d_1.5_(28044).exe` {
		t.Fatalf("installer = %q", installer.Path)
	}

	game.SourceGames[0].Files = append(game.SourceGames[0].Files, core.GameFile{Path: "extra.exe", FileName: "extra.exe"})
	source, installer, companions, err = findSupportedGogInnoPackage(game, "source-1")
	if err != nil || source == nil || installer != nil || companions != nil {
		t.Fatalf("expected ambiguous multi-exe rejection, got source=%v installer=%v companions=%v err=%v", source, installer, companions, err)
	}

	game.SourceGames[0].Kind = core.GameKindDLC
	game.SourceGames[0].Files = game.SourceGames[0].Files[:2]
	_, _, _, err = findSupportedGogInnoPackage(game, "source-1")
	if err == nil {
		t.Fatal("expected DLC rejection")
	}
	_ = devicev1.GogInnoInstallerFamily
}
