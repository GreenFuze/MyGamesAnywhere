package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/buildinfo"
	"github.com/GreenFuze/MyGamesAnywhere/client/internal/clientapp"
	clientconfig "github.com/GreenFuze/MyGamesAnywhere/client/internal/config"
)

type fakeClientService struct{}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("console unavailable")
}

func (fakeClientService) Pair(context.Context, clientapp.PairOptions) (clientconfig.Binding, error) {
	return clientconfig.Binding{}, nil
}
func (fakeClientService) Start(context.Context, clientapp.StartOptions) error { return nil }
func (fakeClientService) RunAgent(context.Context) error                      { return nil }
func (fakeClientService) RunAgentReplacingExisting(context.Context) error     { return nil }
func (fakeClientService) Status() (clientapp.Status, error)                   { return clientapp.Status{}, nil }
func (fakeClientService) Doctor(context.Context) (clientapp.DoctorResult, error) {
	return clientapp.DoctorResult{}, nil
}
func (fakeClientService) Unpair(clientapp.UnpairOptions) error { return nil }
func (fakeClientService) Installations() ([]clientapp.InstallationOwnershipRecord, error) {
	return nil, nil
}
func (fakeClientService) ReleaseInstallation(clientapp.ReleaseInstallationOptions) error { return nil }
func (fakeClientService) AdoptInstallation(clientapp.AdoptInstallationOptions) error     { return nil }
func (fakeClientService) ConfirmAndReleaseInstallation(context.Context, clientapp.ReleaseInstallationOptions) error {
	return nil
}
func (fakeClientService) ConfirmAndAdoptInstallation(context.Context, clientapp.AdoptInstallationOptions) error {
	return nil
}

func TestNewApplicationFailsWithoutWriters(t *testing.T) {
	t.Parallel()

	info := buildinfo.Info{Version: "test"}
	if _, err := NewApplication(Dependencies{Err: &bytes.Buffer{}, BuildInfo: info, Client: fakeClientService{}}); err == nil {
		t.Fatal("NewApplication() without output writer error = nil, want error")
	}
	if _, err := NewApplication(Dependencies{Out: &bytes.Buffer{}, BuildInfo: info, Client: fakeClientService{}}); err == nil {
		t.Fatal("NewApplication() without error writer error = nil, want error")
	}
}

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	out := &bytes.Buffer{}
	application, err := NewApplication(Dependencies{
		Out: out,
		Err: &bytes.Buffer{},
		BuildInfo: buildinfo.Info{
			Version:   "1.2.3",
			Commit:    "abc123",
			BuildDate: "2026-07-13T12:00:00Z",
		},
		Client: fakeClientService{},
	})
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	if err := application.Execute(context.Background(), []string{"version"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	for _, expected := range []string{"mga-client 1.2.3", "commit: abc123", "protocol: 1-1"} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("version output %q does not contain %q", out.String(), expected)
		}
	}
}

func TestUnknownCommandFails(t *testing.T) {
	t.Parallel()

	application, err := NewApplication(Dependencies{
		Out:       &bytes.Buffer{},
		Err:       &bytes.Buffer{},
		BuildInfo: buildinfo.Info{Version: "test"},
		Client:    fakeClientService{},
	})
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	if err := application.Execute(context.Background(), []string{"unknown"}); err == nil {
		t.Fatal("Execute() error = nil, want unknown command error")
	}
}

func TestProtocolPairDoesNotRequireConsoleOutput(t *testing.T) {
	t.Parallel()

	application, err := NewApplication(Dependencies{
		Out:       failingWriter{},
		Err:       &bytes.Buffer{},
		BuildInfo: buildinfo.Info{Version: "test"},
		Client:    fakeClientService{},
	})
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	if err := application.Execute(context.Background(), []string{
		"protocol",
		"mga://pair?server=http%3A%2F%2Ftv2%3A8900&code=one-time",
	}); err != nil {
		t.Fatalf("Execute(protocol pair) error = %v", err)
	}
}
