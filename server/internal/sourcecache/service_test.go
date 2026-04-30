package sourcecache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	dbpkg "github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
)

type testConfig struct {
	values map[string]string
}

func (c testConfig) Get(key string) string { return c.values[key] }
func (c testConfig) GetInt(string) int     { return 0 }
func (c testConfig) GetBool(string) bool   { return false }
func (c testConfig) Validate() error       { return nil }

type testLogger struct{}

func (testLogger) Info(string, ...any)         {}
func (testLogger) Error(string, error, ...any) {}
func (testLogger) Debug(string, ...any)        {}
func (testLogger) Warn(string, ...any)         {}

type testIntegrationRepo struct {
	integration *core.Integration
}

func (r *testIntegrationRepo) Create(context.Context, *core.Integration) error   { return nil }
func (r *testIntegrationRepo) Update(context.Context, *core.Integration) error   { return nil }
func (r *testIntegrationRepo) Delete(context.Context, string) error              { return nil }
func (r *testIntegrationRepo) List(context.Context) ([]*core.Integration, error) { return nil, nil }
func (r *testIntegrationRepo) ListByPluginID(context.Context, string) ([]*core.Integration, error) {
	return nil, nil
}
func (r *testIntegrationRepo) GetByID(context.Context, string) (*core.Integration, error) {
	return r.integration, nil
}

type testPluginHost struct {
	plugin         *core.Plugin
	mu             sync.Mutex
	calls          int
	activeCalls    int
	maxActiveCalls int
	delay          time.Duration
	body           []byte
}

func (h *testPluginHost) Discover(context.Context) error { return nil }
func (h *testPluginHost) Call(_ context.Context, pluginID, method string, params any, result any) error {
	if h.plugin == nil || pluginID != h.plugin.Manifest.ID || method != sourceFileMaterializeMethod {
		return nil
	}
	h.beginCall()
	defer h.endCall()
	req, ok := params.(core.SourceMaterializeRequest)
	if !ok {
		return nil
	}
	if err := os.WriteFile(req.DestPath, h.body, 0o644); err != nil {
		return err
	}
	payload, err := json.Marshal(core.SourceMaterializeResult{
		Size:     int64(len(h.body)),
		Revision: req.Revision,
		ModTime:  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, result)
}
func (h *testPluginHost) Close() error { return nil }
func (h *testPluginHost) GetPluginIDs() []string {
	if h.plugin == nil {
		return nil
	}
	return []string{h.plugin.Manifest.ID}
}
func (h *testPluginHost) GetPlugin(pluginID string) (*core.Plugin, bool) {
	if h.plugin == nil || pluginID != h.plugin.Manifest.ID {
		return nil, false
	}
	return h.plugin, true
}
func (h *testPluginHost) ListPlugins() []plugins.PluginInfo { return nil }
func (h *testPluginHost) GetPluginIDsProviding(string) []string {
	return nil
}

func (h *testPluginHost) beginCall() {
	h.mu.Lock()
	h.calls++
	h.activeCalls++
	if h.activeCalls > h.maxActiveCalls {
		h.maxActiveCalls = h.activeCalls
	}
	delay := h.delay
	h.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
}

func (h *testPluginHost) endCall() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.activeCalls--
}

func (h *testPluginHost) callCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

func (h *testPluginHost) maxConcurrentCalls() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.maxActiveCalls
}

