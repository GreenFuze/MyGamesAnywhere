package clientapp

import (
	"strings"
	"testing"

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
