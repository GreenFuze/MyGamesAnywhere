package clientapp

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/google/uuid"
)

var ErrSaveDomainLocalConflict = errors.New("local save files changed since the last MGA snapshot")

func (m *LocalSaveDomainManager) Snapshot(ctx context.Context, request devicev1.SaveDomainSnapshotRequest) (devicev1.SaveDomainSnapshotResult, error) {
	var result devicev1.SaveDomainSnapshotResult
	if err := m.validateTransferManager(); err != nil {
		return result, err
	}
	if err := request.Validate(); err != nil {
		return result, err
	}
	domain, release, err := m.beginOwnedSaveOperation(request.LocalSaveDomainID)
	if err != nil {
		return result, err
	}
	defer release()
	snapshot, err := captureSaveDomain(domain.ResolvedPaths[0], m.now().UTC())
	if err != nil {
		return result, err
	}
	response, err := m.uploadSnapshot(ctx, request.UploadURL, request.UploadToken, snapshot)
	if err != nil {
		return result, err
	}
	result = devicev1.SaveDomainSnapshotResult{
		GameID: request.GameID, SourceGameID: request.SourceGameID, LocalSaveDomainID: request.LocalSaveDomainID,
		LocalFingerprint: snapshot.LocalFingerprint, CompletedAt: m.now().UTC(),
	}
	if response.Stored {
		result.State = "stored"
		result.ManifestHash = response.ManifestHash
		if err := m.catalog.RecordSnapshot(domain.LocalSaveDomainID, m.bindingID, snapshot.LocalFingerprint); err != nil {
			return devicev1.SaveDomainSnapshotResult{}, err
		}
	} else {
		result.State = "conflict"
		result.Conflict = response.Conflict
	}
	return result, result.Validate()
}

func (m *LocalSaveDomainManager) Restore(ctx context.Context, request devicev1.SaveDomainRestoreRequest) (devicev1.SaveDomainRestoreResult, error) {
	var result devicev1.SaveDomainRestoreResult
	if err := m.validateTransferManager(); err != nil {
		return result, err
	}
	if err := request.Validate(); err != nil {
		return result, err
	}
	domain, release, err := m.beginOwnedSaveOperation(request.LocalSaveDomainID)
	if err != nil {
		return result, err
	}
	defer release()
	incoming, err := m.downloadSnapshot(ctx, request.DownloadURL, request.DownloadToken)
	if err != nil {
		return result, err
	}
	current, err := captureSaveDomain(domain.ResolvedPaths[0], m.now().UTC())
	if err != nil {
		return result, err
	}
	changed := len(current.Files) > 0 && (domain.LastSnapshotFingerprint == "" || !strings.EqualFold(current.LocalFingerprint, domain.LastSnapshotFingerprint))
	if changed && !request.PreserveLocal {
		return result, ErrSaveDomainLocalConflict
	}
	if changed {
		approved, err := m.confirmer.ConfirmRestore(ctx, request.Title, m.serverURL)
		if err != nil {
			return result, err
		}
		if !approved {
			return result, ErrSaveDomainConfirmationDeclined
		}
	}
	backupCreated, err := restoreSaveDomain(domain.ResolvedPaths[0], incoming, changed, m.now().UTC())
	if err != nil {
		return result, err
	}
	if err := m.catalog.RecordSnapshot(domain.LocalSaveDomainID, m.bindingID, incoming.LocalFingerprint); err != nil {
		return result, err
	}
	result = devicev1.SaveDomainRestoreResult{
		GameID: request.GameID, SourceGameID: request.SourceGameID, LocalSaveDomainID: request.LocalSaveDomainID,
		LocalFingerprint: incoming.LocalFingerprint, ManifestHash: strings.ToLower(request.ManifestHash),
		BackupPathCreated: backupCreated, RestoredAt: m.now().UTC(),
	}
	return result, result.Validate()
}

