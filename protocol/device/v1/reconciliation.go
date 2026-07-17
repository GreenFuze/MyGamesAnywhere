package v1

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	InstallationValidationSchemaVersion uint16 = 1
	MaxInstallationValidationItems             = 256

	InstallStateMissing     = "missing"
	InstallStateNeedsRepair = "needs_repair"

	ValidationReasonHealthy                         = "healthy"
	ValidationReasonInstallPathMissing              = "install_path_missing"
	ValidationReasonManifestMissing                 = "manifest_missing"
	ValidationReasonManifestInvalid                 = "manifest_invalid"
	ValidationReasonManifestIdentityMismatch        = "manifest_identity_mismatch"
	ValidationReasonManifestSchemaUnsupported       = "manifest_schema_unsupported"
	ValidationReasonLaunchTargetMissing             = "launch_target_missing"
	ValidationReasonUninstallTargetMissing          = "uninstall_target_missing"
	ValidationReasonRegisteredProgramMissing        = "registered_program_missing"
	ValidationReasonFilesMissingRegistrationPresent = "files_missing_registration_present"
	ValidationReasonUnsafeReparsePoint              = "unsafe_reparse_point"
)

type InstallationValidationRequest struct {
	Trigger string                              `json:"trigger"`
	Items   []InstallationValidationRequestItem `json:"items"`
}

type InstallationValidationRequestItem struct {
	GameID          string `json:"game_id"`
	SourceGameID    string `json:"source_game_id"`
	InstallKind     string `json:"install_kind"`
	InstallRoot     string `json:"install_root"`
	InstallPath     string `json:"install_path"`
	LaunchTarget    string `json:"launch_target,omitempty"`
	UninstallTarget string `json:"uninstall_target,omitempty"`
}

func (r InstallationValidationRequest) Validate() error {
	if r.Trigger != "manual" && r.Trigger != "background" {
		return errors.New("trigger must be manual or background")
	}
	if len(r.Items) == 0 || len(r.Items) > MaxInstallationValidationItems {
		return fmt.Errorf("items must contain 1 to %d installations", MaxInstallationValidationItems)
	}
	seen := make(map[string]struct{}, len(r.Items))
	for index, item := range r.Items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("items[%d]: %w", index, err)
		}
		key := item.GameID + "\x00" + item.SourceGameID
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate installation identity at items[%d]", index)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (i InstallationValidationRequestItem) Validate() error {
	if strings.TrimSpace(i.GameID) == "" || strings.TrimSpace(i.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if !filepath.IsAbs(strings.TrimSpace(i.InstallRoot)) || !filepath.IsAbs(strings.TrimSpace(i.InstallPath)) {
		return errors.New("install_root and install_path must be absolute")
	}
	switch i.InstallKind {
	case InstallKindManagedArchive:
		if i.UninstallTarget != "" {
			return errors.New("managed archive must not include uninstall_target")
		}
	case InstallKindGogInno:
		if err := ValidateUninstallTarget(i.UninstallTarget); err != nil {
			return fmt.Errorf("uninstall_target: %w", err)
		}
	default:
		return fmt.Errorf("unsupported install_kind %q", i.InstallKind)
	}
	if i.LaunchTarget != "" {
		if err := ValidateLaunchTarget(i.LaunchTarget); err != nil {
			return fmt.Errorf("launch_target: %w", err)
		}
	}
	return nil
}

type InstallationValidationResult struct {
	Items              []InstallationValidationResultItem `json:"items"`
	Installed          int                                `json:"installed"`
	Missing            int                                `json:"missing"`
	NeedsRepair        int                                `json:"needs_repair"`
	ChangedMissing     int                                `json:"changed_missing,omitempty"`
	ChangedNeedsRepair int                                `json:"changed_needs_repair,omitempty"`
	Restored           int                                `json:"restored,omitempty"`
}

type InstallationValidationResultItem struct {
	GameID            string    `json:"game_id"`
	SourceGameID      string    `json:"source_game_id"`
	State             string    `json:"state"`
	ReasonCode        string    `json:"reason_code"`
	CheckedAt         time.Time `json:"checked_at"`
	ManifestSchema    int       `json:"manifest_schema,omitempty"`
	RegisteredProgram *bool     `json:"registered_program,omitempty"`
}

func (r InstallationValidationResult) Validate() error {
	if len(r.Items) == 0 || len(r.Items) > MaxInstallationValidationItems {
		return fmt.Errorf("items must contain 1 to %d validation results", MaxInstallationValidationItems)
	}
	seen := make(map[string]struct{}, len(r.Items))
	installed, missing, repair := 0, 0, 0
	for index, item := range r.Items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("items[%d]: %w", index, err)
		}
		key := item.GameID + "\x00" + item.SourceGameID
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate validation identity at items[%d]", index)
		}
		seen[key] = struct{}{}
		switch item.State {
		case InstallStateInstalled:
			installed++
		case InstallStateMissing:
			missing++
		case InstallStateNeedsRepair:
			repair++
		}
	}
	if r.Installed != installed || r.Missing != missing || r.NeedsRepair != repair {
		return errors.New("validation summary counts do not match items")
	}
	if r.ChangedMissing < 0 || r.ChangedNeedsRepair < 0 || r.Restored < 0 {
		return errors.New("server transition counts cannot be negative")
	}
	return nil
}

func (i InstallationValidationResultItem) Validate() error {
	if strings.TrimSpace(i.GameID) == "" || strings.TrimSpace(i.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if i.CheckedAt.IsZero() {
		return errors.New("checked_at is required")
	}
	if !validValidationStateReason(i.State, i.ReasonCode) {
		return fmt.Errorf("reason_code %q is invalid for state %q", i.ReasonCode, i.State)
	}
	if i.ManifestSchema < 0 {
		return errors.New("manifest_schema cannot be negative")
	}
	return nil
}

func validValidationStateReason(state, reason string) bool {
	switch state {
	case InstallStateInstalled:
		return reason == ValidationReasonHealthy
	case InstallStateMissing:
		return reason == ValidationReasonInstallPathMissing
	case InstallStateNeedsRepair:
		switch reason {
		case ValidationReasonManifestMissing, ValidationReasonManifestInvalid,
			ValidationReasonManifestIdentityMismatch, ValidationReasonManifestSchemaUnsupported,
			ValidationReasonLaunchTargetMissing, ValidationReasonUninstallTargetMissing,
			ValidationReasonRegisteredProgramMissing, ValidationReasonFilesMissingRegistrationPresent,
			ValidationReasonUnsafeReparsePoint:
			return true
		}
	}
	return false
}
