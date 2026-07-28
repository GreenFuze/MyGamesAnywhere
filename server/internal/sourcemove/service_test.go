package sourcemove

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestSourceMoveCommitsDestinationBeforeDeletingSource(t *testing.T) {
	t.Parallel()
	fixture := newMoveFixture(t)
	jobs, err := fixture.service.Start(fixture.ctx, core.SourceMoveStartRequest{Items: []core.SourceMoveSelection{fixture.selection}})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("Start() jobs = %d, want 1", len(jobs))
	}
	job := fixture.waitForStatus(t, jobs[0].ID, core.SourceMoveStatusCompleted)
	if job.ProgressCurrent != 1 || job.ProgressTotal != 1 || job.FinishedAt == nil {
		t.Fatalf("completed job = %#v", job)
	}
	events := fixture.events.snapshot()
	assertEventBefore(t, events, "source.transfer.commit", "delete-source")
	assertEventBefore(t, events, "source.transfer.commit", "scan-destination")
	assertEventBefore(t, events, "scan-destination", "delete-source")
	if fixture.deletion.calls != 1 {
		t.Fatalf("delete calls = %d, want 1", fixture.deletion.calls)
	}
}

func TestSourceMoveFailureBeforeCommitLeavesOriginalAndCanCleanStage(t *testing.T) {
	t.Parallel()
	fixture := newMoveFixture(t)
	fixture.plugins.failMethod = transferPutMethod
	jobs, err := fixture.service.Start(fixture.ctx, core.SourceMoveStartRequest{Items: []core.SourceMoveSelection{fixture.selection}})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	job := fixture.waitForStatus(t, jobs[0].ID, core.SourceMoveStatusFailedBeforeCommit)
	if fixture.deletion.calls != 0 {
		t.Fatalf("delete calls = %d, want 0", fixture.deletion.calls)
	}
	cleaned, err := fixture.service.CleanupStage(fixture.ctx, job.ID)
	if err != nil {
		t.Fatalf("CleanupStage() error = %v", err)
	}
	if cleaned.FinishedAt == nil || cleaned.Error != "" {
		t.Fatalf("cleaned job = %#v", cleaned)
	}
	if !containsEvent(fixture.events.snapshot(), transferAbortMethod) {
		t.Fatalf("abort event missing: %v", fixture.events.snapshot())
	}
}

func TestSourceMoveCleanupFailureKeepsVerifiedDestinationAndCanKeepBoth(t *testing.T) {
	t.Parallel()
	fixture := newMoveFixture(t)
	fixture.deletion.err = errors.New("provider cleanup failed")
	jobs, err := fixture.service.Start(fixture.ctx, core.SourceMoveStartRequest{Items: []core.SourceMoveSelection{fixture.selection}})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	job := fixture.waitForStatus(t, jobs[0].ID, core.SourceMoveStatusSourceCleanupRequired)
	if !strings.Contains(job.Error, "provider cleanup failed") {
		t.Fatalf("cleanup error = %q", job.Error)
	}
	kept, err := fixture.service.KeepBoth(fixture.ctx, job.ID)
	if err != nil {
		t.Fatalf("KeepBoth() error = %v", err)
	}
	if !kept.KeepBoth {
		t.Fatal("KeepBoth() did not persist keep_both")
	}
	completed := fixture.waitForStatus(t, job.ID, core.SourceMoveStatusCompleted)
	if !completed.KeepBoth || completed.Message != "Both copies are available" {
		t.Fatalf("completed keep-both job = %#v", completed)
	}
}

func TestSourceMoveLibraryRefreshFailureDoesNotDeleteOriginal(t *testing.T) {
	t.Parallel()
	fixture := newMoveFixture(t)
	fixture.scanner.err = errors.New("scan failed")
	jobs, err := fixture.service.Start(fixture.ctx, core.SourceMoveStartRequest{Items: []core.SourceMoveSelection{fixture.selection}})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	job := fixture.waitForStatus(t, jobs[0].ID, core.SourceMoveStatusSourceCleanupRequired)
	if job.RecoveryPhase != core.SourceMoveStatusRefreshingLibrary {
		t.Fatalf("recovery phase = %q", job.RecoveryPhase)
	}
	if fixture.deletion.calls != 0 {
		t.Fatalf("delete calls = %d, want 0 while destination is absent from the library", fixture.deletion.calls)
	}
}

func TestSourceMovePreviewRejectsAnotherProfilesDestination(t *testing.T) {
	t.Parallel()
	fixture := newMoveFixture(t)
	fixture.integrations.byID[fixture.destination.ID] = nil
	_, err := fixture.service.Preview(fixture.ctx, core.SourceMovePreviewRequest{Items: []core.SourceMoveSelection{fixture.selection}})
	if err == nil || !strings.Contains(err.Error(), "destination connection not found for this profile") {
		t.Fatalf("Preview() error = %v", err)
	}
}

