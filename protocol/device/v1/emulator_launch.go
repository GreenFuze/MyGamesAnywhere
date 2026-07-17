package v1

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

const EmulatorLaunchSchemaVersion uint16 = 1

var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type EmulatorContentArtifact struct {
	Path          string `json:"path"`
	SizeBytes     uint64 `json:"size_bytes"`
	SHA256        string `json:"sha256"`
	DownloadURL   string `json:"download_url"`
	DownloadToken string `json:"download_token"`
}

func (a EmulatorContentArtifact) Validate() error {
	clean := path.Clean(strings.TrimSpace(a.Path))
	if clean == "." || clean == "" || clean != a.Path || path.IsAbs(clean) || strings.HasPrefix(clean, "../") || strings.ContainsAny(clean, `\:`) {
		return fmt.Errorf("unsafe emulator content path %q", a.Path)
	}
	if !sha256Pattern.MatchString(strings.ToLower(strings.TrimSpace(a.SHA256))) {
		return errors.New("emulator content sha256 must contain 64 hexadecimal characters")
	}
	if strings.TrimSpace(a.DownloadToken) == "" {
		return errors.New("emulator content download_token is required")
	}
	if a.SizeBytes > uint64(math.MaxInt64-1) {
		return errors.New("emulator content size is too large")
	}
	parsed, err := url.Parse(strings.TrimSpace(a.DownloadURL))
	if err != nil || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("emulator content download_url is invalid")
	}
	if parsed.IsAbs() {
		if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return errors.New("emulator content download_url must use HTTP(S)")
		}
	} else if parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return errors.New("emulator content download_url must be absolute HTTP(S) or origin-relative")
	}
	return nil
}

type EmulatorLaunchRequest struct {
	GameID       string                    `json:"game_id"`
	SourceGameID string                    `json:"source_game_id"`
	Title        string                    `json:"title"`
	Platform     string                    `json:"platform"`
	EmulatorID   string                    `json:"emulator_id"`
	CoreID       string                    `json:"core_id,omitempty"`
	ContentPath  string                    `json:"content_path,omitempty"`
	Artifacts    []EmulatorContentArtifact `json:"artifacts"`
}

func (r EmulatorLaunchRequest) Validate() error {
	for name, value := range map[string]string{
		"game_id": r.GameID, "source_game_id": r.SourceGameID, "title": r.Title,
		"platform": r.Platform, "emulator_id": r.EmulatorID,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if len(r.Artifacts) == 0 || len(r.Artifacts) > 4096 {
		return errors.New("emulator launch requires between 1 and 4096 content artifacts")
	}
	if r.EmulatorID != strings.TrimSpace(r.EmulatorID) || !emulatorSetupIDPattern.MatchString(r.EmulatorID) {
		return fmt.Errorf("invalid emulator_id %q", r.EmulatorID)
	}
	if r.EmulatorID == "retroarch" {
		if r.CoreID != strings.TrimSpace(r.CoreID) || !emulatorSetupIDPattern.MatchString(r.CoreID) {
			return errors.New("retroarch launch requires core_id")
		}
		if err := validateEmulatorRelativePath("content_path", r.ContentPath); err != nil {
			return err
		}
	} else if r.CoreID != "" || r.ContentPath != "" {
		return errors.New("core_id and content_path are only supported for typed core adapters")
	}
	seen := make(map[string]bool, len(r.Artifacts))
	for _, artifact := range r.Artifacts {
		if err := artifact.Validate(); err != nil {
			return err
		}
		key := strings.ToLower(artifact.Path)
		if seen[key] {
			return fmt.Errorf("duplicate emulator content path %q", artifact.Path)
		}
		seen[key] = true
	}
	return nil
}

type EmulatorLaunchResult struct {
	GameID       string    `json:"game_id"`
	SourceGameID string    `json:"source_game_id"`
	EmulatorID   string    `json:"emulator_id"`
	CoreID       string    `json:"core_id,omitempty"`
	ProcessID    int       `json:"process_id"`
	StartedAt    time.Time `json:"started_at"`
}

func validateEmulatorRelativePath(field, value string) error {
	clean := path.Clean(strings.TrimSpace(value))
	if clean == "." || clean == "" || clean != value || path.IsAbs(clean) || strings.HasPrefix(clean, "../") || strings.ContainsAny(clean, `\:`) {
		return fmt.Errorf("unsafe emulator %s %q", field, value)
	}
	return nil
}

func (r EmulatorLaunchResult) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" || strings.TrimSpace(r.EmulatorID) == "" || r.ProcessID <= 0 || r.StartedAt.IsZero() {
		return errors.New("game_id, source_game_id, emulator_id, process_id, and started_at are required")
	}
	return nil
}
