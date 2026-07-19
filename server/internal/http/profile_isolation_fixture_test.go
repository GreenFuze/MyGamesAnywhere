package http

import (
	"encoding/json"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/auth"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
)

// profileIsolationFixture is the reusable adversarial identity/data topology
// for route, job, SSE and repository tests. Secrets are test-only sentinels.
type profileIsolationFixture struct {
	Admin, Player, Open *core.Profile
	Credentials         map[string]auth.Credential
	Connections         map[string]*core.Integration
	Games               map[string]*core.CanonicalGame
	Saves               map[string]core.SaveSyncSlotRef
	Jobs                map[string]string
	Notifications       map[string]events.Event
	DeviceGrants        map[string]string
}

func newProfileIsolationFixture(t *testing.T) profileIsolationFixture {
	t.Helper()
	admin := &core.Profile{ID: "profile-admin", DisplayName: "Admin", Role: core.ProfileRoleAdminPlayer}
	player := &core.Profile{ID: "profile-player", DisplayName: "Player", Role: core.ProfileRolePlayer}
	open := &core.Profile{ID: "profile-open", DisplayName: "Guest", Role: core.ProfileRolePlayer}
	ownedEvent := func(profileID, jobID string) events.Event {
		data, err := json.Marshal(map[string]any{"profile_id": profileID, "job_id": jobID, "title": "Shared Title"})
		if err != nil {
			t.Fatal(err)
		}
		return events.Event{Type: "scan_complete", ProfileID: profileID, Data: data}
	}
	return profileIsolationFixture{
		Admin: admin, Player: player, Open: open,
		Credentials:   map[string]auth.Credential{admin.ID: {ProfileID: admin.ID, Hash: "admin-hash"}, player.ID: {ProfileID: player.ID, Hash: "player-hash"}},
		Connections:   map[string]*core.Integration{admin.ID: {ID: "connection-admin", ProfileID: admin.ID, PluginID: "game-source-xbox", Label: "Admin Xbox"}, player.ID: {ID: "connection-player", ProfileID: player.ID, PluginID: "game-source-xbox", Label: "Player Xbox"}},
		Games:         map[string]*core.CanonicalGame{admin.ID: {ID: "game-admin", Title: "Shared Title", SourceGames: []*core.SourceGame{{ID: "source-admin"}}}, player.ID: {ID: "game-player", Title: "Shared Title", SourceGames: []*core.SourceGame{{ID: "source-player"}}}},
		Saves:         map[string]core.SaveSyncSlotRef{admin.ID: {OwnerProfileID: admin.ID, CanonicalGameID: "game-admin", SourceGameID: "source-admin", SlotID: "state-1"}, player.ID: {OwnerProfileID: player.ID, CanonicalGameID: "game-player", SourceGameID: "source-player", SlotID: "state-1"}},
		Jobs:          map[string]string{admin.ID: "job-admin", player.ID: "job-player"},
		Notifications: map[string]events.Event{admin.ID: ownedEvent(admin.ID, "job-admin"), player.ID: ownedEvent(player.ID, "job-player")},
		DeviceGrants:  map[string]string{admin.ID: "manage", player.ID: "play"},
	}
}

func TestProfileIsolationFixtureHasOverlappingTitlesButDistinctAuthorities(t *testing.T) {
	fixture := newProfileIsolationFixture(t)
	if fixture.Games[fixture.Admin.ID].Title != fixture.Games[fixture.Player.ID].Title {
		t.Fatal("fixture must exercise overlapping titles")
	}
	if fixture.Games[fixture.Admin.ID].SourceGames[0].ID == fixture.Games[fixture.Player.ID].SourceGames[0].ID {
		t.Fatal("source identities collided")
	}
	if fixture.Connections[fixture.Admin.ID].ID == fixture.Connections[fixture.Player.ID].ID || fixture.Jobs[fixture.Admin.ID] == fixture.Jobs[fixture.Player.ID] {
		t.Fatal("profile authorities collided")
	}
	if _, protected := fixture.Credentials[fixture.Open.ID]; protected {
		t.Fatal("open profile unexpectedly protected")
	}
}
