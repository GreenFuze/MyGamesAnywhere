package clientapp

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func TestManagedArchiveInstallerInstallAndUninstallZIP(t *testing.T) {
	t.Parallel()
	archive := buildTestZIP(t, map[string]string{"Game/game.exe": "binary", "Game/readme.txt": "hello"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	installer, err := NewManagedArchiveInstaller(server.URL)
	if err != nil {
		t.Fatalf("NewManagedArchiveInstaller() error = %v", err)
	}
	root := t.TempDir()
	request := devicev1.ArchiveInstallRequest{
		GameID: "game-1", SourceGameID: "source-1", Title: "Game", ArchiveName: "game.zip", ArchiveFormat: "zip",
		ArchiveSize: uint64(len(archive)), DownloadURL: "/transfer", DestinationRoot: root, DestinationName: "Game",
		DownloadToken: "secret-token",
	}
	result, err := installer.Install(context.Background(), "command-1", request, nil)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.LaunchTarget != "Game/game.exe" || len(result.LaunchCandidates) != 1 {
		t.Fatalf("launch discovery = target %q, candidates %v", result.LaunchTarget, result.LaunchCandidates)
	}
	if _, err := os.Stat(filepath.Join(result.InstallPath, "Game", "game.exe")); err != nil {
		t.Fatalf("installed game file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.InstallPath, installManifestName)); err != nil {
		t.Fatalf("install manifest missing: %v", err)
	}
	uninstall, err := installer.Uninstall(context.Background(), devicev1.GameUninstallRequest{
		GameID: "game-1", SourceGameID: "source-1", InstallPath: result.InstallPath,
	}, nil)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if !uninstall.Removed {
		t.Fatal("Uninstall() did not report removal")
	}
	if _, err := os.Stat(result.InstallPath); !os.IsNotExist(err) {
		t.Fatalf("install path still exists: %v", err)
	}
}

func TestOwnedArchiveInstallUsesBindingRootAndRejectsOtherServerUninstall(t *testing.T) {
	archive := buildTestZIP(t, map[string]string{"game.exe": "binary"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	catalog, err := OpenOwnershipCatalog(filepath.Join(t.TempDir(), "ownership.json"))
	if err != nil {
		t.Fatal(err)
	}
	coordinator := NewInstallationCoordinator()
	one, _ := NewInstallationOwnership(testBindingOne, server.URL, 2, catalog, coordinator)
	two, _ := NewInstallationOwnership(testBindingTwo, server.URL, 2, catalog, coordinator)
	installerOne, _ := NewOwnedManagedArchiveInstaller(server.URL, one)
	installerTwo, _ := NewOwnedManagedArchiveInstaller(server.URL, two)
	base := t.TempDir()
	request := devicev1.ArchiveInstallRequest{GameID: "same-game", SourceGameID: "same-source", Title: "Game", ArchiveName: "game.zip", ArchiveFormat: "zip", ArchiveSize: uint64(len(archive)), DownloadURL: server.URL, DownloadToken: "token", DestinationRoot: base, DestinationName: "Game"}
	result, err := installerOne.Install(context.Background(), "owned-command", request, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Join(base, "MGA", "127.0.0.1-11111111")
	if !sameLocalPath(result.InstallRoot, wantRoot) {
		t.Fatalf("install root = %s, want %s", result.InstallRoot, wantRoot)
	}
	manifest, err := readInstallManifest(result.InstallPath)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != devicev1.InstallManifestSchemaVersion || manifest.OwnerBindingID != testBindingOne || manifest.LocalInstallationID == "" {
		t.Fatalf("owned manifest = %+v", manifest)
	}
	uninstallRequest := devicev1.GameUninstallRequest{GameID: request.GameID, SourceGameID: request.SourceGameID, InstallPath: result.InstallPath}
	if _, err := installerTwo.Uninstall(context.Background(), uninstallRequest, nil); err == nil {
		t.Fatal("other server uninstalled owned installation")
	}
	if _, err := installerOne.Uninstall(context.Background(), uninstallRequest, nil); err != nil {
		t.Fatal(err)
	}
	if len(catalog.List()) != 0 {
		t.Fatalf("ownership catalog retained removed installation: %+v", catalog.List())
	}
}

func TestDiscoverLaunchTargetsPrefersGameAndSkipsInstallers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, name := range []string{`Plasma Pong/Plasma Pong.exe`, `Plasma Pong/unins000.exe`, `tools/helper.exe`, `Plasma Pong/alternate.exe`} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	candidates, selected, err := discoverLaunchTargets(root, "Plasma Pong")
	if err != nil {
		t.Fatal(err)
	}
	if selected != "Plasma Pong/Plasma Pong.exe" {
		t.Fatalf("selected = %q", selected)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %v", candidates)
	}
}

func TestDiscoverLaunchTargetsRequiresSelectionWhenCandidatesAreAmbiguous(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, name := range []string{"first.exe", "second.exe"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	candidates, selected, err := discoverLaunchTargets(root, "Different Title")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || selected != "" {
		t.Fatalf("launch discovery = target %q, candidates %v", selected, candidates)
	}
}

func TestDiscoverLaunchTargetsDoesNotTreatGameTitleAsCrashHelper(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	executable := filepath.Join(root, "Crash Bandicoot.exe")
	if err := os.WriteFile(executable, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidates, selected, err := discoverLaunchTargets(root, "Crash Bandicoot")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || selected != "Crash Bandicoot.exe" {
		t.Fatalf("launch discovery = target %q, candidates %v", selected, candidates)
	}
}

func TestArchiveInstallerReportsSeparateDownloadAndInstallStages(t *testing.T) {
	t.Parallel()
	archive := buildTestZIP(t, map[string]string{"Game/game.exe": "binary"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	installer, _ := NewManagedArchiveInstaller(server.URL)
	var updates []CommandProgressUpdate
	_, err := installer.Install(context.Background(), "command-progress", devicev1.ArchiveInstallRequest{
		GameID: "game-1", SourceGameID: "source-1", Title: "Game", ArchiveName: "game.zip", ArchiveFormat: "zip",
		ArchiveSize: uint64(len(archive)), DownloadURL: server.URL, DownloadToken: "token", DestinationRoot: t.TempDir(), DestinationName: "Game",
	}, func(update CommandProgressUpdate) error {
		updates = append(updates, update)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	foundDownload, foundInstall := false, false
	for _, update := range updates {
		foundDownload = foundDownload || update.Stage == "download"
		foundInstall = foundInstall || update.Stage == "install"
	}
	if !foundDownload || !foundInstall || updates[len(updates)-1].StagePercent != 100 {
		t.Fatalf("staged updates = %+v", updates)
	}
}

func TestManagedArchiveInstallerRejectsZIPTraversal(t *testing.T) {
	t.Parallel()
	archive := buildTestZIP(t, map[string]string{"../outside.txt": "bad"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	installer, _ := NewManagedArchiveInstaller(server.URL)
	_, err := installer.Install(context.Background(), "command-1", devicev1.ArchiveInstallRequest{
		GameID: "game-1", SourceGameID: "source-1", Title: "Game", ArchiveName: "game.zip", ArchiveFormat: "zip",
		ArchiveSize: uint64(len(archive)), DownloadURL: server.URL, DestinationRoot: t.TempDir(), DestinationName: "Game",
		DownloadToken: "secret-token",
	}, nil)
	if err == nil {
		t.Fatal("Install() accepted a traversal entry")
	}
}

func TestManagedArchiveInstallerInstallsBundled7zAndRARFormats(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		format      string
		fixture     string
		archive     string
		destination string
	}{
		{name: "7z", format: devicev1.ArchiveFormat7Z, fixture: "sevenzip-t1.7z", archive: "game.7z", destination: "SevenZip Game"},
		{name: "rar", format: devicev1.ArchiveFormatRAR, fixture: "rar5-subdirs.rar", archive: "game.rar", destination: "RAR Game"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			archive, err := os.ReadFile(filepath.Join("testdata", test.fixture))
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(archive)
			}))
			defer server.Close()
			installer, err := NewManagedArchiveInstaller(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			result, err := installer.Install(context.Background(), "command-"+test.name, devicev1.ArchiveInstallRequest{
				GameID: "game-" + test.name, SourceGameID: "source-" + test.name, Title: test.destination,
				ArchiveName: test.archive, ArchiveFormat: test.format, ArchiveSize: uint64(len(archive)),
				DownloadURL: server.URL, DownloadToken: "token", DestinationRoot: t.TempDir(), DestinationName: test.destination,
			}, nil)
			if err != nil {
				t.Fatalf("Install() error = %v", err)
			}
			if result.ArchiveBytes != uint64(len(archive)) || countExtractedFiles(t, result.InstallPath) == 0 {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestRARArchiveExtractorRejectsSymbolicLinks(t *testing.T) {
	t.Parallel()
	extractor, err := archiveExtractorForFormat(devicev1.ArchiveFormatRAR)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := extractor.Validate(filepath.Join("testdata", "rar5-symlink-unix.rar")); err == nil {
		t.Fatal("RAR extractor accepted a symbolic link")
	}
}

func countExtractedFiles(t *testing.T, root string) int {
	t.Helper()
	count := 0
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() != installManifestName {
			count++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return count
}

func buildTestZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