func TestServicePrepareMaterializesAndReusesCache(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "cache.db")
	cacheRoot := filepath.Join(t.TempDir(), "source-cache")

	database := dbpkg.NewSQLiteDatabase(testLogger{}, testConfig{values: map[string]string{"DB_PATH": dbPath}})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	if _, err := database.GetDB().ExecContext(ctx, `INSERT INTO source_games
		(id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, root_path, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"source-1", "integration-1", "game-source-google-drive", "drive-file-1", "Drive Game",
		string(core.PlatformGBA), string(core.GameKindBaseGame), string(core.GroupKindSelfContained), "Drive/Game", "found", time.Now().Unix(),
	); err != nil {
		t.Fatal(err)
	}

	store := dbpkg.NewSourceCacheStore(database)
	host := &testPluginHost{
		plugin: &core.Plugin{
			Manifest: core.PluginManifest{
				ID:       "game-source-google-drive",
				Provides: []string{sourceFileMaterializeMethod},
			},
		},
		body: []byte("gba-data"),
	}
	svc := NewService(
		store,
		&testIntegrationRepo{integration: &core.Integration{ID: "integration-1", PluginID: "game-source-google-drive", ConfigJSON: `{}`}},
		host,
		testConfig{values: map[string]string{"SOURCE_CACHE_ROOT": cacheRoot}},
		testLogger{},
	)

	sourceGame := &core.SourceGame{
		ID:            "source-1",
		IntegrationID: "integration-1",
		PluginID:      "game-source-google-drive",
		RawTitle:      "Drive Game",
		Platform:      core.PlatformGBA,
		GroupKind:     core.GroupKindSelfContained,
		Status:        "found",
		Files: []core.GameFile{
			{
				GameID:   "source-1",
				Path:     "roms/game.gba",
				FileName: "game.gba",
				Role:     core.GameFileRoleRoot,
				Size:     8,
				ObjectID: "object-1",
				Revision: "rev-1",
			},
		},
	}

	delivery := svc.DescribeSourceGame(ctx, core.PlatformGBA, sourceGame)
	if len(delivery) != 1 || delivery[0].Mode != core.SourceDeliveryModeMaterialized || !delivery[0].PrepareRequired {
		t.Fatalf("unexpected delivery: %+v", delivery)
	}

	job, immediate, err := svc.Prepare(ctx, core.SourceCachePrepareRequest{
		CanonicalGameID: "game-1",
		CanonicalTitle:  "Drive Game",
		SourceGameID:    "source-1",
		Profile:         core.BrowserProfileEmulatorJS,
	}, core.PlatformGBA, sourceGame)
	if err != nil {
		t.Fatal(err)
	}
	if immediate {
		t.Fatal("expected async materialization job")
	}
	if job == nil || job.JobID == "" {
		t.Fatalf("expected job, got %+v", job)
	}

	var completed *core.SourceCacheJobStatus
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current, err := svc.GetJob(ctx, job.JobID)
		if err != nil {
			t.Fatal(err)
		}
		if current != nil && current.Status == "completed" {
			completed = current
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if completed == nil {
		t.Fatalf("job did not complete: %+v", job)
	}
	if calls := host.callCount(); calls != 1 {
		t.Fatalf("materialize calls = %d, want 1", calls)
	}

	entry, file, localPath, err := svc.ResolveCachedFile(ctx, "source-1", core.BrowserProfileEmulatorJS, "roms/game.gba")
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil || file == nil || localPath == "" {
		t.Fatalf("expected cached file resolution, got entry=%+v file=%+v path=%q", entry, file, localPath)
	}
	content, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "gba-data" {
		t.Fatalf("cached content = %q", string(content))
	}

	cacheHitJob, immediate, err := svc.Prepare(ctx, core.SourceCachePrepareRequest{
		CanonicalGameID: "game-1",
		CanonicalTitle:  "Drive Game",
		SourceGameID:    "source-1",
		Profile:         core.BrowserProfileEmulatorJS,
	}, core.PlatformGBA, sourceGame)
	if err != nil {
		t.Fatal(err)
	}
	if !immediate {
		t.Fatal("expected cache hit to return immediate status")
	}
	if cacheHitJob == nil || cacheHitJob.Status != "completed" {
		t.Fatalf("expected completed cache-hit job, got %+v", cacheHitJob)
	}
	if calls := host.callCount(); calls != 1 {
		t.Fatalf("materialize calls after cache hit = %d, want 1", calls)
	}
}

func TestServicePrepareMaterializesFilesConcurrently(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "cache.db")
	cacheRoot := filepath.Join(t.TempDir(), "source-cache")

	database := dbpkg.NewSQLiteDatabase(testLogger{}, testConfig{values: map[string]string{"DB_PATH": dbPath}})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetDB().ExecContext(ctx, `INSERT INTO source_games
		(id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, root_path, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"source-scumm", "integration-smb", "game-source-smb", "smb-scumm", "Scumm Game",
		string(core.PlatformScummVM), string(core.GameKindBaseGame), string(core.GroupKindSelfContained), "Games/Scumm", "found", time.Now().Unix(),
	); err != nil {
		t.Fatal(err)
	}

	store := dbpkg.NewSourceCacheStore(database)
	host := &testPluginHost{
		plugin: &core.Plugin{
			Manifest: core.PluginManifest{
				ID:       "game-source-smb",
				Provides: []string{sourceFileMaterializeMethod},
			},
		},
		body:  []byte("scumm-data"),
		delay: 75 * time.Millisecond,
	}
	svc := NewService(
		store,
		&testIntegrationRepo{integration: &core.Integration{ID: "integration-smb", PluginID: "game-source-smb", ConfigJSON: `{}`}},
		host,
		testConfig{values: map[string]string{"SOURCE_CACHE_ROOT": cacheRoot}},
		testLogger{},
	)

	files := make([]core.GameFile, 12)
	for i := range files {
		files[i] = core.GameFile{
			GameID:   "source-scumm",
			Path:     filepath.ToSlash(filepath.Join("scumm", "file-"+string(rune('a'+i))+".dat")),
			FileName: "file.dat",
			Role:     core.GameFileRoleRequired,
			Size:     10,
		}
	}
	sourceGame := &core.SourceGame{
		ID:            "source-scumm",
		IntegrationID: "integration-smb",
		PluginID:      "game-source-smb",
		RawTitle:      "Scumm Game",
		Platform:      core.PlatformScummVM,
		GroupKind:     core.GroupKindSelfContained,
		RootPath:      "Games/Scumm",
		Status:        "found",
		Files:         files,
	}

	job, immediate, err := svc.Prepare(ctx, core.SourceCachePrepareRequest{
		CanonicalGameID: "game-scumm",
		CanonicalTitle:  "Scumm Game",
		SourceGameID:    "source-scumm",
		Profile:         core.BrowserProfileScummVM,
	}, core.PlatformScummVM, sourceGame)
	if err != nil {
		t.Fatal(err)
	}
	if immediate {
		t.Fatal("expected async materialization job")
	}

	completed := waitForCompletedCacheJob(t, ctx, svc, job.JobID)
	if completed.ProgressCurrent != len(files) || completed.ProgressTotal != len(files) {
		t.Fatalf("progress = %d/%d, want %d/%d", completed.ProgressCurrent, completed.ProgressTotal, len(files), len(files))
	}
	if calls := host.callCount(); calls != len(files) {
		t.Fatalf("materialize calls = %d, want %d", calls, len(files))
	}
	if maxConcurrent := host.maxConcurrentCalls(); maxConcurrent < 2 || maxConcurrent > sourceCacheMaterializeConcurrency {
		t.Fatalf("max concurrent materialize calls = %d, want between 2 and %d", maxConcurrent, sourceCacheMaterializeConcurrency)
	}
}

