package clientapp

import (
	"testing"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func TestAgentExecutesTypedStopCommand(t *testing.T) {
	t.Parallel()

	agent := &Agent{}
	payload, stop, code, err := agent.executeEndpointCommand(devicev1.CapabilityEndpointStop)
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
	_, _, code, err := agent.executeEndpointCommand("endpoint.unknown")
	if err == nil || code != "unsupported_command" {
		t.Fatalf("executeEndpointCommand() error = %v, code = %q", err, code)
	}
}
