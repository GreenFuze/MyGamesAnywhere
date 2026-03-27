package main

import "testing"

func TestBuildSteamAchievementEntriesKeepsMixedStates(t *testing.T) {
	schema := []schemaAchievement{
		{Name: "ach-1", DisplayName: "Unlocked One", Description: "First", Icon: "u1", IconGray: "l1"},
		{Name: "ach-2", DisplayName: "Locked Two", Description: "Second", Icon: "u2", IconGray: "l2"},
	}
	playerMap := map[string]playerAchievement{
		"ach-1": {APIName: "ach-1", Achieved: 1, UnlockTime: 1710000000},
		"ach-2": {APIName: "ach-2", Achieved: 0, UnlockTime: 1710001111},
	}
	globalRarity := map[string]float64{
		"ach-1": 12.5,
		"ach-2": 48.0,
	}

	entries, unlocked := buildSteamAchievementEntries(schema, playerMap, globalRarity)

	if unlocked != 1 {
		t.Fatalf("unlocked = %d, want 1", unlocked)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if !entries[0].Unlocked || entries[0].UnlockedAt != 1710000000 {
		t.Fatalf("entry[0] = %+v, want unlocked with unix timestamp", entries[0])
	}
	if entries[1].Unlocked {
		t.Fatalf("entry[1] should remain locked: %+v", entries[1])
	}
	if entries[1].UnlockedAt != 0 {
		t.Fatalf("locked entry should not keep unlock time, got %d", entries[1].UnlockedAt)
	}
}
