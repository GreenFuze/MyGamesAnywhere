package clientapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/google/uuid"
)

const preparedCopyManifestName = ".mga-prepared.json"

type FileDownloader interface {
	Download(context.Context, string, devicev1.FileDownloadRequest, CommandProgressReporter) (devicev1.FileDownloadResult, error)
}

type ManagedFileDownloader struct {
	serverURL string
	client    *http.Client
	now       func() time.Time
	ownership *InstallationOwnership
	catalog   *PreparedCopyCatalog
}

func NewManagedFileDownloader(serverURL string, ownership *InstallationOwnership, catalog *PreparedCopyCatalog) (*ManagedFileDownloader, error) {
	if ownership == nil || catalog == nil {
		return nil, errors.New("download ownership and prepared copy catalog are required")
	}
	parsed, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("valid MGA Server URL is required")
	}
	return &ManagedFileDownloader{
		serverURL: parsed.Scheme + "://" + parsed.Host,
		client:    &http.Client{Timeout: 0}, now: time.Now, ownership: ownership, catalog: catalog,
	}, nil
}

func (d *ManagedFileDownloader) Download(ctx context.Context, commandID string, request devicev1.FileDownloadRequest, report CommandProgressReporter) (devicev1.FileDownloadResult, error) {
	if d == nil || d.client == nil || d.catalog == nil || d.ownership == nil {
		return devicev1.FileDownloadResult{}, errors.New("file downloader is unavailable")
	}
	if strings.TrimSpace(commandID) == "" {
		return devicev1.FileDownloadResult{}, errors.New("command_id is required")
	}
	if _, err := uuid.Parse(commandID); err != nil {
		return devicev1.FileDownloadResult{}, errors.New("command_id must be a UUID")
	}
	if err := request.Validate(); err != nil {
		return devicev1.FileDownloadResult{}, err
	}
	rootTemplate := strings.TrimSpace(request.DestinationRoot)
	if rootTemplate == "" {
		rootTemplate = devicev1.DefaultInstallRootTemplate
	}
	root, err := expandInstallRoot(rootTemplate)
	if err != nil {
		return devicev1.FileDownloadResult{}, err
	}
	if err := validateDestinationName(request.DestinationName); err != nil {
		return devicev1.FileDownloadResult{}, err
	}
	root = d.ownership.NamespacedRoot(root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return devicev1.FileDownloadResult{}, fmt.Errorf("create prepared-copy root: %w", err)
	}
	required := request.TotalBytes() + 64*1024*1024
	if free, err := availableDiskBytes(root); err != nil {
		return devicev1.FileDownloadResult{}, fmt.Errorf("check free disk space: %w", err)
	} else if free < required {
		return devicev1.FileDownloadResult{}, errors.New("not enough free space for this download")
	}
	target := filepath.Join(root, request.DestinationName)
	if _, err := os.Lstat(target); err == nil {
		return devicev1.FileDownloadResult{}, fmt.Errorf("destination already exists: %s", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return devicev1.FileDownloadResult{}, fmt.Errorf("inspect destination: %w", err)
	}
	stage := filepath.Join(root, ".mga", "staging", "download-"+commandID)
	if err := os.RemoveAll(stage); err != nil {
		return devicev1.FileDownloadResult{}, fmt.Errorf("clear stale download staging: %w", err)
	}
	defer os.RemoveAll(stage)
	content := filepath.Join(stage, "content")
	if err := os.MkdirAll(content, 0o755); err != nil {
		return devicev1.FileDownloadResult{}, fmt.Errorf("create download staging: %w", err)
	}

	var downloaded uint64
	total := request.TotalBytes()
	for index, file := range request.Files {
		if err := ctx.Err(); err != nil {
			return devicev1.FileDownloadResult{}, err
		}
		relative := filepath.FromSlash(strings.ReplaceAll(file.RelativePath, `\`, "/"))
		destination := filepath.Join(content, relative)
		inside, err := pathWithinRoot(content, destination)
		if err != nil || !inside {
			return devicev1.FileDownloadResult{}, fmt.Errorf("download path escaped staging: %s", file.RelativePath)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return devicev1.FileDownloadResult{}, err
		}
		resolvedURL, err := d.resolveURL(file.DownloadURL)
		if err != nil {
			return devicev1.FileDownloadResult{}, err
		}
		hash, size, err := d.downloadFile(ctx, resolvedURL, file.DownloadToken, destination, file.SizeBytes, func(delta uint64) error {
			downloaded += delta
			percent := uint8(100)
			if total > 0 {
				percent = uint8(min(uint64(100), downloaded*100/total))
			}
			return reportProgress(report, "downloading", fmt.Sprintf("Downloading file %d of %d", index+1, len(request.Files)), uint8(uint16(percent)*85/100), "download", percent)
		})
		if err != nil {
			return devicev1.FileDownloadResult{}, err
		}
		if size != file.SizeBytes || !strings.EqualFold(hash, file.SHA256) {
			return devicev1.FileDownloadResult{}, fmt.Errorf("verification failed for %s", file.RelativePath)
		}
	}
	if err := reportProgress(report, "verifying", "Verifying downloaded files", 92, "verify", 50); err != nil {
		return devicev1.FileDownloadResult{}, err
	}
	preparedAt := d.now().UTC()
	localID := uuid.NewString()
	manifest := map[string]any{
		"schema_version":         devicev1.PreparedCopyManifestSchemaVersion,
		"local_prepared_copy_id": localID, "binding_id": d.ownership.bindingID,
		"game_id": request.GameID, "source_game_id": request.SourceGameID, "title": request.Title,
		"prepared_root": root, "file_count": len(request.Files), "total_bytes": total, "prepared_at": preparedAt,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return devicev1.FileDownloadResult{}, err
	}
	if err := os.WriteFile(filepath.Join(content, preparedCopyManifestName), append(data, '\n'), 0o600); err != nil {
		return devicev1.FileDownloadResult{}, fmt.Errorf("write prepared copy manifest: %w", err)
	}
	if err := os.Rename(content, target); err != nil {
		return devicev1.FileDownloadResult{}, fmt.Errorf("publish prepared copy: %w", err)
	}
	record := PreparedCopyRecord{
		LocalPreparedCopyID: localID, BindingID: d.ownership.bindingID,
		GameID: request.GameID, SourceGameID: request.SourceGameID, Title: request.Title,
		PreparedRoot: root, PreparedPath: target, FileCount: len(request.Files), TotalBytes: total, PreparedAt: preparedAt,
	}
	if err := d.catalog.Add(record); err != nil {
		_ = os.RemoveAll(target)
		return devicev1.FileDownloadResult{}, fmt.Errorf("record prepared copy: %w", err)
	}
	_ = reportProgress(report, "complete", "Downloaded and verified", 100, "prepare", 100)
	return devicev1.FileDownloadResult{
		GameID: request.GameID, SourceGameID: request.SourceGameID,
		PreparedRoot: root, PreparedPath: target, FileCount: len(request.Files),
		TotalBytes: total, PreparedAt: preparedAt,
	}, nil
}

func (d *ManagedFileDownloader) resolveURL(raw string) (string, error) {
	server, _ := url.Parse(d.serverURL)
	download, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("download URL is invalid")
	}
	if !download.IsAbs() {
		download = server.ResolveReference(download)
	}
	if !strings.EqualFold(server.Scheme, download.Scheme) || !strings.EqualFold(server.Host, download.Host) {
		return "", errors.New("download URL must use the paired MGA Server origin")
	}
	return download.String(), nil
}

func (d *ManagedFileDownloader) downloadFile(ctx context.Context, rawURL, token, destination string, expected uint64, progress func(uint64) error) (string, uint64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := d.client.Do(request)
	if err != nil {
		return "", 0, fmt.Errorf("download file: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("download file: MGA Server returned %s", response.Status)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", 0, err
	}
	hasher := sha256.New()
	buffer := make([]byte, 1024*1024)
	var written uint64
	for {
		count, readErr := response.Body.Read(buffer)
		if count > 0 {
			n, writeErr := io.MultiWriter(output, hasher).Write(buffer[:count])
			written += uint64(n)
			if writeErr != nil {
				_ = output.Close()
				return "", written, writeErr
			}
			if written > expected {
				_ = output.Close()
				return "", written, errors.New("download exceeded expected size")
			}
			if err := progress(uint64(n)); err != nil {
				_ = output.Close()
				return "", written, err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			_ = output.Close()
			return "", written, readErr
		}
	}
	if err := output.Close(); err != nil {
		return "", written, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), written, nil
}
