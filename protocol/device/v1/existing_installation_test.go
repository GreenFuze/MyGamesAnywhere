package v1

import (
	"testing"
	"time"
)

func TestUseExistingInstallationResultValidation(t *testing.T) {
	result := UseExistingInstallationResult{
		LocalInstallationID: "local-1", GameID: "game-1", SourceGameID: "source-1",
		InstallRoot: `C:\Games\MGA\one`, InstallPath: `C:\Games\MGA\one\Game`,
		LaunchTarget: "game.exe", LaunchCandidates: []string{"game.exe"}, GrantedAt: time.Now(),
		NativeProducts: []NativeProductObservation{{Provider: "windows_uninstall", ProductID: "windows-uninstall:abc", DisplayName: "Game", Capabilities: []string{"uninstall"}}},
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	result.LaunchTarget = "other.exe"
	if err := result.Validate(); err == nil {
		t.Fatal("unrecorded launch target was accepted")
	}
}
