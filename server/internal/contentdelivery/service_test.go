package contentdelivery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
)

type serviceTestRepository struct {
	copy *Copy
}

func (r serviceTestRepository) GetCopy(ctx context.Context, copyID string) (*Copy, error) {
	if core.ProfileIDFromContext(ctx) == "" || r.copy == nil || r.copy.SourceGame == nil || r.copy.SourceGame.ID != copyID {
		return nil, nil
	}
	return r.copy, nil
}

type serviceTestCache struct {
	ready          bool
	resolvedPath   string
	prepared       core.SourceCachePrepareRequest
	job            *core.SourceCacheJobStatus
	cancelAccepted bool
}

func (c *serviceTestCache) DescribeSourceGame(context.Context, core.Platform, *core.SourceGame) []core.SourceDeliveryProfile {
	return nil
}
func (c *serviceTestCache) CanPrepareSourceGame(*core.SourceGame) bool { return true }
func (c *serviceTestCache) IsReady(context.Context, *core.SourceGame, string) (bool, error) {
	return c.ready, nil
}
func (c *serviceTestCache) Prepare(_ context.Context, request core.SourceCachePrepareRequest, _ core.Platform, _ *core.SourceGame) (*core.SourceCacheJobStatus, bool, error) {
	c.prepared = request
	return c.job, false, nil
}
func (c *serviceTestCache) GetJob(context.Context, string) (*core.SourceCacheJobStatus, error) {
	return c.job, nil
}
func (c *serviceTestCache) CancelJob(context.Context, string) (*core.SourceCacheJobStatus, bool, error) {
	return c.job, c.cancelAccepted, nil
}
func (c *serviceTestCache) ListJobs(context.Context, int) ([]*core.SourceCacheJobStatus, error) {
	return nil, nil
}
func (c *serviceTestCache) ListEntries(context.Context) ([]*core.SourceCacheEntry, error) {
	return nil, nil
}
func (c *serviceTestCache) DeleteEntry(context.Context, string) error { return nil }
func (c *serviceTestCache) ClearEntries(context.Context) error        { return nil }
func (c *serviceTestCache) ResolveCachedFile(context.Context, string, string, string) (*core.SourceCacheEntry, *core.SourceCacheEntryFile, string, error) {
	return &core.SourceCacheEntry{Status: "ready"}, &core.SourceCacheEntryFile{Size: 6}, c.resolvedPath, nil
}

