package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/buildinfo"
)

// An update asset is well over 100 MB and an HTTP request on this server is
// bounded at 60 seconds, so downloading inside the request meant the update
// only installed when the network happened to be fast enough.
//
// That is not hypothetical. On 2026-09-07 a server fetched 111 MB of a 130 MB
// installer and was killed at 85.6% with "context deadline exceeded". Earlier
// releases had succeeded only by being just fast enough, and the margin narrows
// every time MGA grows.
//
// So the transfer must outlive the request that asked for it.

func TestADownloadOutlivesTheRequestThatStartedIt(t *testing.T) {
	oldVersion := buildinfo.Version
	buildinfo.Version = "1.0.0"
	defer func() { buildinfo.Version = oldVersion }()

	assetBytes := []byte("installer bytes that arrive slowly")
	sum := sha256.Sum256(assetBytes)
	want := hex.EncodeToString(sum[:])

	// The asset is served in two halves with a pause between them, so the
	// request that started the download is long cancelled before the last byte
	// arrives.
	release := make(chan struct{})
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			_, _ = fmt.Fprintf(w, `{
				"version":"1.1.0",
				"assets":[
					{"os":"%s","arch":"%s","type":"portable","name":"mga.zip","url":"%s/mga.zip","sha256":"%s","size":%d}
				]
			}`, runtimeGOOS(), runtimeGOARCH(), server.URL, want, len(assetBytes))
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(assetBytes)))
		half := len(assetBytes) / 2
		_, _ = w.Write(assetBytes[:half])
		w.(http.Flusher).Flush()
		<-release
		_, _ = w.Write(assetBytes[half:])
	}))
	defer server.Close()
	// Registered after server.Close() so it runs before it: defers unwind in
	// reverse, and Close waits for handlers that are still running. Deferring
	// it at all is what makes a failing assertion report itself instead of
	// parking the handler and hanging the package until the binary times out.
	defer closeOnce(release)

	updatesDir := t.TempDir()
	svc := NewService(testConfig{
		"UPDATE_MANIFEST_URL": server.URL + "/manifest.json",
		"APP_INSTALL_TYPE":    "portable",
		"UPDATES_DIR":         updatesDir,
	}, testLogger{})

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	status, err := svc.StartDownload(requestCtx)
	if err != nil {
		t.Fatalf("StartDownload() error = %v", err)
	}
	if !status.DownloadInProgress {
		t.Fatal("StartDownload returned a status that does not say a download is running")
	}

	// The handler returns and its context dies, exactly as it does in a real
	// request. Under the old design this killed the transfer.
	cancelRequest()
	closeOnce(release)

	deadline := time.Now().Add(10 * time.Second)
	for {
		current, statusErr := svc.Status(context.Background())
		if statusErr != nil {
			t.Fatalf("Status() error = %v", statusErr)
		}
		if !current.DownloadInProgress {
			if current.DownloadedSHA256 != want {
				t.Fatalf("download finished with sha %q, want %q (message: %q)",
					current.DownloadedSHA256, want, current.Message)
			}
			if current.DownloadedPath != filepath.Join(updatesDir, "mga.zip") {
				t.Fatalf("DownloadedPath = %q", current.DownloadedPath)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the download never finished after its request was cancelled (message: %q)", current.Message)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestASecondDownloadIsRefusedWhileOneIsRunning(t *testing.T) {
	oldVersion := buildinfo.Version
	buildinfo.Version = "1.0.0"
	defer func() { buildinfo.Version = oldVersion }()

	assetBytes := []byte("installer")
	sum := sha256.Sum256(assetBytes)
	want := hex.EncodeToString(sum[:])

	release := make(chan struct{})
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			_, _ = fmt.Fprintf(w, `{
				"version":"1.1.0",
				"assets":[
					{"os":"%s","arch":"%s","type":"portable","name":"mga.zip","url":"%s/mga.zip","sha256":"%s","size":%d}
				]
			}`, runtimeGOOS(), runtimeGOARCH(), server.URL, want, len(assetBytes))
			return
		}
		<-release
		_, _ = w.Write(assetBytes)
	}))
	defer server.Close()
	defer closeOnce(release)

	svc := NewService(testConfig{
		"UPDATE_MANIFEST_URL": server.URL + "/manifest.json",
		"APP_INSTALL_TYPE":    "portable",
		"UPDATES_DIR":         t.TempDir(),
	}, testLogger{})

	if _, err := svc.StartDownload(context.Background()); err != nil {
		t.Fatalf("first StartDownload() error = %v", err)
	}
	// Impatient clicking must not start a second transfer writing to the same
	// file as the first.
	if _, err := svc.StartDownload(context.Background()); err == nil {
		t.Fatal("a second download was allowed to start while one was already running")
	}
}

// closeOnce releases a handler that may already have been released. Tests here
// close on the happy path and again on the way out, and closing a closed
// channel panics.
func closeOnce(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}
