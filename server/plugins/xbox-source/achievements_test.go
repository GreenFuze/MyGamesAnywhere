package main

import "testing"

func TestBuildXboxAchievementEntriesKeepsMixedStates(t *testing.T) {
	entries, unlocked, totalPoints, earnedPoints := buildXboxAchievementEntries([]xboxAchievement{
		{
			ID:          "1",
			Name:        "Unlocked One",
			Description: "First",
			Rewards:     []xboxReward{{Type: "Gamerscore", ValueType: "Int", Value: "15"}},
			Progression: &xboxProgression{TimeUnlocked: "2024-03-09T16:00:00Z"},
		},
		{
			ID:          "2",
			Name:        "Locked Two",
			Description: "Second",
			Rewards:     []xboxReward{{Type: "Gamerscore", ValueType: "Int", Value: "5"}},
			Progression: &xboxProgression{TimeUnlocked: "0001-01-01T00:00:00Z"},
		},
	})

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if unlocked != 1 {
		t.Fatalf("unlocked = %d, want 1", unlocked)
	}
	if totalPoints != 20 {
		t.Fatalf("totalPoints = %d, want 20", totalPoints)
	}
	if earnedPoints != 15 {
		t.Fatalf("earnedPoints = %d, want 15", earnedPoints)
	}
	if !entries[0].Unlocked || entries[0].UnlockedAt == "" {
		t.Fatalf("entry[0] = %+v, want unlocked with timestamp", entries[0])
	}
	if entries[1].Unlocked {
		t.Fatalf("entry[1] should remain locked: %+v", entries[1])
	}
	if entries[1].UnlockedAt != "" {
		t.Fatalf("locked entry should not keep unlocked_at, got %q", entries[1].UnlockedAt)
	}
}
