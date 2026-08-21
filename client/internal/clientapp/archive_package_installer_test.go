package clientapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func TestLegacyArchiveInstallerRejectsCompressedInstallerWithUpdateAction(t *testing.T) {
	archive := buildTestZIP(t, map[string]string{"setup_game.exe": "installer"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	installer, err := NewManagedArchiveInstaller(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = installer.Install(context.Background(), "legacy-compressed", devicev1.ArchiveInstallRequest{
		GameID: "game", SourceGameID: "source", Title: "Game", DestinationRoot: t.TempDir(), DestinationName: "Game",
		ArchiveName: "game.zip", ArchiveFormat: devicev1.ArchiveFormatZIP, ArchiveSize: uint64(len(archive)), DownloadURL: server.URL, DownloadToken: "token",
	}, nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "update mga server") {
		t.Fatalf("legacy compressed installer error = %v", err)
	}
}

func TestManagedArchivePackageInstallerInstallsCompressedGogWithContainerEvidence(t *testing.T) {
	archive := buildTestZIP(t, map[string]string{
		"package/setup_package_game.exe":   "MZ signed Inno Setup package",
		"package/setup_package_game-1.bin": "companion payload",
		"package/readme.txt":               "Package Game",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer archive-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	catalog, err := OpenOwnershipCatalog(filepath.Join(t.TempDir(), "ownership.json"))
	if err != nil {
		t.Fatal(err)
	}
	ownership, err := NewInstallationOwnership(testBindingOne, server.URL, 1, catalog, NewInstallationCoordinator())
	if err != nil {
		t.Fatal(err)
	}
	archiveInstaller, err := NewOwnedManagedArchiveInstaller(server.URL, ownership)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeInstallerProcessRunner{}
	runner.onStart = func(spec InstallerProcessSpec) error {
		destination := fixedArgumentValue(spec.Arguments, "/DIR=")
		if destination == "" {
			return errors.New("missing fixed destination argument")
		}
		if err := os.MkdirAll(destination, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destination, "Package Game.exe"), []byte("game"), 0o600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(destination, "unins000.exe"), []byte("uninstall"), 0o600)
	}
	gogInstaller, err := NewOwnedManagedGogInnoInstaller(server.URL, validFakeVerifier(), fakeInnoDetector{inno: true}, fakeLocalConfirmer{}, runner, ownership)
	if err != nil {
		t.Fatal(err)
	}
	installer, err := NewManagedArchivePackageInstaller(archiveInstaller, gogInstaller)
	if err != nil {
		t.Fatal(err)
	}

	result, err := installer.Install(context.Background(), "compressed-gog", devicev1.ArchivePackageInstallRequest{
		GameID: "game", SourceGameID: "source", Title: "Package Game", DestinationRoot: t.TempDir(), DestinationName: "Package Game",
		ArchiveName: "package.zip", ArchiveFormat: devicev1.ArchiveFormatZIP, ArchiveSize: uint64(len(archive)),
		DownloadURL: server.URL, DownloadToken: "archive-token",
	}, nil)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.ResolvedKind != devicev1.ArchivePackageKindGogInno || result.GogInno == nil || result.Archive != nil {
		t.Fatalf("archive package result = %#v", result)
	}
	manifest, err := readGogInnoManifest(result.GogInno.InstallPath)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != devicev1.ExecutableInstallManifestSchemaVersion || manifest.PackageContainer == nil ||
		manifest.PackageContainer.FileName != "package.zip" || manifest.PackageContainer.SizeBytes != uint64(len(archive)) {
		t.Fatalf("compressed installer manifest = %+v", manifest)
	}
}

func TestClassifyExtractedArchiveReadyToRunWithPublisherUninstaller(t *testing.T) {
	root := t.TempDir()
	writeArchivePackageTestFile(t, root, "Plasma Pong/Plasma Pong.exe")
	writeArchivePackageTestFile(t, root, "Plasma Pong/unins000.exe")
	writeArchivePackageTestFile(t, root, "Plasma Pong/readme.txt")

	classification, err := classifyExtractedArchive(root)
	if err != nil {
		t.Fatalf("classifyExtractedArchive() error = %v", err)
	}
	if classification.Kind != archivePackageReadyToRun {
		t.Fatalf("classification = %#v, want ready-to-run", classification)
	}
}

func TestClassifyExtractedArchiveExactGogInnoPackage(t *testing.T) {
	root := t.TempDir()
	installer := writeArchivePackageTestFile(t, root, "package/setup_game_1.0_(123).exe")
	companion := writeArchivePackageTestFile(t, root, "package/setup_game_1.0_(123)-1.bin")
	writeArchivePackageTestFile(t, root, "package/readme.txt")

	classification, err := classifyExtractedArchive(root)
	if err != nil {
		t.Fatalf("classifyExtractedArchive() error = %v", err)
	}
	if classification.Kind != archivePackageGogInno || !sameLocalPath(classification.Installer, installer) || len(classification.Companions) != 1 || !sameLocalPath(classification.Companions[0], companion) {
		t.Fatalf("classification = %#v, want exact GOG/Inno package", classification)
	}
}

func TestClassifyExtractedArchiveFailsClosedForAmbiguousExecutableContent(t *testing.T) {
	tests := []struct {
		name  string
		files []string
	}{
		{name: "mixed setup and game", files: []string{"setup_game.exe", "game.exe"}},
		{name: "multiple setups", files: []string{"setup_one.exe", "setup_two.exe"}},
		{name: "mismatched companion", files: []string{"one/setup_game.exe", "two/setup_game-1.bin"}},
		{name: "generic installer", files: []string{"GameInstaller.exe"}},
		{name: "script", files: []string{"launch.cmd"}},
		{name: "MSI", files: []string{"game.msi"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for _, file := range test.files {
				writeArchivePackageTestFile(t, root, file)
			}
			if _, err := classifyExtractedArchive(root); err == nil {
				t.Fatalf("classifyExtractedArchive() accepted %v", test.files)
			}
		})
	}
}

func TestStagedGogInnoRequestCarriesNoLocalPaths(t *testing.T) {
	root := t.TempDir()
	installer := writeArchivePackageTestFile(t, root, "setup_game.exe")
	companion := writeArchivePackageTestFile(t, root, "setup_game-1.bin")
	request, transport, err := stagedGogInnoRequest("command-1", devicev1.ArchiveInstallRequest{
		GameID: "game", SourceGameID: "source", Title: "Game", DestinationRoot: `C:\Games`, DestinationName: "Game",
	}, archivePackageClassification{Kind: archivePackageGogInno, Installer: installer, Companions: []string{companion}})
	if err != nil {
		t.Fatalf("stagedGogInnoRequest() error = %v", err)
	}
	if filepath.IsAbs(request.Installer.DownloadURL) || request.Installer.FileName != "setup_game.exe" || len(request.Companions) != 1 || len(transport.files) != 2 {
		t.Fatalf("staged request leaked or lost package authority: %#v", request)
	}
}

func writeArchivePackageTestFile(t *testing.T, root, relative string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test package bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
