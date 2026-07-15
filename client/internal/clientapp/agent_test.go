package clientapp

import (
	"context"
	"encoding/json"
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
