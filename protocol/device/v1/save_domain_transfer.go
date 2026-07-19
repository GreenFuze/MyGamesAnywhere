package v1

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	SaveDomainSlotID         = "autosave"
	SaveDomainMaxFiles       = 4096
	SaveDomainMaxBytes int64 = 64 << 20
)

type SaveDomainSnapshotRequest struct {
	GameID            string `json:"game_id"`
	SourceGameID      string `json:"source_game_id"`
	Title             string `json:"title"`
	LocalSaveDomainID string `json:"local_save_domain_id"`
	UploadURL         string `json:"upload_url"`
	UploadToken       string `json:"upload_token"`
}

func (r SaveDomainSnapshotRequest) Validate() error {
	return validateSaveDomainTransferIdentity(r.GameID, r.SourceGameID, r.Title, r.LocalSaveDomainID, r.UploadURL, r.UploadToken)
}

type SaveDomainRestoreRequest struct {
	GameID            string `json:"game_id"`
	SourceGameID      string `json:"source_game_id"`
	Title             string `json:"title"`
	LocalSaveDomainID string `json:"local_save_domain_id"`
	DownloadURL       string `json:"download_url"`
	DownloadToken     string `json:"download_token"`
	ManifestHash      string `json:"manifest_hash"`
	PreserveLocal     bool   `json:"preserve_local,omitempty"`
}

type SaveDomainReconcileRequest struct {
	GameID            string `json:"game_id"`
	SourceGameID      string `json:"source_game_id"`
	Title             string `json:"title"`
	LocalSaveDomainID string `json:"local_save_domain_id"`
	Strategy          string `json:"strategy"`
	TransferURL       string `json:"transfer_url"`
	TransferToken     string `json:"transfer_token"`
	ManifestHash      string `json:"manifest_hash,omitempty"`
}

func (r SaveDomainReconcileRequest) Validate() error {
	if err := validateSaveDomainTransferIdentity(r.GameID, r.SourceGameID, r.Title, r.LocalSaveDomainID, r.TransferURL, r.TransferToken); err != nil {
		return err
	}
	if r.Strategy != "keep_local" && r.Strategy != "keep_server" {
		return errors.New("save reconciliation strategy must be keep_local or keep_server")
	}
	if r.Strategy == "keep_server" && !sha256Pattern.MatchString(strings.ToLower(strings.TrimSpace(r.ManifestHash))) {
		return errors.New("keep_server reconciliation requires a manifest hash")
	}
	return nil
}

func (r SaveDomainRestoreRequest) Validate() error {
	if err := validateSaveDomainTransferIdentity(r.GameID, r.SourceGameID, r.Title, r.LocalSaveDomainID, r.DownloadURL, r.DownloadToken); err != nil {
		return err
	}
	if !sha256Pattern.MatchString(strings.ToLower(strings.TrimSpace(r.ManifestHash))) {
		return errors.New("manifest_hash must contain 64 hexadecimal characters")
	}
	return nil
}

func validateSaveDomainTransferIdentity(gameID, sourceGameID, title, localID, transferURL, token string) error {
	for name, value := range map[string]string{"game_id": gameID, "source_game_id": sourceGameID, "title": title, "local_save_domain_id": localID, "transfer_url": transferURL, "transfer_token": token} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	parsed, err := url.Parse(strings.TrimSpace(transferURL))
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path != "/api/device-transfers/save-domain" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("save-domain transfer URL must use the bounded MGA endpoint")
	}
	return nil
}

type SaveDomainSnapshotFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Hash string `json:"hash"`
}

type SaveDomainSnapshot struct {
	LocalFingerprint string                   `json:"local_fingerprint"`
	CapturedAt       time.Time                `json:"captured_at"`
	TotalSize        int64                    `json:"total_size"`
	Files            []SaveDomainSnapshotFile `json:"files"`
	ArchiveBase64    string                   `json:"archive_base64"`
}

func (s SaveDomainSnapshot) Validate() error {
	if !sha256Pattern.MatchString(strings.ToLower(strings.TrimSpace(s.LocalFingerprint))) || s.CapturedAt.IsZero() {
		return errors.New("snapshot fingerprint and capture time are required")
	}
	if len(s.Files) > SaveDomainMaxFiles || s.TotalSize < 0 || s.TotalSize > SaveDomainMaxBytes {
		return errors.New("snapshot exceeds the bounded file or size limit")
	}
	seen := map[string]bool{}
	var total int64
	for _, file := range s.Files {
		normalized := path.Clean(strings.ReplaceAll(strings.TrimSpace(file.Path), "\\", "/"))
		if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") || path.IsAbs(normalized) || normalized != file.Path {
			return fmt.Errorf("unsafe snapshot path %q", file.Path)
		}
		if seen[strings.ToLower(normalized)] || file.Size < 0 || !sha256Pattern.MatchString(strings.ToLower(strings.TrimSpace(file.Hash))) {
			return fmt.Errorf("invalid snapshot file %q", file.Path)
		}
		seen[strings.ToLower(normalized)] = true
		total += file.Size
		if total > SaveDomainMaxBytes {
			return errors.New("snapshot exceeds the bounded size limit")
		}
	}
	if total != s.TotalSize {
		return errors.New("snapshot total size does not match its file list")
	}
	archive, err := base64.StdEncoding.DecodeString(s.ArchiveBase64)
	if err != nil || len(archive) == 0 || int64(len(archive)) > SaveDomainMaxBytes+(1<<20) {
		return errors.New("snapshot archive is invalid or too large")
	}
	return nil
}

