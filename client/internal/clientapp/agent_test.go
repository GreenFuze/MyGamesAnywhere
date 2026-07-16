package clientapp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func TestAgentExecutesTypedStopCommand(t *testing.T) {
	t.Parallel()

	agent := &Agent{}
	payload, stop, code, err := agent.executeEndpointCommand(context.Background(), "command-1", devicev1.CapabilityEndpointStop, json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("executeEndpointCommand() error = %v", err)
	}
	if !stop || code != "" {
		t.Fatalf("executeEndpointCommand() stop = %t, code = %q", stop, code)
	}
	result, ok := payload.(map[string]any)
	if !ok || result["stopping"] != true {
		t.Fatalf("executeEndpointCommand() payload = %#v", payload)
	}
}

func TestAgentRejectsUnknownEndpointCommand(t *testing.T) {
	t.Parallel()

	agent := &Agent{}
	_, _, code, err := agent.executeEndpointCommand(context.Background(), "command-1", "endpoint.unknown", json.RawMessage(`{}`), nil)
	if err == nil || code != "unsupported_command" {
		t.Fatalf("executeEndpointCommand() error = %v, code = %q", err, code)
	}
}

type testInventoryCollector struct{}

func (testInventoryCollector) Collect(context.Context) (devicev1.DeviceInventory, error) {
	return devicev1.DeviceInventory{
		SchemaVersion: devicev1.InventorySchemaVersion,
		CapturedAt:    time.Now(),
		Storage:       []devicev1.StorageInventory{{ID: "c", Root: `C:\`, TotalBytes: 100, FreeBytes: 25}},
	}, nil
}

func TestAgentCollectsTypedDeviceInventory(t *testing.T) {
	t.Parallel()
	agent := &Agent{inventory: testInventoryCollector{}}
	payload, stop, code, err := agent.executeEndpointCommand(context.Background(), "command-1", devicev1.CapabilityInventoryRefresh, json.RawMessage(`{}`), nil)
	if err != nil || stop || code != "" {
		t.Fatalf("executeEndpointCommand() stop=%t code=%q error=%v", stop, code, err)
	}
	inventory, ok := payload.(devicev1.DeviceInventory)
	if !ok || len(inventory.Storage) != 1 {
		t.Fatalf("inventory payload = %#v", payload)
	}
}

type testGogInnoInstaller struct {
	installResult devicev1.GogInnoInstallResult
	installErr    error
}

func (i testGogInnoInstaller) Install(context.Context, string, devicev1.GogInnoInstallRequest, CommandProgressReporter) (devicev1.GogInnoInstallResult, error) {
	return i.installResult, i.installErr
}

func (testGogInnoInstaller) Uninstall(context.Context, devicev1.GogInnoUninstallRequest, CommandProgressReporter) (devicev1.GogInnoUninstallResult, error) {
	return devicev1.GogInnoUninstallResult{}, nil
}

func (testGogInnoInstaller) CleanupFailed(context.Context, devicev1.GogInnoFailedCleanupRequest, CommandProgressReporter) (devicev1.GogInnoFailedCleanupResult, error) {
	return devicev1.GogInnoFailedCleanupResult{}, nil
}

func TestAgentReturnsTypedGogInnoFailureWithPartialPayload(t *testing.T) {
	t.Parallel()
	partial := devicev1.GogInnoInstallResult{GameID: "game", SourceGameID: "source", ProcessID: 4242}
	commandErr := &GogInnoCommandError{Code: "installer_timeout", Message: "installer may still be running", Payload: partial}
	agent := &Agent{gogInstaller: testGogInnoInstaller{installResult: partial, installErr: commandErr}}
	raw, err := json.Marshal(devicev1.GogInnoInstallRequest{})
	if err != nil {
		t.Fatal(err)
	}
	payload, stop, code, err := agent.executeEndpointCommand(
		context.Background(), "command-1", devicev1.CapabilityGameInstallGogInno, raw, nil,
	)
	if !errors.Is(err, commandErr) || stop || code != "installer_timeout" {
		t.Fatalf("executeEndpointCommand() stop=%t code=%q error=%v", stop, code, err)
	}
	result, ok := payload.(devicev1.GogInnoInstallResult)
	if !ok || result.ProcessID != 4242 {
		t.Fatalf("partial payload = %#v", payload)
	}
}

func TestLocalMetadataAdvertisesGogInnoCommands(t *testing.T) {
	metadata, err := localMetadata("Test device", devicev1.ClientExecutionModeStandard)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := make(map[string]bool, len(metadata.Capabilities))
	for _, capability := range metadata.Capabilities {
		capabilities[capability] = true
	}
	if !capabilities[devicev1.CapabilityGameInstallGogInno] || !capabilities[devicev1.CapabilityGameUninstallGogInno] || !capabilities[devicev1.CapabilityGameCleanupGogInnoFailed] {
		t.Fatalf("capabilities = %v", metadata.Capabilities)
	}
}
