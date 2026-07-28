package clientapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func TestManagedFileDownloaderPublishesVerifiedBindingScopedCopy(t *testing.T) {
	files := map[string][]byte{
		"game-token":  []byte("game data"),
		"empty-token": {},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		content, ok := files[token]
		if !ok {
			http.Error(w, "unknown token", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(content)
	}))
	defer server.Close()

	ownershipCatalog, err := OpenOwnershipCatalog(filepath.Join(t.TempDir(), "ownership.json"))
	if err != nil {
		t.Fatal(err)
	}
	ownership, err := NewInstallationOwnership(testBindingOne, server.URL, 2, ownershipCatalog, NewInstallationCoordinator())
	if err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(t.TempDir(), "prepared-copies.json")
	catalog, err := OpenPreparedCopyCatalog(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	downloader, err := NewManagedFileDownloader(server.URL, ownership, catalog)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	downloader.now = func() time.Time { return now }
	base := t.TempDir()
	request := devicev1.FileDownloadRequest{
		SchemaVersion: devicev1.FileDownloadSchemaVersion,
		GameID:        "game-1", SourceGameID: "source-1", Title: "Game",
		DestinationRoot: base, DestinationName: "Game",
		Files: []devicev1.FileDownloadItem{
			testDownloadItem("data/game.bin", files["game-token"], server.URL+"/file", "game-token"),
			testDownloadItem("data/empty.bin", files["empty-token"], "/file", "empty-token"),
		},
	}
	var stages []string
	result, err := downloader.Download(context.Background(), "11111111-2222-4333-8444-555555555555", request, func(update CommandProgressUpdate) error {
		stages = append(stages, update.Stage)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Join(base, "MGA", "127.0.0.1-11111111")
	if !sameLocalPath(result.PreparedRoot, wantRoot) || !sameLocalPath(result.PreparedPath, filepath.Join(wantRoot, "Game")) {
		t.Fatalf("prepared result = %+v", result)
	}
	if content, err := os.ReadFile(filepath.Join(result.PreparedPath, "data", "game.bin")); err != nil || string(content) != "game data" {
		t.Fatalf("prepared content = %q, error = %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(result.PreparedPath, preparedCopyManifestName)); err != nil {
		t.Fatalf("prepared manifest missing: %v", err)
	}
	if !containsString(stages, "download") || !containsString(stages, "verify") || !containsString(stages, "prepare") {
		t.Fatalf("progress stages = %v", stages)
	}
	reloaded, err := OpenPreparedCopyCatalog(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.ListForBinding(testBindingOne); len(got) != 1 || got[0].GameID != "game-1" {
		t.Fatalf("owner catalog records = %+v", got)
	}
	if got := reloaded.ListForBinding(testBindingTwo); len(got) != 0 {
		t.Fatalf("other binding observed records = %+v", got)
	}
	if _, err := downloader.Download(context.Background(), "22222222-3333-4444-8555-666666666666", request, nil); err == nil {
		t.Fatal("existing prepared-copy destination was overwritten")
	}
}

func TestManagedFileDownloaderRejectsOtherServerOrigin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("other-origin request reached the network")
	}))
	defer server.Close()
	ownershipCatalog, _ := OpenOwnershipCatalog(filepath.Join(t.TempDir(), "ownership.json"))
	ownership, _ := NewInstallationOwnership(testBindingOne, server.URL, 1, ownershipCatalog, NewInstallationCoordinator())
	catalog, _ := OpenPreparedCopyCatalog(filepath.Join(t.TempDir(), "prepared-copies.json"))
	downloader, _ := NewManagedFileDownloader(server.URL, ownership, catalog)
	content := []byte("game")
	request := devicev1.FileDownloadRequest{
		SchemaVersion: devicev1.FileDownloadSchemaVersion,
		GameID:        "game-1", SourceGameID: "source-1", Title: "Game",
		DestinationRoot: t.TempDir(), DestinationName: "Game",
		Files: []devicev1.FileDownloadItem{
			testDownloadItem("game.bin", content, "http://different-server.invalid/file", "token"),
		},
	}
	if _, err := downloader.Download(context.Background(), "11111111-2222-4333-8444-555555555555", request, nil); err == nil {
		t.Fatal("other-server download URL was accepted")
	}
}

func testDownloadItem(relative string, content []byte, url, token string) devicev1.FileDownloadItem {
	hash := sha256.Sum256(content)
	return devicev1.FileDownloadItem{
		RelativePath: relative, SizeBytes: uint64(len(content)),
		SHA256: hex.EncodeToString(hash[:]), DownloadURL: url, DownloadToken: token,
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
