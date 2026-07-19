package clientapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type fakeGameProcessStarter struct {
	executable       string
	workingDirectory string
	processID        int
	err              error
}

func (s *fakeGameProcessStarter) Start(executable, workingDirectory string) (int, error) {
	s.executable = executable
	s.workingDirectory = workingDirectory
	return s.processID, s.err
}

func TestWindowsGameLauncherStartsRecordedRegularExecutable(t *testing.T) {
	t.Parallel()
	installPath, executable := writeLauncherTestInstallation(t)
	starter := &fakeGameProcessStarter{processID: 4242}
	startedAt := time.Date(2026, 7, 14, 18, 0, 0, 0, time.UTC)
	launcher := &WindowsGameLauncher{
		now:         func() time.Time { return startedAt },
		start:       starter,
		resolvePath: filepath.EvalSymlinks,
	}

	result, err := launcher.Launch(context.Background(), devicev1.GameLaunchRequest{
		GameID: "game-1", SourceGameID: "source-1", InstallPath: installPath, LaunchTarget: "Game/game.exe",
	})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	resolvedExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	if starter.executable != resolvedExecutable || starter.workingDirectory != filepath.Dir(resolvedExecutable) {
		t.Fatalf("starter paths = %q / %q", starter.executable, starter.workingDirectory)
	}
	if result.ProcessID != 4242 || !result.StartedAt.Equal(startedAt) {
		t.Fatalf("result = %#v", result)
	}
}

func TestWindowsGameLauncherStartsRecordedExecutableFromGogManifest(t *testing.T) {
	t.Parallel()
	installPath, executable := writeLauncherTestInstallationWithSchemaVersion(t, devicev1.ExecutableInstallManifestSchemaVersion)
	starter := &fakeGameProcessStarter{processID: 4343}
	launcher := &WindowsGameLauncher{now: time.Now, start: starter, resolvePath: filepath.EvalSymlinks}

	result, err := launcher.Launch(context.Background(), devicev1.GameLaunchRequest{
		GameID: "game-1", SourceGameID: "source-1", InstallPath: installPath, LaunchTarget: "Game/game.exe",
	})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	resolvedExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	if starter.executable != resolvedExecutable || result.ProcessID != 4343 {
		t.Fatalf("starter executable = %q, result = %#v", starter.executable, result)
	}
}

func TestWindowsGameLauncherRejectsTargetOutsideManifestCandidates(t *testing.T) {
	t.Parallel()
	installPath, _ := writeLauncherTestInstallation(t)
	starter := &fakeGameProcessStarter{processID: 4242}
	launcher := &WindowsGameLauncher{now: time.Now, start: starter, resolvePath: filepath.EvalSymlinks}

	_, err := launcher.Launch(context.Background(), devicev1.GameLaunchRequest{
		GameID: "game-1", SourceGameID: "source-1", InstallPath: installPath, LaunchTarget: "other.exe",
	})
	if err == nil || starter.executable != "" {
		t.Fatalf("Launch() error = %v, starter executable = %q", err, starter.executable)
	}
}

func TestWindowsGameLauncherRejectsResolvedTargetOutsideInstallDirectory(t *testing.T) {
	t.Parallel()
	installPath, executable := writeLauncherTestInstallation(t)
	outside := filepath.Join(t.TempDir(), "outside.exe")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	starter := &fakeGameProcessStarter{processID: 4242}
	launcher := &WindowsGameLauncher{
		now:   time.Now,
		start: starter,
		resolvePath: func(value string) (string, error) {
			switch filepath.Clean(value) {
			case filepath.Clean(installPath):
				return installPath, nil
			case filepath.Clean(executable):
				return outside, nil
			default:
				return "", errors.New("unexpected path")
			}
		},
	}

	_, err := launcher.Launch(context.Background(), devicev1.GameLaunchRequest{
		GameID: "game-1", SourceGameID: "source-1", InstallPath: installPath, LaunchTarget: "Game/game.exe",
	})
	if err == nil || starter.executable != "" {
		t.Fatalf("Launch() error = %v, starter executable = %q", err, starter.executable)
	}
}

func TestWindowsGameLauncherHonorsLaunchOnlyGrantAcrossServerGameIDs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Games", "MGA", "owner")
	installPath := filepath.Join(root, "Game")
	if err := os.MkdirAll(installPath, 0o755); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(installPath, "game.exe")
	if err := os.WriteFile(executable, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := writeInstallManifest(installPath, installManifest{SchemaVersion: devicev1.InstallManifestSchemaVersion, LocalInstallationID: testInstallID, OwnerBindingID: testBindingOne, OwnershipState: string(OwnershipOwned), GameID: "owner-game", SourceGameID: "owner-source", InstallRoot: root, LaunchTarget: "game.exe", LaunchCandidates: []string{"game.exe"}, InstalledAt: now}); err != nil {
		t.Fatal(err)
	}
	catalog, err := OpenOwnershipCatalog(filepath.Join(t.TempDir(), "ownership.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.BeginInstall(InstallationOwnershipRecord{LocalInstallationID: testInstallID, OwnerBindingID: testBindingOne, State: OwnershipInstalling, InstallKind: devicev1.InstallKindManagedArchive, InstallRoot: root, InstallPath: installPath, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.CompleteInstall(testInstallID, testBindingOne); err != nil {
		t.Fatal(err)
	}
	if err := catalog.GrantUse(testInstallID, testBindingTwo, now); err != nil {
		t.Fatal(err)
	}
	ownership, err := NewInstallationOwnership(testBindingTwo, "http://tv2:8900", 2, catalog, NewInstallationCoordinator())
	if err != nil {
		t.Fatal(err)
	}
	starter := &fakeGameProcessStarter{processID: 4545}
	launcher := &WindowsGameLauncher{now: func() time.Time { return now }, start: starter, resolvePath: filepath.EvalSymlinks, ownership: ownership}
	result, err := launcher.Launch(context.Background(), devicev1.GameLaunchRequest{GameID: "target-game", SourceGameID: "target-source", InstallPath: installPath, LaunchTarget: "game.exe", LocalInstallationID: testInstallID})
	if err != nil {
		t.Fatal(err)
	}
	if result.GameID != "target-game" || starter.processID != result.ProcessID {
		t.Fatalf("shared launch = %+v", result)
	}
}

func writeLauncherTestInstallation(t *testing.T) (string, string) {
	t.Helper()
	return writeLauncherTestInstallationWithSchemaVersion(t, devicev1.InstallManifestSchemaVersion)
}

func writeLauncherTestInstallationWithSchemaVersion(t *testing.T, schemaVersion int) (string, string) {
	t.Helper()
	installPath := t.TempDir()
	executable := filepath.Join(installPath, "Game", "game.exe")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("not executed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeInstallManifest(installPath, installManifest{
		SchemaVersion: schemaVersion,
		GameID:        "game-1",
		SourceGameID:  "source-1",
		InstallRoot:   filepath.Dir(installPath),
		LaunchTarget:  "Game/game.exe",
		LaunchCandidates: []string{
			"Game/game.exe",
		},
	}); err != nil {
		t.Fatal(err)
	}
	return installPath, executable
}
