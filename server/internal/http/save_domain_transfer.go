package http

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const saveDomainTransferLifetime = 10 * time.Minute

type saveDomainUpload struct {
	Ref              core.SaveSyncSlotRef
	BaseManifestHash string
	Force            bool
	ExpiresAt        time.Time
}

type saveDomainDownload struct {
	Snapshot  devicev1.SaveDomainSnapshot
	ExpiresAt time.Time
}

type saveDomainTransferRegistry struct {
	mu        sync.Mutex
	uploads   map[string]saveDomainUpload
	downloads map[string]saveDomainDownload
	now       func() time.Time
}

func newSaveDomainTransferRegistry() *saveDomainTransferRegistry {
	return &saveDomainTransferRegistry{uploads: map[string]saveDomainUpload{}, downloads: map[string]saveDomainDownload{}, now: time.Now}
}

func (r *saveDomainTransferRegistry) CreateUpload(upload saveDomainUpload) (string, error) {
	if r == nil || strings.TrimSpace(upload.Ref.CanonicalGameID) == "" || strings.TrimSpace(upload.Ref.IntegrationID) == "" {
		return "", errors.New("save-domain upload registration is invalid")
	}
	token, err := saveDomainTransferToken()
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	upload.ExpiresAt = r.now().Add(saveDomainTransferLifetime)
	r.uploads[token] = upload
	return token, nil
}

func (r *saveDomainTransferRegistry) TakeUpload(token string) (saveDomainUpload, bool) {
	if r == nil {
		return saveDomainUpload{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	upload, ok := r.uploads[strings.TrimSpace(token)]
	if ok {
		delete(r.uploads, strings.TrimSpace(token))
	}
	return upload, ok
}

func (r *saveDomainTransferRegistry) CreateDownload(snapshot devicev1.SaveDomainSnapshot) (string, error) {
	if r == nil {
		return "", errors.New("save-domain download registry is unavailable")
	}
	if err := snapshot.Validate(); err != nil {
		return "", err
	}
	token, err := saveDomainTransferToken()
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	r.downloads[token] = saveDomainDownload{Snapshot: snapshot, ExpiresAt: r.now().Add(saveDomainTransferLifetime)}
	return token, nil
}

func (r *saveDomainTransferRegistry) GetDownload(token string) (devicev1.SaveDomainSnapshot, bool) {
	if r == nil {
		return devicev1.SaveDomainSnapshot{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	download, ok := r.downloads[strings.TrimSpace(token)]
	return download.Snapshot, ok
}

func (r *saveDomainTransferRegistry) pruneLocked() {
	now := r.now()
	for token, upload := range r.uploads {
		if !now.Before(upload.ExpiresAt) {
			delete(r.uploads, token)
		}
	}
	for token, download := range r.downloads {
		if !now.Before(download.ExpiresAt) {
			delete(r.downloads, token)
		}
	}
}

func saveDomainTransferToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (c *DeviceController) ServeSaveDomainTransfer(w http.ResponseWriter, r *http.Request) {
	if c.saveDomainTransfers == nil || c.saveSync == nil {
		http.Error(w, "save transfer is unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodPut:
		c.receiveSaveDomainSnapshot(w, r)
	case http.MethodGet:
		c.sendSaveDomainSnapshot(w, r)
	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (c *DeviceController) receiveSaveDomainSnapshot(w http.ResponseWriter, r *http.Request) {
	upload, ok := c.saveDomainTransfers.TakeUpload(saveDomainBearerToken(r))
	if !ok {
		http.Error(w, "save upload token is invalid or expired", http.StatusUnauthorized)
		return
	}
	maxBody := int64(base64.StdEncoding.EncodedLen(int(devicev1.SaveDomainMaxBytes+(1<<20)))) + (4 << 20)
	reader := http.MaxBytesReader(w, r.Body, maxBody)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var snapshot devicev1.SaveDomainSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		http.Error(w, "invalid bounded save snapshot", http.StatusBadRequest)
		return
	}
	if err := snapshot.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	coreFiles := make([]core.SaveSyncSnapshotFile, len(snapshot.Files))
	for index, file := range snapshot.Files {
		coreFiles[index] = core.SaveSyncSnapshotFile{Path: file.Path, Size: file.Size, Hash: file.Hash}
	}
	result, err := c.saveSync.PutSlot(context.WithoutCancel(r.Context()), core.SaveSyncPutRequest{
		SaveSyncSlotRef: upload.Ref, BaseManifestHash: upload.BaseManifestHash, Force: upload.Force,
		Snapshot: core.SaveSyncSnapshot{
			CanonicalGameID: upload.Ref.CanonicalGameID, SourceGameID: upload.Ref.SourceGameID, Runtime: upload.Ref.Runtime,
			SlotID: upload.Ref.SlotID, TotalSize: snapshot.TotalSize, FileCount: len(coreFiles), Files: coreFiles, ArchiveBase64: snapshot.ArchiveBase64,
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	response := devicev1.SaveDomainUploadResponse{Stored: result.OK}
	status := http.StatusOK
	if result.Conflict != nil {
		status = http.StatusConflict
		response.Conflict = &devicev1.SaveDomainTransferConflict{ManifestHash: result.Conflict.RemoteManifestHash, UpdatedAt: result.Conflict.RemoteUpdatedAt, FileCount: result.Conflict.RemoteFileCount, TotalSize: result.Conflict.RemoteTotalSize}
	} else {
		response.ManifestHash = result.Summary.ManifestHash
	}
	if err := response.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, status, response)
}

func (c *DeviceController) sendSaveDomainSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, ok := c.saveDomainTransfers.GetDownload(saveDomainBearerToken(r))
	if !ok {
		http.Error(w, "save download token is invalid or expired", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func protocolSaveDomainSnapshot(snapshot *core.SaveSyncSnapshot) (devicev1.SaveDomainSnapshot, error) {
	if snapshot == nil {
		return devicev1.SaveDomainSnapshot{}, errors.New("save snapshot is required")
	}
	files := make([]devicev1.SaveDomainSnapshotFile, len(snapshot.Files))
	for index, file := range snapshot.Files {
		files[index] = devicev1.SaveDomainSnapshotFile{Path: file.Path, Size: file.Size, Hash: file.Hash}
	}
	result := devicev1.SaveDomainSnapshot{LocalFingerprint: serverSaveDomainFingerprint(files), CapturedAt: snapshot.UpdatedAt, TotalSize: snapshot.TotalSize, Files: files, ArchiveBase64: snapshot.ArchiveBase64}
	return result, result.Validate()
}

func serverSaveDomainFingerprint(files []devicev1.SaveDomainSnapshotFile) string {
	hasher := sha256.New()
	for _, file := range files {
		_, _ = io.WriteString(hasher, strings.ToLower(file.Path))
		_, _ = io.WriteString(hasher, "\x00"+strconv.FormatInt(file.Size, 10)+"\x00"+strings.ToLower(file.Hash)+"\x00")
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func saveDomainBearerToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	scheme, token, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