func (m *LocalSaveDomainManager) Reconcile(ctx context.Context, request devicev1.SaveDomainReconcileRequest) (devicev1.SaveDomainReconcileResult, error) {
	var result devicev1.SaveDomainReconcileResult
	if err := m.validateTransferManager(); err != nil {
		return result, err
	}
	if err := request.Validate(); err != nil {
		return result, err
	}
	domain, found := m.catalog.FindByID(request.LocalSaveDomainID)
	if !found || domain.State != SaveDomainReconciliationRequired || !strings.EqualFold(domain.PendingWriterBindingID, m.bindingID) || len(domain.ResolvedPaths) != 1 {
		return result, errors.New("this MGA Server does not own the pending save reconciliation")
	}
	release, err := m.coordinator.Reserve(m.bindingID, domain.ResolvedPaths[0], "save-domain:"+domain.LocalSaveDomainID)
	if err != nil {
		return result, err
	}
	defer release()
	result = devicev1.SaveDomainReconcileResult{GameID: request.GameID, SourceGameID: request.SourceGameID, LocalSaveDomainID: request.LocalSaveDomainID, Strategy: request.Strategy, State: "owned_here", CompletedAt: m.now().UTC()}
	if request.Strategy == "keep_local" {
		snapshot, err := captureSaveDomain(domain.ResolvedPaths[0], m.now().UTC())
		if err != nil {
			return devicev1.SaveDomainReconcileResult{}, err
		}
		upload, err := m.uploadSnapshot(ctx, request.TransferURL, request.TransferToken, snapshot)
		if err != nil {
			return devicev1.SaveDomainReconcileResult{}, err
		}
		if !upload.Stored {
			return devicev1.SaveDomainReconcileResult{}, errors.New("forced reconciliation upload was not stored")
		}
		result.LocalFingerprint, result.ManifestHash = snapshot.LocalFingerprint, upload.ManifestHash
	} else {
		approved, err := m.confirmer.ConfirmRestore(ctx, request.Title, m.serverURL)
		if err != nil {
			return devicev1.SaveDomainReconcileResult{}, err
		}
		if !approved {
			return devicev1.SaveDomainReconcileResult{}, ErrSaveDomainConfirmationDeclined
		}
		snapshot, err := m.downloadSnapshot(ctx, request.TransferURL, request.TransferToken)
		if err != nil {
			return devicev1.SaveDomainReconcileResult{}, err
		}
		backupCreated, err := restoreSaveDomain(domain.ResolvedPaths[0], snapshot, true, m.now().UTC())
		if err != nil {
			return devicev1.SaveDomainReconcileResult{}, err
		}
		result.LocalFingerprint, result.ManifestHash, result.BackupPathCreated = snapshot.LocalFingerprint, strings.ToLower(request.ManifestHash), backupCreated
	}
	if err := m.catalog.CompleteReconciliation(domain.LocalSaveDomainID, m.bindingID, result.LocalFingerprint); err != nil {
		return devicev1.SaveDomainReconcileResult{}, err
	}
	return result, result.Validate()
}

func (m *LocalSaveDomainManager) validateTransferManager() error {
	if m == nil || m.catalog == nil || m.client == nil || m.coordinator == nil || m.confirmer == nil || m.now == nil {
		return errors.New("save domain transfer manager is unavailable")
	}
	return nil
}

func (m *LocalSaveDomainManager) beginOwnedSaveOperation(localID string) (LocalSaveDomainRecord, func(), error) {
	domain, found := m.catalog.FindByID(localID)
	if !found {
		return LocalSaveDomainRecord{}, nil, errors.New("save domain record not found")
	}
	if domain.State != SaveDomainOwned || !strings.EqualFold(domain.WriterBindingID, m.bindingID) || len(domain.ResolvedPaths) != 1 {
		return LocalSaveDomainRecord{}, nil, errors.New("this MGA Server is not the save writer")
	}
	release, err := m.coordinator.Reserve(m.bindingID, domain.ResolvedPaths[0], "save-domain:"+domain.LocalSaveDomainID)
	if err != nil {
		return LocalSaveDomainRecord{}, nil, err
	}
	return domain, release, nil
}

