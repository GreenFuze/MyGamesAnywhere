package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/runtimeartifact"
	"github.com/go-chi/chi/v5"
)

type fakeRuntimeArtifactService struct {
	artifact *runtimeartifact.Artifact
	opened   *runtimeartifact.OpenResult
	err      error
}

func (s *fakeRuntimeArtifactService) List(context.Context) ([]runtimeartifact.Artifact, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.artifact == nil {
		return []runtimeartifact.Artifact{}, nil
	}
	return []runtimeartifact.Artifact{*s.artifact}, nil
}
func (s *fakeRuntimeArtifactService) Get(context.Context, string) (*runtimeartifact.Artifact, error) {
	return s.artifact, s.err
}
func (s *fakeRuntimeArtifactService) Upsert(_ context.Context, artifact runtimeartifact.Artifact) (*runtimeartifact.Artifact, error) {
	s.artifact = &artifact
	return &artifact, s.err
}
func (s *fakeRuntimeArtifactService) Open(context.Context, string) (*runtimeartifact.OpenResult, error) {
	return s.opened, s.err
}

func runtimeArtifactFixture() runtimeartifact.Artifact {
	return runtimeartifact.Artifact{ID: "retroarch-1", PackageID: "retroarch", DisplayName: "RetroArch", Category: runtimeartifact.CategoryEmulator, Version: "1.0", Channel: "stable", OS: "windows", Architecture: "amd64", Compatibility: json.RawMessage(`{}`), LicenseSPDX: "GPL-3.0-only", UpstreamURL: "https://example.test/retroarch.zip", AcquisitionMode: runtimeartifact.AcquisitionCached, Redistributable: true, ComplianceState: runtimeartifact.ComplianceApproved, SHA256: strings.Repeat("a", 64), SizeBytes: 6, UpdatedAt: time.Unix(1_700_000_000, 0)}
}

func TestRuntimeArtifactControllerRejectsInvalidMutation(t *testing.T) {
	controller, err := NewRuntimeArtifactController(&fakeRuntimeArtifactService{}, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/runtime-artifacts", strings.NewReader(`{"id":"bios","package_id":"bios","display_name":"BIOS","category":"firmware"}`))
	recorder := httptest.NewRecorder()
	controller.Create(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid_artifact") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestRuntimeArtifactControllerServesVerifiedRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact.bin")
	if err := os.WriteFile(path, []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	artifact := runtimeArtifactFixture()
	service := &fakeRuntimeArtifactService{artifact: &artifact, opened: &runtimeartifact.OpenResult{Artifact: artifact, File: file}}
	controller, err := NewRuntimeArtifactController(service, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/api/runtime-artifacts/{artifact_id}/content", controller.Content)
	request := httptest.NewRequest(http.MethodGet, "/api/runtime-artifacts/retroarch-1/content", nil)
	request.Header.Set("Range", "bytes=1-3")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusPartialContent || recorder.Body.String() != "bcd" {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" || recorder.Header().Get("ETag") == "" {
		t.Fatalf("headers = %v", recorder.Header())
	}
}

func TestRuntimeArtifactControllerSanitizesBlockedDelivery(t *testing.T) {
	controller, err := NewRuntimeArtifactController(&fakeRuntimeArtifactService{err: runtimeartifact.ErrDeliveryBlocked}, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/runtime-artifacts/a/content", nil)
	router := chi.NewRouter()
	router.Get("/api/runtime-artifacts/{artifact_id}/content", controller.Content)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || strings.Contains(recorder.Body.String(), "path") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}
