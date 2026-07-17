package clientapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type fixedInventory struct {
	inventory devicev1.DeviceInventory
}

func (f fixedInventory) Collect(context.Context) (devicev1.DeviceInventory, error) {
	return f.inventory, nil
}

type recordingEmulatorStarter struct {
	executable string
	arguments  []string
	workingDir string
}

func (s *recordingEmulatorStarter) Start(executable string, arguments []string, workingDirectory string) (int, error) {
	s.executable = executable
	s.arguments = append([]string(nil), arguments...)
	s.workingDir = workingDirectory
	return 1234, nil
}

func TestManagedEmulatorLauncherDownloadsVerifiedContentAndUsesTypedScummVMArguments(t *testing.T) {
	content := []byte("scumm game content")
	digest := sha256.Sum256(content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write(content)
	}))
	defer server.Close()
	executable := filepath.Join(t.TempDir(), "scummvm.exe")
	writeTestFile(t, executable, []byte("exe"))
	inventory := fixedInventory{inventory: devicev1.DeviceInventory{Runtimes: []devicev1.RuntimeInventory{{ID: "scummvm", Name: "ScummVM", Path: executable}}}}
	launcher, err := NewManagedEmulatorLauncher(server.URL, inventory)
	if err != nil {
		t.Fatal(err)
	}
	launcher.cacheRoot = t.TempDir()
	starter := &recordingEmulatorStarter{}
	launcher.start = starter
	request := devicev1.EmulatorLaunchRequest{
		GameID: "game", SourceGameID: "source", Title: "Game", Platform: "scummvm", EmulatorID: "scummvm",
		Artifacts: []devicev1.EmulatorContentArtifact{{Path: "data/game.dat", SizeBytes: uint64(len(content)), SHA256: hex.EncodeToString(digest[:]), DownloadURL: "/content", DownloadToken: "secret"}},
	}
	result, err := launcher.Launch(context.Background(), "command", request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProcessID != 1234 || starter.executable != executable || starter.workingDir == "" {
		t.Fatalf("result=%#v starter=%#v", result, starter)
	}
	if !reflect.DeepEqual(starter.arguments, []string{"--path=" + starter.workingDir, "--auto-detect"}) {
		t.Fatalf("arguments = %#v", starter.arguments)
	}
}

func TestManagedEmulatorLauncherRejectsNonAllowlistedRoute(t *testing.T) {
	launcher := &ManagedEmulatorLauncher{client: http.DefaultClient, inventory: fixedInventory{}, start: &recordingEmulatorStarter{}, now: time.Now}
	request := devicev1.EmulatorLaunchRequest{
		GameID: "game", SourceGameID: "source", Title: "Game", Platform: "ps1", EmulatorID: "custom",
		Artifacts: []devicev1.EmulatorContentArtifact{{Path: "game.iso", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", DownloadURL: "/content", DownloadToken: "secret"}},
	}
	if _, err := launcher.Launch(context.Background(), "command", request, nil); err == nil {
		t.Fatal("non-allowlisted emulator route was accepted")
	}
}

func TestManagedEmulatorLauncherUsesDiscoveredRetroArchCoreAndTypedContent(t *testing.T) {
	content := []byte("rom")
	digest := sha256.Sum256(content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(content) }))
	defer server.Close()
	root := t.TempDir()
	executable := filepath.Join(root, "retroarch.exe")
	coreDirectory := filepath.Join(root, "cores")
	if err := os.MkdirAll(coreDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, executable, []byte("exe"))
	corePath := filepath.Join(coreDirectory, "snes9x_libretro.dll")
	writeTestFile(t, corePath, []byte("core"))
	inventory := fixedInventory{inventory: devicev1.DeviceInventory{Runtimes: []devicev1.RuntimeInventory{{ID: "retroarch", Name: "RetroArch", Path: executable}}}}
	launcher, err := NewManagedEmulatorLauncher(server.URL, inventory)
	if err != nil {
		t.Fatal(err)
	}
	launcher.cacheRoot = t.TempDir()
	starter := &recordingEmulatorStarter{}
	launcher.start = starter
	request := devicev1.EmulatorLaunchRequest{
		GameID: "game", SourceGameID: "source", Title: "Game", Platform: "snes", EmulatorID: "retroarch", CoreID: "snes9x", ContentPath: "game.sfc",
		Artifacts: []devicev1.EmulatorContentArtifact{{Path: "game.sfc", SizeBytes: uint64(len(content)), SHA256: hex.EncodeToString(digest[:]), DownloadURL: "/content", DownloadToken: "secret"}},
	}
	result, err := launcher.Launch(context.Background(), "command", request, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantedContent := filepath.Join(starter.workingDir, "game.sfc")
	if result.CoreID != "snes9x" || !reflect.DeepEqual(starter.arguments, []string{"-L", corePath, wantedContent}) {
		t.Fatalf("result=%#v arguments=%#v", result, starter.arguments)
	}
}

func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}