func (m *LocalSaveDomainManager) uploadSnapshot(ctx context.Context, transferPath, token string, snapshot devicev1.SaveDomainSnapshot) (devicev1.SaveDomainUploadResponse, error) {
	var response devicev1.SaveDomainUploadResponse
	body, err := json.Marshal(snapshot)
	if err != nil {
		return response, err
	}
	endpoint, err := boundedSaveTransferURL(m.serverURL, transferPath)
	if err != nil {
		return response, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return response, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	res, err := m.client.Do(req)
	if err != nil {
		return response, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusConflict {
		message, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return response, fmt.Errorf("save upload failed: %s", strings.TrimSpace(string(message)))
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&response); err != nil {
		return response, err
	}
	return response, response.Validate()
}

func (m *LocalSaveDomainManager) downloadSnapshot(ctx context.Context, transferPath, token string) (devicev1.SaveDomainSnapshot, error) {
	var snapshot devicev1.SaveDomainSnapshot
	endpoint, err := boundedSaveTransferURL(m.serverURL, transferPath)
	if err != nil {
		return snapshot, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return snapshot, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	res, err := m.client.Do(req)
	if err != nil {
		return snapshot, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return snapshot, fmt.Errorf("save download failed: %s", strings.TrimSpace(string(message)))
	}
	limit := base64.StdEncoding.EncodedLen(int(devicev1.SaveDomainMaxBytes+(1<<20))) + (4 << 20)
	if err := json.NewDecoder(io.LimitReader(res.Body, int64(limit))).Decode(&snapshot); err != nil {
		return snapshot, err
	}
	return snapshot, snapshot.Validate()
}

func boundedSaveTransferURL(serverURL, transferPath string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return "", errors.New("valid MGA Server URL is required")
	}
	relative, err := url.Parse(transferPath)
	if err != nil || relative.IsAbs() || relative.Host != "" || relative.Path != "/api/device-transfers/save-domain" || relative.RawQuery != "" || relative.Fragment != "" {
		return "", errors.New("invalid save-domain transfer path")
	}
	return base.ResolveReference(relative).String(), nil
}

func captureSaveDomain(root string, capturedAt time.Time) (devicev1.SaveDomainSnapshot, error) {
	root = filepath.Clean(root)
	type candidate struct{ absolute, relative string }
	files := make([]candidate, 0)
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeDevice != 0 || info.Mode()&os.ModeNamedPipe != 0 || info.Mode()&os.ModeSocket != 0 {
			return fmt.Errorf("save domain contains unsupported linked or special entry %q", current)
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("save domain contains non-regular file %q", current)
		}
		relative, err := filepath.Rel(root, current)
		if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("save file escaped its domain")
		}
		files = append(files, candidate{absolute: current, relative: filepath.ToSlash(relative)})
		if len(files) > devicev1.SaveDomainMaxFiles {
			return errors.New("save domain contains too many files")
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return devicev1.SaveDomainSnapshot{}, err
		}
		err = nil
	}
	if err != nil {
		return devicev1.SaveDomainSnapshot{}, err
	}
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].relative) < strings.ToLower(files[j].relative) })
	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	metadata := make([]devicev1.SaveDomainSnapshotFile, 0, len(files))
	var total int64
	for _, candidate := range files {
		data, err := os.ReadFile(candidate.absolute)
		if err != nil {
			_ = zipWriter.Close()
			return devicev1.SaveDomainSnapshot{}, err
		}
		total += int64(len(data))
		if total > devicev1.SaveDomainMaxBytes {
			_ = zipWriter.Close()
			return devicev1.SaveDomainSnapshot{}, errors.New("save domain is larger than 64 MiB")
		}
		hash := sha256.Sum256(data)
		metadata = append(metadata, devicev1.SaveDomainSnapshotFile{Path: candidate.relative, Size: int64(len(data)), Hash: hex.EncodeToString(hash[:])})
		header := &zip.FileHeader{Name: candidate.relative, Method: zip.Deflate}
		header.SetMode(0o600)
		header.Modified = time.Unix(0, 0).UTC()
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			_ = zipWriter.Close()
			return devicev1.SaveDomainSnapshot{}, err
		}
		if _, err := writer.Write(data); err != nil {
			_ = zipWriter.Close()
			return devicev1.SaveDomainSnapshot{}, err
		}
	}
	if err := zipWriter.Close(); err != nil {
		return devicev1.SaveDomainSnapshot{}, err
	}
	snapshot := devicev1.SaveDomainSnapshot{LocalFingerprint: saveDomainFingerprint(metadata), CapturedAt: capturedAt, TotalSize: total, Files: metadata, ArchiveBase64: base64.StdEncoding.EncodeToString(archive.Bytes())}
	return snapshot, snapshot.Validate()
}

