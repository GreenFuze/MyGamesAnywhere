package v1

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const InstallationRecoverySchemaVersion uint16 = 1

const (
	InstallationRecoveryRepair    = "repair"
	InstallationRecoveryReinstall = "reinstall"
	InstallationRecoveryForget    = "forget"
)

// InstallationRecoveryRequest identifies one already-persisted managed
// installation. The server builds every path field from its installation row;
// the browser never supplies a filesystem path.
type InstallationRecoveryRequest struct {
	Action              string `json:"action"`
	GameID              string `json:"game_id"`
	SourceGameID        string `json:"source_game_id"`
	LocalInstallationID string `json:"local_installation_id,omitempty"`
	InstallKind         string `json:"install_kind"`
	InstallRoot         string `json:"install_root"`
	InstallPath         string `json:"install_path"`
	InstallState        string `json:"install_state"`
	ReasonCode          string `json:"reason_code"`
}

func (r InstallationRecoveryRequest) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if !filepath.IsAbs(strings.TrimSpace(r.InstallRoot)) || !filepath.IsAbs(strings.TrimSpace(r.InstallPath)) {
		return errors.New("install_root and install_path must be absolute")
	}
	inside, err := filepath.Rel(filepath.Clean(r.InstallRoot), filepath.Clean(r.InstallPath))
	if err != nil || inside == "." || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return errors.New("install_path must be a non-root child of install_root")
	}
	switch r.InstallKind {
	case InstallKindManagedArchive, InstallKindGogInno:
	default:
		return fmt.Errorf("unsupported install_kind %q", r.InstallKind)
	}
	switch r.Action {
	case InstallationRecoveryRepair:
		if r.InstallState != InstallStateNeedsRepair || r.ReasonCode != ValidationReasonLaunchTargetMissing {
			return errors.New("repair requires needs_repair with launch_target_missing")
		}
	case InstallationRecoveryReinstall:
		if r.InstallState != InstallStateMissing || r.ReasonCode != ValidationReasonInstallPathMissing {
			return errors.New("reinstall requires missing with install_path_missing")
		}
	case InstallationRecoveryForget:
		if !forgettableInstallationState(r.InstallState, r.ReasonCode) {
			return errors.New("forget requires a safely releasable missing or repair state")
		}
	default:
		return fmt.Errorf("unsupported recovery action %q", r.Action)
	}
	return nil
}

func forgettableInstallationState(state, reason string) bool {
	if state == InstallStateMissing {
		return reason == ValidationReasonInstallPathMissing
	}
	if state != InstallStateNeedsRepair {
		return false
	}
	switch reason {
	case ValidationReasonLaunchTargetMissing,
		ValidationReasonUninstallTargetMissing,
		ValidationReasonRegisteredProgramMissing,
		ValidationReasonFilesMissingRegistrationPresent:
		return true
	default:
		return false
	}
}

type InstallationRecoveryResult struct {
	Action              string   `json:"action"`
	GameID              string   `json:"game_id"`
	SourceGameID        string   `json:"source_game_id"`
	LocalInstallationID string   `json:"local_installation_id"`
	LaunchTarget        string   `json:"launch_target,omitempty"`
	LaunchCandidates    []string `json:"launch_candidates,omitempty"`
	Released            bool     `json:"released,omitempty"`
	PathPresent         bool     `json:"path_present"`
}

type InstallationCleanupRequest struct {
	GameID          string `json:"game_id"`
	SourceGameID    string `json:"source_game_id"`
	InstallKind     string `json:"install_kind"`
	InstallPath     string `json:"install_path"`
	UninstallTarget string `json:"uninstall_target,omitempty"`
}

func (r InstallationCleanupRequest) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if !filepath.IsAbs(strings.TrimSpace(r.InstallPath)) {
		return errors.New("install_path must be absolute")
	}
	switch r.InstallKind {
	case InstallKindManagedArchive:
		if r.UninstallTarget != "" {
			return errors.New("managed archive cleanup must not include uninstall_target")
		}
	case InstallKindGogInno:
		if err := ValidateUninstallTarget(r.UninstallTarget); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported install_kind %q", r.InstallKind)
	}
	return nil
}

type InstallationCleanupResult struct {
	GameID       string `json:"game_id"`
	SourceGameID string `json:"source_game_id"`
	InstallKind  string `json:"install_kind"`
	Removed      bool   `json:"removed"`
}

func (r InstallationCleanupResult) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	switch r.InstallKind {
	case InstallKindManagedArchive, InstallKindGogInno:
	default:
		return fmt.Errorf("unsupported install_kind %q", r.InstallKind)
	}
	if !r.Removed {
		return errors.New("cleanup result must report removal")
	}
	return nil
}

func (r InstallationRecoveryResult) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" || strings.TrimSpace(r.LocalInstallationID) == "" {
		return errors.New("game_id, source_game_id, and local_installation_id are required")
	}
	switch r.Action {
	case InstallationRecoveryRepair:
		if r.Released || !r.PathPresent || strings.TrimSpace(r.LaunchTarget) == "" {
			return errors.New("repair result requires a present path and launch target")
		}
		if err := ValidateLaunchTarget(r.LaunchTarget); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, candidate := range r.LaunchCandidates {
			if err := ValidateLaunchTarget(candidate); err != nil {
				return err
			}
			seen[NormalizeLaunchTarget(candidate)] = true
		}
		if !seen[NormalizeLaunchTarget(r.LaunchTarget)] {
			return errors.New("repair launch_target must be included in launch_candidates")
		}
	case InstallationRecoveryReinstall:
		if !r.Released || r.PathPresent || r.LaunchTarget != "" || len(r.LaunchCandidates) != 0 {
			return errors.New("reinstall result must release a missing installation")
		}
	case InstallationRecoveryForget:
		if !r.Released || r.LaunchTarget != "" || len(r.LaunchCandidates) != 0 {
			return errors.New("forget result must release installation ownership")
		}
	default:
		return fmt.Errorf("unsupported recovery action %q", r.Action)
	}
	return nil
}
