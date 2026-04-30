package save_sync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	dbpkg "github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
)

func TestValidateSlotRefAgainstGameAllowsCanonicalPlatformFallback(t *testing.T) {
	game := &core.CanonicalGame{
		ID:       "game-1",
		Title:    "Advance Wars",
		Platform: core.PlatformGBA,
		SourceGames: []*core.SourceGame{
			{
				ID:        "source-1",
				Platform:  core.PlatformUnknown,
				Status:    "found",
				CreatedAt: time.Unix(1700000000, 0),
			},
		},
	}

	err := validateSlotRefAgainstGame(game, core.SaveSyncSlotRef{
		CanonicalGameID: "game-1",
		SourceGameID:    "source-1",
		Runtime:         "emulatorjs",
		IntegrationID:   "integration-1",
		SlotID:          "autosave",
	})
	if err != nil {
		t.Fatalf("expected canonical platform fallback to allow runtime, got %v", err)
	}
}

func TestValidateSlotRefAgainstGameAllowsKnownSourceThatIsNotCurrentlyFound(t *testing.T) {
	game := &core.CanonicalGame{
		ID:       "game-1b",
		Title:    "Advance Wars",
		Platform: core.PlatformGBA,
		SourceGames: []*core.SourceGame{
			{
				ID:        "source-1b",
				Platform:  core.PlatformGBA,
				Status:    "missing",
				CreatedAt: time.Unix(1700000000, 0),
			},
		},
	}

	err := validateSlotRefAgainstGame(game, core.SaveSyncSlotRef{
		CanonicalGameID: "game-1b",
		SourceGameID:    "source-1b",
		Runtime:         "emulatorjs",
		IntegrationID:   "integration-1",
		SlotID:          "autosave",
	})
	if err != nil {
		t.Fatalf("expected known source game to allow runtime even when not currently found, got %v", err)
	}
}

func TestValidateSlotRefAgainstGameRejectsMismatchedEffectiveRuntime(t *testing.T) {
	game := &core.CanonicalGame{
		ID:       "game-2",
		Title:    "Monkey Island",
		Platform: core.PlatformScummVM,
		SourceGames: []*core.SourceGame{
			{
				ID:        "source-2",
				Platform:  core.PlatformUnknown,
				Status:    "found",
				CreatedAt: time.Unix(1700000000, 0),
			},
		},
	}

	err := validateSlotRefAgainstGame(game, core.SaveSyncSlotRef{
		CanonicalGameID: "game-2",
		SourceGameID:    "source-2",
		Runtime:         "emulatorjs",
		IntegrationID:   "integration-1",
		SlotID:          "autosave",
	})
	if err == nil || err.Error() != "runtime does not match source game platform" {
		t.Fatalf("expected runtime mismatch, got %v", err)
	}
}

func TestValidateSlotRefAgainstGameRejectsForeignSourceGame(t *testing.T) {
	game := &core.CanonicalGame{
		ID:       "game-3",
		Title:    "Doom",
		Platform: core.PlatformMSDOS,
		SourceGames: []*core.SourceGame{
			{
				ID:        "source-3",
				Platform:  core.PlatformMSDOS,
				Status:    "found",
				CreatedAt: time.Unix(1700000000, 0),
			},
		},
	}

	err := validateSlotRefAgainstGame(game, core.SaveSyncSlotRef{
		CanonicalGameID: "game-3",
		SourceGameID:    "source-missing",
		Runtime:         "jsdos",
		IntegrationID:   "integration-1",
		SlotID:          "autosave",
	})
	if err == nil || err.Error() != "source game does not belong to canonical game" {
		t.Fatalf("expected foreign source rejection, got %v", err)
	}
}

