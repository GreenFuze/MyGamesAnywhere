package v1

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	GogInnoInstallSchemaVersion                uint16 = 1
	GogInnoUninstallSchemaVersion              uint16 = 1
	GogInnoCleanupSchemaVersion                uint16 = 1
	ExecutableInstallManifestSchemaVersion            = 3
	GogInnoInstallerFamily                            = "gog_inno"
	GogInnoInvocationFixedSilent                      = "fixed_silent_inno"
	InstallKindManagedArchive                         = "managed_archive"
	InstallKindGogInno                                = "gog_inno"
	InstallStateInstalled                             = "installed"
	InstallStateAttentionRequired                     = "attention_required"
	InstallStateCleanupRequired                       = "cleanup_required"
	InstallStateCleanupRunning                        = "cleanup_running"
	InstallStateCleanupFailed                         = "cleanup_failed"
	InstallStateIgnoredFailure                        = "ignored_failure"
	GogInnoCompletionExitZero                         = "exit_zero"
	GogInnoCompletionValidatedPostSuccessCrash        = "validated_post_success_crash"
	PackageTransferRoleInstaller                      = "installer"
	PackageTransferRoleCompanion                      = "companion"
	MaxGogInnoCompanions                              = 64
	GogInnoLocalConfirmationTimeout                   = 10 * time.Minute
	GogInnoInstallCommandLifetime                     = 4 * time.Hour
	GogInnoUninstallCommandLifetime                   = 1 * time.Hour
	GogInnoCleanupCommandLifetime                     = 1 * time.Hour
	GogInnoInstallerProcessTimeout                    = 2 * time.Hour
	GogInnoUninstallerProcessTimeout                  = 30 * time.Minute
)

var (
	gogInnoSetupNamePattern     = regexp.MustCompile(`(?i)^setup_.+\.exe$`)
	gogInnoCompanionNamePattern = regexp.MustCompile(`(?i)^setup_.+-\d+\.bin$`)
)

// PackageTransferDescriptor identifies one origin-relative package file transfer.
// It never carries executable arguments, shell content, or absolute filesystem paths.
type PackageTransferDescriptor struct {
	FileName      string `json:"file_name"`
	Role          string `json:"role"`
	SizeBytes     uint64 `json:"size_bytes"`
	DownloadURL   string `json:"download_url"`
	DownloadToken string `json:"download_token"`
}

func (d PackageTransferDescriptor) Validate() error {
	name := strings.TrimSpace(d.FileName)
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return errors.New("file_name must be one path segment")
	}
	switch strings.TrimSpace(d.Role) {
	case PackageTransferRoleInstaller, PackageTransferRoleCompanion:
	default:
		return fmt.Errorf("unsupported transfer role %q", d.Role)
	}
	if d.SizeBytes == 0 {
		return errors.New("size_bytes must be greater than zero")
	}
	if strings.TrimSpace(d.DownloadToken) == "" {
		return errors.New("download_token is required")
	}
	return validateOriginRelativeOrHTTPURL(d.DownloadURL)
}

// GogInnoInstallRequest installs one signed GOG Inno Setup package after local confirmation.
type GogInnoInstallRequest struct {
	GameID          string                      `json:"game_id"`
	SourceGameID    string                      `json:"source_game_id"`
	Title           string                      `json:"title"`
	DestinationRoot string                      `json:"destination_root,omitempty"`
	DestinationName string                      `json:"destination_name"`
	Installer       PackageTransferDescriptor   `json:"installer"`
	Companions      []PackageTransferDescriptor `json:"companions,omitempty"`
}

