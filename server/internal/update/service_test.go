package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/buildinfo"
)

type testConfig map[string]string

func (c testConfig) Get(key string) string   { return c[key] }
func (c testConfig) GetInt(key string) int   { return 0 }
func (c testConfig) GetBool(key string) bool { return false }
func (c testConfig) Validate() error         { return nil }

type testLogger struct{}

func (testLogger) Info(string, ...any)         {}
func (testLogger) Error(string, error, ...any) {}
func (testLogger) Debug(string, ...any)        {}
func (testLogger) Warn(string, ...any)         {}

func TestCheckSelectsPortableAsset(t *testing.T) {
	oldVersion := buildinfo.Version
	buildinfo.Version = "1.0.0"
	defer func() { buildinfo.Version = oldVersion }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{
			"version":"1.1.0",
			"release_notes_url":"https://example.invalid/release",
			"assets":[
				{"os":"windows","arch":"amd64","type":"portable","url":"https://example.invalid/mga.zip","sha256":"abc","size":12}
			]
		}`)
	}))
	defer server.Close()

	svc := NewService(testConfig{
		"UPDATE_MANIFEST_URL": server.URL,
		"APP_INSTALL_TYPE":    "portable",
	}, testLogger{})

	status, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !status.UpdateAvailable {
		t.Fatalf("UpdateAvailable = false")
	}
	if status.SelectedAsset == nil || status.SelectedAsset.Type != "portable" {
		t.Fatalf("SelectedAsset = %#v", status.SelectedAsset)
	}
}

func TestCheckTreatsStableReleaseAsNewerThanInstalledPrerelease(t *testing.T) {
	oldVersion := buildinfo.Version
	buildinfo.Version = "v0.0.8-beta"
	defer func() { buildinfo.Version = oldVersion }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{
			"version":"v0.0.8",
			"assets":[
				{"os":"%s","arch":"%s","type":"portable","url":"https://example.invalid/mga.zip","sha256":"abc","size":12}
			]
		}`, runtimeGOOS(), runtimeGOARCH())
	}))
	defer server.Close()

	svc := NewService(testConfig{
		"UPDATE_MANIFEST_URL": server.URL,
		"APP_INSTALL_TYPE":    "portable",
	}, testLogger{})

	status, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !status.UpdateAvailable {
		t.Fatalf("UpdateAvailable = false for stable release after installed prerelease")
	}
}

func TestCompareVersionsSupportsSemverPrereleasePrecedence(t *testing.T) {
	tests := []struct {
		name    string
		latest  string
		current string
		want    int
	}{
		{name: "stable beats beta", latest: "v0.0.8", current: "v0.0.8-beta", want: 1},
		{name: "beta is older than stable", latest: "v0.0.8-beta", current: "v0.0.8", want: -1},
		{name: "newer beta beats older stable", latest: "v0.0.9-beta", current: "v0.0.8", want: 1},
		{name: "numeric prerelease ordering", latest: "v0.0.8-beta.10", current: "v0.0.8-beta.2", want: 1},
		{name: "build metadata ignored", latest: "v0.0.8+build.2", current: "v0.0.8+build.1", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := compareVersions(tt.latest, tt.current)
			if !ok {
				t.Fatalf("compareVersions(%q, %q) failed", tt.latest, tt.current)
			}
			if got != tt.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestDownloadRejectsSHA256Mismatch(t *testing.T) {
	oldVersion := buildinfo.Version
	buildinfo.Version = "1.0.0"
	defer func() { buildinfo.Version = oldVersion }()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			_, _ = fmt.Fprintf(w, `{
				"version":"1.1.0",
				"assets":[
					{"os":"%s","arch":"%s","type":"portable","url":"%s/asset.zip","sha256":"0000","size":4}
				]
			}`, runtimeGOOS(), runtimeGOARCH(), server.URL)
			return
		}
		_, _ = w.Write([]byte("data"))
	}))
	defer server.Close()

	svc := NewService(testConfig{
		"UPDATE_MANIFEST_URL": server.URL + "/manifest.json",
		"APP_INSTALL_TYPE":    "portable",
		"UPDATES_DIR":         t.TempDir(),
	}, testLogger{})

	if _, err := svc.Download(context.Background()); err == nil {
		t.Fatalf("Download() expected SHA error")
	}
}

func TestDownloadAcceptsVerifiedAsset(t *testing.T) {
	oldVersion := buildinfo.Version
	buildinfo.Version = "1.0.0"
	defer func() { buildinfo.Version = oldVersion }()

	assetBytes := []byte("installer")
	sum := sha256.Sum256(assetBytes)
	want := hex.EncodeToString(sum[:])

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			_, _ = fmt.Fprintf(w, `{
				"version":"1.1.0",
				"assets":[
					{"os":"%s","arch":"%s","type":"portable","name":"mga.zip","url":"%s/mga.zip","sha256":"%s","size":%d}
				]
			}`, runtimeGOOS(), runtimeGOARCH(), server.URL, want, len(assetBytes))
			return
		}
		_, _ = w.Write(assetBytes)
	}))
	defer server.Close()

	updatesDir := t.TempDir()
	svc := NewService(testConfig{
		"UPDATE_MANIFEST_URL": server.URL + "/manifest.json",
		"APP_INSTALL_TYPE":    "portable",
		"UPDATES_DIR":         updatesDir,
	}, testLogger{})

	result, err := svc.Download(context.Background())
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.SHA256 != want {
		t.Fatalf("SHA256 = %q", result.SHA256)
	}
	if result.Path != filepath.Join(updatesDir, "mga.zip") {
		t.Fatalf("Path = %q", result.Path)
	}
}

func runtimeGOOS() string   { return runtime.GOOS }
func runtimeGOARCH() string { return runtime.GOARCH }
