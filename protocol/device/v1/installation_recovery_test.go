package v1

import (
	"path/filepath"
	"testing"
)

func TestInstallationRecoveryRequestValidate(t *testing.T) {
	base := InstallationRecoveryRequest{
		GameID: "game", SourceGameID: "source", InstallKind: InstallKindManagedArchive,
		InstallRoot: t.TempDir(),
	}
	base.InstallPath = filepath.Join(base.InstallRoot, "MGA", "Game")
	tests := []struct {
		name    string
		mutate  func(*InstallationRecoveryRequest)
		wantErr bool
	}{
		{"repair", func(r *InstallationRecoveryRequest) {
			r.Action, r.InstallState, r.ReasonCode = InstallationRecoveryRepair, InstallStateNeedsRepair, ValidationReasonLaunchTargetMissing
		}, false},
		{"reinstall", func(r *InstallationRecoveryRequest) {
			r.Action, r.InstallState, r.ReasonCode = InstallationRecoveryReinstall, InstallStateMissing, ValidationReasonInstallPathMissing
		}, false},
		{"forget missing", func(r *InstallationRecoveryRequest) {
			r.Action, r.InstallState, r.ReasonCode = InstallationRecoveryForget, InstallStateMissing, ValidationReasonInstallPathMissing
		}, false},
		{"forget unsafe", func(r *InstallationRecoveryRequest) {
			r.Action, r.InstallState, r.ReasonCode = InstallationRecoveryForget, InstallStateNeedsRepair, ValidationReasonManifestInvalid
		}, true},
		{"repair wrong reason", func(r *InstallationRecoveryRequest) {
			r.Action, r.InstallState, r.ReasonCode = InstallationRecoveryRepair, InstallStateNeedsRepair, ValidationReasonManifestMissing
		}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.mutate(&request)
			if err := request.Validate(); (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestInstallationRecoveryResultValidate(t *testing.T) {
	result := InstallationRecoveryResult{
		Action: InstallationRecoveryRepair, GameID: "game", SourceGameID: "source",
		LocalInstallationID: "local", LaunchTarget: "Game.exe", LaunchCandidates: []string{"Game.exe"}, PathPresent: true,
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	result.LaunchCandidates = []string{"Other.exe"}
	if err := result.Validate(); err == nil {
		t.Fatal("Validate() accepted a target outside candidates")
	}
}
