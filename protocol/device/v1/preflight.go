package v1

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const InstallationPreflightSchemaVersion uint16 = 1

type InstallationCategory string

const (
	InstallationCategoryManagedArchive  InstallationCategory = "managed_archive"
	InstallationCategoryNativeInstaller InstallationCategory = "native_installer"
	InstallationCategoryStorefront      InstallationCategory = "storefront"
	InstallationCategoryEmulated        InstallationCategory = "emulated"
)

func (c InstallationCategory) Validate() error {
	switch c {
	case InstallationCategoryManagedArchive, InstallationCategoryNativeInstaller, InstallationCategoryStorefront, InstallationCategoryEmulated:
		return nil
	default:
		return fmt.Errorf("unsupported installation category %q", c)
	}
}

type PrerequisiteKind string

const (
	PrerequisiteKindStorefront PrerequisiteKind = "storefront"
	PrerequisiteKindEmulator   PrerequisiteKind = "emulator"
	PrerequisiteKindRuntime    PrerequisiteKind = "runtime"
)

func (k PrerequisiteKind) Validate() error {
	switch k {
	case PrerequisiteKindStorefront, PrerequisiteKindEmulator, PrerequisiteKindRuntime:
		return nil
	default:
		return fmt.Errorf("unsupported prerequisite kind %q", k)
	}
}

var prerequisiteIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)+$`)

type PrerequisiteRequirement struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Kind     PrerequisiteKind `json:"kind"`
	Required bool             `json:"required"`
}

func (r PrerequisiteRequirement) Validate() error {
	if !prerequisiteIDPattern.MatchString(strings.TrimSpace(r.ID)) {
		return fmt.Errorf("invalid prerequisite id %q", r.ID)
	}
	if name := strings.TrimSpace(r.Name); name == "" || len(name) > 100 || strings.ContainsAny(name, "\r\n\x00") {
		return errors.New("prerequisite name must be between 1 and 100 characters")
	}
	return r.Kind.Validate()
}

type InstallationPreflightRequest struct {
	SchemaVersion        uint16                    `json:"schema_version"`
	GameID               string                    `json:"game_id"`
	SourceGameID         string                    `json:"source_game_id"`
	Category             InstallationCategory      `json:"category"`
	DestinationRoot      string                    `json:"destination_root"`
	RequiredStorageBytes uint64                    `json:"required_storage_bytes,omitempty"`
	Requirements         []PrerequisiteRequirement `json:"requirements,omitempty"`
}

func (r InstallationPreflightRequest) Validate() error {
	if r.SchemaVersion != InstallationPreflightSchemaVersion {
		return fmt.Errorf("unsupported installation preflight schema version %d", r.SchemaVersion)
	}
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if err := r.Category.Validate(); err != nil {
		return err
	}
	root := strings.TrimSpace(r.DestinationRoot)
	if root == "" || len(root) > 1024 || strings.ContainsAny(root, "\r\n\x00") {
		return errors.New("destination_root must be between 1 and 1024 characters")
	}
	seen := map[string]bool{}
	for _, requirement := range r.Requirements {
		if err := requirement.Validate(); err != nil {
			return err
		}
		if seen[requirement.ID] {
			return fmt.Errorf("duplicate prerequisite id %q", requirement.ID)
		}
		seen[requirement.ID] = true
	}
	if (r.Category == InstallationCategoryStorefront || r.Category == InstallationCategoryEmulated) && len(r.Requirements) == 0 {
		return errors.New("storefront and emulated preflight require at least one prerequisite")
	}
	return nil
}

type PreflightCheckStatus string

const (
	PreflightCheckReady            PreflightCheckStatus = "ready"
	PreflightCheckMissing          PreflightCheckStatus = "missing"
	PreflightCheckUnknown          PreflightCheckStatus = "unknown"
	PreflightCheckInstallerManaged PreflightCheckStatus = "installer_managed"
	PreflightCheckNotApplicable    PreflightCheckStatus = "not_applicable"
)

func (s PreflightCheckStatus) Validate() error {
	switch s {
	case PreflightCheckReady, PreflightCheckMissing, PreflightCheckUnknown, PreflightCheckInstallerManaged, PreflightCheckNotApplicable:
		return nil
	default:
		return fmt.Errorf("unsupported preflight check status %q", s)
	}
}

type InstallationPreflightCheck struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Kind           string               `json:"kind"`
	Status         PreflightCheckStatus `json:"status"`
	Required       bool                 `json:"required"`
	Message        string               `json:"message"`
	RequiredBytes  uint64               `json:"required_bytes,omitempty"`
	AvailableBytes uint64               `json:"available_bytes,omitempty"`
}

func (c InstallationPreflightCheck) Validate() error {
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Kind) == "" || strings.TrimSpace(c.Message) == "" {
		return errors.New("preflight check id, name, kind, and message are required")
	}
	return c.Status.Validate()
}

type InstallationPreflightResult struct {
	SchemaVersion uint16                       `json:"schema_version"`
	CanInstall    bool                         `json:"can_install"`
	Checks        []InstallationPreflightCheck `json:"checks"`
}

func (r InstallationPreflightResult) Validate() error {
	if r.SchemaVersion != InstallationPreflightSchemaVersion {
		return fmt.Errorf("unsupported installation preflight result schema version %d", r.SchemaVersion)
	}
	if len(r.Checks) == 0 {
		return errors.New("at least one preflight check is required")
	}
	blocked := false
	seen := map[string]bool{}
	for _, check := range r.Checks {
		if err := check.Validate(); err != nil {
			return err
		}
		if seen[check.ID] {
			return fmt.Errorf("duplicate preflight check id %q", check.ID)
		}
		seen[check.ID] = true
		if check.Required && check.Status == PreflightCheckMissing {
			blocked = true
		}
	}
	if r.CanInstall == blocked {
		return errors.New("can_install does not match required missing checks")
	}
	return nil
}
