package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

type fakeSaveSyncService struct {
	listSlots func(context.Context, core.SaveSyncListRequest) ([]core.SaveSyncSlotSummary, error)
	getSlot   func(context.Context, core.SaveSyncSlotRef) (*core.SaveSyncSnapshot, error)
	putSlot   func(context.Context, core.SaveSyncPutRequest) (*core.SaveSyncPutResult, error)
}

func (f *fakeSaveSyncService) ListSlots(ctx context.Context, req core.SaveSyncListRequest) ([]core.SaveSyncSlotSummary, error) {
	return f.listSlots(ctx, req)
}

func (f *fakeSaveSyncService) GetSlot(ctx context.Context, req core.SaveSyncSlotRef) (*core.SaveSyncSnapshot, error) {
	return f.getSlot(ctx, req)
}

func (f *fakeSaveSyncService) PutSlot(ctx context.Context, req core.SaveSyncPutRequest) (*core.SaveSyncPutResult, error) {
	return f.putSlot(ctx, req)
}

func (f *fakeSaveSyncService) StartMigration(context.Context, core.SaveSyncMigrationRequest) (*core.SaveSyncMigrationStatus, error) {
	panic("unexpected call")
}

func (f *fakeSaveSyncService) GetMigrationStatus(context.Context, string) (*core.SaveSyncMigrationStatus, error) {
	panic("unexpected call")
}

func TestCanonicalToGameDetailRejectsUnknownRootlessScummVMLaunch(t *testing.T) {
	game := &core.CanonicalGame{
		ID:       "game-3",
		Title:    "Mystery Folder",
		Platform: core.PlatformScummVM,
		Kind:     core.GameKindBaseGame,
		SourceGames: []*core.SourceGame{
			{
				ID:        "source-3",
				Platform:  core.PlatformScummVM,
				GroupKind: core.GroupKindSelfContained,
				Status:    "found",
				CreatedAt: time.Unix(1700000000, 0),
				Files: []core.GameFile{
					{GameID: "source-3", Path: "README.TXT", Role: core.GameFileRoleRequired, Size: 128},
					{GameID: "source-3", Path: "notes.doc", Role: core.GameFileRoleRequired, Size: 256},
				},
			},
		},
	}

	detail := canonicalToGameDetail(game)
	if detail.Play == nil {
		t.Fatal("expected play metadata")
	}
	if detail.Play.Available {
		t.Fatalf("expected unknown rootless scummvm source to stay unlaunchable, got %+v", detail.Play)
	}
	if detail.SourceGames[0].Play == nil || detail.SourceGames[0].Play.Launchable {
		t.Fatalf("expected source play metadata to remain false, got %+v", detail.SourceGames[0].Play)
	}
}

func TestSaveSyncControllerListSlots(t *testing.T) {
	controller := NewSaveSyncController(&fakeSaveSyncService{
		listSlots: func(_ context.Context, req core.SaveSyncListRequest) ([]core.SaveSyncSlotSummary, error) {
			if req.CanonicalGameID != "game-1" || req.SourceGameID != "source-1" || req.Runtime != "scummvm" || req.IntegrationID != "integration-1" {
				t.Fatalf("unexpected request: %+v", req)
			}
			return []core.SaveSyncSlotSummary{{SlotID: "autosave", Exists: true, ManifestHash: "abc"}}, nil
		},
		getSlot: func(context.Context, core.SaveSyncSlotRef) (*core.SaveSyncSnapshot, error) { return nil, nil },
		putSlot: func(context.Context, core.SaveSyncPutRequest) (*core.SaveSyncPutResult, error) { return nil, nil },
	}, noopLogger{})

	router := chi.NewRouter()
	router.Get("/api/games/{id}/save-sync/slots", controller.ListSlots)

	req := httptest.NewRequest(http.MethodGet, "/api/games/game-1/save-sync/slots?integration_id=integration-1&source_game_id=source-1&runtime=scummvm", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Slots []core.SaveSyncSlotSummary `json:"slots"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Slots) != 1 || !body.Slots[0].Exists {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestSaveSyncControllerPutSlotReturnsConflict(t *testing.T) {
	controller := NewSaveSyncController(&fakeSaveSyncService{
		listSlots: func(context.Context, core.SaveSyncListRequest) ([]core.SaveSyncSlotSummary, error) { return nil, nil },
		getSlot: func(context.Context, core.SaveSyncSlotRef) (*core.SaveSyncSnapshot, error) { return nil, nil },
		putSlot: func(_ context.Context, req core.SaveSyncPutRequest) (*core.SaveSyncPutResult, error) {
			if req.SaveSyncSlotRef.SlotID != "slot-1" || req.Runtime != "jsdos" || req.SourceGameID != "source-2" {
				t.Fatalf("unexpected put request: %+v", req)
			}
			return &core.SaveSyncPutResult{
				OK: false,
				Summary: core.SaveSyncSlotSummary{SlotID: "slot-1", Exists: true},
				Conflict: &core.SaveSyncConflict{
					SlotID:             "slot-1",
					Message:            "remote changed",
					RemoteManifestHash: "remote-hash",
					RemoteUpdatedAt:    "2026-03-28T12:00:00Z",
				},
			}, nil
		},
	}, noopLogger{})

	router := chi.NewRouter()
	router.Put("/api/games/{id}/save-sync/slots/{slot_id}", controller.PutSlot)

	payload := map[string]any{
		"integration_id":     "integration-1",
		"source_game_id":     "source-2",
		"runtime":            "jsdos",
		"base_manifest_hash": "local-hash",
		"snapshot": map[string]any{
			"canonical_game_id": "game-2",
			"source_game_id":    "source-2",
			"runtime":           "jsdos",
			"slot_id":           "slot-1",
			"files":             []map[string]any{},
			"archive_base64":    "UEsFBgAAAAAAAAAAAAAAAAAAAAAAAA==",
		},
	}
	data, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/games/game-2/save-sync/slots/slot-1", bytes.NewReader(data))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 body=%s", rec.Code, rec.Body.String())
	}
}
