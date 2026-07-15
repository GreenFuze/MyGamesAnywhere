package v1

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	ArchiveInstallSchemaVersion  uint16 = 1
	InstallManifestSchemaVersion        = 2
	DefaultInstallRootTemplate          = `%USERPROFILE%\Games`
	ArchiveFormatZIP                    = "zip"
	ArchiveFormat7Z                     = "7z"
	ArchiveFormatRAR                    = "rar"
)

// ArchiveInstallRequest is a bounded request to download and install one
// archive. The client accepts only same-server HTTP(S) URLs and never executes
// archive contents during installation.
type ArchiveInstallRequest struct {
	GameID          string `json:"game_id"`
	SourceGameID    string `json:"source_game_id"`
	Title           string `json:"title"`
	ArchiveName     string `json:"archive_name"`
	ArchiveFormat   string `json:"archive_format"`
	ArchiveSize     uint64 `json:"archive_size"`
	DownloadURL     string `json:"download_url"`
	DownloadToken   string `json:"download_token"`
	DestinationRoot string `json:"destination_root,omitempty"`
	DestinationName string `json:"destination_name"`
}

func (r ArchiveInstallRequest) Validate() error {
	for name, value := range map[string]string{
		"game_id": r.GameID, "source_game_id": r.SourceGameID, "title": r.Title,
		"archive_name": r.ArchiveName, "archive_format": r.ArchiveFormat,
		"download_url": r.DownloadURL, "download_token": r.DownloadToken, "destination_name": r.DestinationName,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if err := ValidateArchiveFormat(r.ArchiveFormat); err != nil {
		return err
	}
	if r.ArchiveSize == 0 {
		return errors.New("archive_size must be greater than zero")
	}
	parsed, err := url.Parse(r.DownloadURL)
	if err != nil || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("download_url is invalid")
	}
	if parsed.IsAbs() {
		if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return errors.New("download_url must use HTTP(S)")
		}
	} else if parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return errors.New("download_url must be absolute HTTP(S) or an origin-relative path")
	}
	if filepath.Base(r.DestinationName) != r.DestinationName || strings.ContainsAny(r.DestinationName, `/\`) {
		return errors.New("destination_name must be one path segment")
	}
	return nil
}

func NormalizeArchiveFormat(value string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "."))
}

func ValidateArchiveFormat(value string) error {
	switch NormalizeArchiveFormat(value) {
	case ArchiveFormatZIP, ArchiveFormat7Z, ArchiveFormatRAR:
		return nil
	default:
		return fmt.Errorf("unsupported archive format %q", value)
	}
}

type ArchiveInstallResult struct {
	GameID           string    `json:"game_id"`
	SourceGameID     string    `json:"source_game_id"`
	InstallRoot      string    `json:"install_root"`
	InstallPath      string    `json:"install_path"`
	ArchiveSHA256    string    `json:"archive_sha256"`
	ArchiveBytes     uint64    `json:"archive_bytes"`
	InstalledAt      time.Time `json:"installed_at"`
	LaunchTarget     string    `json:"launch_target,omitempty"`
	LaunchCandidates []string  `json:"launch_candidates,omitempty"`
}

func (r ArchiveInstallResult) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if !filepath.IsAbs(r.InstallRoot) || !filepath.IsAbs(r.InstallPath) {
		return errors.New("install_root and install_path must be absolute")
	}
	if len(r.ArchiveSHA256) != 64 || r.ArchiveBytes == 0 || r.InstalledAt.IsZero() {
		return errors.New("archive hash, size, and installed_at are required")
	}
	if r.LaunchTarget != "" {
		if err := ValidateLaunchTarget(r.LaunchTarget); err != nil {
			return err
		}
	}
	seen := make(map[string]struct{}, len(r.LaunchCandidates))
	for _, candidate := range r.LaunchCandidates {
		if err := ValidateLaunchTarget(candidate); err != nil {
			return err
		}
		normalized := NormalizeLaunchTarget(candidate)
		if _, exists := seen[normalized]; exists {
			return fmt.Errorf("duplicate launch candidate %q", candidate)
		}
		seen[normalized] = struct{}{}
	}
	if r.LaunchTarget != "" {
		if _, exists := seen[NormalizeLaunchTarget(r.LaunchTarget)]; !exists {
			return errors.New("launch_target must be included in launch_candidates")
		}
	}
	return nil
}

type GameLaunchRequest struct {
	GameID       string `json:"game_id"`
	SourceGameID string `json:"source_game_id"`
	InstallPath  string `json:"install_path"`
	LaunchTarget string `json:"launch_target"`
}

func (r GameLaunchRequest) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if !filepath.IsAbs(strings.TrimSpace(r.InstallPath)) {
		return errors.New("install_path must be absolute")
	}
	return ValidateLaunchTarget(r.LaunchTarget)
}

type GameLaunchResult struct {
	GameID       string    `json:"game_id"`
	SourceGameID string    `json:"source_game_id"`
	ProcessID    int       `json:"process_id"`
	StartedAt    time.Time `json:"started_at"`
}

func (r GameLaunchResult) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" || r.ProcessID <= 0 || r.StartedAt.IsZero() {
		return errors.New("game_id, source_game_id, process_id, and started_at are required")
	}
	return nil
}

func NormalizeLaunchTarget(value string) string {
	return path.Clean(strings.ReplaceAll(strings.TrimSpace(value), `\`, "/"))
}

func ValidateLaunchTarget(value string) error {
	normalized := NormalizeLaunchTarget(value)
	if normalized == "." || normalized == "" || strings.HasPrefix(normalized, "../") || normalized == ".." || path.IsAbs(normalized) || strings.Contains(normalized, ":") {
		return fmt.Errorf("unsafe launch target %q", value)
	}
	if !strings.EqualFold(path.Ext(normalized), ".exe") {
		return errors.New("launch target must be a Windows executable")
	}
	return nil
}

type GameUninstallRequest struct {
	GameID       string `json:"game_id"`
	SourceGameID string `json:"source_game_id"`
	InstallPath  string `json:"install_path"`
}

func (r GameUninstallRequest) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if !filepath.IsAbs(strings.TrimSpace(r.InstallPath)) {
		return errors.New("install_path must be absolute")
	}
	return nil
}

type GameUninstallResult struct {
	GameID       string `json:"game_id"`
	SourceGameID string `json:"source_game_id"`
	Removed      bool   `json:"removed"`
}
