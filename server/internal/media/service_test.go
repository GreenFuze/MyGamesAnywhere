package media

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	dbstore "github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
)

type testLogger struct{}

func (testLogger) Info(string, ...any)         {}
func (testLogger) Error(string, error, ...any) {}
func (testLogger) Debug(string, ...any)        {}
func (testLogger) Warn(string, ...any)         {}

type testConfig struct {
	dbPath    string
	mediaRoot string
	ints      map[string]int
}

func (c testConfig) Get(key string) string {
	switch key {
	case "DB_PATH":
		return c.dbPath
	case "MEDIA_ROOT":
		return c.mediaRoot
	default:
		return ""
	}
}

func (c testConfig) GetInt(key string) int {
	if c.ints == nil {
		return 0
	}
	return c.ints[key]
}

func (testConfig) GetBool(string) bool { return false }
func (testConfig) Validate() error     { return nil }

func TestServiceStartDownloadsExistingPendingAssets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-payload"))
	}))
	defer server.Close()

	store, cfg := newTestStore(t)
	assetID := seedPendingAsset(t, ctx, store, server.URL+"/cover.png")

	svc, ok := NewService(store, cfg, testLogger{}).(*Service)
	if !ok {
		t.Fatal("expected concrete media service")
	}
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}

	asset := waitForAssetDownload(t, ctx, store, assetID)
	if asset.LocalPath == "" {
		t.Fatal("expected local_path to be populated")
	}
	if asset.Hash == "" {
		t.Fatal("expected hash to be populated")
	}
	fullPath := filepath.Join(cfg.mediaRoot, filepath.FromSlash(asset.LocalPath))
	if _, err := os.Stat(fullPath); err != nil {
		t.Fatalf("expected downloaded file at %s: %v", fullPath, err)
	}
}

func TestServiceDoesNotMarkLocalPathOnFailedDownload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	store, cfg := newTestStore(t)
	assetID := seedPendingAsset(t, ctx, store, server.URL+"/broken.png")

	svc, ok := NewService(store, cfg, testLogger{}).(*Service)
	if !ok {
		t.Fatal("expected concrete media service")
	}
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}

	waitForCondition(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return requests > 0
	})

	asset, err := store.GetMediaAssetByID(ctx, assetID)
	if err != nil {
		t.Fatal(err)
	}
	if asset == nil {
		t.Fatal("expected media asset row")
	}
	if asset.LocalPath != "" {
		t.Fatalf("local_path = %q, want empty after failed download", asset.LocalPath)
	}
	if asset.Hash != "" {
		t.Fatalf("hash = %q, want empty after failed download", asset.Hash)
	}
}

func TestServiceDeduplicatesInFlightDownloads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})
	release := make(chan struct{})

	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		current := requests
		mu.Unlock()
		if current == 1 {
			close(started)
		}
		<-release
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-payload"))
	}))
	defer server.Close()

	store, cfg := newTestStore(t)
	cfg.ints["MEDIA_DOWNLOAD_CONCURRENCY"] = 2
	assetID := seedPendingAsset(t, ctx, store, server.URL+"/dedupe.png")

	svc, ok := NewService(store, cfg, testLogger{}).(*Service)
	if !ok {
		t.Fatal("expected concrete media service")
	}
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first media request")
	}

	for i := 0; i < 3; i++ {
		if err := svc.EnqueuePending(ctx); err != nil {
			t.Fatal(err)
		}
	}
	close(release)

	waitForAssetDownload(t, ctx, store, assetID)

	mu.Lock()
	defer mu.Unlock()
	if requests != 1 {
		t.Fatalf("request count = %d, want 1", requests)
	}
}

