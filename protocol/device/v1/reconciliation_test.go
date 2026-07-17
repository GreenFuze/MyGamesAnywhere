package v1

import (
	"strings"
	"testing"
	"time"
)

func TestInstallationValidationRequestRejectsDuplicateAndUnsafeItems(t *testing.T) {
	item := InstallationValidationRequestItem{
		GameID: "game", SourceGameID: "source", InstallKind: InstallKindManagedArchive,
		InstallRoot: `C:\Games`, InstallPath: `C:\Games\Game`, LaunchTarget: `Game.exe`,
	}
	if err := (InstallationValidationRequest{Trigger: "manual", Items: []InstallationValidationRequestItem{item}}).Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if err := (InstallationValidationRequest{Trigger: "manual", Items: []InstallationValidationRequestItem{item, item}}).Validate(); err == nil {
		t.Fatal("duplicate request accepted")
	}
	item.InstallPath = `..\Game`
	if err := item.Validate(); err == nil {
		t.Fatal("relative install path accepted")
	}
	item.InstallPath = `C:\Games\Game`
	item.InstallKind = InstallKindGogInno
	if err := item.Validate(); err == nil {
		t.Fatal("GOG validation without uninstaller accepted")
	}
	items := make([]InstallationValidationRequestItem, MaxInstallationValidationItems+1)
	for index := range items {
		items[index] = InstallationValidationRequestItem{GameID: strings.Repeat("g", index+1), SourceGameID: "s", InstallKind: InstallKindManagedArchive, InstallRoot: `C:\Games`, InstallPath: `C:\Games\Game`}
	}
	if err := (InstallationValidationRequest{Trigger: "background", Items: items}).Validate(); err == nil {
		t.Fatal("oversized validation batch accepted")
	}
}

func TestInstallationValidationResultRequiresExactCountsAndReasons(t *testing.T) {
	now := time.Now().UTC()
	result := InstallationValidationResult{
		Items: []InstallationValidationResultItem{
			{GameID: "a", SourceGameID: "sa", State: InstallStateInstalled, ReasonCode: ValidationReasonHealthy, CheckedAt: now},
			{GameID: "b", SourceGameID: "sb", State: InstallStateMissing, ReasonCode: ValidationReasonInstallPathMissing, CheckedAt: now},
			{GameID: "c", SourceGameID: "sc", State: InstallStateNeedsRepair, ReasonCode: ValidationReasonManifestMissing, CheckedAt: now},
		}, Installed: 1, Missing: 1, NeedsRepair: 1,
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}
	result.Missing = 0
	if err := result.Validate(); err == nil {
		t.Fatal("incorrect summary accepted")
	}
	result.Missing = 1
	result.Items[0].ReasonCode = ValidationReasonManifestMissing
	if err := result.Validate(); err == nil {
		t.Fatal("invalid installed reason accepted")
	}
}