func TestGoogleDriveDestinationAuthoritySeparatesStableSharedFolders(t *testing.T) {
	t.Parallel()
	fixture := newMoveFixture(t)
	first := &core.Integration{
		PluginID:   "game-source-google-drive",
		ConfigJSON: `{"include_paths":[{"path":"Shared with me/Games","object_id":"folder-a","recursive":true}]}`,
	}
	second := &core.Integration{
		PluginID:   "game-source-google-drive",
		ConfigJSON: `{"include_paths":[{"path":"Shared with me/Games","object_id":"folder-b","recursive":true}]}`,
	}
	firstConfig, err := parseIntegrationConfig(first)
	if err != nil {
		t.Fatal(err)
	}
	secondConfig, err := parseIntegrationConfig(second)
	if err != nil {
		t.Fatal(err)
	}
	firstAuthority, err := fixture.service.destinationAuthority(fixture.ctx, first, firstConfig, "Shared with me/Games/Title")
	if err != nil {
		t.Fatal(err)
	}
	secondAuthority, err := fixture.service.destinationAuthority(fixture.ctx, second, secondConfig, "Shared with me/Games/Title")
	if err != nil {
		t.Fatal(err)
	}
	if firstAuthority == secondAuthority {
		t.Fatalf("stable Shared with me folders collided: %q", firstAuthority)
	}
	if firstAuthority != "gdrive:test|object:folder-a" {
		t.Fatalf("first authority = %q", firstAuthority)
	}
	if secondAuthority != "gdrive:test|object:folder-b" {
		t.Fatalf("second authority = %q", secondAuthority)
	}
}

func TestInterruptedPostCommitMoveRetriesOnlySourceCleanup(t *testing.T) {
	t.Parallel()
	fixture := newMoveFixture(t)
	job := fixture.newJob()
	job.Status = core.SourceMoveStatusInterrupted
	job.RecoveryPhase = core.SourceMoveStatusDeletingSource
	if err := fixture.store.CreateJob(fixture.ctx, job); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Retry(fixture.ctx, job.ID); err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	fixture.waitForStatus(t, job.ID, core.SourceMoveStatusCompleted)
	events := fixture.events.snapshot()
	if containsEvent(events, materializeMethod) || containsEvent(events, transferPutMethod) {
		t.Fatalf("post-commit retry recopied files: %v", events)
	}
	if !containsEvent(events, "delete-source") {
		t.Fatalf("post-commit retry did not clean source: %v", events)
	}
}

type moveFixture struct {
	ctx          context.Context
	service      *Service
	store        *memoryMoveStore
	plugins      *fakeMovePlugins
	integrations *fakeIntegrationRepo
	deletion     *fakeDeletion
	scanner      *fakeScanner
	events       *eventLog
	source       *core.SourceGame
	destination  *core.Integration
	selection    core.SourceMoveSelection
}

func newMoveFixture(t *testing.T) *moveFixture {
	t.Helper()
	profile := &core.Profile{ID: "profile-1", DisplayName: "Player", Role: core.ProfileRolePlayer}
	ctx := core.WithProfile(context.Background(), profile)
	events := &eventLog{}
	source := &core.SourceGame{
		ID: "source-1", IntegrationID: "source-integration", PluginID: "game-source-smb",
		RawTitle: "Game", RootPath: "Library/Game", Status: "found",
		Files: []core.GameFile{{Path: "Library/Game/game.bin", FileName: "game.bin", Size: 7}},
	}
	destination := &core.Integration{
		ID: "destination-integration", ProfileID: profile.ID, PluginID: "game-source-google-drive",
		Label: "Drive", ConfigJSON: `{"include_paths":[{"path":"Games","recursive":true}]}`,
	}
	store := newMemoryMoveStore()
	plugins := &fakeMovePlugins{events: events, contents: []byte("1234567")}
	plugins.manifests = map[string]*core.Plugin{
		source.PluginID: {
			Enabled:  true,
			Manifest: core.PluginManifest{Provides: []string{materializeMethod}},
		},
		destination.PluginID: {
			Enabled:  true,
			Manifest: core.PluginManifest{Provides: []string{transferBeginMethod, transferPutMethod, transferCommitMethod, transferAbortMethod}},
		},
	}
	integrations := &fakeIntegrationRepo{byID: map[string]*core.Integration{
		source.IntegrationID: sourceIntegration(source),
		destination.ID:       destination,
	}}
	gameStore := &fakeMoveGameStore{
		game:      &core.CanonicalGame{ID: "game-1", Title: "Game", SourceGames: []*core.SourceGame{source}},
		source:    source,
		exclusive: true,
	}
	deletion := &fakeDeletion{events: events}
	scanner := &fakeScanner{events: events}
	service := NewService(
		store, gameStore, integrations, plugins, deletion, scanner,
		fakeConfig{"DB_PATH": filepath.Join(t.TempDir(), "mga.sqlite")},
		fakeLogger{},
	)
	return &moveFixture{
		ctx: ctx, service: service, store: store, plugins: plugins, integrations: integrations,
		deletion: deletion, scanner: scanner, events: events, source: source, destination: destination,
		selection: core.SourceMoveSelection{
			CanonicalGameID: "game-1", SourceGameID: source.ID,
			DestinationIntegrationID: destination.ID, DestinationPath: "Games/Game",
		},
	}
}

