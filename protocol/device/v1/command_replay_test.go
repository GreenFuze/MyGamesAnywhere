package v1

import (
	"encoding/json"
	"testing"
	"time"
)

func replayTestRequest() CommandRequest {
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	return CommandRequest{
		CommandID: "command-1", IdempotencyKey: "idem-1", Name: CapabilityGameLaunch,
		SchemaVersion: 1, RequiredLevel: AccessPlay,
		Authorization: AuthorizationContext{ProfileID: "profile-1", GrantedLevel: AccessPlay},
		CreatedAt:     now, ExpiresAt: now.Add(time.Minute), Payload: json.RawMessage(`{"source_game_id":"source-1","game_id":"game-1"}`),
	}
}

func TestCommandRequestFingerprintIgnoresDeliveryIdentityAndWhitespace(t *testing.T) {
	first := replayTestRequest()
	second := first
	second.CommandID = "command-2"
	second.IdempotencyKey = "idem-2"
	second.CreatedAt = second.CreatedAt.Add(time.Second)
	second.ExpiresAt = second.ExpiresAt.Add(time.Second)
	second.Payload = json.RawMessage(`{ "game_id": "game-1", "source_game_id": "source-1" }`)
	left, err := CommandRequestFingerprint(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := CommandRequestFingerprint(second)
	if err != nil {
		t.Fatal(err)
	}
	if left != right || len(left) != 64 {
		t.Fatalf("fingerprints = %q, %q", left, right)
	}
	second.Authorization.ProfileID = "profile-2"
	changed, err := CommandRequestFingerprint(second)
	if err != nil {
		t.Fatal(err)
	}
	if changed == left {
		t.Fatal("profile authority change did not change fingerprint")
	}
}

func TestCommandResultFingerprintIgnoresReplayIdentity(t *testing.T) {
	first := CommandResult{CommandID: "command-1", IdempotencyKey: "idem-1", RequestFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: CommandSucceeded, Payload: json.RawMessage(`{"ok":true}`)}
	second := first
	second.CommandID = "command-2"
	second.IdempotencyKey = "idem-2"
	left, err := CommandResultFingerprint(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := CommandResultFingerprint(second)
	if err != nil {
		t.Fatal(err)
	}
	if left != right {
		t.Fatalf("fingerprints = %q, %q", left, right)
	}
}

func TestCommandReplayContractsValidate(t *testing.T) {
	if !CommandRequiresDurableReplay(CapabilityGameLaunch) || CommandRequiresDurableReplay(CapabilityInventoryRefresh) || !CommandRequiresDurableReplay("future.mutation") {
		t.Fatal("durable replay classification is not fail-closed")
	}
	if err := (CommandReplayAck{CommandID: "command-1", Disposition: CommandReplayRecorded}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (CommandReplayAck{CommandID: "command-1", Disposition: "maybe"}).Validate(); err == nil {
		t.Fatal("unknown replay disposition accepted")
	}
	if err := (CommandReplayComplete{LedgerSchemaVersion: CommandLedgerSchemaVersion, CompletedAt: time.Now()}).Validate(); err != nil {
		t.Fatal(err)
	}
}