func TestServiceDirectManifestAndOpaqueFileResolution(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "folder", "game.bin")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	copy := &Copy{CanonicalGameID: "canonical-1", SourceGame: &core.SourceGame{
		ID: "copy-1", RawTitle: "Game", Platform: core.PlatformWindowsPC, RootPath: root,
		Files: []core.GameFile{{Path: "folder/game.bin", Role: core.GameFileRoleRoot, Size: 6, Revision: "rev-1"}},
	}}
	service, err := NewService(serviceTestRepository{copy: copy}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := core.WithProfile(context.Background(), &core.Profile{ID: "profile-1", Role: core.ProfileRolePlayer})
	manifest, err := service.Manifest(ctx, copy.SourceGame.ID)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Delivery.Mode != core.SourceDeliveryModeDirect || !manifest.Delivery.Ready || len(manifest.Files) != 1 {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	opened, err := service.OpenFile(ctx, copy.SourceGame.ID, manifest.Files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Reader.Close()
	buffer := make([]byte, 6)
	if _, err := opened.Reader.Read(buffer); err != nil || string(buffer) != "abcdef" {
		t.Fatalf("read = %q, %v", buffer, err)
	}
	if _, err := service.OpenFile(ctx, copy.SourceGame.ID, "folder/game.bin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("raw path was accepted as file id: %v", err)
	}
	if _, err := service.Manifest(context.Background(), copy.SourceGame.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ownerless manifest error = %v", err)
	}
}

func TestServiceRejectsChangedDirectSource(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "game.bin")
	if err := os.WriteFile(filePath, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	copy := &Copy{CanonicalGameID: "canonical-1", SourceGame: &core.SourceGame{
		ID: "copy-1", RootPath: root, Files: []core.GameFile{{Path: "game.bin", Size: 6}},
	}}
	service, err := NewService(serviceTestRepository{copy: copy}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := core.WithProfile(context.Background(), &core.Profile{ID: "profile-1"})
	manifest, err := service.Manifest(ctx, copy.SourceGame.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenFile(ctx, copy.SourceGame.ID, manifest.Files[0].ID); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("changed source error = %v", err)
	}
}

func TestServiceMaterializationUsesCompatibilityProfile(t *testing.T) {
	cachedPath := filepath.Join(t.TempDir(), "game.bin")
	if err := os.WriteFile(cachedPath, []byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := &serviceTestCache{job: &core.SourceCacheJobStatus{JobID: "job-1"}, resolvedPath: cachedPath}
	copy := &Copy{CanonicalGameID: "canonical-remote", SourceGame: &core.SourceGame{
		ID: "copy-remote", RawTitle: "Remote", Platform: core.PlatformGBA, PluginID: "game-source-google-drive", RootPath: "Drive/Game",
		Files: []core.GameFile{{Path: "game.bin", Role: core.GameFileRoleRoot, Size: 6}},
	}}
	service, err := NewService(serviceTestRepository{copy: copy}, nil, cache)
	if err != nil {
		t.Fatal(err)
	}
	ctx := core.WithProfile(context.Background(), &core.Profile{ID: "profile-1"})
	manifest, err := service.Manifest(ctx, copy.SourceGame.ID)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Delivery.Mode != core.SourceDeliveryModeMaterialized || !manifest.Delivery.MaterializationRequired {
		t.Fatalf("unexpected remote delivery: %+v", manifest.Delivery)
	}
	if _, err := service.OpenFile(ctx, copy.SourceGame.ID, manifest.Files[0].ID); !errors.Is(err, ErrMaterializationRequired) {
		t.Fatalf("unready open error = %v", err)
	}
	if _, _, err := service.Prepare(ctx, copy.SourceGame.ID); err != nil {
		t.Fatal(err)
	}
	if cache.prepared.Profile != core.FileDeliverySourceProfile || cache.prepared.SourceGameID != copy.SourceGame.ID {
		t.Fatalf("prepare request = %+v", cache.prepared)
	}
	cache.ready = true
	opened, err := service.OpenFile(ctx, copy.SourceGame.ID, manifest.Files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = opened.Reader.Close()
}

func TestSMBDeliveryIsDirectAndPathScopeIsRevalidated(t *testing.T) {
	copy := &Copy{CanonicalGameID: "canonical-smb", SourceGame: &core.SourceGame{
		ID: "copy-smb", PluginID: "game-source-smb", RootPath: "Games/SNES",
		Files: []core.GameFile{{Path: "Games/SNES/game.sfc", Size: 6}},
	}}
	service, err := NewService(serviceTestRepository{copy: copy}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := core.WithProfile(context.Background(), &core.Profile{ID: "profile-1"})
	manifest, err := service.Manifest(ctx, copy.SourceGame.ID)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Delivery.Mode != core.SourceDeliveryModeDirect || !manifest.Delivery.Ready {
		t.Fatalf("unexpected SMB delivery: %+v", manifest.Delivery)
	}

	include := sourcescope.IncludePath{Path: "Games", Recursive: true, ExcludePaths: []string{"Games/Private"}}
	if !smbPathAllowed("Games/SNES/game.sfc", []sourcescope.IncludePath{include}) {
		t.Fatal("in-scope SMB path was rejected")
	}
	if smbPathAllowed("Games/Private/secret.sfc", []sourcescope.IncludePath{include}) {
		t.Fatal("excluded SMB path was accepted")
	}
	if smbPathAllowed("Other/game.sfc", []sourcescope.IncludePath{include}) {
		t.Fatal("out-of-scope SMB path was accepted")
	}
	if _, err := resolveSMBPath("../escape.sfc"); err == nil {
		t.Fatal("SMB traversal path was accepted")
	}
}
