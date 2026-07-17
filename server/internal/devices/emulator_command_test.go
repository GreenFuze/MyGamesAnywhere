package devices

import (
	"encoding/json"
	"testing"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func TestEmulatorCommandRequiresPlayAndRedactsEveryTransferToken(t *testing.T) {
	request := devicev1.EmulatorLaunchRequest{
		GameID: "game", SourceGameID: "source", Title: "Game", Platform: "scummvm", EmulatorID: "scummvm",
		Artifacts: []devicev1.EmulatorContentArtifact{
			{Path: "one.dat", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", DownloadURL: "/api/device-transfers/content", DownloadToken: "first-secret"},
			{Path: "two.dat", SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", DownloadURL: "/api/device-transfers/content", DownloadToken: "second-secret"},
		},
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCommandPayload(devicev1.CapabilityGameLaunchEmulator, payload); err != nil {
		t.Fatal(err)
	}
	access, err := requiredAccessForCommand(devicev1.CapabilityGameLaunchEmulator)
	if err != nil || access != devicev1.AccessPlay {
		t.Fatalf("access = %q, error = %v", access, err)
	}
	audit, err := commandPayloadForAudit(devicev1.CapabilityGameLaunchEmulator, payload)
	if err != nil {
		t.Fatal(err)
	}
	var redacted devicev1.EmulatorLaunchRequest
	if err := json.Unmarshal(audit, &redacted); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range redacted.Artifacts {
		if artifact.DownloadToken != "[redacted]" {
			t.Fatalf("audit token = %q", artifact.DownloadToken)
		}
	}
}

func TestEmulatorSetupCommandRequiresOwnerAndValidatesTypedPayload(t *testing.T) {
	payload, err := json.Marshal(devicev1.EmulatorSetupRequest{EmulatorID: "retroarch", Action: "install"})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCommandPayload(devicev1.CapabilityEmulatorSetup, payload); err != nil {
		t.Fatal(err)
	}
	access, err := requiredAccessForCommand(devicev1.CapabilityEmulatorSetup)
	if err != nil || access != devicev1.AccessOwner {
		t.Fatalf("access = %q, error = %v", access, err)
	}
	if err := validateCommandPayload(devicev1.CapabilityEmulatorSetup, json.RawMessage(`{"emulator_id":"retroarch","action":"uninstall"}`)); err == nil {
		t.Fatal("uninstall action was accepted")
	}
}
