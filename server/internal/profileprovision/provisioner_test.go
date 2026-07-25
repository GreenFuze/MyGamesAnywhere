package profileprovision

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
)

// stubCatalog serves a fixed plugin list.
type stubCatalog struct{ infos []plugins.PluginInfo }

func (c stubCatalog) ListPlugins() []plugins.PluginInfo { return c.infos }

// stubRepo records created integrations and enforces profile scoping the same
// way the SQLite repository does.
type stubRepo struct {
	existing   []*core.Integration
	created    []*core.Integration
	createErr  error
	listErr    error
	seenProfil []string
}

func (r *stubRepo) Create(ctx context.Context, integration *core.Integration) error {
	if r.createErr != nil {
		return r.createErr
	}
	if core.ProfileIDFromContext(ctx) == "" {
		return core.ErrProfileRequired
	}
	r.seenProfil = append(r.seenProfil, core.ProfileIDFromContext(ctx))
	r.created = append(r.created, integration)
	return nil
}

func (r *stubRepo) Update(context.Context, *core.Integration) error { return nil }
func (r *stubRepo) Delete(context.Context, string) error           { return nil }
func (r *stubRepo) List(ctx context.Context) ([]*core.Integration, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	if core.ProfileIDFromContext(ctx) == "" {
		return nil, core.ErrProfileRequired
	}
	return r.existing, nil
}
func (r *stubRepo) GetByID(context.Context, string) (*core.Integration, error) { return nil, nil }
func (r *stubRepo) ListByPluginID(context.Context, string) ([]*core.Integration, error) {
	return nil, nil
}

// realWorldCatalog mirrors the shipped plugin manifests.
func realWorldCatalog() stubCatalog {
	required := map[string]any{"api_key": map[string]any{"required": true}}
	return stubCatalog{infos: []plugins.PluginInfo{
		// Eligible: zero-setup metadata providers.
		{PluginID: "metadata-gog", Capabilities: []string{"metadata"}},
		{PluginID: "metadata-hltb", Capabilities: []string{"metadata"}},
		{PluginID: "metadata-launchbox", Capabilities: []string{"metadata"}},
		{PluginID: "metadata-mame-dat", Capabilities: []string{"metadata"}},
		{PluginID: "metadata-steam", Capabilities: []string{"metadata"}},
		// Eligible: local-only save storage.
		{PluginID: "save-sync-local-disk", Capabilities: []string{"save_sync"}},
		// Ineligible: required config.
		{PluginID: "metadata-rawg", Capabilities: []string{"metadata"}, ConfigSchema: required},
		{PluginID: "retroachievements", Capabilities: []string{"metadata", "achievements"}, ConfigSchema: required},
		// Ineligible: game sources are player intent.
		{PluginID: "game-source-xbox", Capabilities: []string{"source", "achievements"},
			Provides: []string{"auth.oauth.callback"}},
		{PluginID: "game-source-epic", Capabilities: []string{"source"}},
		// Ineligible: cloud destinations need an account sign-in.
		{PluginID: "save-sync-google-drive", Capabilities: []string{"save_sync"},
			Provides: []string{"auth.oauth.callback"}},
		{PluginID: "sync-settings-google-drive", Capabilities: []string{"sync"},
			Provides: []string{"auth.oauth.callback"}},
	}}
}

func TestEligiblePluginsMatchesZeroSetupPluginsOnly(t *testing.T) {
	p := New(realWorldCatalog(), &stubRepo{}, nil)

	got := make([]string, 0)
	for _, info := range p.EligiblePlugins() {
		got = append(got, info.PluginID)
	}

	want := []string{
		"metadata-gog", "metadata-hltb", "metadata-launchbox",
		"metadata-mame-dat", "metadata-steam", "save-sync-local-disk",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("eligible plugins = %v, want %v", got, want)
	}
}

