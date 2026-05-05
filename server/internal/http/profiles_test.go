package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type restoreProfileRepo struct{}

func (restoreProfileRepo) Create(context.Context, *core.Profile) error { return nil }
func (restoreProfileRepo) Update(context.Context, *core.Profile) error { return nil }
func (restoreProfileRepo) Delete(context.Context, string) error        { return nil }
func (restoreProfileRepo) List(context.Context) ([]*core.Profile, error) {
	return nil, nil
}
func (restoreProfileRepo) GetByID(context.Context, string) (*core.Profile, error) {
	return nil, nil
}
func (restoreProfileRepo) Count(context.Context) (int, error)       { return 0, nil }
func (restoreProfileRepo) CountAdmins(context.Context) (int, error) { return 0, nil }
func (restoreProfileRepo) EnsureDefaultForExistingData(context.Context) (*core.Profile, error) {
	return nil, nil
}

type restoreSyncService struct{}

func (restoreSyncService) Push(context.Context, string) (*core.PushResult, error) { return nil, nil }
func (restoreSyncService) Pull(context.Context, string) (*core.PullResult, error) { return nil, nil }
func (restoreSyncService) CheckBootstrap(context.Context, core.RestoreSyncRequest) (*core.RestoreSyncResult, error) {
	return nil, nil
}
func (restoreSyncService) BrowseBootstrap(context.Context, core.RestoreSyncBrowseRequest) (any, error) {
	return nil, nil
}
func (restoreSyncService) ListBootstrapPayloads(context.Context, core.RestoreSyncRequest) (*core.RestoreSyncPointsResult, error) {
	return &core.RestoreSyncPointsResult{Status: "ok"}, nil
}
func (restoreSyncService) RestoreBootstrap(context.Context, core.RestoreSyncRequest) (*core.RestoreSyncResult, error) {
	return &core.RestoreSyncResult{Status: "ok", ProfileID: "profile-1", IntegrationID: "sync-1"}, nil
}
func (restoreSyncService) Status(context.Context) (*core.SyncStatus, error) { return nil, nil }
func (restoreSyncService) StoreKey(context.Context, string, string) error   { return nil }
func (restoreSyncService) ClearKey() error                                  { return nil }

type restoreScanStarter struct {
	started   bool
	profileID string
}

func (s *restoreScanStarter) StartScan(ctx context.Context, _ ScanRequest) (*core.ScanJobStatus, bool, error) {
	s.started = true
	if profile, ok := core.ProfileFromContext(ctx); ok {
		s.profileID = profile.ID
	}
	return &core.ScanJobStatus{JobID: "scan-1", Status: "queued"}, false, nil
}

type restoreConfig map[string]string

func (c restoreConfig) Get(key string) string    { return c[key] }
func (c restoreConfig) GetInt(string) int        { return 0 }
func (c restoreConfig) GetBool(string) bool      { return false }
func (c restoreConfig) Validate() error          { return nil }
func (c restoreConfig) Path() string             { return "" }
func (c restoreConfig) Set(string, string) error { return nil }

func TestRestoreSyncStartsFirstRunScanForRestoredProfile(t *testing.T) {
	scanner := &restoreScanStarter{}
	ctrl := NewProfileController(
		restoreProfileRepo{},
		restoreSyncService{},
		scanner,
		restoreConfig{"PORT": "8900", "LISTEN_IP": "127.0.0.1"},
		noopLogger{},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/setup/restore-sync", strings.NewReader(`{"plugin_id":"sync-settings-google-drive","config":{}}`))
	rec := httptest.NewRecorder()
	ctrl.RestoreSync(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !scanner.started {
		t.Fatal("expected first-run scan to start")
	}
	if scanner.profileID != "profile-1" {
		t.Fatalf("scan profile id = %q, want profile-1", scanner.profileID)
	}
	if !strings.Contains(rec.Body.String(), `"scan_job"`) {
		t.Fatalf("response missing scan_job: %s", rec.Body.String())
	}
}