func (f *moveFixture) waitForStatus(t *testing.T, jobID, status string) *core.SourceMoveJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, _ := f.store.GetJob(f.ctx, jobID)
		if job != nil && job.Status == status {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, _ := f.store.GetJob(f.ctx, jobID)
	t.Fatalf("job status = %#v, want %q", job, status)
	return nil
}

func (f *moveFixture) newJob() *core.SourceMoveJob {
	files, _ := moveFiles(f.source)
	return &core.SourceMoveJob{
		ID: "interrupted-job", TransferID: "interrupted-transfer",
		CanonicalGameID: "game-1", CanonicalTitle: "Game",
		SourceGameID: f.source.ID, SourceTitle: f.source.RawTitle,
		SourceIntegrationID: f.source.IntegrationID, SourcePluginID: f.source.PluginID,
		SourceRootPath:           f.source.RootPath,
		DestinationIntegrationID: f.destination.ID, DestinationPluginID: f.destination.PluginID,
		DestinationAuthority: "gdrive:test|Games",
		DestinationLabel:     f.destination.Label, DestinationPath: "Games/Game",
		Status: core.SourceMoveStatusQueued, ProgressTotal: len(files), Files: files,
	}
}

func sourceIntegration(source *core.SourceGame) *core.Integration {
	config, _ := json.Marshal(map[string]any{"include_paths": []map[string]any{{"path": "Library", "recursive": true}}})
	return &core.Integration{
		ID: source.IntegrationID, ProfileID: "profile-1", PluginID: source.PluginID,
		Label: "NAS", ConfigJSON: string(config),
	}
}

type memoryMoveStore struct {
	mu   sync.Mutex
	jobs map[string]*core.SourceMoveJob
}

func newMemoryMoveStore() *memoryMoveStore {
	return &memoryMoveStore{jobs: map[string]*core.SourceMoveJob{}}
}

func (s *memoryMoveStore) MarkInFlightJobsInterrupted(context.Context) error { return nil }
func (s *memoryMoveStore) CreateJob(_ context.Context, job *core.SourceMoveJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return errors.New("duplicate")
	}
	s.jobs[job.ID] = cloneJob(job)
	return nil
}
func (s *memoryMoveStore) DeleteJob(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
	return nil
}
func (s *memoryMoveStore) UpdateJob(_ context.Context, job *core.SourceMoveJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = cloneJob(job)
	return nil
}
func (s *memoryMoveStore) ReplaceFiles(_ context.Context, jobID string, files []core.SourceMoveFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[jobID].Files = append([]core.SourceMoveFile(nil), files...)
	return nil
}
func (s *memoryMoveStore) UpdateFile(_ context.Context, jobID string, file core.SourceMoveFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	for index := range job.Files {
		if job.Files[index].Ordinal == file.Ordinal {
			job.Files[index] = file
		}
	}
	return nil
}
func (s *memoryMoveStore) GetJob(_ context.Context, id string) (*core.SourceMoveJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneJob(s.jobs[id]), nil
}
func (s *memoryMoveStore) ListJobs(context.Context, int) ([]*core.SourceMoveJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*core.SourceMoveJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		result = append(result, cloneJob(job))
	}
	return result, nil
}

type fakeMoveGameStore struct {
	game      *core.CanonicalGame
	source    *core.SourceGame
	exclusive bool
}

func (s *fakeMoveGameStore) GetCanonicalGameByID(context.Context, string) (*core.CanonicalGame, error) {
	return s.game, nil
}
func (s *fakeMoveGameStore) GetSourceGamesForCanonical(context.Context, string) ([]*core.SourceGame, error) {
	return []*core.SourceGame{s.source}, nil
}
func (s *fakeMoveGameStore) IsSourceRootExclusive(context.Context, string, string, string) (bool, error) {
	return s.exclusive, nil
}

