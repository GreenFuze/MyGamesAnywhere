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
	FileDownloadSchemaVersion         uint16 = 1
	PreparedCopyManifestSchemaVersion        = 1
	DeviceDownloadSourceProfile              = "device.files.v1"
	FileDownloadCommandLifetime              = 12 * time.Hour
)

// FileDownloadItem describes one immutable, token-authorized file exposed by
// the paired MGA Server. RelativePath is always relative to the prepared copy.
type FileDownloadItem struct {
	RelativePath  string `json:"relative_path"`
	SizeBytes     uint64 `json:"size_bytes"`
	SHA256        string `json:"sha256"`
	DownloadURL   string `json:"download_url"`
	DownloadToken string `json:"download_token"`
}

func (i FileDownloadItem) Validate() error {
	cleaned := path.Clean(strings.ReplaceAll(strings.TrimSpace(i.RelativePath), `\`, "/"))
	if cleaned == "." || cleaned == "" || cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) || strings.Contains(cleaned, ":") {
		return fmt.Errorf("unsafe relative file path %q", i.RelativePath)
	}
	if len(i.SHA256) != 64 {
		return errors.New("download file SHA-256 is required")
	}
	if strings.TrimSpace(i.DownloadToken) == "" {
		return errors.New("download token is required")
	}
	parsed, err := url.Parse(strings.TrimSpace(i.DownloadURL))
	if err != nil || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("download URL is invalid")
	}
	if parsed.IsAbs() {
		if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return errors.New("download URL must use HTTP(S)")
		}
	} else if parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return errors.New("download URL must be absolute HTTP(S) or origin-relative")
	}
	return nil
}

// FileDownloadRequest creates a verified Prepared copy without installing or
// executing any content.
type FileDownloadRequest struct {
	SchemaVersion   uint16             `json:"schema_version"`
	GameID          string             `json:"game_id"`
	SourceGameID    string             `json:"source_game_id"`
	Title           string             `json:"title"`
	DestinationRoot string             `json:"destination_root,omitempty"`
	DestinationName string             `json:"destination_name"`
	Files           []FileDownloadItem `json:"files"`
}

func (r FileDownloadRequest) Validate() error {
	if r.SchemaVersion != FileDownloadSchemaVersion {
		return fmt.Errorf("unsupported file download schema version %d", r.SchemaVersion)
	}
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" || strings.TrimSpace(r.Title) == "" {
		return errors.New("game_id, source_game_id, and title are required")
	}
	if filepath.Base(r.DestinationName) != r.DestinationName || strings.ContainsAny(r.DestinationName, `/\`) {
		return errors.New("destination_name must be one path segment")
	}
	if len(r.Files) == 0 || len(r.Files) > 4096 {
		return errors.New("between 1 and 4096 files are required")
	}
	seen := map[string]bool{}
	var total uint64
	for index, file := range r.Files {
		if err := file.Validate(); err != nil {
			return fmt.Errorf("file %d: %w", index, err)
		}
		key := strings.ToLower(path.Clean(strings.ReplaceAll(file.RelativePath, `\`, "/")))
		if seen[key] {
			return fmt.Errorf("duplicate relative file path %q", file.RelativePath)
		}
		seen[key] = true
		if ^uint64(0)-total < file.SizeBytes {
			return errors.New("download size overflow")
		}
		total += file.SizeBytes
	}
	return nil
}

func (r FileDownloadRequest) TotalBytes() uint64 {
	var total uint64
	for _, file := range r.Files {
		total += file.SizeBytes
	}
	return total
}

type FileDownloadResult struct {
	GameID       string    `json:"game_id"`
	SourceGameID string    `json:"source_game_id"`
	PreparedRoot string    `json:"prepared_root"`
	PreparedPath string    `json:"prepared_path"`
	FileCount    int       `json:"file_count"`
	TotalBytes   uint64    `json:"total_bytes"`
	PreparedAt   time.Time `json:"prepared_at"`
}

func (r FileDownloadResult) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if !filepath.IsAbs(r.PreparedRoot) || !filepath.IsAbs(r.PreparedPath) || r.FileCount <= 0 || r.PreparedAt.IsZero() {
		return errors.New("absolute prepared paths, file facts, and prepared_at are required")
	}
	return nil
}
