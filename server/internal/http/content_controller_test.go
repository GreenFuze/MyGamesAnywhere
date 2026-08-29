package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/contentdelivery"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

type fakeContentDeliveryService struct {
	manifest  *contentdelivery.Manifest
	opened    *contentdelivery.OpenFileResult
	job       *core.SourceCacheJobStatus
	immediate bool
	err       error
}

func (f *fakeContentDeliveryService) Manifest(context.Context, string) (*contentdelivery.Manifest, error) {
	return f.manifest, f.err
}
func (f *fakeContentDeliveryService) OpenFile(context.Context, string, string) (*contentdelivery.OpenFileResult, error) {
	return f.opened, f.err
}
func (f *fakeContentDeliveryService) Prepare(context.Context, string) (*core.SourceCacheJobStatus, bool, error) {
	return f.job, f.immediate, f.err
}
func (f *fakeContentDeliveryService) GetJob(context.Context, string) (*core.SourceCacheJobStatus, error) {
	return f.job, f.err
}
func (f *fakeContentDeliveryService) CancelJob(context.Context, string) (*core.SourceCacheJobStatus, bool, error) {
	return f.job, f.err == nil, f.err
}

func contentRequest(method, target string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	profile := &core.Profile{ID: "profile-content", Role: core.ProfileRolePlayer}
	return request.WithContext(core.WithProfile(request.Context(), profile))
}

func TestContentControllerManifestSupportsETagWithoutPaths(t *testing.T) {
	service := &fakeContentDeliveryService{manifest: &contentdelivery.Manifest{
		SchemaVersion: 1,
		CopyID:        "copy-1",
		Revision:      "revision-1",
		ETag:          `"revision-1"`,
		Delivery:      contentdelivery.Delivery{Mode: core.SourceDeliveryModeDirect, Ready: true},
		Files: []contentdelivery.ManifestFile{{
			ID: "opaque-file", RelativePath: "game/game.bin", Name: "game.bin", Length: 6, Revision: "file-revision", ETag: `"file-revision"`,
		}},
	}}
	controller, err := NewContentController(service, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/api/content/v1/copies/{copy_id}/manifest", controller.Manifest)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, contentRequest(http.MethodGet, "/api/content/v1/copies/copy-1/manifest"))
	if recorder.Code != http.StatusOK || recorder.Header().Get("ETag") != `"revision-1"` {
		t.Fatalf("manifest response = %d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "server-root") || strings.Contains(recorder.Body.String(), "cache-root") {
		t.Fatalf("manifest leaked a server path: %s", recorder.Body.String())
	}

	notModified := httptest.NewRecorder()
	request := contentRequest(http.MethodGet, "/api/content/v1/copies/copy-1/manifest")
	request.Header.Set("If-None-Match", `"revision-1"`)
	router.ServeHTTP(notModified, request)
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Fatalf("conditional manifest = %d body=%q", notModified.Code, notModified.Body.String())
	}
}

func TestContentControllerFileSupportsHeadRangeAndSafeHeaders(t *testing.T) {
	fileID := contentdelivery.FileID("copy-1", "folder/game.bin")
	path := filepath.Join(t.TempDir(), "game.bin")
	if err := os.WriteFile(path, []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	newService := func(t *testing.T) *fakeContentDeliveryService {
		t.Helper()
		opened, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		return &fakeContentDeliveryService{opened: &contentdelivery.OpenFileResult{
			Reader: opened,
			File:   contentdelivery.ManifestFile{ID: fileID, RelativePath: "folder/game.bin", Name: "game.bin", Length: 6, ETag: `"file-revision"`},
			Name:   "game.bin", Size: 6, ModTime: time.Unix(1_700_000_000, 0),
		}}
	}

	for _, test := range []struct {
		name       string
		method     string
		rangeValue string
		wantStatus int
		wantBody   string
	}{
		{name: "head", method: http.MethodHead, wantStatus: http.StatusOK},
		{name: "range", method: http.MethodGet, rangeValue: "bytes=1-3", wantStatus: http.StatusPartialContent, wantBody: "bcd"},
	} {
		t.Run(test.name, func(t *testing.T) {
			controller, err := NewContentController(newService(t), noopLogger{})
			if err != nil {
				t.Fatal(err)
			}
			router := chi.NewRouter()
			router.MethodFunc(test.method, "/api/content/v1/copies/{copy_id}/files/{file_id}", controller.File)
			request := contentRequest(test.method, "/api/content/v1/copies/copy-1/files/"+fileID)
			request.Header.Set("Range", test.rangeValue)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus || recorder.Body.String() != test.wantBody {
				t.Fatalf("response = %d body=%q", recorder.Code, recorder.Body.String())
			}
			if got := recorder.Header().Get("Content-Disposition"); got != "attachment; filename=game.bin" {
				t.Fatalf("content disposition = %q", got)
			}
			if recorder.Header().Get("ETag") != `"file-revision"` || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("unsafe/missing headers: %v", recorder.Header())
			}
		})
	}
}

func TestContentControllerRejectsMultipleRangesBeforeOpening(t *testing.T) {
	fileID := contentdelivery.FileID("copy-1", "folder/game.bin")
	controller, err := NewContentController(&fakeContentDeliveryService{err: errors.New("must not open")}, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/api/content/v1/copies/{copy_id}/files/{file_id}", controller.File)
	request := contentRequest(http.MethodGet, "/api/content/v1/copies/copy-1/files/"+fileID)
	request.Header.Set("Range", "bytes=0-1,4-5")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestedRangeNotSatisfiable || !strings.Contains(recorder.Body.String(), "multiple_ranges_unsupported") {
		t.Fatalf("response = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestContentControllerSanitizesMaterializationFailure(t *testing.T) {
	now := time.Now().UTC()
	service := &fakeContentDeliveryService{job: &core.SourceCacheJobStatus{
		JobID: "job-1", SourceGameID: "copy-1", Status: "failed", Message: `failed C:\\secret\\provider.token`, Error: "credential=secret", CreatedAt: now, UpdatedAt: now,
	}}
	controller, err := NewContentController(service, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/api/content/v1/materializations/{job_id}", controller.GetMaterialization)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, contentRequest(http.MethodGet, "/api/content/v1/materializations/job-1"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "secret") || strings.Contains(recorder.Body.String(), "credential") {
		t.Fatalf("job leaked sensitive error: %s", recorder.Body.String())
	}
	var body materializationJobDTO
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.ErrorCode != "source_materialization_failed" {
		t.Fatalf("unexpected job response: %+v err=%v", body, err)
	}
}

func TestProfileStreamLimiterIsFailFastAndProfileScoped(t *testing.T) {
	limiter := newProfileStreamLimiter(1)
	releaseA, ok := limiter.Acquire("profile-a")
	if !ok {
		t.Fatal("first profile-a stream rejected")
	}
	if _, ok := limiter.Acquire("profile-a"); ok {
		t.Fatal("second profile-a stream should be rejected")
	}
	releaseB, ok := limiter.Acquire("profile-b")
	if !ok {
		t.Fatal("profile-b should have an independent limit")
	}
	releaseA()
	releaseB()
	if release, ok := limiter.Acquire("profile-a"); !ok {
		t.Fatal("released profile-a stream was not reusable")
	} else {
		release()
	}
}