func (r GogInnoInstallRequest) Validate() error {
	for name, value := range map[string]string{
		"game_id": r.GameID, "source_game_id": r.SourceGameID, "title": r.Title, "destination_name": r.DestinationName,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if filepath.Base(r.DestinationName) != r.DestinationName || strings.ContainsAny(r.DestinationName, `/\`) {
		return errors.New("destination_name must be one path segment")
	}
	if err := r.Installer.Validate(); err != nil {
		return fmt.Errorf("installer: %w", err)
	}
	if r.Installer.Role != PackageTransferRoleInstaller {
		return errors.New("installer role must be installer")
	}
	if !IsGogInnoSetupFileName(r.Installer.FileName) {
		return fmt.Errorf("unsupported installer file name %q", r.Installer.FileName)
	}
	if len(r.Companions) > MaxGogInnoCompanions {
		return fmt.Errorf("companions exceed maximum of %d", MaxGogInnoCompanions)
	}
	seen := map[string]struct{}{
		strings.ToLower(strings.TrimSpace(r.Installer.FileName)): {},
	}
	stem := GogInnoSetupStem(r.Installer.FileName)
	for index, companion := range r.Companions {
		if err := companion.Validate(); err != nil {
			return fmt.Errorf("companions[%d]: %w", index, err)
		}
		if companion.Role != PackageTransferRoleCompanion {
			return fmt.Errorf("companions[%d]: role must be companion", index)
		}
		if !IsGogInnoCompanionFileName(companion.FileName) {
			return fmt.Errorf("companions[%d]: unsupported companion file name %q", index, companion.FileName)
		}
		if !strings.EqualFold(GogInnoCompanionStem(companion.FileName), stem) {
			return fmt.Errorf("companions[%d]: companion stem does not match installer", index)
		}
		key := strings.ToLower(strings.TrimSpace(companion.FileName))
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate package file name %q", companion.FileName)
		}
		seen[key] = struct{}{}
	}
	return nil
}

type GogInnoPackageFile struct {
	FileName  string `json:"file_name"`
	Role      string `json:"role"`
	SizeBytes uint64 `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

func (f GogInnoPackageFile) Validate() error {
	if strings.TrimSpace(f.FileName) == "" || filepath.Base(f.FileName) != f.FileName || strings.ContainsAny(f.FileName, `/\`) {
		return errors.New("file_name must be one path segment")
	}
	switch strings.TrimSpace(f.Role) {
	case PackageTransferRoleInstaller, PackageTransferRoleCompanion:
	default:
		return fmt.Errorf("unsupported package file role %q", f.Role)
	}
	if f.SizeBytes == 0 || len(strings.TrimSpace(f.SHA256)) != 64 || !isHexSHA256(f.SHA256) {
		return errors.New("size_bytes and sha256 are required")
	}
	return nil
}

type GogInnoInstallResult struct {
	GameID            string               `json:"game_id"`
	SourceGameID      string               `json:"source_game_id"`
	InstallRoot       string               `json:"install_root"`
	InstallPath       string               `json:"install_path"`
	InstallerFamily   string               `json:"installer_family"`
	PrimarySHA256     string               `json:"primary_sha256"`
	TotalPackageBytes uint64               `json:"total_package_bytes"`
	PackageFiles      []GogInnoPackageFile `json:"package_files"`
	SignerSubject     string               `json:"signer_subject"`
	SignerThumbprint  string               `json:"signer_thumbprint"`
	InvocationMode    string               `json:"invocation_mode"`
	UninstallTarget   string               `json:"uninstall_target"`
	LaunchTarget      string               `json:"launch_target,omitempty"`
	LaunchCandidates  []string             `json:"launch_candidates,omitempty"`
	ProcessID         int                  `json:"process_id,omitempty"`
	ExitCode          *int                 `json:"exit_code,omitempty"`
	DiagnosticRef     string               `json:"diagnostic_ref,omitempty"`
	InstalledAt       time.Time            `json:"installed_at"`
	CompletionBasis   string               `json:"completion_basis,omitempty"`
	CleanupMarkerID   string               `json:"cleanup_marker_id,omitempty"`
}

func (r GogInnoInstallResult) Validate() error {
	if err := r.ValidateFailureEvidence(); err != nil {
		return err
	}
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if !filepath.IsAbs(r.InstallRoot) || !filepath.IsAbs(r.InstallPath) {
		return errors.New("install_root and install_path must be absolute")
	}
	if r.InstallerFamily != GogInnoInstallerFamily {
		return fmt.Errorf("unsupported installer_family %q", r.InstallerFamily)
	}
	if len(r.PrimarySHA256) != 64 || !isHexSHA256(r.PrimarySHA256) || r.TotalPackageBytes == 0 || r.InstalledAt.IsZero() {
		return errors.New("primary hash, total package bytes, and installed_at are required")
	}
	if strings.TrimSpace(r.SignerSubject) == "" || strings.TrimSpace(r.SignerThumbprint) == "" {
		return errors.New("signer_subject and signer_thumbprint are required")
	}
	if r.InvocationMode != GogInnoInvocationFixedSilent {
		return fmt.Errorf("unsupported invocation_mode %q", r.InvocationMode)
	}
	switch r.CompletionBasis {
	case GogInnoCompletionExitZero, GogInnoCompletionValidatedPostSuccessCrash:
	default:
		return fmt.Errorf("unsupported completion_basis %q", r.CompletionBasis)
	}
	if err := ValidateUninstallTarget(r.UninstallTarget); err != nil {
		return err
	}
	if len(r.PackageFiles) == 0 {
		return errors.New("package_files are required")
	}
	seen := make(map[string]struct{}, len(r.PackageFiles))
	var total uint64
	foundInstaller := false
	for _, file := range r.PackageFiles {
		if err := file.Validate(); err != nil {
			return err
		}
		key := strings.ToLower(file.FileName)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate package file %q", file.FileName)
		}
		seen[key] = struct{}{}
		total += file.SizeBytes
		if file.Role == PackageTransferRoleInstaller {
			if foundInstaller {
				return errors.New("package_files must contain exactly one installer")
			}
			foundInstaller = true
			if !strings.EqualFold(file.SHA256, r.PrimarySHA256) {
				return errors.New("primary_sha256 must match the installer file hash")
			}
		}
	}
	if !foundInstaller {
		return errors.New("package_files must contain exactly one installer")
	}
	if total != r.TotalPackageBytes {
		return errors.New("total_package_bytes must equal the sum of package file sizes")
	}
	candidateSeen := make(map[string]struct{}, len(r.LaunchCandidates))
	for _, candidate := range r.LaunchCandidates {
		if err := ValidateLaunchTarget(candidate); err != nil {
			return err
		}
		normalized := NormalizeLaunchTarget(candidate)
		if _, exists := candidateSeen[normalized]; exists {
			return fmt.Errorf("duplicate launch candidate %q", candidate)
		}
		candidateSeen[normalized] = struct{}{}
	}
	if r.LaunchTarget != "" {
		if err := ValidateLaunchTarget(r.LaunchTarget); err != nil {
			return err
		}
		if _, exists := candidateSeen[NormalizeLaunchTarget(r.LaunchTarget)]; !exists {
			return errors.New("launch_target must be included in launch_candidates")
		}
	}
	return nil
}

// ValidateFailureEvidence validates the sanitized post-start evidence that may
// be persisted when installation did not commit. It deliberately does not
// require a completion basis, installed timestamp, uninstaller, or launch
// target, all of which can be absent after a true failure.
func (r GogInnoInstallResult) ValidateFailureEvidence() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if !filepath.IsAbs(r.InstallRoot) || !filepath.IsAbs(r.InstallPath) {
		return errors.New("install_root and install_path must be absolute")
	}
	relative, err := filepath.Rel(filepath.Clean(r.InstallRoot), filepath.Clean(r.InstallPath))
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return errors.New("install_path must be a non-root child of install_root")
	}
	if r.InstallerFamily != GogInnoInstallerFamily {
		return fmt.Errorf("unsupported installer_family %q", r.InstallerFamily)
	}
	if !isHexSHA256(r.PrimarySHA256) || r.TotalPackageBytes == 0 {
		return errors.New("primary hash and total package bytes are required")
	}
	if strings.TrimSpace(r.SignerSubject) == "" || strings.TrimSpace(r.SignerThumbprint) == "" {
		return errors.New("signer_subject and signer_thumbprint are required")
	}
	if r.InvocationMode != GogInnoInvocationFixedSilent {
		return fmt.Errorf("unsupported invocation_mode %q", r.InvocationMode)
	}
	if r.CleanupMarkerID != "" && !isBase64URLMarker(r.CleanupMarkerID) {
		return errors.New("cleanup_marker_id must be a 256-bit base64url value")
	}
	if len(r.DiagnosticRef) > 512 {
		return errors.New("diagnostic_ref exceeds the bounded limit")
	}
	if strings.TrimSpace(r.UninstallTarget) != "" {
		if err := ValidateUninstallTarget(r.UninstallTarget); err != nil {
			return err
		}
	}
	if len(r.PackageFiles) == 0 {
		return errors.New("package_files are required")
	}
	seen := make(map[string]struct{}, len(r.PackageFiles))
	var total uint64
	foundInstaller := false
	for _, file := range r.PackageFiles {
		if err := file.Validate(); err != nil {
			return err
		}
		key := strings.ToLower(file.FileName)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate package file %q", file.FileName)
		}
		seen[key] = struct{}{}
		if ^uint64(0)-total < file.SizeBytes {
			return errors.New("package file size total overflows")
		}
		total += file.SizeBytes
		if file.Role == PackageTransferRoleInstaller {
			if foundInstaller {
				return errors.New("package_files must contain exactly one installer")
			}
			foundInstaller = true
			if !strings.EqualFold(file.SHA256, r.PrimarySHA256) {
				return errors.New("primary_sha256 must match the installer file hash")
			}
		}
	}
	if !foundInstaller || total != r.TotalPackageBytes {
		return errors.New("package_files require one installer and must sum to total_package_bytes")
	}
	return nil
}

type GogInnoFailedCleanupRequest struct {
	GameID          string `json:"game_id"`
	SourceGameID    string `json:"source_game_id"`
	InstallRoot     string `json:"install_root"`
	InstallPath     string `json:"install_path"`
	InstallerFamily string `json:"installer_family"`
	CleanupMarkerID string `json:"cleanup_marker_id"`
	PrimarySHA256   string `json:"primary_sha256"`
	UninstallTarget string `json:"uninstall_target,omitempty"`
}

func (r GogInnoFailedCleanupRequest) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if !filepath.IsAbs(strings.TrimSpace(r.InstallRoot)) || !filepath.IsAbs(strings.TrimSpace(r.InstallPath)) {
		return errors.New("install_root and install_path must be absolute")
	}
	if r.InstallerFamily != GogInnoInstallerFamily {
		return fmt.Errorf("unsupported installer_family %q", r.InstallerFamily)
	}
	if !isBase64URLMarker(r.CleanupMarkerID) {
		return errors.New("cleanup_marker_id must be a 256-bit base64url value")
	}
	if !isHexSHA256(r.PrimarySHA256) {
		return errors.New("primary_sha256 is invalid")
	}
	if strings.TrimSpace(r.UninstallTarget) != "" {
		return ValidateUninstallTarget(r.UninstallTarget)
	}
	return nil
}

type GogInnoFailedCleanupResult struct {
	GameID                   string `json:"game_id"`
	SourceGameID             string `json:"source_game_id"`
	Removed                  bool   `json:"removed"`
	PublisherUninstallerUsed bool   `json:"publisher_uninstaller_used"`
	BoundedDeleteUsed        bool   `json:"bounded_delete_used"`
	SystemChangesMayRemain   bool   `json:"system_changes_may_remain"`
	LeftoverDirectory        bool   `json:"leftover_directory"`
	ProcessID                int    `json:"process_id,omitempty"`
	ExitCode                 *int   `json:"exit_code,omitempty"`
	DiagnosticRef            string `json:"diagnostic_ref,omitempty"`
	Summary                  string `json:"summary,omitempty"`
}

func (r GogInnoFailedCleanupResult) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if !r.Removed {
		return errors.New("successful cleanup result must report removed")
	}
	if len(r.Summary) > 512 || len(r.DiagnosticRef) > 512 {
		return errors.New("cleanup result text exceeds the bounded limit")
	}
	return nil
}

type GogInnoUninstallRequest struct {
	GameID          string `json:"game_id"`
	SourceGameID    string `json:"source_game_id"`
	InstallPath     string `json:"install_path"`
	InstallerFamily string `json:"installer_family"`
	UninstallTarget string `json:"uninstall_target"`
}

func (r GogInnoUninstallRequest) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if !filepath.IsAbs(strings.TrimSpace(r.InstallPath)) {
		return errors.New("install_path must be absolute")
	}
	if r.InstallerFamily != GogInnoInstallerFamily {
		return fmt.Errorf("unsupported installer_family %q", r.InstallerFamily)
	}
	return ValidateUninstallTarget(r.UninstallTarget)
}

type GogInnoUninstallResult struct {
	GameID            string `json:"game_id"`
	SourceGameID      string `json:"source_game_id"`
	Removed           bool   `json:"removed"`
	LeftoverDirectory bool   `json:"leftover_directory"`
	ProcessID         int    `json:"process_id,omitempty"`
	ExitCode          *int   `json:"exit_code,omitempty"`
	DiagnosticRef     string `json:"diagnostic_ref,omitempty"`
}

func (r GogInnoUninstallResult) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	return nil
}

func IsGogInnoSetupFileName(name string) bool {
	base := filepath.Base(strings.TrimSpace(name))
	return gogInnoSetupNamePattern.MatchString(base)
}

func IsGogInnoCompanionFileName(name string) bool {
	base := filepath.Base(strings.TrimSpace(name))
	return gogInnoCompanionNamePattern.MatchString(base)
}

func GogInnoSetupStem(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	ext := filepath.Ext(base)
	if ext == "" {
		return strings.ToLower(base)
	}
	return strings.ToLower(strings.TrimSuffix(base, ext))
}

func GogInnoCompanionStem(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	lower := strings.ToLower(base)
	if !strings.HasSuffix(lower, ".bin") {
		return GogInnoSetupStem(name)
	}
	withoutExt := strings.TrimSuffix(lower, ".bin")
	dash := strings.LastIndex(withoutExt, "-")
	if dash <= 0 {
		return withoutExt
	}
	suffix := withoutExt[dash+1:]
	for _, r := range suffix {
		if !unicode.IsDigit(r) {
			return withoutExt
		}
	}
	return withoutExt[:dash]
}

func ValidateUninstallTarget(value string) error {
	normalized := NormalizeLaunchTarget(value)
	if normalized == "." || normalized == "" || strings.HasPrefix(normalized, "../") || normalized == ".." || path.IsAbs(normalized) || strings.Contains(normalized, ":") {
		return fmt.Errorf("unsafe uninstall target %q", value)
	}
	base := path.Base(normalized)
	if !strings.HasPrefix(strings.ToLower(base), "unins") || !strings.EqualFold(path.Ext(base), ".exe") {
		return errors.New("uninstall target must be a relative Inno uninstaller executable")
	}
	return nil
}

func validateOriginRelativeOrHTTPURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("download_url is invalid")
	}
	if parsed.IsAbs() {
		if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return errors.New("download_url must use HTTP(S)")
		}
		return nil
	}
	if parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return errors.New("download_url must be absolute HTTP(S) or an origin-relative path")
	}
	return nil
}

func isHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func isBase64URLMarker(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 43 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}
