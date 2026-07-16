package v1

import (
	"testing"
	"time"
)

func TestGogInnoInstallRequestValidate(t *testing.T) {
	t.Parallel()
	request := GogInnoInstallRequest{
		GameID: "game-1", SourceGameID: "source-1", Title: "Duke Nukem 3D", DestinationName: "Duke Nukem 3D",
		Installer: PackageTransferDescriptor{
			FileName: "setup_duke_nukem_3d_1.5_(28044).exe", Role: PackageTransferRoleInstaller, SizeBytes: 39_000_000,
			DownloadURL: "/api/device-transfers/package", DownloadToken: "token-installer",
		},
		Companions: []PackageTransferDescriptor{{
			FileName: "setup_duke_nukem_3d_1.5_(28044)-1.bin", Role: PackageTransferRoleCompanion, SizeBytes: 100,
			DownloadURL: "/api/device-transfers/package", DownloadToken: "token-bin",
		}},
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	bad := request
	bad.Installer.FileName = "game.exe"
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate() accepted a non setup_*.exe installer")
	}

	bad = request
	bad.Companions[0].FileName = "setup_other_game-1.bin"
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate() accepted a mismatched companion stem")
	}

	bad = request
	bad.Companions = append(bad.Companions, bad.Companions[0])
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate() accepted duplicate companion names")
	}

	bad = request
	bad.Installer.DownloadURL = "file:///tmp/setup.exe"
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate() accepted a non-HTTP download URL")
	}
}

func TestGogInnoInstallResultValidate(t *testing.T) {
	t.Parallel()
	exitCode := 0
	result := GogInnoInstallResult{
		GameID: "game-1", SourceGameID: "source-1", InstallRoot: `C:\Games`, InstallPath: `C:\Games\Game`,
		InstallerFamily:   GogInnoInstallerFamily,
		PrimarySHA256:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		TotalPackageBytes: 42,
		PackageFiles: []GogInnoPackageFile{{
			FileName: "setup_game.exe", Role: PackageTransferRoleInstaller, SizeBytes: 42,
			SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
		SignerSubject: "GOG Sp. z o.o.", SignerThumbprint: "thumb", InvocationMode: GogInnoInvocationFixedSilent,
		UninstallTarget: "unins000.exe", LaunchTarget: "game.exe", LaunchCandidates: []string{"game.exe"},
		ProcessID: 12, ExitCode: &exitCode, InstalledAt: time.Now().UTC(), CompletionBasis: GogInnoCompletionExitZero,
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	result.UninstallTarget = "Game/game.exe"
	if err := result.Validate(); err == nil {
		t.Fatal("Validate() accepted a non-uninstaller target")
	}
}

func TestGogInnoInstallResultValidateFailureEvidence(t *testing.T) {
	t.Parallel()
	result := GogInnoInstallResult{
		GameID: "game-1", SourceGameID: "source-1", InstallRoot: `C:\Games`, InstallPath: `C:\Games\Failed Game`,
		InstallerFamily: GogInnoInstallerFamily, PrimarySHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		TotalPackageBytes: 42, PackageFiles: []GogInnoPackageFile{{
			FileName: "setup_game.exe", Role: PackageTransferRoleInstaller, SizeBytes: 42,
			SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
		SignerSubject: "GOG Sp. z o.o.", SignerThumbprint: "thumb", InvocationMode: GogInnoInvocationFixedSilent,
		CleanupMarkerID: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	if err := result.ValidateFailureEvidence(); err != nil {
		t.Fatalf("ValidateFailureEvidence() error = %v", err)
	}
	bad := result
	bad.InstallPath = bad.InstallRoot
	if err := bad.ValidateFailureEvidence(); err == nil {
		t.Fatal("ValidateFailureEvidence() accepted install root as destination")
	}
	bad = result
	bad.CleanupMarkerID = "bad"
	if err := bad.ValidateFailureEvidence(); err == nil {
		t.Fatal("ValidateFailureEvidence() accepted an invalid marker")
	}
}

func TestGogInnoFailedCleanupRequestValidate(t *testing.T) {
	t.Parallel()
	request := GogInnoFailedCleanupRequest{
		GameID: "game-1", SourceGameID: "source-1", InstallRoot: `C:\Games`, InstallPath: `C:\Games\Game`,
		InstallerFamily: GogInnoInstallerFamily,
		CleanupMarkerID: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		PrimarySHA256:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		UninstallTarget: "unins000.exe",
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	bad := request
	bad.CleanupMarkerID = "not-a-marker"
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate() accepted an invalid cleanup marker")
	}

	bad = request
	bad.UninstallTarget = `..\Windows\notepad.exe`
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate() accepted an unsafe uninstaller")
	}
}

func TestGogInnoFailedCleanupResultValidate(t *testing.T) {
	t.Parallel()
	result := GogInnoFailedCleanupResult{GameID: "game-1", SourceGameID: "source-1", Removed: true}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	result.Removed = false
	if err := result.Validate(); err == nil {
		t.Fatal("Validate() accepted a result that did not remove the failed install")
	}
}

func TestGogInnoUninstallRequestValidate(t *testing.T) {
	t.Parallel()
	request := GogInnoUninstallRequest{
		GameID: "game-1", SourceGameID: "source-1", InstallPath: `C:\Games\Game`,
		InstallerFamily: GogInnoInstallerFamily, UninstallTarget: "unins000.exe",
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	request.UninstallTarget = `..\Windows\notepad.exe`
	if err := request.Validate(); err == nil {
		t.Fatal("Validate() accepted an unsafe uninstall target")
	}
}

func TestGogInnoStemHelpers(t *testing.T) {
	t.Parallel()
	if got := GogInnoSetupStem("setup_duke_nukem_3d_1.5_(28044).exe"); got != "setup_duke_nukem_3d_1.5_(28044)" {
		t.Fatalf("GogInnoSetupStem() = %q", got)
	}
	if got := GogInnoCompanionStem("setup_duke_nukem_3d_1.5_(28044)-1.bin"); got != "setup_duke_nukem_3d_1.5_(28044)" {
		t.Fatalf("GogInnoCompanionStem() = %q", got)
	}
}