type SaveDomainTransferConflict struct {
	ManifestHash string `json:"manifest_hash"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	FileCount    int    `json:"file_count,omitempty"`
	TotalSize    int64  `json:"total_size,omitempty"`
}

type SaveDomainUploadResponse struct {
	Stored       bool                        `json:"stored"`
	ManifestHash string                      `json:"manifest_hash,omitempty"`
	Conflict     *SaveDomainTransferConflict `json:"conflict,omitempty"`
}

func (r SaveDomainUploadResponse) Validate() error {
	if r.Stored {
		if !sha256Pattern.MatchString(strings.ToLower(r.ManifestHash)) || r.Conflict != nil {
			return errors.New("stored upload requires one manifest hash")
		}
		return nil
	}
	if r.ManifestHash != "" || r.Conflict == nil || !sha256Pattern.MatchString(strings.ToLower(r.Conflict.ManifestHash)) {
		return errors.New("conflicting upload requires remote conflict metadata")
	}
	return nil
}

type SaveDomainSnapshotResult struct {
	GameID            string                      `json:"game_id"`
	SourceGameID      string                      `json:"source_game_id"`
	LocalSaveDomainID string                      `json:"local_save_domain_id"`
	LocalFingerprint  string                      `json:"local_fingerprint"`
	State             string                      `json:"state"`
	ManifestHash      string                      `json:"manifest_hash,omitempty"`
	Conflict          *SaveDomainTransferConflict `json:"conflict,omitempty"`
	CompletedAt       time.Time                   `json:"completed_at"`
}

func (r SaveDomainSnapshotResult) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" || strings.TrimSpace(r.LocalSaveDomainID) == "" || !sha256Pattern.MatchString(strings.ToLower(r.LocalFingerprint)) || r.CompletedAt.IsZero() {
		return errors.New("save-domain snapshot result identity is invalid")
	}
	switch r.State {
	case "stored":
		if !sha256Pattern.MatchString(strings.ToLower(r.ManifestHash)) || r.Conflict != nil {
			return errors.New("stored snapshot requires one manifest hash")
		}
	case "conflict":
		if r.Conflict == nil || !sha256Pattern.MatchString(strings.ToLower(r.Conflict.ManifestHash)) || r.ManifestHash != "" {
			return errors.New("conflicting snapshot requires remote conflict metadata")
		}
	default:
		return fmt.Errorf("unsupported snapshot result state %q", r.State)
	}
	return nil
}

type SaveDomainRestoreResult struct {
	GameID            string    `json:"game_id"`
	SourceGameID      string    `json:"source_game_id"`
	LocalSaveDomainID string    `json:"local_save_domain_id"`
	LocalFingerprint  string    `json:"local_fingerprint"`
	ManifestHash      string    `json:"manifest_hash"`
	BackupPathCreated bool      `json:"backup_path_created,omitempty"`
	RestoredAt        time.Time `json:"restored_at"`
}

type SaveDomainReconcileResult struct {
	GameID            string    `json:"game_id"`
	SourceGameID      string    `json:"source_game_id"`
	LocalSaveDomainID string    `json:"local_save_domain_id"`
	Strategy          string    `json:"strategy"`
	LocalFingerprint  string    `json:"local_fingerprint"`
	ManifestHash      string    `json:"manifest_hash"`
	BackupPathCreated bool      `json:"backup_path_created,omitempty"`
	State             string    `json:"state"`
	CompletedAt       time.Time `json:"completed_at"`
}

func (r SaveDomainReconcileResult) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" || strings.TrimSpace(r.LocalSaveDomainID) == "" || r.State != "owned_here" || r.CompletedAt.IsZero() ||
		(r.Strategy != "keep_local" && r.Strategy != "keep_server") || !sha256Pattern.MatchString(strings.ToLower(r.LocalFingerprint)) || !sha256Pattern.MatchString(strings.ToLower(r.ManifestHash)) {
		return errors.New("save-domain reconciliation result is invalid")
	}
	return nil
}

func (r SaveDomainRestoreResult) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" || strings.TrimSpace(r.LocalSaveDomainID) == "" || r.RestoredAt.IsZero() ||
		!sha256Pattern.MatchString(strings.ToLower(r.LocalFingerprint)) || !sha256Pattern.MatchString(strings.ToLower(r.ManifestHash)) {
		return errors.New("save-domain restore result is invalid")
	}
	return nil
}