func TestKnownSlotIDsIncludeLegacyAndEmulatorJSNativeSlots(t *testing.T) {
	for _, slotID := range []string{
		"autosave",
		"slot-1",
		"slot-5",
		"state-1",
		"state-9",
		"save-ram",
	} {
		if !isKnownSlotID(slotID) {
			t.Fatalf("expected slot %q to be accepted", slotID)
		}
	}

	for _, slotID := range []string{"", "state-0", "state-10", "slot-6", "save-state"} {
		if isKnownSlotID(slotID) {
			t.Fatalf("expected slot %q to be rejected", slotID)
		}
	}
}

func TestPutSlotWritesLocalCacheBeforeRemoteUploadCompletes(t *testing.T) {
	ctx := context.Background()
	svc, host, gameID := newTestService(t)
	host.blockPut = make(chan struct{})

	ref := core.SaveSyncSlotRef{
		CanonicalGameID: gameID,
		SourceGameID:    "source-1",
		Runtime:         "emulatorjs",
		SlotID:          "state-1",
		IntegrationID:   "integration-1",
	}
	result, err := svc.PutSlot(ctx, core.SaveSyncPutRequest{
		SaveSyncSlotRef: ref,
		Force:           true,
		Snapshot:        testSnapshot(ref, []core.SaveSyncSnapshotFile{}),
	})
	if err != nil {
		t.Fatalf("put slot: %v", err)
	}
	if !result.OK || result.Summary.SyncState != "uploading" || !result.Summary.UploadPending {
		t.Fatalf("put result = %+v, want uploading local cache", result)
	}

	snapshot, err := svc.GetSlot(ctx, ref)
	if err != nil {
		t.Fatalf("get cached slot: %v", err)
	}
	if snapshot == nil || snapshot.SlotID != "state-1" {
		t.Fatalf("snapshot = %+v, want cached state-1", snapshot)
	}

	close(host.blockPut)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		summary, err := svc.readSlotSummaryFromCache(ref)
		if err != nil {
			t.Fatal(err)
		}
		if summary.SyncState == "synced" {
			return
		}
		if summary.SyncState == "failed" {
			t.Fatalf("upload failed: %+v", summary)
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("slot did not become synced after remote upload unblocked")
}

func TestPrefetchCachesRemoteSlotsAndMarksMissingSlots(t *testing.T) {
	ctx := context.Background()
	svc, host, gameID := newTestService(t)

	ref := core.SaveSyncSlotRef{
		CanonicalGameID: gameID,
		SourceGameID:    "source-1",
		Runtime:         "emulatorjs",
		SlotID:          "state-1",
		IntegrationID:   "integration-1",
	}
	manifest := saveSyncStoredManifest{
		Version:         1,
		CanonicalGameID: ref.CanonicalGameID,
		SourceGameID:    ref.SourceGameID,
		Runtime:         ref.Runtime,
		SlotID:          ref.SlotID,
		UpdatedAt:       time.Unix(1700000000, 0).UTC(),
		Files:           []core.SaveSyncSnapshotFile{},
	}
	manifestBytes, _, err := marshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	host.putRemote(slotManifestPath(ref), manifestBytes)
	host.putRemote(slotArchivePath(ref), emptyZipBytes(t))
	staleRef := ref
	staleRef.SlotID = "state-2"
	if err := svc.writeSlotCache(staleRef, manifestBytes, emptyZipBytes(t), saveSyncCacheStatus{
		SyncState: "synced",
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	status, err := svc.StartPrefetch(ctx, core.SaveSyncPrefetchRequest{
		CanonicalGameID: ref.CanonicalGameID,
		SourceGameID:    ref.SourceGameID,
		Runtime:         ref.Runtime,
		IntegrationID:   ref.IntegrationID,
	})
	if err != nil {
		t.Fatalf("start prefetch: %v", err)
	}

	status = waitPrefetch(t, svc, status.JobID)
	if status.Status != "completed" {
		t.Fatalf("prefetch status = %+v, want completed", status)
	}
	if status.SlotsCached != 1 || status.SlotsMissing != len(emulatorJSSlotIDs())-1 {
		t.Fatalf("prefetch counts = %+v", status)
	}

	snapshot, err := svc.GetSlot(ctx, ref)
	if err != nil {
		t.Fatalf("get prefetched slot: %v", err)
	}
	if snapshot == nil || snapshot.ManifestHash == "" {
		t.Fatalf("snapshot = %+v, want cached remote slot", snapshot)
	}

	missingSummary, err := svc.readSlotSummaryFromCache(core.SaveSyncSlotRef{
		CanonicalGameID: ref.CanonicalGameID,
		SourceGameID:    ref.SourceGameID,
		Runtime:         ref.Runtime,
		SlotID:          "state-2",
		IntegrationID:   ref.IntegrationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if missingSummary.Exists || missingSummary.SyncState != "missing" || !missingSummary.Cached {
		t.Fatalf("missing summary = %+v, want cached missing", missingSummary)
	}
	staleSnapshot, err := svc.GetSlot(ctx, staleRef)
	if err != nil {
		t.Fatal(err)
	}
	if staleSnapshot != nil {
		t.Fatalf("stale snapshot = %+v, want nil after missing prefetch", staleSnapshot)
	}
}

func TestPrefetchDownloadsRemoteSlotsConcurrently(t *testing.T) {
	ctx := context.Background()
	svc, host, gameID := newTestService(t)
	host.getDelay = 100 * time.Millisecond

	for _, slotID := range emulatorJSSlotIDs() {
		ref := core.SaveSyncSlotRef{
			CanonicalGameID: gameID,
			SourceGameID:    "source-1",
			Runtime:         "emulatorjs",
			SlotID:          slotID,
			IntegrationID:   "integration-1",
		}
		manifest := saveSyncStoredManifest{
			Version:         1,
			CanonicalGameID: ref.CanonicalGameID,
			SourceGameID:    ref.SourceGameID,
			Runtime:         ref.Runtime,
			SlotID:          ref.SlotID,
			UpdatedAt:       time.Unix(1700000000, 0).UTC(),
			Files:           []core.SaveSyncSnapshotFile{},
		}
		manifestBytes, _, err := marshalManifest(manifest)
		if err != nil {
			t.Fatal(err)
		}
		host.putRemote(slotManifestPath(ref), manifestBytes)
		host.putRemote(slotArchivePath(ref), emptyZipBytes(t))
	}

	status, err := svc.StartPrefetch(ctx, core.SaveSyncPrefetchRequest{
		CanonicalGameID: gameID,
		SourceGameID:    "source-1",
		Runtime:         "emulatorjs",
		IntegrationID:   "integration-1",
	})
	if err != nil {
		t.Fatalf("start prefetch: %v", err)
	}
	status = waitPrefetch(t, svc, status.JobID)
	if status.Status != "completed" || status.SlotsCached != len(emulatorJSSlotIDs()) {
		t.Fatalf("prefetch status = %+v, want all slots cached", status)
	}

	host.mu.Lock()
	maxActiveGets := host.maxActiveGets
	host.mu.Unlock()
	if maxActiveGets < 2 {
		t.Fatalf("max active remote gets = %d, want concurrent prefetch", maxActiveGets)
	}
	if maxActiveGets > saveSyncPrefetchConcurrency {
		t.Fatalf("max active remote gets = %d, want bounded by %d", maxActiveGets, saveSyncPrefetchConcurrency)
	}
}

func TestUploadFailureMarksCachedSlotFailed(t *testing.T) {
	ctx := context.Background()
	svc, host, gameID := newTestService(t)
	host.putErr = true

	ref := core.SaveSyncSlotRef{
		CanonicalGameID: gameID,
		SourceGameID:    "source-1",
		Runtime:         "emulatorjs",
		SlotID:          "save-ram",
		IntegrationID:   "integration-1",
	}
	if _, err := svc.PutSlot(ctx, core.SaveSyncPutRequest{
		SaveSyncSlotRef: ref,
		Force:           true,
		Snapshot:        testSnapshot(ref, []core.SaveSyncSnapshotFile{}),
	}); err != nil {
		t.Fatalf("put slot: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		summary, err := svc.readSlotSummaryFromCache(ref)
		if err != nil {
			t.Fatal(err)
		}
		if summary.SyncState == "failed" {
			if summary.LastSyncError == "" {
				t.Fatal("expected failed summary to include last_sync_error")
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("slot did not become failed after upload error")
}

type saveSyncTestConfig struct {
	values map[string]string
}

func (c saveSyncTestConfig) Get(key string) string {
	return c.values[key]
}

func (c saveSyncTestConfig) GetInt(string) int   { return 0 }
func (c saveSyncTestConfig) GetBool(string) bool { return false }
func (c saveSyncTestConfig) Validate() error     { return nil }

type saveSyncTestLogger struct{}

func (saveSyncTestLogger) Info(string, ...any)         {}
func (saveSyncTestLogger) Error(string, error, ...any) {}
func (saveSyncTestLogger) Debug(string, ...any)        {}
func (saveSyncTestLogger) Warn(string, ...any)         {}

type saveSyncTestPluginHost struct {
	mu            sync.Mutex
	remote        map[string][]byte
	blockPut      chan struct{}
	putErr        bool
	getDelay      time.Duration
	activeGets    int
	maxActiveGets int
}

func (h *saveSyncTestPluginHost) Discover(context.Context) error { return nil }
func (h *saveSyncTestPluginHost) Close() error                   { return nil }
func (h *saveSyncTestPluginHost) GetPluginIDs() []string         { return []string{"save-sync-test"} }
func (h *saveSyncTestPluginHost) GetPlugin(string) (*core.Plugin, bool) {
	return &core.Plugin{Manifest: core.PluginManifest{
		ID:       "save-sync-test",
		Provides: []string{"save_sync.get", "save_sync.put", "save_sync.list"},
	}}, true
}
func (h *saveSyncTestPluginHost) ListPlugins() []plugins.PluginInfo {
	return nil
}
func (h *saveSyncTestPluginHost) GetPluginIDsProviding(string) []string { return nil }

func (h *saveSyncTestPluginHost) Call(_ context.Context, _ string, method string, params any, result any) error {
	data, _ := json.Marshal(params)
	var req struct {
		Path       string `json:"path"`
		Prefix     string `json:"prefix"`
		DataBase64 string `json:"data_base64"`
	}
	_ = json.Unmarshal(data, &req)

	switch method {
	case "save_sync.get":
		h.beginGet()
		defer h.endGet()
		h.mu.Lock()
		value, ok := h.remote[req.Path]
		h.mu.Unlock()
		if !ok {
			setJSONResult(result, map[string]any{"status": "not_found"})
			return nil
		}
		setJSONResult(result, map[string]any{
			"status":      "ok",
			"data_base64": base64.StdEncoding.EncodeToString(value),
		})
		return nil
	case "save_sync.put":
		if h.blockPut != nil {
			<-h.blockPut
		}
		if h.putErr {
			return fmt.Errorf("upload failed")
		}
		value, err := base64.StdEncoding.DecodeString(req.DataBase64)
		if err != nil {
			return err
		}
		h.putRemote(req.Path, value)
		setJSONResult(result, map[string]any{"status": "ok"})
		return nil
	case "save_sync.list":
		files := make([]map[string]string, 0)
		h.mu.Lock()
		for remotePath := range h.remote {
			if strings.HasPrefix(remotePath, req.Prefix) {
				files = append(files, map[string]string{"path": remotePath})
			}
		}
		h.mu.Unlock()
		setJSONResult(result, map[string]any{"status": "ok", "files": files})
		return nil
	default:
		return nil
	}
}

func (h *saveSyncTestPluginHost) beginGet() {
	h.mu.Lock()
	h.activeGets++
	if h.activeGets > h.maxActiveGets {
		h.maxActiveGets = h.activeGets
	}
	delay := h.getDelay
	h.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
}

func (h *saveSyncTestPluginHost) endGet() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.activeGets--
}

func (h *saveSyncTestPluginHost) putRemote(path string, data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.remote == nil {
		h.remote = make(map[string][]byte)
	}
	h.remote[path] = append([]byte(nil), data...)
}

func newTestService(t *testing.T) (*service, *saveSyncTestPluginHost, string) {
	t.Helper()
	ctx := context.Background()
	database := dbpkg.NewSQLiteDatabase(saveSyncTestLogger{}, saveSyncTestConfig{
		values: map[string]string{"DB_PATH": filepath.Join(t.TempDir(), "mga.db")},
	})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	integrationRepo := dbpkg.NewIntegrationRepository(database)
	gameStore := dbpkg.NewGameStore(database, saveSyncTestLogger{})
	if err := integrationRepo.Create(ctx, &core.Integration{
		ID:              "integration-1",
		PluginID:        "save-sync-test",
		Label:           "Test Save Sync",
		IntegrationType: "save_sync",
		ConfigJSON:      `{}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := integrationRepo.Create(ctx, &core.Integration{
		ID:              "source-integration",
		PluginID:        "game-source-test",
		Label:           "Test Source",
		IntegrationType: "game_source",
		ConfigJSON:      `{}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := gameStore.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "source-integration",
		SourceGames: []*core.SourceGame{{
			ID:            "source-1",
			IntegrationID: "source-integration",
			PluginID:      "game-source-test",
			ExternalID:    "source-1",
			RawTitle:      "Test Game",
			Platform:      core.PlatformGBA,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			Status:        "found",
			ReviewState:   core.ManualReviewStateMatched,
			Files: []core.GameFile{{
				GameID:   "source-1",
				Path:     "game.gba",
				Role:     core.GameFileRoleRoot,
				FileKind: "rom",
				Size:     1,
			}},
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"source-1": {{
				PluginID:   "metadata-test",
				Title:      "Test Game",
				Platform:   string(core.PlatformGBA),
				ExternalID: "test-game",
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	gameID := "game-1"
	now := time.Now().Unix()
	if _, err := database.GetDB().ExecContext(ctx, `INSERT OR IGNORE INTO canonical_games (id, created_at) VALUES (?, ?)`, gameID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetDB().ExecContext(ctx, `INSERT OR IGNORE INTO canonical_source_games_link (canonical_id, source_game_id) VALUES (?, ?)`, gameID, "source-1"); err != nil {
		t.Fatal(err)
	}
	host := &saveSyncTestPluginHost{remote: make(map[string][]byte)}
	svc := NewService(integrationRepo, gameStore, host, saveSyncTestLogger{}, nil).(*service)
	svc.cacheRoot = t.TempDir()
	return svc, host, gameID
}

func testSnapshot(ref core.SaveSyncSlotRef, files []core.SaveSyncSnapshotFile) core.SaveSyncSnapshot {
	return core.SaveSyncSnapshot{
		CanonicalGameID: ref.CanonicalGameID,
		SourceGameID:    ref.SourceGameID,
		Runtime:         ref.Runtime,
		SlotID:          ref.SlotID,
		Files:           files,
		ArchiveBase64:   base64.StdEncoding.EncodeToString(emptyZipData()),
	}
}

func emptyZipBytes(t *testing.T) []byte {
	t.Helper()
	return emptyZipData()
}

func emptyZipData() []byte {
	data, err := base64.StdEncoding.DecodeString("UEsFBgAAAAAAAAAAAAAAAAAAAAAAAA==")
	if err != nil {
		panic(err)
	}
	return data
}

func setJSONResult(result any, value any) {
	data, _ := json.Marshal(value)
	_ = json.Unmarshal(data, result)
}

func waitPrefetch(t *testing.T, svc *service, jobID string) *core.SaveSyncPrefetchStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status, err := svc.GetPrefetchStatus(context.Background(), jobID)
		if err != nil {
			t.Fatal(err)
		}
		if status != nil && (status.Status == "completed" || status.Status == "failed") {
			return status
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("prefetch did not finish")
	return nil
}
