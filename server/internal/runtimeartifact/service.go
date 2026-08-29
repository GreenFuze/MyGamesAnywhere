package runtimeartifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type Repository interface {
	List(ctx context.Context) ([]Artifact, error)
	Get(ctx context.Context, artifactID string) (*Artifact, error)
	Upsert(ctx context.Context, artifact Artifact) (*Artifact, error)
}

type Configuration interface {
	Get(key string) string
}

type Service struct {
	repository Repository
	root       string
}

type OpenResult struct {
	Artifact Artifact
	File     *os.File
	Path     string
}

func NewService(repository Repository, configuration Configuration) (*Service, error) {
	if repository == nil || configuration == nil {
		return nil, errors.New("runtime artifact repository and configuration are required")
	}
	root := strings.TrimSpace(configuration.Get("RUNTIME_ARTIFACT_ROOT"))
	if root == "" {
		root = "runtime-artifacts"
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime artifact root: %w", err)
	}
	return &Service{repository: repository, root: filepath.Clean(absolute)}, nil
}

func (s *Service) List(ctx context.Context) ([]Artifact, error) {
	if err := requireProfile(ctx); err != nil {
		return nil, err
	}
	return s.repository.List(ctx)
}

func (s *Service) Get(ctx context.Context, artifactID string) (*Artifact, error) {
	if err := requireProfile(ctx); err != nil {
		return nil, err
	}
	artifactID = strings.ToLower(strings.TrimSpace(artifactID))
	if !identifierPattern.MatchString(artifactID) {
		return nil, ErrNotFound
	}
	artifact, err := s.repository.Get(ctx, artifactID)
	if err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, ErrNotFound
	}
	artifact.Normalize()
	if err := artifact.Validate(); err != nil {
		return nil, ErrIntegrity
	}
	return artifact, nil
}

func (s *Service) Upsert(ctx context.Context, artifact Artifact) (*Artifact, error) {
	if err := requireProfile(ctx); err != nil {
		return nil, err
	}
	artifact.Normalize()
	if err := artifact.Validate(); err != nil {
		return nil, err
	}
	return s.repository.Upsert(ctx, artifact)
}

func (s *Service) Open(ctx context.Context, artifactID string) (*OpenResult, error) {
	artifact, err := s.Get(ctx, artifactID)
	if err != nil {
		return nil, err
	}
	if !artifact.Deliverable() {
		return nil, ErrDeliveryBlocked
	}
	path := s.blobPath(artifact.SHA256)
	opened, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrDeliveryBlocked
		}
		return nil, fmt.Errorf("open runtime artifact: %w", err)
	}
	info, err := opened.Stat()
	if err != nil || info.IsDir() || info.Size() != artifact.SizeBytes {
		_ = opened.Close()
		return nil, ErrIntegrity
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, opened); err != nil {
		_ = opened.Close()
		return nil, ErrIntegrity
	}
	if hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		_ = opened.Close()
		return nil, ErrIntegrity
	}
	if _, err := opened.Seek(0, io.SeekStart); err != nil {
		_ = opened.Close()
		return nil, ErrIntegrity
	}
	return &OpenResult{Artifact: *artifact, File: opened, Path: path}, nil
}

func (s *Service) blobPath(digest string) string {
	return filepath.Join(s.root, "sha256", digest[:2], digest)
}

func requireProfile(ctx context.Context) error {
	if strings.TrimSpace(core.ProfileIDFromContext(ctx)) == "" {
		return ErrProfileRequired
	}
	return nil
}
