package http

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestSaveDomainUploadTokenIsBoundedAndSingleUse(t *testing.T) {
	registry := newSaveDomainTransferRegistry()
	token, err := registry.CreateUpload(saveDomainUpload{Ref: core.SaveSyncSlotRef{CanonicalGameID: "game", SourceGameID: "source", Runtime: "scummvm", SlotID: "autosave", IntegrationID: "sync"}})
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	controller := &DeviceController{saveDomainTransfers: registry, saveSync: &fakeSaveSyncService{
		listSlots: func(context.Context, core.SaveSyncListRequest) ([]core.SaveSyncSlotSummary, error) { return nil, nil },
		getSlot:   func(context.Context, core.SaveSyncSlotRef) (*core.SaveSyncSnapshot, error) { return nil, nil },
		putSlot: func(_ context.Context, request core.SaveSyncPutRequest) (*core.SaveSyncPutResult, error) {
			called++
			if request.CanonicalGameID != "game" || request.SourceGameID != "source" || request.Runtime != "scummvm" {
				t.Fatalf("put request = %+v", request)
			}
			return &core.SaveSyncPutResult{OK: true, Summary: core.SaveSyncSlotSummary{ManifestHash: strings.Repeat("b", 64)}}, nil
		},
	}}
	snapshot := emptyDeviceSaveSnapshot(t)
	body, _ := json.Marshal(snapshot)
	for attempt := 0; attempt < 2; attempt++ {
		req := httptest.NewRequest(http.MethodPut, "/api/device-transfers/save-domain", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		controller.ServeSaveDomainTransfer(recorder, req)
		if attempt == 0 && recorder.Code != http.StatusOK {
			t.Fatalf("first upload status = %d, body %s", recorder.Code, recorder.Body.String())
		}
		if attempt == 1 && recorder.Code != http.StatusUnauthorized {
			t.Fatalf("reused upload status = %d", recorder.Code)
		}
	}
	if called != 1 {
		t.Fatalf("put calls = %d", called)
	}
}

func TestSaveDomainDownloadTokenExpires(t *testing.T) {
	registry := newSaveDomainTransferRegistry()
	now := time.Date(2026, 7, 18, 14, 0, 0, 0, time.UTC)
	registry.now = func() time.Time { return now }
	token, err := registry.CreateDownload(emptyDeviceSaveSnapshot(t))
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(saveDomainTransferLifetime)
	if _, ok := registry.GetDownload(token); ok {
		t.Fatal("expired save download token remained available")
	}
}

func TestSaveDomainTransferRejectsTokenInURL(t *testing.T) {
	registry := newSaveDomainTransferRegistry()
	token, err := registry.CreateDownload(emptyDeviceSaveSnapshot(t))
	if err != nil {
		t.Fatal(err)
	}
	controller := &DeviceController{saveDomainTransfers: registry, saveSync: &fakeSaveSyncService{}}
	request := httptest.NewRequest(http.MethodGet, "/api/device-transfers/save-domain?token="+token, nil)
	recorder := httptest.NewRecorder()
	controller.ServeSaveDomainTransfer(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("query token status = %d", recorder.Code)
	}
}

func emptyDeviceSaveSnapshot(t *testing.T) devicev1.SaveDomainSnapshot {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return devicev1.SaveDomainSnapshot{LocalFingerprint: strings.Repeat("a", 64), CapturedAt: time.Now().UTC(), Files: []devicev1.SaveDomainSnapshotFile{}, ArchiveBase64: base64.StdEncoding.EncodeToString(archive.Bytes())}
}