func saveDomainFingerprint(files []devicev1.SaveDomainSnapshotFile) string {
	hasher := sha256.New()
	for _, file := range files {
		_, _ = io.WriteString(hasher, strings.ToLower(file.Path))
		_, _ = io.WriteString(hasher, "\x00"+strconv.FormatInt(file.Size, 10)+"\x00"+strings.ToLower(file.Hash)+"\x00")
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func restoreSaveDomain(root string, snapshot devicev1.SaveDomainSnapshot, preserveCurrent bool, now time.Time) (bool, error) {
	archiveBytes, _ := base64.StdEncoding.DecodeString(snapshot.ArchiveBase64)
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return false, err
	}
	staging := root + ".mga-restore-" + uuid.NewString()
	if err := os.Mkdir(staging, 0o700); err != nil {
		return false, err
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = os.RemoveAll(staging)
		}
	}()
	if err := extractSaveSnapshot(staging, archiveBytes, snapshot.Files); err != nil {
		return false, err
	}
	staged, err := captureSaveDomain(staging, now)
	if err != nil || !strings.EqualFold(staged.LocalFingerprint, snapshot.LocalFingerprint) {
		return false, errors.New("restored save snapshot fingerprint does not match its manifest")
	}
	backup := ""
	if info, err := os.Stat(root); err == nil && info.IsDir() {
		if preserveCurrent {
			backup = root + ".mga-backup-" + now.UTC().Format("20060102-150405")
		} else {
			backup = root + ".mga-rollback-" + uuid.NewString()
		}
		if err := os.Rename(root, backup); err != nil {
			return false, fmt.Errorf("stage current save folder: %w", err)
		}
	}
	if err := os.Rename(staging, root); err != nil {
		if backup != "" {
			_ = os.Rename(backup, root)
		}
		return false, fmt.Errorf("activate restored save folder: %w", err)
	}
	cleanupStaging = false
	if backup != "" && !preserveCurrent {
		if err := os.RemoveAll(backup); err != nil {
			return false, fmt.Errorf("remove restore rollback folder: %w", err)
		}
	}
	return preserveCurrent && backup != "", nil
}

func extractSaveSnapshot(root string, archive []byte, files []devicev1.SaveDomainSnapshotFile) error {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return err
	}
	expected := make(map[string]devicev1.SaveDomainSnapshotFile, len(files))
	for _, file := range files {
		expected[strings.ToLower(file.Path)] = file
	}
	seen := map[string]bool{}
	for _, item := range reader.File {
		if item.FileInfo().IsDir() {
			continue
		}
		name := filepath.ToSlash(filepath.Clean(item.Name))
		metadata, ok := expected[strings.ToLower(name)]
		if !ok || seen[strings.ToLower(name)] || name != item.Name || strings.HasPrefix(name, "../") || filepath.IsAbs(name) {
			return fmt.Errorf("snapshot contains unexpected path %q", item.Name)
		}
		seen[strings.ToLower(name)] = true
		destination := filepath.Join(root, filepath.FromSlash(name))
		inside, err := pathWithinRoot(root, destination)
		if err != nil || !inside {
			return errors.New("snapshot file escaped restore staging")
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return err
		}
		input, err := item.Open()
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(input, metadata.Size+1))
		closeErr := input.Close()
		if readErr != nil || closeErr != nil || int64(len(data)) != metadata.Size {
			return errors.New("snapshot file size does not match its manifest")
		}
		hash := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(hash[:]), metadata.Hash) {
			return errors.New("snapshot file hash does not match its manifest")
		}
		if err := os.WriteFile(destination, data, 0o600); err != nil {
			return err
		}
	}
	if len(seen) != len(expected) {
		return errors.New("snapshot archive is missing manifest files")
	}
	return nil
}