func TestRetrySQLiteBusyRetriesUntilSuccess(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	err := retrySQLiteBusy(ctx, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("database is locked (5) (SQLITE_BUSY)")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestRetrySQLiteBusyStopsOnNonBusyError(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	wantErr := errors.New("boom")
	err := retrySQLiteBusy(ctx, func() error {
		attempts++
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestMediaRequestPolicyRegistryAppliesHLTBHeaders(t *testing.T) {
	parsed, err := url.Parse("https://howlongtobeat.com/games/Portal2cover.jpg")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		t.Fatal(err)
	}

	registry := mediaRequestPolicyRegistry{policies: []mediaRequestPolicyMatcher{hltbImageRequestPolicy{}}}
	if err := registry.Apply(req, parsed); err != nil {
		t.Fatal(err)
	}

	if got := req.Header.Get("Referer"); got != "https://howlongtobeat.com/" {
		t.Fatalf("Referer = %q, want %q", got, "https://howlongtobeat.com/")
	}
	if got := req.Header.Get("User-Agent"); got != defaultBrowserUserAgent {
		t.Fatalf("User-Agent = %q, want %q", got, defaultBrowserUserAgent)
	}
	if got := req.Header.Get("Accept"); got == "" {
		t.Fatal("Accept header was not set for HLTB image fetch")
	}
}

func TestMediaRequestPolicyRegistryAppliesRetroAchievementsHeaders(t *testing.T) {
	parsed, err := url.Parse("https://retroachievements.org/Images/000001.png")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		t.Fatal(err)
	}

	registry := mediaRequestPolicyRegistry{policies: []mediaRequestPolicyMatcher{retroAchievementsImageRequestPolicy{}}}
	if err := registry.Apply(req, parsed); err != nil {
		t.Fatal(err)
	}

	if got := req.Header.Get("Referer"); got != "https://retroachievements.org/" {
		t.Fatalf("Referer = %q, want %q", got, "https://retroachievements.org/")
	}
	if got := req.Header.Get("User-Agent"); got != defaultBrowserUserAgent {
		t.Fatalf("User-Agent = %q, want %q", got, defaultBrowserUserAgent)
	}
	if got := req.Header.Get("Accept"); got == "" {
		t.Fatal("Accept header was not set for RetroAchievements image fetch")
	}
	if got := req.Header.Get("Accept-Language"); got == "" {
		t.Fatal("Accept-Language header was not set for RetroAchievements image fetch")
	}
}

func TestMediaRequestPolicyRegistryLeavesOtherHostsUnchanged(t *testing.T) {
	parsed, err := url.Parse("https://images.igdb.com/igdb/image/upload/t_cover_big/co1abc.jpg")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		t.Fatal(err)
	}

	registry := mediaRequestPolicyRegistry{policies: []mediaRequestPolicyMatcher{hltbImageRequestPolicy{}}}
	if err := registry.Apply(req, parsed); err != nil {
		t.Fatal(err)
	}

	if got := req.Header.Get("Referer"); got != "" {
		t.Fatalf("Referer = %q, want empty", got)
	}
	if got := req.Header.Get("User-Agent"); got != "" {
		t.Fatalf("User-Agent = %q, want empty", got)
	}
	if got := req.Header.Get("Accept"); got != "" {
		t.Fatalf("Accept = %q, want empty", got)
	}
}

func TestMediaRequestPolicyRegistryFailsFastOnNilInputs(t *testing.T) {
	registry := mediaRequestPolicyRegistry{policies: []mediaRequestPolicyMatcher{hltbImageRequestPolicy{}}}
	if err := registry.Apply(nil, nil); err == nil {
		t.Fatal("expected nil input error")
	}
}

func newTestStore(t *testing.T) (core.GameStore, testConfig) {
	t.Helper()

	root := t.TempDir()
	cfg := testConfig{
		dbPath:    filepath.Join(root, "media.sqlite"),
		mediaRoot: filepath.Join(root, "media"),
		ints:      map[string]int{"MEDIA_DOWNLOAD_CONCURRENCY": 1},
	}
	database := dbstore.NewSQLiteDatabase(testLogger{}, cfg)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	return dbstore.NewGameStore(database, testLogger{}), cfg
}

func seedPendingAsset(t *testing.T, ctx context.Context, store core.GameStore, assetURL string) int {
	t.Helper()

	if err := store.PersistScanResults(ctx, &core.ScanBatch{
		IntegrationID: "integration-1",
		SourceGames: []*core.SourceGame{{
			ID:            "scan:media-seed",
			IntegrationID: "integration-1",
			PluginID:      "game-source-steam",
			ExternalID:    fmt.Sprintf("media-%d", time.Now().UnixNano()),
			RawTitle:      "Media Seed",
			Platform:      core.PlatformWindowsPC,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			Status:        "found",
		}},
		ResolverMatches: map[string][]core.ResolverMatch{
			"scan:media-seed": {{
				PluginID:   "metadata-igdb",
				Title:      "Media Seed",
				Platform:   string(core.PlatformWindowsPC),
				ExternalID: "media-match",
			}},
		},
		MediaItems: map[string][]core.MediaRef{
			"scan:media-seed": {{
				Type: core.MediaTypeCover,
				URL:  assetURL,
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	assets, err := store.GetPendingMediaDownloads(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 {
		t.Fatalf("len(pending assets) = %d, want 1", len(assets))
	}
	return assets[0].ID
}

func waitForAssetDownload(t *testing.T, ctx context.Context, store core.GameStore, assetID int) *core.MediaAsset {
	t.Helper()

	var asset *core.MediaAsset
	waitForCondition(t, 2*time.Second, func() bool {
		var err error
		asset, err = store.GetMediaAssetByID(ctx, assetID)
		if err != nil {
			t.Fatalf("GetMediaAssetByID: %v", err)
		}
		return asset != nil && asset.LocalPath != "" && asset.Hash != ""
	})
	return asset
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition was not satisfied before timeout")
}
