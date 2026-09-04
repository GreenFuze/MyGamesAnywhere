package contentdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// localTestIntegrations stands in for the profile-scoped integration
// repository. Returning nothing for an unknown id is exactly how the real
// repository behaves for another profile's row.
type localTestIntegrations struct {
	integration *core.Integration
}

func (r localTestIntegrations) Create(context.Context, *core.Integration) error { return nil }
func (r localTestIntegrations) Update(context.Context, *core.Integration) error { return nil }
func (r localTestIntegrations) Delete(context.Context, string) error            { return nil }
func (r localTestIntegrations) List(context.Context) ([]*core.Integration, error) {
	return nil, nil
}
func (r localTestIntegrations) GetByID(_ context.Context, id string) (*core.Integration, error) {
	if r.integration == nil || r.integration.ID != id {
		return nil, nil
	}
	return r.integration, nil
}
func (r localTestIntegrations) ListByPluginID(context.Context, string) ([]*core.Integration, error) {
	return nil, nil
}

func localConfigJSON(t *testing.T, basePath string, includePaths []map[string]any) string {
	t.Helper()
	// Built through the encoder because a Windows base path carries backslashes
	// that a JSON string literal would eat.
	encoded, err := json.Marshal(map[string]any{"base_path": basePath, "include_paths": includePaths})
	if err != nil {
		t.Fatalf("encode config: %v", err)
	}
	return string(encoded)
}

// localFixture wires one local connection with a single game file on disk.
func localFixture(t *testing.T, includePaths []map[string]any) (*Service, context.Context, *Copy, string) {
	t.Helper()
	base := t.TempDir()
	filePath := filepath.Join(base, "Games", "Chrono", "game.sfc")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("create folders: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("abcdef"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// RootPath is deliberately the relative group directory the scanner
	// actually writes, not an absolute path.
	copy := &Copy{CanonicalGameID: "canonical-local", SourceGame: &core.SourceGame{
		ID:            "copy-local",
		IntegrationID: "integration-local",
		PluginID:      "game-source-local",
		RawTitle:      "Chrono",
		Platform:      core.PlatformWindowsPC,
		RootPath:      "Games/Chrono",
		Files:         []core.GameFile{{Path: "Games/Chrono/game.sfc", Role: core.GameFileRoleRoot, Size: 6}},
	}}
	integrations := localTestIntegrations{integration: &core.Integration{
		ID:         "integration-local",
		PluginID:   "game-source-local",
		ConfigJSON: localConfigJSON(t, base, includePaths),
	}}
	service, err := NewService(serviceTestRepository{copy: copy}, integrations, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := core.WithProfile(context.Background(), &core.Profile{ID: "profile-1", Role: core.ProfileRolePlayer})
	return service, ctx, copy, base
}

func wholeBase() []map[string]any {
	return []map[string]any{{"path": "", "recursive": true}}
}

func TestLocalDeliveryIsDirectDespiteARelativeRootPath(t *testing.T) {
	// The scanner writes RootPath as a relative group directory, so the
	// absolute-path rule alone would never fire for a local source. If this
	// regresses, local games silently fall back to materialization.
	service, ctx, copy, _ := localFixture(t, wholeBase())

	manifest, err := service.Manifest(ctx, copy.SourceGame.ID)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if manifest.Delivery.Mode != core.SourceDeliveryModeDirect || !manifest.Delivery.Ready {
		t.Fatalf("unexpected local delivery: %+v", manifest.Delivery)
	}
	if manifest.Delivery.MaterializationRequired {
		t.Fatal("a local folder should never need materialization")
	}
}

func TestLocalFileIsServedFromTheConfiguredBase(t *testing.T) {
	service, ctx, copy, _ := localFixture(t, wholeBase())

	manifest, err := service.Manifest(ctx, copy.SourceGame.ID)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	opened, err := service.OpenFile(ctx, copy.SourceGame.ID, manifest.Files[0].ID)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer opened.Reader.Close()

	buffer := make([]byte, 6)
	if _, err := opened.Reader.Read(buffer); err != nil || string(buffer) != "abcdef" {
		t.Fatalf("read = %q, %v", buffer, err)
	}
}

func TestLocalFileOutsideTheIncludeScopeIsRefused(t *testing.T) {
	// Scope is a separate gate from path containment: the file is genuinely
	// inside the base, and must still be refused because the operator removed
	// the folder that authorized it.
	service, ctx, copy, _ := localFixture(t, []map[string]any{{"path": "Elsewhere", "recursive": true}})

	manifest, err := service.Manifest(ctx, copy.SourceGame.ID)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if _, err := service.OpenFile(ctx, copy.SourceGame.ID, manifest.Files[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("out-of-scope file was served: %v", err)
	}
}

func TestLocalFileExcludedByTheConnectionIsRefused(t *testing.T) {
	service, ctx, copy, _ := localFixture(t, []map[string]any{{
		"path":          "Games",
		"recursive":     true,
		"exclude_paths": []any{"Games/Chrono"},
	}})

	manifest, err := service.Manifest(ctx, copy.SourceGame.ID)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if _, err := service.OpenFile(ctx, copy.SourceGame.ID, manifest.Files[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("excluded file was served: %v", err)
	}
}

func TestLocalTraversalPathIsRefused(t *testing.T) {
	service, ctx, copy, _ := localFixture(t, wholeBase())

	// A tampered row must not reach outside the base.
	copy.SourceGame.Files = []core.GameFile{{Path: "../outside.txt", Size: 6}}
	if _, err := service.Manifest(ctx, copy.SourceGame.ID); err == nil {
		t.Fatal("a traversal path should not survive manifest construction")
	}
}

func TestLocalDeliveryRefusesAConnectionWithoutAnAbsoluteBase(t *testing.T) {
	for _, basePath := range []string{"", "Games/Relative"} {
		service, ctx, copy, _ := localFixture(t, wholeBase())
		manifest, err := service.Manifest(ctx, copy.SourceGame.ID)
		if err != nil {
			t.Fatalf("manifest: %v", err)
		}

		service.integrationRepo = localTestIntegrations{integration: &core.Integration{
			ID:         "integration-local",
			PluginID:   "game-source-local",
			ConfigJSON: localConfigJSON(t, basePath, wholeBase()),
		}}
		if _, err := service.OpenFile(ctx, copy.SourceGame.ID, manifest.Files[0].ID); err == nil {
			t.Fatalf("base path %q was accepted", basePath)
		}
	}
}

func TestLocalDeliveryRefusesAnIntegrationItCannotSee(t *testing.T) {
	// The repository is profile-scoped, so another profile's connection comes
	// back empty rather than as someone else's config.
	service, ctx, copy, _ := localFixture(t, wholeBase())
	manifest, err := service.Manifest(ctx, copy.SourceGame.ID)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}

	service.integrationRepo = localTestIntegrations{integration: nil}
	if _, err := service.OpenFile(ctx, copy.SourceGame.ID, manifest.Files[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for an unreachable integration, got %v", err)
	}
}

func TestLocalDeliveryRefusesADirectoryServedAsAFile(t *testing.T) {
	service, ctx, copy, _ := localFixture(t, wholeBase())

	copy.SourceGame.Files = []core.GameFile{{Path: "Games/Chrono", Size: 0}}
	manifest, err := service.Manifest(ctx, copy.SourceGame.ID)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if _, err := service.OpenFile(ctx, copy.SourceGame.ID, manifest.Files[0].ID); err == nil {
		t.Fatal("a directory was opened as a file")
	}
}