func TestServiceDescribeSMBSourceGameRequiresMaterialization(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "cache.db")
	database := dbpkg.NewSQLiteDatabase(testLogger{}, testConfig{values: map[string]string{"DB_PATH": dbPath}})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	store := dbpkg.NewSourceCacheStore(database)
	host := &testPluginHost{
		plugin: &core.Plugin{
			Manifest: core.PluginManifest{
				ID:       "game-source-smb",
				Provides: []string{sourceFileMaterializeMethod},
			},
		},
	}
	svc := NewService(
		store,
		&testIntegrationRepo{integration: &core.Integration{ID: "integration-smb", PluginID: "game-source-smb", ConfigJSON: `{}`}},
		host,
		testConfig{values: map[string]string{}},
		testLogger{},
	)

	sourceGame := &core.SourceGame{
		ID:            "source-smb",
		IntegrationID: "integration-smb",
		PluginID:      "game-source-smb",
		RawTitle:      "Island of Dr. Brain",
		Platform:      core.PlatformScummVM,
		GroupKind:     core.GroupKindSelfContained,
		RootPath:      "ScummVM/Island",
		Status:        "found",
		Files: []core.GameFile{
			{GameID: "source-smb", Path: "ScummVM/Island/ISLAND.EXE", FileName: "ISLAND.EXE", Role: core.GameFileRoleRoot, Size: 10},
			{GameID: "source-smb", Path: "ScummVM/Island/resource.001", FileName: "resource.001", Role: core.GameFileRoleRequired, Size: 20},
		},
	}

	delivery := svc.DescribeSourceGame(ctx, core.PlatformScummVM, sourceGame)
	if len(delivery) != 1 {
		t.Fatalf("delivery count = %d, want 1: %+v", len(delivery), delivery)
	}
	if delivery[0].Mode != core.SourceDeliveryModeMaterialized || !delivery[0].PrepareRequired || delivery[0].Ready {
		t.Fatalf("unexpected SMB delivery: %+v", delivery[0])
	}
}

func waitForCompletedCacheJob(t *testing.T, ctx context.Context, svc core.SourceCacheService, jobID string) *core.SourceCacheJobStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last *core.SourceCacheJobStatus
	for time.Now().Before(deadline) {
		current, err := svc.GetJob(ctx, jobID)
		if err != nil {
			t.Fatal(err)
		}
		last = current
		if current != nil && current.Status == "completed" {
			return current
		}
		if current != nil && current.Status == "failed" {
			t.Fatalf("job failed: %+v", current)
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("job did not complete: %+v", last)
	return nil
}
