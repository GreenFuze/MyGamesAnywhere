package clientapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/buildinfo"
	"github.com/GreenFuze/MyGamesAnywhere/client/internal/commandipc"
	"github.com/GreenFuze/MyGamesAnywhere/client/internal/singleinstance"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func TestParseProtocolActionPreservesSupportedRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		uri  string
		host string
	}{
		{name: "start", uri: "mga://start?server=http%3A%2F%2Ftv2%3A8900&launch_id=launch&token=secret&mode=elevated", host: "start"},
		{name: "pair", uri: "mga://pair?server=http%3A%2F%2Ftv2%3A8900&code=one-time", host: "pair"},
		{name: "release", uri: "mga://release?installation_id=game-1&server=http%3A%2F%2Ftv2%3A8900", host: "release"},
		{name: "adopt", uri: "mga://adopt?installation_id=game-1&server=http%3A%2F%2Ftv2%3A8900", host: "adopt"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			action, err := parseProtocolAction(test.uri)
			if err != nil {
				t.Fatalf("parseProtocolAction() error = %v", err)
			}
			if action.host != test.host || action.rawURI != test.uri {
				t.Fatalf("action = %#v", action)
			}
		})
	}
}

func TestParseProtocolActionRejectsMalformedOrUnsupportedURI(t *testing.T) {
	t.Parallel()

	for _, rawURI := range []string{
		"",
		"https://tv2:8900",
		"mga://unknown",
		"mga://user@pair?code=secret",
		"mga://pair?code=secret#fragment",
	} {
		if _, err := parseProtocolAction(rawURI); err == nil {
			t.Fatalf("parseProtocolAction(%q) error = nil", rawURI)
		}
	}
}

func TestParseProtocolStartMode(t *testing.T) {
	t.Parallel()

	action, err := parseProtocolAction("mga://start?server=http%3A%2F%2Ftv2%3A8900&mode=elevated")
	if err != nil {
		t.Fatalf("parseProtocolAction() error = %v", err)
	}
	if action.start.ExecutionMode != devicev1.ClientExecutionModeElevated {
		t.Fatalf("execution mode = %q", action.start.ExecutionMode)
	}
}

func TestHandleProtocolRejectsOversizedURIBeforeIPC(t *testing.T) {
	t.Parallel()

	service := &Service{}
	err := service.HandleProtocol(t.Context(), "mga://pair?code="+strings.Repeat("x", 17*1024))
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("HandleProtocol() error = %v, want too large", err)
	}
}

func TestStopForUpgradeSucceedsWhenClientIsNotRunning(t *testing.T) {
	t.Parallel()

	service, err := New(t.TempDir(), buildinfo.Info{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	if err := service.StopForUpgrade(t.Context()); err != nil {
		t.Fatalf("StopForUpgrade() error = %v", err)
	}
}

func TestStopForUpgradeWaitsForAcknowledgedOwnerExit(t *testing.T) {
	t.Parallel()

	service, err := New(t.TempDir(), buildinfo.Info{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	lock, err := singleinstance.Acquire(service.instanceLockName())
	if err != nil {
		t.Fatal(err)
	}
	serverContext, stopServer := context.WithCancel(context.Background())
	server, err := commandipc.NewServer(service.commandEndpoint(), func(_ context.Context, request commandipc.Request) (commandipc.Outcome, error) {
		if request.Action != commandipc.ActionStopForUpgrade || request.URI != "" {
			t.Fatalf("upgrade request = %+v", request)
		}
		return commandipc.Outcome{StopPrimary: true}, nil
	}, func() {
		_ = lock.Close()
		stopServer()
	})
	if err != nil {
		t.Fatal(err)
	}
	serverResult := make(chan error, 1)
	go func() { serverResult <- server.Serve(serverContext) }()

	if err := service.StopForUpgrade(t.Context()); err != nil {
		t.Fatalf("StopForUpgrade() error = %v", err)
	}
	select {
	case err := <-serverResult:
		if err != nil {
			t.Fatalf("command server error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("command server did not stop")
	}
}

func TestStopForUpgradeSurfacesCommandFailure(t *testing.T) {
	t.Parallel()

	service, err := New(t.TempDir(), buildinfo.Info{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	lock, err := singleinstance.Acquire(service.instanceLockName())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	serverContext, stopServer := context.WithCancel(context.Background())
	t.Cleanup(stopServer)
	server, err := commandipc.NewServer(service.commandEndpoint(), func(context.Context, commandipc.Request) (commandipc.Outcome, error) {
		return commandipc.Outcome{}, errors.New("shutdown refused")
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(serverContext) }()

	err = service.StopForUpgrade(t.Context())
	if err == nil || !strings.Contains(err.Error(), "shutdown refused") {
		t.Fatalf("StopForUpgrade() error = %v", err)
	}
}
