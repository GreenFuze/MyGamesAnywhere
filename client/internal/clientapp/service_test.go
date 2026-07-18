package clientapp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/buildinfo"
	clientconfig "github.com/GreenFuze/MyGamesAnywhere/client/internal/config"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func TestValidateServerURLAllowsHTTPFromLAN(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "host name", input: "http://tv2:8900", want: "http://tv2:8900"},
		{name: "LAN address", input: "http://192.168.68.51:8900/", want: "http://192.168.68.51:8900"},
		{name: "HTTPS remains supported", input: "https://mga.example.test/", want: "https://mga.example.test"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := validateServerURL(test.input)
			if err != nil {
				t.Fatalf("validateServerURL(%q) error = %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("validateServerURL(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestValidateServerURLRejectsUnsupportedTransport(t *testing.T) {
	if _, err := validateServerURL("ftp://tv2:8900"); err == nil {
		t.Fatal("validateServerURL() accepted unsupported FTP transport")
	}
}

func TestSamePairedServerURLAllowsOnlyEquivalentLoopbackOrigins(t *testing.T) {
	tests := []struct {
		name   string
		launch string
		paired string
		want   bool
	}{
		{name: "localhost and IPv4", launch: "http://127.0.0.1:8900", paired: "http://localhost:8900", want: true},
		{name: "localhost and IPv6", launch: "http://[::1]:8900", paired: "http://localhost:8900", want: true},
		{name: "same remote origin", launch: "https://MGA.example.test", paired: "https://mga.example.test:443", want: true},
		{name: "different remote host", launch: "http://tv2:8900", paired: "http://tc-pc:8900", want: false},
		{name: "remote and loopback", launch: "http://tv2:8900", paired: "http://localhost:8900", want: false},
		{name: "different port", launch: "http://127.0.0.1:8901", paired: "http://localhost:8900", want: false},
		{name: "different scheme", launch: "https://127.0.0.1:8900", paired: "http://localhost:8900", want: false},
		{name: "different path", launch: "http://127.0.0.1:8900/mga", paired: "http://localhost:8900", want: false},
		{name: "lookalike localhost", launch: "http://localhost.example:8900", paired: "http://localhost:8900", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := samePairedServerURL(test.launch, test.paired); got != test.want {
				t.Fatalf("samePairedServerURL(%q, %q) = %t, want %t", test.launch, test.paired, got, test.want)
			}
		})
	}
}

func TestStartURIIncludesRequestedExecutionMode(t *testing.T) {
	uri := startURI(StartOptions{ServerURL: "http://tv2:8900", LaunchID: "launch-1", Token: "token-1"}, devicev1.ClientExecutionModeElevated)
	if !strings.Contains(uri, "mode=elevated") {
		t.Fatalf("startURI() = %q, want elevated mode", uri)
	}
}

func TestAcknowledgeLaunchReportsUnknownServerWithoutRemovingBindings(t *testing.T) {
	service, err := New(t.TempDir(), buildinfo.Info{Version: "test"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })
	binding := clientconfig.Binding{ServerURL: "http://127.0.0.1:8900",
		WebSocketURL: "ws://127.0.0.1:8900/api/devices/connect", EndpointID: "endpoint-1",
		ClientInstanceID: "instance-1", DisplayName: "PC / player",
	}
	if err := service.configs.Save(clientconfig.Document{SchemaVersion: clientconfig.SchemaVersion, Bindings: []clientconfig.Binding{binding}}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	err = service.acknowledgeLaunch(context.Background(), StartOptions{ServerURL: "http://tv2:8900"})
	var mismatch *ServerBindingNotFoundError
	if !errors.As(err, &mismatch) {
		t.Fatalf("acknowledgeLaunch() error = %v, want ServerBindingNotFoundError", err)
	}
	if len(mismatch.BoundServers) != 1 || mismatch.BoundServers[0] != binding.ServerURL || mismatch.RequestedServer != "http://tv2:8900" {
		t.Fatalf("mismatch = %+v", mismatch)
	}
	status, statusErr := service.Status()
	if statusErr != nil || len(status.Bindings) != 1 {
		t.Fatalf("binding was changed: status=%+v err=%v", status, statusErr)
	}
}

func TestLoadBindingsMigratesLegacyOnlyAfterKeyVerification(t *testing.T) {
	dataDir := t.TempDir()
	service, err := New(dataDir, buildinfo.Info{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	legacy := `{"schema_version":1,"server_url":"http://localhost:8900","websocket_url":"ws://localhost:8900/connect","endpoint_id":"old-endpoint","client_instance_id":"old-instance","display_name":"PC / player"}`
	if err := os.WriteFile(filepath.Join(dataDir, "config.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.identityStore(clientconfig.Binding{LegacyIdentity: true}).Save(privateKey); err != nil {
		t.Fatal(err)
	}
	document, err := service.loadBindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Bindings) != 1 || !document.Bindings[0].LegacyIdentity {
		t.Fatalf("migrated document = %+v", document)
	}
	loaded, err := service.configs.Load()
	if err != nil || loaded.LegacyLoaded {
		t.Fatalf("persisted migration = %+v, err=%v", loaded, err)
	}
}

func TestLoadBindingsLeavesLegacyConfigUntouchedWhenKeyIsMissing(t *testing.T) {
	dataDir := t.TempDir()
	service, err := New(dataDir, buildinfo.Info{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	legacy := `{"schema_version":1,"server_url":"http://localhost:8900","websocket_url":"ws://localhost:8900/connect","endpoint_id":"old-endpoint","client_instance_id":"old-instance","display_name":"PC / player"}`
	path := filepath.Join(dataDir, "config.json")
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.loadBindings(); err == nil || !strings.Contains(err.Error(), "verify legacy") {
		t.Fatalf("loadBindings() error = %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != legacy {
		t.Fatal("failed migration modified legacy config")
	}
}

func TestClearBindingRemovesOnlyRequestedServerAndKey(t *testing.T) {
	service, err := New(t.TempDir(), buildinfo.Info{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	one := clientconfig.Binding{ServerURL: "http://localhost:8900", WebSocketURL: "ws://localhost:8900/connect", EndpointID: "one", ClientInstanceID: "one", DisplayName: "PC / player"}
	two := clientconfig.Binding{ServerURL: "http://tv2:8900", WebSocketURL: "ws://tv2:8900/connect", EndpointID: "two", ClientInstanceID: "two", DisplayName: "PC / player"}
	for _, binding := range []clientconfig.Binding{one, two} {
		_, key, keyErr := ed25519.GenerateKey(rand.Reader)
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		if keyErr = service.identityStore(binding).Save(key); keyErr != nil {
			t.Fatal(keyErr)
		}
	}
	if err := service.configs.Save(clientconfig.Document{SchemaVersion: clientconfig.SchemaVersion, Bindings: []clientconfig.Binding{one, two}}); err != nil {
		t.Fatal(err)
	}
	if err := service.clearBinding("http://127.0.0.1:8900", false); err != nil {
		t.Fatal(err)
	}
	status, err := service.Status()
	if err != nil || len(status.Bindings) != 1 || status.Bindings[0].ServerURL != two.ServerURL {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if _, err := service.identityStore(one).Load(); err == nil {
		t.Fatal("removed binding key still exists")
	}
	if _, err := service.identityStore(two).Load(); err != nil {
		t.Fatalf("remaining binding key unavailable: %v", err)
	}
}

func TestPairRejectsEquivalentExistingServerBeforeNetworkRequest(t *testing.T) {
	service, err := New(t.TempDir(), buildinfo.Info{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	binding := clientconfig.Binding{ServerURL: "http://localhost:8900", WebSocketURL: "ws://localhost:8900/connect", EndpointID: "one", ClientInstanceID: "one", DisplayName: "PC / player"}
	if err := service.configs.Save(clientconfig.Document{SchemaVersion: clientconfig.SchemaVersion, Bindings: []clientconfig.Binding{binding}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Pair(context.Background(), PairOptions{ServerURL: "http://127.0.0.1:8900", Code: "unused"}); err == nil || !strings.Contains(err.Error(), "already paired") {
		t.Fatalf("Pair() error = %v", err)
	}
}

func TestLoadBindingsRejectsEquivalentLoopbackOrigins(t *testing.T) {
	service, err := New(t.TempDir(), buildinfo.Info{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	one := clientconfig.Binding{ServerURL: "http://localhost:8900", WebSocketURL: "ws://localhost:8900/connect", EndpointID: "one", ClientInstanceID: "one", DisplayName: "PC / player"}
	two := clientconfig.Binding{ServerURL: "http://127.0.0.1:8900", WebSocketURL: "ws://127.0.0.1:8900/connect", EndpointID: "two", ClientInstanceID: "two", DisplayName: "PC / player"}
	if err := service.configs.Save(clientconfig.Document{SchemaVersion: clientconfig.SchemaVersion, Bindings: []clientconfig.Binding{one, two}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.loadBindings(); err == nil || !strings.Contains(err.Error(), "duplicate equivalent") {
		t.Fatalf("loadBindings() error = %v", err)
	}
}
