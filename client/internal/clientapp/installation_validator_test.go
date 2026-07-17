package clientapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type fakeRegisteredPrograms struct {
	associated bool
	err        error
}

func (f fakeRegisteredPrograms) HasAssociation(string) (bool, error) { return f.associated, f.err }

func TestInstallationValidatorManagedArchiveHealthyMissingRepairAndRestored(t *testing.T) {
	root := t.TempDir()
	installPath := filepath.Join(root, "Game")
	if err := os.Mkdir(installPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installPath, "Game.exe"), []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := installManifest{SchemaVersion: devicev1.InstallManifestSchemaVersion, GameID: "game", SourceGameID: "source", InstallRoot: root, LaunchTarget: "Game.exe", LaunchCandidates: []string{"Game.exe"}, InstalledAt: time.Now()}
	if err := writeInstallManifest(installPath, manifest); err != nil {
		t.Fatal(err)
	}
	validator, _ := NewLocalInstallationValidator(fakeRegisteredPrograms{})
	request := validationRequest(root, installPath, devicev1.InstallKindManagedArchive, "Game.exe", "")
	result, err := validator.Validate(context.Background(), request, nil)
	if err != nil || result.Installed != 1 || result.Items[0].ReasonCode != devicev1.ValidationReasonHealthy {
		t.Fatalf("healthy result = %#v, error = %v", result, err)
	}
	if err := os.Remove(filepath.Join(installPath, "Game.exe")); err != nil {
		t.Fatal(err)
	}
	result, err = validator.Validate(context.Background(), request, nil)
	if err != nil || result.NeedsRepair != 1 || result.Items[0].ReasonCode != devicev1.ValidationReasonLaunchTargetMissing {
		t.Fatalf("damaged result = %#v, error = %v", result, err)
	}
	if err := os.RemoveAll(installPath); err != nil {
		t.Fatal(err)
	}
	result, err = validator.Validate(context.Background(), request, nil)
	if err != nil || result.Missing != 1 {
		t.Fatalf("missing result = %#v, error = %v", result, err)
	}
}

func TestInstallationValidatorGogRequiresFilesManifestAndRegistration(t *testing.T) {
	root := t.TempDir()
	installPath := filepath.Join(root, "GOG")
	if err := os.Mkdir(installPath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Game.exe", "unins000.exe"} {
		if err := os.WriteFile(filepath.Join(installPath, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := gogInnoManifest{SchemaVersion: devicev1.ExecutableInstallManifestSchemaVersion, GameID: "game", SourceGameID: "source", InstallRoot: root, InstallPath: installPath, InstallerFamily: devicev1.GogInnoInstallerFamily, UninstallTarget: "unins000.exe", LaunchTarget: "Game.exe", InstalledAt: time.Now()}
	if err := writeGogInnoManifest(installPath, manifest); err != nil {
		t.Fatal(err)
	}
	request := validationRequest(root, installPath, devicev1.InstallKindGogInno, "Game.exe", "unins000.exe")
	validator, _ := NewLocalInstallationValidator(fakeRegisteredPrograms{associated: true})
	result, err := validator.Validate(context.Background(), request, nil)
	if err != nil || result.Installed != 1 {
		t.Fatalf("healthy GOG result = %#v, error = %v", result, err)
	}
	validator, _ = NewLocalInstallationValidator(fakeRegisteredPrograms{associated: false})
	result, err = validator.Validate(context.Background(), request, nil)
	if err != nil || result.NeedsRepair != 1 || result.Items[0].ReasonCode != devicev1.ValidationReasonRegisteredProgramMissing {
		t.Fatalf("unregistered GOG result = %#v, error = %v", result, err)
	}
	if err := os.RemoveAll(installPath); err != nil {
		t.Fatal(err)
	}
	validator, _ = NewLocalInstallationValidator(fakeRegisteredPrograms{associated: true})
	result, err = validator.Validate(context.Background(), request, nil)
	if err != nil || result.Items[0].ReasonCode != devicev1.ValidationReasonFilesMissingRegistrationPresent {
		t.Fatalf("registered missing files result = %#v, error = %v", result, err)
	}
}

func validationRequest(root, installPath, kind, launch, uninstall string) devicev1.InstallationValidationRequest {
	return devicev1.InstallationValidationRequest{Trigger: "manual", Items: []devicev1.InstallationValidationRequestItem{{
		GameID: "game", SourceGameID: "source", InstallKind: kind, InstallRoot: root,
		InstallPath: installPath, LaunchTarget: launch, UninstallTarget: uninstall,
	}}}
}
