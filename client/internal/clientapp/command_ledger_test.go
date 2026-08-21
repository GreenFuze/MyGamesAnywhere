package clientapp

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func ledgerTestRequest(commandID, idempotencyKey string, createdAt time.Time) devicev1.CommandRequest {
	return devicev1.CommandRequest{
		CommandID: commandID, IdempotencyKey: idempotencyKey, Name: devicev1.CapabilityGameLaunch,
		SchemaVersion: 1, RequiredLevel: devicev1.AccessPlay,
		Authorization: devicev1.AuthorizationContext{ProfileID: "profile-1", GrantedLevel: devicev1.AccessPlay},
		CreatedAt:     createdAt, ExpiresAt: createdAt.Add(2 * time.Minute),
		Payload: json.RawMessage(`{"game_id":"game-1","source_game_id":"source-1"}`),
	}
}

func TestCommandLedgerPersistsTerminalResultAndReplaysDuplicateOnce(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "command-ledger.json")
	ledger, err := openCommandLedger(path, now)
	if err != nil {
		t.Fatal(err)
	}
	request := ledgerTestRequest("command-1", "idem-1", now)
	decision, err := ledger.Begin(testBindingOne, request, now)
	if err != nil || !decision.Execute || decision.Result != nil {
		t.Fatalf("Begin() = %+v, %v", decision, err)
	}
	result := devicev1.CommandResult{CommandID: request.CommandID, Status: devicev1.CommandSucceeded, Payload: json.RawMessage(`{"launched":true}`)}
	if err := ledger.Complete(testBindingOne, request.CommandID, result, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	reloaded, err := openCommandLedger(path, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := reloaded.Begin(testBindingOne, request, now.Add(2*time.Second))
	if err != nil || duplicate.Execute || duplicate.Result == nil || duplicate.Result.Status != devicev1.CommandSucceeded {
		t.Fatalf("duplicate Begin() = %+v, %v", duplicate, err)
	}
	if got := reloaded.ReplayCandidates(testBindingOne, now.Add(2*time.Second)); len(got) != 1 || got[0].IdempotencyKey != "idem-1" {
		t.Fatalf("ReplayCandidates() = %+v", got)
	}
}

func TestCommandLedgerAliasesSameIdempotencyAndRejectsCollision(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	ledger, _ := openCommandLedger(filepath.Join(t.TempDir(), "ledger.json"), now)
	first := ledgerTestRequest("command-1", "idem-shared", now)
	if _, err := ledger.Begin(testBindingOne, first, now); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Complete(testBindingOne, first.CommandID, devicev1.CommandResult{CommandID: first.CommandID, Status: devicev1.CommandSucceeded, Payload: json.RawMessage(`{"ok":true}`)}, now); err != nil {
		t.Fatal(err)
	}
	alias := ledgerTestRequest("command-2", "idem-shared", now.Add(time.Minute))
	decision, err := ledger.Begin(testBindingOne, alias, now.Add(time.Minute))
	if err != nil || decision.Result == nil || decision.Result.CommandID != alias.CommandID {
		t.Fatalf("alias Begin() = %+v, %v", decision, err)
	}
	collision := alias
	collision.CommandID = "command-3"
	collision.Payload = json.RawMessage(`{"game_id":"different","source_game_id":"source-1"}`)
	if _, err := ledger.Begin(testBindingOne, collision, now.Add(time.Minute)); !errors.Is(err, ErrCommandIdempotencyConflict) {
		t.Fatalf("collision error = %v", err)
	}
	otherBinding := ledgerTestRequest("command-3", "idem-shared", now.Add(time.Minute))
	if decision, err := ledger.Begin(testBindingTwo, otherBinding, now.Add(time.Minute)); err != nil || !decision.Execute {
		t.Fatalf("other binding Begin() = %+v, %v", decision, err)
	}
}

func TestCommandLedgerRecoversRunningAsUnknownAndAcknowledges(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "ledger.json")
	ledger, _ := openCommandLedger(path, now)
	request := ledgerTestRequest("command-1", "idem-1", now)
	if _, err := ledger.Begin(testBindingOne, request, now); err != nil {
		t.Fatal(err)
	}
	recovered, err := openCommandLedger(path, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	results := recovered.ReplayCandidates(testBindingOne, now.Add(time.Minute))
	if len(results) != 1 || results[0].Status != devicev1.CommandFailed || results[0].Error == nil || results[0].Error.Code != "command_outcome_unknown" {
		t.Fatalf("recovered results = %+v", results)
	}
	if err := recovered.Acknowledge(testBindingOne, devicev1.CommandReplayAck{CommandID: "command-1", Disposition: devicev1.CommandReplayRecorded}, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := recovered.ReplayCandidates(testBindingOne, request.ExpiresAt.Add(commandLedgerReplaySafety)); len(got) != 0 {
		t.Fatalf("acknowledged expired replay candidates = %+v", got)
	}
}

func TestCommandLedgerRejectsFutureSchemaWithoutChangingFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "ledger.json")
	data := []byte(`{"schema_version":2,"policy_version":1,"records":[]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openCommandLedger(path, time.Now()); err == nil {
		t.Fatal("future command ledger schema was accepted")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(data) {
		t.Fatal("failed ledger load modified the last-good file")
	}
}
