package save_sync

import (
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestValidateSlotRefAgainstGameAllowsCanonicalPlatformFallback(t *testing.T) {
	game := &core.CanonicalGame{
		ID:       "game-1",
		Title:    "Advance Wars",
		Platform: core.PlatformGBA,
		SourceGames: []*core.SourceGame{
			{
				ID:        "source-1",
				Platform:  core.PlatformUnknown,
				Status:    "found",
				CreatedAt: time.Unix(1700000000, 0),
			},
		},
	}

	err := validateSlotRefAgainstGame(game, core.SaveSyncSlotRef{
		CanonicalGameID: "game-1",
		SourceGameID:    "source-1",
		Runtime:         "emulatorjs",
		IntegrationID:   "integration-1",
		SlotID:          "autosave",
	})
	if err != nil {
		t.Fatalf("expected canonical platform fallback to allow runtime, got %v", err)
	}
}

func TestValidateSlotRefAgainstGameAllowsKnownSourceThatIsNotCurrentlyFound(t *testing.T) {
	game := &core.CanonicalGame{
		ID:       "game-1b",
		Title:    "Advance Wars",
		Platform: core.PlatformGBA,
		SourceGames: []*core.SourceGame{
			{
				ID:        "source-1b",
				Platform:  core.PlatformGBA,
				Status:    "missing",
				CreatedAt: time.Unix(1700000000, 0),
			},
		},
	}

	err := validateSlotRefAgainstGame(game, core.SaveSyncSlotRef{
		CanonicalGameID: "game-1b",
		SourceGameID:    "source-1b",
		Runtime:         "emulatorjs",
		IntegrationID:   "integration-1",
		SlotID:          "autosave",
	})
	if err != nil {
		t.Fatalf("expected known source game to allow runtime even when not currently found, got %v", err)
	}
}

func TestValidateSlotRefAgainstGameRejectsMismatchedEffectiveRuntime(t *testing.T) {
	game := &core.CanonicalGame{
		ID:       "game-2",
		Title:    "Monkey Island",
		Platform: core.PlatformScummVM,
		SourceGames: []*core.SourceGame{
			{
				ID:        "source-2",
				Platform:  core.PlatformUnknown,
				Status:    "found",
				CreatedAt: time.Unix(1700000000, 0),
			},
		},
	}

	err := validateSlotRefAgainstGame(game, core.SaveSyncSlotRef{
		CanonicalGameID: "game-2",
		SourceGameID:    "source-2",
		Runtime:         "emulatorjs",
		IntegrationID:   "integration-1",
		SlotID:          "autosave",
	})
	if err == nil || err.Error() != "runtime does not match source game platform" {
		t.Fatalf("expected runtime mismatch, got %v", err)
	}
}

func TestValidateSlotRefAgainstGameRejectsForeignSourceGame(t *testing.T) {
	game := &core.CanonicalGame{
		ID:       "game-3",
		Title:    "Doom",
		Platform: core.PlatformMSDOS,
		SourceGames: []*core.SourceGame{
			{
				ID:        "source-3",
				Platform:  core.PlatformMSDOS,
				Status:    "found",
				CreatedAt: time.Unix(1700000000, 0),
			},
		},
	}

	err := validateSlotRefAgainstGame(game, core.SaveSyncSlotRef{
		CanonicalGameID: "game-3",
		SourceGameID:    "source-missing",
		Runtime:         "jsdos",
		IntegrationID:   "integration-1",
		SlotID:          "autosave",
	})
	if err == nil || err.Error() != "source game does not belong to canonical game" {
		t.Fatalf("expected foreign source rejection, got %v", err)
	}
}
