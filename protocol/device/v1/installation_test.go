package v1

import (
	"testing"
	"time"
)

func TestArchiveInstallRequestValidate(t *testing.T) {
	t.Parallel()
	request := ArchiveInstallRequest{
		GameID: "game-1", SourceGameID: "source-1", Title: "Game", ArchiveName: "game.zip",
		ArchiveFormat: "zip", ArchiveSize: 42, DownloadURL: "http://mga.test/api/device-transfers/token",
		DownloadToken:   "secret-token",
		DestinationRoot: DefaultInstallRootTemplate, DestinationName: "Game",
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	request.DownloadURL = "/api/device-transfers/archive"
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() rejected origin-relative transfer path: %v", err)
	}
	for _, format := range []string{"zip", ".7Z", "RAR"} {
		request.ArchiveFormat = format
		if err := request.Validate(); err != nil {
			t.Fatalf("Validate() rejected archive format %q: %v", format, err)
		}
	}
	request.ArchiveFormat = "exe"
	if err := request.Validate(); err == nil {
		t.Fatal("Validate() accepted an executable installer as a managed archive")
	}
	request.ArchiveFormat = "zip"
	request.DestinationName = `..\outside`
	if err := request.Validate(); err == nil {
		t.Fatal("Validate() accepted a path-like destination name")
	}
}

func TestArchiveInstallResultValidate(t *testing.T) {
	t.Parallel()
	result := ArchiveInstallResult{
		GameID: "game-1", SourceGameID: "source-1", InstallRoot: `C:\Games`, InstallPath: `C:\Games\Game`,
		ArchiveSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ArchiveBytes:  42, InstalledAt: time.Now(), LaunchTarget: "Game/game.exe",
		LaunchCandidates: []string{"Game/game.exe", "Game/alternate.exe"},
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	result.LaunchTarget = "Game/not-recorded.exe"
	if err := result.Validate(); err == nil {
		t.Fatal("Validate() accepted a launch target outside launch_candidates")
	}
}

func TestGameLaunchRequestRejectsUnsafeExecutablePaths(t *testing.T) {
	t.Parallel()
	request := GameLaunchRequest{
		GameID: "game-1", SourceGameID: "source-1", InstallPath: `C:\Games\Game`, LaunchTarget: "Game/game.exe",
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	for _, target := range []string{`..\outside.exe`, `C:\Windows\notepad.exe`, "Game/readme.txt"} {
		request.LaunchTarget = target
		if err := request.Validate(); err == nil {
			t.Fatalf("Validate() accepted unsafe launch target %q", target)
		}
	}
}
