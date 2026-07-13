package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/buildinfo"
	"github.com/GreenFuze/MyGamesAnywhere/client/internal/clientapp"
	clientconfig "github.com/GreenFuze/MyGamesAnywhere/client/internal/config"
)

type fakeClientService struct{}

func (fakeClientService) Pair(context.Context, clientapp.PairOptions) (clientconfig.Config, error) {
	return clientconfig.Config{}, nil
}
func (fakeClientService) RunAgent(context.Context) error    { return nil }
func (fakeClientService) Status() (clientapp.Status, error) { return clientapp.Status{}, nil }
func (fakeClientService) Doctor(context.Context) (clientapp.DoctorResult, error) {
	return clientapp.DoctorResult{}, nil
}
func (fakeClientService) Unpair() error { return nil }

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