func TestProvisionDefaultsCreatesConnectionsScopedToNewProfile(t *testing.T) {
	repo := &stubRepo{}
	p := New(realWorldCatalog(), repo, nil)

	// The caller's context belongs to the admin creating the profile; the new
	// profile's connections must still be written under the new profile.
	adminCtx := core.WithProfile(context.Background(), &core.Profile{ID: "admin-profile"})
	created, err := p.ProvisionDefaults(adminCtx, "new-profile")
	if err != nil {
		t.Fatalf("ProvisionDefaults error: %v", err)
	}
	if len(created) != 6 {
		t.Fatalf("created %d connections, want 6", len(created))
	}
	for _, integration := range created {
		if integration.ProfileID != "new-profile" {
			t.Fatalf("connection %s profile = %q, want new-profile", integration.PluginID, integration.ProfileID)
		}
		if integration.ConfigJSON != emptyConfigJSON {
			t.Fatalf("connection %s config = %q, want %q", integration.PluginID, integration.ConfigJSON, emptyConfigJSON)
		}
		if integration.ID == "" || integration.Label == "" || integration.IntegrationType == "" {
			t.Fatalf("connection %s is missing identity/label/type: %+v", integration.PluginID, integration)
		}
	}
	for _, seen := range repo.seenProfil {
		if seen != "new-profile" {
			t.Fatalf("repository call scoped to %q, want new-profile", seen)
		}
	}

	// Labels and types follow the same conventions as hand-created connections.
	if created[0].PluginID != "metadata-gog" || created[0].Label != "GOG Metadata" || created[0].IntegrationType != "metadata" {
		t.Fatalf("first connection = %+v, want GOG Metadata/metadata", created[0])
	}
	if last := created[len(created)-1]; last.Label != "Local Disk Save Sync" || last.IntegrationType != "save_sync" {
		t.Fatalf("last connection = %+v, want Local Disk Save Sync/save_sync", last)
	}
}

func TestProvisionDefaultsIsIdempotentAndSkipsExisting(t *testing.T) {
	repo := &stubRepo{existing: []*core.Integration{
		{ID: "keep-me", PluginID: "metadata-steam", Label: "My Steam Metadata", ConfigJSON: `{"tuned":true}`},
	}}
	p := New(realWorldCatalog(), repo, nil)

	created, err := p.ProvisionDefaults(context.Background(), "new-profile")
	if err != nil {
		t.Fatalf("ProvisionDefaults error: %v", err)
	}
	if len(created) != 5 {
		t.Fatalf("created %d connections, want 5 (metadata-steam already present)", len(created))
	}
	for _, integration := range created {
		if integration.PluginID == "metadata-steam" {
			t.Fatal("existing metadata-steam connection was duplicated")
		}
	}
	if repo.existing[0].Label != "My Steam Metadata" || repo.existing[0].ConfigJSON != `{"tuned":true}` {
		t.Fatalf("existing connection was modified: %+v", repo.existing[0])
	}
}

func TestProvisionDefaultsRequiresProfileAndReportsFailures(t *testing.T) {
	p := New(realWorldCatalog(), &stubRepo{}, nil)
	if _, err := p.ProvisionDefaults(context.Background(), "  "); err == nil {
		t.Fatal("blank profile ID: expected an error")
	}

	createErr := errors.New("disk is full")
	failing := New(realWorldCatalog(), &stubRepo{createErr: createErr}, nil)
	if _, err := failing.ProvisionDefaults(context.Background(), "new-profile"); !errors.Is(err, createErr) {
		t.Fatalf("create failure error = %v, want wrapped %v", err, createErr)
	}

	listErr := errors.New("db offline")
	listFailing := New(realWorldCatalog(), &stubRepo{listErr: listErr}, nil)
	if _, err := listFailing.ProvisionDefaults(context.Background(), "new-profile"); !errors.Is(err, listErr) {
		t.Fatalf("list failure error = %v, want wrapped %v", err, listErr)
	}
}

func TestProvisionDefaultsWithoutCatalogOrRepoDoesNothing(t *testing.T) {
	var nilProvisioner *Provisioner
	if created, err := nilProvisioner.ProvisionDefaults(context.Background(), "p"); err != nil || created != nil {
		t.Fatalf("nil provisioner = (%v, %v), want (nil, nil)", created, err)
	}
	if created, err := New(nil, nil, nil).ProvisionDefaults(context.Background(), "p"); err != nil || created != nil {
		t.Fatalf("empty provisioner = (%v, %v), want (nil, nil)", created, err)
	}
}

func TestLabelFallsBackToHumanizedIdentifier(t *testing.T) {
	if got := Label("metadata-launchbox"); got != "LaunchBox" {
		t.Fatalf("known label = %q, want LaunchBox", got)
	}
	if got := Label("metadata-some-new-provider"); got != "Metadata Some NEW Provider" {
		t.Fatalf("fallback label = %q", got)
	}
}
