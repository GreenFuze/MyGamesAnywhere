package runtimeartifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type memoryRepository struct{ artifact *Artifact }

func (r *memoryRepository) List(context.Context) ([]Artifact, error) {
	if r.artifact == nil {
		return []Artifact{}, nil
	}
	return []Artifact{*r.artifact}, nil
}
func (r *memoryRepository) Get(_ context.Context, id string) (*Artifact, error) {
	if r.artifact == nil || r.artifact.ID != id {
		return nil, nil
	}
	copy := *r.artifact
	return &copy, nil
}
func (r *memoryRepository) Upsert(_ context.Context, artifact Artifact) (*Artifact, error) {
	r.artifact = &artifact
	return r.Get(context.Background(), artifact.ID)
}

type mapConfig map[string]string

func (c mapConfig) Get(key string) string { return c[key] }

func TestServiceVerifiesContentAddressedArtifact(t *testing.T) {
	root := t.TempDir()
	payload := []byte("runtime bytes")
	hash := sha256.Sum256(payload)
	digest := hex.EncodeToString(hash[:])
	artifact := validArtifact()
	artifact.SHA256 = digest
	artifact.SizeBytes = int64(len(payload))
	repository := &memoryRepository{artifact: &artifact}
	service, err := NewService(repository, mapConfig{"RUNTIME_ARTIFACT_ROOT": root})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sha256", digest[:2], digest)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := core.WithProfile(context.Background(), &core.Profile{ID: "profile-1"})
	opened, err := service.Open(ctx, artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = opened.File.Close()
	if opened.Path != path {
		t.Fatalf("path = %q", opened.Path)
	}
	if _, err := service.Open(context.Background(), artifact.ID); !errors.Is(err, ErrProfileRequired) {
		t.Fatalf("ownerless open = %v", err)
	}
	if err := os.WriteFile(path, []byte("tampered bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Open(ctx, artifact.ID); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("tampered open = %v", err)
	}
}

func TestServiceBlocksUnknownAndUpstreamOnlyDelivery(t *testing.T) {
	for _, mutate := range []func(*Artifact){
		func(a *Artifact) { a.ComplianceState = ComplianceUnknown },
		func(a *Artifact) { a.AcquisitionMode = AcquisitionUpstreamLink; a.Redistributable = false },
	} {
		artifact := validArtifact()
		mutate(&artifact)
		service, err := NewService(&memoryRepository{artifact: &artifact}, mapConfig{"RUNTIME_ARTIFACT_ROOT": t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		ctx := core.WithProfile(context.Background(), &core.Profile{ID: "profile-1"})
		if _, err := service.Open(ctx, artifact.ID); !errors.Is(err, ErrDeliveryBlocked) {
			t.Fatalf("blocked open = %v", err)
		}
	}
}
