package scan

import (
	"context"
	"encoding/json"
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
