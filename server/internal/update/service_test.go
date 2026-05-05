package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/buildinfo"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
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

func TestFetchNewestGitHubReleaseManifestIncludesPrereleases(t *testing.T) {
	oldBase := githubReleasesAPIBase
	t.Cleanup(func() { githubReleasesAPIBase = oldBase })

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases":
			_, _ = fmt.Fprintf(w, `[
				{"tag_name":"v0.0.7","draft":false,"prerelease":false,"assets":[]},
				{"tag_name":"v0.0.8-beta","draft":false,"prerelease":true,"assets":[
					{"name":"mga-update.json","browser_download_url":"%s/v0.0.8-beta/mga-update.json"}
				]}
			]`, server.URL)
		case "/v0.0.8-beta/mga-update.json":
			_, _ = fmt.Fprint(w, `{
				"version":"v0.0.8-beta",
				"assets":[
					{"os":"windows","arch":"amd64","type":"portable","url":"https://example.invalid/mga.zip","sha256":"abc","size":12}
				]
			}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	githubReleasesAPIBase = server.URL + "/repos"

	svc := NewService(testConfig{}, testLogger{})
	manifest, err := svc.fetchNewestGitHubReleaseManifest(context.Background(), "https://github.com/owner/repo/releases/latest/download/mga-update.json")
	if err != nil {
		t.Fatalf("fetchNewestGitHubReleaseManifest() error = %v", err)
	}
	if manifest.Version != "v0.0.8-beta" {
		t.Fatalf("manifest version = %q, want v0.0.8-beta", manifest.Version)
	}
}

func TestFetchNewestGitHubReleaseManifestPrefersStableOverMatchingBeta(t *testing.T) {
	oldBase := githubReleasesAPIBase
	t.Cleanup(func() { githubReleasesAPIBase = oldBase })

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases":
			_, _ = fmt.Fprintf(w, `[
				{"tag_name":"v0.0.8-beta","draft":false,"prerelease":true,"assets":[
					{"name":"mga-update.json","browser_download_url":"%s/v0.0.8-beta/mga-update.json"}
				]},
				{"tag_name":"v0.0.8","draft":false,"prerelease":false,"assets":[
					{"name":"mga-update.json","browser_download_url":"%s/v0.0.8/mga-update.json"}
				]}
			]`, server.URL, server.URL)
		case "/v0.0.8-beta/mga-update.json":
			_, _ = fmt.Fprint(w, `{"version":"v0.0.8-beta","assets":[]}`)
		case "/v0.0.8/mga-update.json":
			_, _ = fmt.Fprint(w, `{"version":"v0.0.8","assets":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	githubReleasesAPIBase = server.URL + "/repos"

	svc := NewService(testConfig{}, testLogger{})
	manifest, err := svc.fetchNewestGitHubReleaseManifest(context.Background(), "https://github.com/owner/repo/releases/latest/download/mga-update.json")
	if err != nil {
		t.Fatalf("fetchNewestGitHubReleaseManifest() error = %v", err)
	}
	if manifest.Version != "v0.0.8" {
		t.Fatalf("manifest version = %q, want v0.0.8", manifest.Version)
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

func TestApplyInstallerLaunchesSilentUpdateWithInstallMode(t *testing.T) {
	oldStarter := startDetachedCommand
	t.Cleanup(func() { startDetachedCommand = oldStarter })

	var captured []string
	startDetachedCommand = func(cmd *exec.Cmd) error {
		captured = append([]string{}, cmd.Args...)
		return nil
	}

	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(filepath.Join(appDir, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	svc := NewService(testConfig{
		"APP_INSTALL_TYPE": "service",
		"PLUGINS_DIR":      filepath.Join(appDir, "plugins"),
		"UPDATES_DIR":      filepath.Join(dataDir, "updates"),
	}, testLogger{})

	if err := svc.applyInstaller(coreUpdateStatus("1.1.0", filepath.Join(root, "installer.exe"))); err != nil {
		t.Fatalf("applyInstaller() error = %v", err)
	}
	joined := strings.Join(captured, " ")
	for _, want := range []string{"/VERYSILENT", "/MGAUPDATE=1", "/MGAINSTALLTYPE=service", "/ALLUSERS", "/MGAAPPDIR=" + appDir, "/MGADATADIR=" + dataDir} {
		if !strings.Contains(joined, want) {
			t.Fatalf("installer args %q do not contain %q", joined, want)
		}
	}
}

func TestValidatePortableZipRejectsMalformedPackage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.zip")
	writeZip(t, path, map[string]string{"mga_server.exe": "server"})
	if err := validatePortableZip(path); err == nil {
		t.Fatal("expected malformed portable ZIP to be rejected")
	}
}

func TestApplyPortableWritesPlanAndLaunchesHelper(t *testing.T) {
	oldStarter := startDetachedCommand
	t.Cleanup(func() { startDetachedCommand = oldStarter })
	var captured []string
	startDetachedCommand = func(cmd *exec.Cmd) error {
		captured = append([]string{}, cmd.Args...)
		return nil
	}

	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	updatesDir := filepath.Join(appDir, "updates")
	if err := os.MkdirAll(updatesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "mga_update.ps1"), []byte("param()"), 0o644); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(updatesDir, "mga.zip")
	writeZip(t, zipPath, map[string]string{
		"mga-v1.1.0-windows-amd64/mga_server.exe":             "server",
		"mga-v1.1.0-windows-amd64/plugins/source/plugin.json": "{}",
		"mga-v1.1.0-windows-amd64/frontend/dist/index.html":   "<html></html>",
	})
	svc := NewService(testConfig{
		"APP_INSTALL_TYPE": "portable",
		"PLUGINS_DIR":      filepath.Join(appDir, "plugins"),
		"UPDATES_DIR":      updatesDir,
	}, testLogger{})
	svc.exitProcess = func(int) {}

	if err := svc.applyPortable(coreUpdateStatus("1.1.0", zipPath)); err != nil {
		t.Fatalf("applyPortable() error = %v", err)
	}
	planPath := filepath.Join(updatesDir, "mga_update_plan.json")
	if _, err := os.Stat(planPath); err != nil {
		t.Fatalf("expected update plan: %v", err)
	}
	joined := strings.Join(captured, " ")
	if !strings.Contains(joined, "mga_update.ps1") || !strings.Contains(joined, planPath) {
		t.Fatalf("portable updater args = %q", joined)
	}
}

func coreUpdateStatus(version, path string) *core.UpdateStatus {
	return &core.UpdateStatus{LatestVersion: version, DownloadedPath: path}
}

func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func runtimeGOOS() string   { return runtime.GOOS }
func runtimeGOARCH() string { return runtime.GOARCH }
