package main

import "testing"

func TestBuildRetroAchievementEntriesKeepsMixedStates(t *testing.T) {
	game := &raGameExtended{
		NumDistinctPlayersCasual: 100,
		Achievements: map[string]raAchievement{
			"1": {
				ID:          1,
				Title:       "Unlocked One",
				Description: "First",
				Points:      10,
				NumAwarded:  25,
				BadgeName:   "badge1",
				DateEarned:  "2024-03-09 16:00:00",
			},
			"2": {
				ID:          2,
				Title:       "Locked Two",
				Description: "Second",
				Points:      5,
				NumAwarded:  50,
				BadgeName:   "badge2",
			},
		},
	}

	entries, unlocked, totalPoints, earnedPoints := buildRetroAchievementEntries(game)

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if unlocked != 1 {
		t.Fatalf("unlocked = %d, want 1", unlocked)
	}
	if totalPoints != 15 {
		t.Fatalf("totalPoints = %d, want 15", totalPoints)
	}
	if earnedPoints != 10 {
		t.Fatalf("earnedPoints = %d, want 10", earnedPoints)
	}

	foundUnlocked := false
	foundLocked := false
	for _, entry := range entries {
		switch entry.ExternalID {
		case "1":
			foundUnlocked = true
			if !entry.Unlocked || entry.UnlockedAt == "" {
				t.Fatalf("entry 1 = %+v, want unlocked with timestamp", entry)
			}
		case "2":
			foundLocked = true
			if entry.Unlocked || entry.UnlockedAt != "" {
				t.Fatalf("entry 2 = %+v, want locked without timestamp", entry)
			}
		}
	}
	if !foundUnlocked || !foundLocked {
		t.Fatalf("expected both entries, got %+v", entries)
	}
}