type fakeMovePlugins struct {
	events     *eventLog
	manifests  map[string]*core.Plugin
	contents   []byte
	failMethod string
}

func (p *fakeMovePlugins) GetPlugin(id string) (*core.Plugin, bool) {
	plugin, ok := p.manifests[id]
	return plugin, ok
}
func (p *fakeMovePlugins) Call(_ context.Context, _ string, method string, params any, result any) error {
	p.events.add(method)
	if method == p.failMethod {
		return errors.New("injected " + method + " failure")
	}
	if method == materializeMethod {
		request := params.(core.SourceMaterializeRequest)
		if err := os.MkdirAll(filepath.Dir(request.DestPath), 0o700); err != nil {
			return err
		}
		return os.WriteFile(request.DestPath, p.contents, 0o600)
	}
	if method == "plugin.check_config" {
		check := result.(*pluginCheckResult)
		check.Status = "ok"
		check.SourceIdentity = "gdrive:test"
	}
	return nil
}

type fakeIntegrationRepo struct {
	byID map[string]*core.Integration
}

func (r *fakeIntegrationRepo) Create(context.Context, *core.Integration) error { return nil }
func (r *fakeIntegrationRepo) Update(context.Context, *core.Integration) error { return nil }
func (r *fakeIntegrationRepo) Delete(context.Context, string) error            { return nil }
func (r *fakeIntegrationRepo) List(context.Context) ([]*core.Integration, error) {
	result := make([]*core.Integration, 0, len(r.byID))
	for _, integration := range r.byID {
		if integration != nil {
			result = append(result, integration)
		}
	}
	return result, nil
}
func (r *fakeIntegrationRepo) GetByID(_ context.Context, id string) (*core.Integration, error) {
	return r.byID[id], nil
}
func (r *fakeIntegrationRepo) ListByPluginID(context.Context, string) ([]*core.Integration, error) {
	return nil, nil
}

type fakeDeletion struct {
	events *eventLog
	err    error
	calls  int
}

func (d *fakeDeletion) DeleteSourceGame(context.Context, string, string) (*core.DeleteSourceGameResult, error) {
	d.calls++
	d.events.add("delete-source")
	if d.err != nil {
		return nil, d.err
	}
	return &core.DeleteSourceGameResult{}, nil
}
func (d *fakeDeletion) PreviewDeleteSourceGame(context.Context, string, string) (*core.DeleteSourceGamePreview, error) {
	return &core.DeleteSourceGamePreview{
		SourceGameID: "source-1",
		PluginID:     "game-source-smb",
		Action:       "delete",
		Summary:      "1 item will be permanently deleted after verification.",
		Items: []core.DeleteSourceGamePreviewItem{{
			Path: "Library/Game/game.bin", Action: "delete", Size: 7,
		}},
	}, nil
}
func (d *fakeDeletion) DeleteSourceGames(context.Context, []core.SourceGameDeleteSelection) (*core.DeleteSourceGamesResult, error) {
	return nil, nil
}
func (d *fakeDeletion) PreviewDeleteReviewCandidateFiles(context.Context, string) (*core.DeleteSourceGamePreview, error) {
	return nil, nil
}
func (d *fakeDeletion) DeleteReviewCandidateFiles(context.Context, string) (*core.DeleteSourceGameResult, error) {
	return nil, nil
}

type fakeScanner struct {
	events *eventLog
	err    error
}

func (s *fakeScanner) RunScan(context.Context, []string) ([]*core.CanonicalGame, error) {
	s.events.add("scan-destination")
	return nil, s.err
}

type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *eventLog) add(value string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, value)
}
func (l *eventLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

func assertEventBefore(t *testing.T, events []string, before, after string) {
	t.Helper()
	beforeIndex, afterIndex := -1, -1
	for index, event := range events {
		if event == before && beforeIndex < 0 {
			beforeIndex = index
		}
		if event == after && afterIndex < 0 {
			afterIndex = index
		}
	}
	if beforeIndex < 0 || afterIndex < 0 || beforeIndex >= afterIndex {
		t.Fatalf("event order %q before %q not found in %v", before, after, events)
	}
}

func containsEvent(events []string, value string) bool {
	for _, event := range events {
		if event == value {
			return true
		}
	}
	return false
}

type fakeConfig map[string]string

func (c fakeConfig) Get(key string) string { return c[key] }
func (c fakeConfig) GetInt(string) int     { return 0 }
func (c fakeConfig) GetBool(string) bool   { return false }
func (c fakeConfig) Validate() error       { return nil }

type fakeLogger struct{}

func (fakeLogger) Info(string, ...any)         {}
func (fakeLogger) Error(string, error, ...any) {}
func (fakeLogger) Debug(string, ...any)        {}
func (fakeLogger) Warn(string, ...any)         {}
