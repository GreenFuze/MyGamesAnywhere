package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestSelectEmulatorContentPathPrefersCueAndRejectsAmbiguity(t *testing.T) {
	artifacts := []devicev1.EmulatorContentArtifact{{Path: "disc.bin"}, {Path: "disc.cue"}}
	got, err := selectEmulatorContentPath("retroarch", artifacts)
	if err != nil || got != "disc.cue" {
		t.Fatalf("content path = %q, error = %v", got, err)
	}
	_, err = selectEmulatorContentPath("retroarch", []devicev1.EmulatorContentArtifact{{Path: "disc1.cue"}, {Path: "disc2.cue"}})
	if err == nil {
		t.Fatal("ambiguous content was accepted")
	}
	got, err = selectEmulatorContentPath("scummvm", artifacts)
	if err != nil || got != "" {
		t.Fatalf("ScummVM content path = %q, error = %v", got, err)
	}
}

func TestCreateEmulatorArtifactsUsesShortLivedVerifiedTransfers(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "data", "game.dat")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("game content"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	registry := newArchiveTransferRegistry()
	registry.now = func() time.Time { return now }
	controller := &DeviceController{archiveTransfers: registry}
	source := &core.SourceGame{
		RootPath: root,
		Files:    []core.GameFile{{Path: "data/game.dat", FileName: "game.dat", Size: 12}},
	}
	artifacts, err := controller.createEmulatorArtifacts(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Path != "data/game.dat" || len(artifacts[0].SHA256) != 64 || artifacts[0].DownloadURL != "/api/device-transfers/content" {
		t.Fatalf("artifacts = %#v", artifacts)
	}
	transfer, ok := registry.Get(artifacts[0].DownloadToken)
	if !ok || transfer.ExpiresAt.Sub(now) != emulatorContentTransferLifetime {
		t.Fatalf("transfer = %#v, live = %t", transfer, ok)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, artifacts[0].DownloadURL, nil)
	request.Header.Set("Authorization", "Bearer "+artifacts[0].DownloadToken)
	controller.ServeArchiveTransfer(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "game content" {
		t.Fatalf("transfer response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestCreateEmulatorArtifactsRejectsSourcePathEscape(t *testing.T) {
	controller := &DeviceController{archiveTransfers: newArchiveTransferRegistry()}
	_, err := controller.createEmulatorArtifacts(&core.SourceGame{
		RootPath: t.TempDir(),
		Files:    []core.GameFile{{Path: "../outside.dat"}},
	})
	if err == nil {
		t.Fatal("source path escape was accepted")
	}
}

func TestCreateEmulatorArtifactsReadsGoogleDriveDesktopSource(t *testing.T) {
	root := t.TempDir()
	relativeRoot := "Games/Platforms/ScummVM/Castle"
	filePath := filepath.Join(root, filepath.FromSlash(relativeRoot), "RESOURCE.MAP")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("game content"), 0o600); err != nil {
		t.Fatal(err)
	}
	controller := &DeviceController{archiveTransfers: newArchiveTransferRegistry(), googleDriveRoot: root}
	source := &core.SourceGame{
		PluginID: "game-source-google-drive",
		RootPath: relativeRoot,
		Files:    []core.GameFile{{Path: relativeRoot + "/RESOURCE.MAP", FileName: "RESOURCE.MAP", Size: 12}},
	}

	artifacts, err := controller.createEmulatorArtifacts(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Path != "RESOURCE.MAP" {
		t.Fatalf("artifacts = %#v", artifacts)
	}
	transfer, ok := controller.archiveTransfers.Get(artifacts[0].DownloadToken)
	if !ok || transfer.Path != filePath {
		t.Fatalf("transfer = %#v, live = %t", transfer, ok)
	}
}

func TestCreateEmulatorArtifactsRejectsGoogleDriveFileOutsideGameRoot(t *testing.T) {
	controller := &DeviceController{archiveTransfers: newArchiveTransferRegistry(), googleDriveRoot: t.TempDir()}
	_, err := controller.createEmulatorArtifacts(&core.SourceGame{
		PluginID: "game-source-google-drive",
		RootPath: "Games/Platforms/ScummVM/Castle",
		Files:    []core.GameFile{{Path: "Games/Platforms/ScummVM/Other/RESOURCE.MAP"}},
	})
	if err == nil {
		t.Fatal("Google Drive file outside the source root was accepted")
	}
}
