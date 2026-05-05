package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
)

type eventTestLogger struct{}

func (eventTestLogger) Info(string, ...any)         {}
func (eventTestLogger) Error(string, error, ...any) {}
func (eventTestLogger) Debug(string, ...any)        {}
func (eventTestLogger) Warn(string, ...any)         {}

type eventTestMediaDownloadQueue struct {
	calls int
}

func (q *eventTestMediaDownloadQueue) EnqueuePending(context.Context) error {
	q.calls++
	return nil
}

type blockingEventTestMediaDownloadQueue struct {
	started chan struct{}
	release chan struct{}
}

func (q *blockingEventTestMediaDownloadQueue) EnqueuePending(ctx context.Context) error {
	select {
	case q.started <- struct{}{}:
	default:
	}
	select {
	case <-q.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPublishEventWithContextInjectsJobID(t *testing.T) {
	bus := events.New()
	defer bus.Close()

	sub := bus.Subscribe()
	defer bus.Unsubscribe(sub)

	o := &Orchestrator{eventBus: bus, logger: eventTestLogger{}}
	o.publishEventWithContext(WithScanJobID(context.Background(), "job-123"), "scan_started", map[string]any{
		"integration_count": 2,
	})

	select {
	case ev := <-sub:
		if ev.Type != "scan_started" {
			t.Fatalf("event type = %q, want scan_started", ev.Type)
		}
		var payload map[string]any
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if payload["job_id"] != "job-123" {
			t.Fatalf("job_id = %v, want job-123", payload["job_id"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

type sourceFilterTestDiscovery struct {
	plugins map[string]*core.Plugin
}

func (d sourceFilterTestDiscovery) GetPluginIDs() []string { return nil }

func (d sourceFilterTestDiscovery) GetPlugin(pluginID string) (*core.Plugin, bool) {
	plugin, ok := d.plugins[pluginID]
	return plugin, ok
}

func (d sourceFilterTestDiscovery) GetPluginIDsProviding(method string) []string { return nil }

func TestFilterSourceIntegrationsIncludesOnlySourceParticipants(t *testing.T) {
	discovery := sourceFilterTestDiscovery{
		plugins: map[string]*core.Plugin{
			"source-filesystem": {
				Manifest: core.PluginManifest{
					ID:       "source-filesystem",
					Provides: []string{sourceFilesystemListMethod},
				},
			},
			"metadata-steam": {
				Manifest: core.PluginManifest{
					ID:       "metadata-steam",
					Provides: []string{metadataGameLookupMethod},
				},
			},
			"sync-google-drive": {
				Manifest: core.PluginManifest{
					ID:       "sync-google-drive",
					Provides: []string{"sync.push"},
				},
			},
		},
	}

	integrations := []*core.Integration{
		{ID: "source-1", PluginID: "source-filesystem", Label: "Main Library", IntegrationType: "source"},
		{ID: "source-missing", PluginID: "missing-source-plugin", Label: "Broken Source", IntegrationType: "source"},
		{ID: "metadata-1", PluginID: "metadata-steam", Label: "Steam Metadata", IntegrationType: "metadata"},
		{ID: "sync-1", PluginID: "sync-google-drive", Label: "Google Drive", IntegrationType: "sync"},
	}

	filtered := filterSourceIntegrations(discovery, integrations, nil)
	if len(filtered) != 2 {
		t.Fatalf("filtered integrations = %d, want 2", len(filtered))
	}
	if filtered[0].ID != "source-1" || filtered[1].ID != "source-missing" {
		t.Fatalf("unexpected filtered source participants: %+v", filtered)
	}
}

func TestRunScanContinuesAfterStorefrontSourceAuthError(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)
	bus := events.New()
	defer bus.Close()

	sub := bus.Subscribe()
	defer bus.Unsubscribe(sub)

	caller := &mockCaller{
		callFn: func(pluginID, method string, params any) (any, error) {
			if method != sourceGamesListMethod {
				return nil, nil
			}
			switch pluginID {
			case "game-source-steam":
				return nil, fmt.Errorf("plugin error [AUTH_REQUIRED]: steam source requires Steam login before it can scan games")
			case "game-source-epic":
				return map[string]any{
					"games": []map[string]any{{
						"external_id": "epic-1",
						"title":       "Control",
						"platform":    "windows_pc",
					}},
				}, nil
			default:
				return map[string]any{"games": []map[string]any{}}, nil
			}
		},
	}

	discovery := sourceFilterTestDiscovery{
		plugins: map[string]*core.Plugin{
			"game-source-steam": {
				Manifest: core.PluginManifest{
					ID:       "game-source-steam",
					Provides: []string{sourceGamesListMethod},
				},
			},
			"game-source-epic": {
				Manifest: core.PluginManifest{
					ID:       "game-source-epic",
					Provides: []string{sourceGamesListMethod},
				},
			},
		},
	}

	repo := manualReviewTestIntegrationRepo{items: []*core.Integration{
		{ID: "steam-1", PluginID: "game-source-steam", Label: "Steam", IntegrationType: "source", ConfigJSON: `{"api_key":"x"}`},
		{ID: "epic-1", PluginID: "game-source-epic", Label: "Epic", IntegrationType: "source", ConfigJSON: `{}`},
	}}

	queue := &eventTestMediaDownloadQueue{}
	orchestrator := NewOrchestrator(caller, discovery, repo, store, queue, eventTestLogger{})
	orchestrator.SetEventBus(bus)

	games, err := orchestrator.RunScan(ctx, nil)
	if err != nil {
		t.Fatalf("RunScan returned error: %v", err)
	}
	if len(games) != 1 || games[0].Title != "Control" {
		t.Fatalf("games = %+v, want one Epic game", games)
	}
	if queue.calls != 1 {
		t.Fatalf("enqueue calls = %d, want 1", queue.calls)
	}

	reports, err := store.GetScanReports(ctx, 1)
	if err != nil {
		t.Fatalf("GetScanReports: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if len(reports[0].Results) != 2 {
		t.Fatalf("expected 2 integration results, got %d", len(reports[0].Results))
	}

	var sawSteamSkip bool
	timeout := time.After(2 * time.Second)
	for !sawSteamSkip {
		select {
		case ev := <-sub:
			if ev.Type != "scan_integration_skipped" {
				continue
			}
			var payload map[string]any
			if err := json.Unmarshal(ev.Data, &payload); err != nil {
				t.Fatalf("unmarshal skipped payload: %v", err)
			}
			if payload["integration_id"] == "steam-1" {
				sawSteamSkip = true
				if payload["reason"] != "auth_required" {
					t.Fatalf("reason = %v, want auth_required", payload["reason"])
				}
			}
		case <-timeout:
			t.Fatal("timed out waiting for steam skip event")
		}
	}
}

func TestRunScanPreparesMultipleIntegrationsConcurrently(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)

	started := make(chan string, 2)
	release := make(chan struct{})

	caller := &mockCaller{
		callFn: func(pluginID, method string, params any) (any, error) {
			if method != sourceGamesListMethod {
				return nil, nil
			}
			started <- pluginID
			<-release
			return map[string]any{
				"games": []map[string]any{{
					"external_id": fmt.Sprintf("%s-game", pluginID),
					"title":       fmt.Sprintf("Title %s", pluginID),
					"platform":    "windows_pc",
				}},
			}, nil
		},
	}

	discovery := sourceFilterTestDiscovery{
		plugins: map[string]*core.Plugin{
			"game-source-alpha": {
				Manifest: core.PluginManifest{
					ID:       "game-source-alpha",
					Provides: []string{sourceGamesListMethod},
				},
			},
			"game-source-beta": {
				Manifest: core.PluginManifest{
					ID:       "game-source-beta",
					Provides: []string{sourceGamesListMethod},
				},
			},
		},
	}

	repo := manualReviewTestIntegrationRepo{items: []*core.Integration{
		{ID: "alpha-1", PluginID: "game-source-alpha", Label: "Alpha", IntegrationType: "source", ConfigJSON: `{}`},
		{ID: "beta-1", PluginID: "game-source-beta", Label: "Beta", IntegrationType: "source", ConfigJSON: `{}`},
	}}

	orchestrator := NewOrchestrator(caller, discovery, repo, store, &eventTestMediaDownloadQueue{}, eventTestLogger{})

	done := make(chan error, 1)
	go func() {
		_, err := orchestrator.RunScan(ctx, nil)
		done <- err
	}()

	seen := map[string]bool{}
	timeout := time.After(2 * time.Second)
	for len(seen) < 2 {
		select {
		case pluginID := <-started:
			seen[pluginID] = true
		case <-timeout:
			t.Fatalf("RunScan did not start both source integrations concurrently; saw=%v", seen)
		}
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("RunScan returned error: %v", err)
	}

	games, err := store.GetCanonicalGames(ctx)
	if err != nil {
		t.Fatalf("GetCanonicalGames: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("canonical games = %d, want 2", len(games))
	}
}

func TestRunScanCompletesBeforePendingMediaEnqueueReturns(t *testing.T) {
	ctx := context.Background()
	store := newManualReviewTestStore(t)
	bus := events.New()
	defer bus.Close()

	sub := bus.Subscribe()
	defer bus.Unsubscribe(sub)

	caller := &mockCaller{
		callFn: func(pluginID, method string, params any) (any, error) {
			if method != sourceGamesListMethod {
				return nil, nil
			}
			return map[string]any{
				"games": []map[string]any{{
					"external_id": "drive-1",
					"title":       "Drive Game",
					"platform":    "windows_pc",
				}},
			}, nil
		},
	}
	discovery := sourceFilterTestDiscovery{
		plugins: map[string]*core.Plugin{
			"game-source-google-drive": {
				Manifest: core.PluginManifest{
					ID:       "game-source-google-drive",
					Provides: []string{sourceGamesListMethod},
				},
			},
		},
	}
	repo := manualReviewTestIntegrationRepo{items: []*core.Integration{
		{ID: "drive-1", PluginID: "game-source-google-drive", Label: "Drive", IntegrationType: "source", ConfigJSON: `{}`},
	}}
	queue := &blockingEventTestMediaDownloadQueue{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	defer close(queue.release)

	orchestrator := NewOrchestrator(caller, discovery, repo, store, queue, eventTestLogger{})
	orchestrator.SetEventBus(bus)

	done := make(chan error, 1)
	go func() {
		_, err := orchestrator.RunScan(ctx, nil)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunScan returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunScan blocked on pending media enqueue")
	}

	select {
	case <-queue.started:
	case <-time.After(2 * time.Second):
		t.Fatal("media enqueue was not started")
	}

	var sawComplete bool
	timeout := time.After(2 * time.Second)
	for !sawComplete {
		select {
		case ev := <-sub:
			if ev.Type == "scan_integration_complete" {
				sawComplete = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for scan_integration_complete")
		}
	}
}
