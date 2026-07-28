package v1

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileDownloadRequestValidation(t *testing.T) {
	request := FileDownloadRequest{
		SchemaVersion: FileDownloadSchemaVersion,
		GameID:        "game-1", SourceGameID: "source-1", Title: "Game",
		DestinationName: "Game",
		Files: []FileDownloadItem{{
			RelativePath: "data/empty.dat", SizeBytes: 0,
			SHA256: strings.Repeat("0", 64), DownloadURL: "/api/device-transfers/file", DownloadToken: "token",
		}},
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	request.Files[0].RelativePath = "../outside.dat"
	if err := request.Validate(); err == nil {
		t.Fatal("traversal path was accepted")
	}
	request.Files[0].RelativePath = "data/game.dat"
	request.Files = append(request.Files, request.Files[0])
	if err := request.Validate(); err == nil {
		t.Fatal("duplicate path was accepted")
	}
}

func TestFileDownloadResultAllowsVerifiedEmptyFiles(t *testing.T) {
	root := t.TempDir()
	result := FileDownloadResult{
		GameID: "game-1", SourceGameID: "source-1",
		PreparedRoot: root, PreparedPath: filepath.Join(root, "Game"),
		FileCount: 1, TotalBytes: 0, PreparedAt: time.Now().UTC(),
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("valid empty-file result rejected: %v", err)
	}
}

func TestPreparedCopyInventoryIsVersioned(t *testing.T) {
	observation := PreparedCopyObservation{
		LocalPreparedCopyID: "copy-1", GameID: "game-1", SourceGameID: "source-1",
		Title: "Game", PreparedPath: filepath.Join(t.TempDir(), "Game"),
		FileCount: 1, PreparedAt: time.Now().UTC(),
	}
	current := DeviceInventory{
		SchemaVersion: InventorySchemaVersion, CapturedAt: time.Now().UTC(),
		PreparedCopies: []PreparedCopyObservation{observation},
	}
	if err := current.Validate(); err != nil {
		t.Fatalf("current prepared-copy inventory rejected: %v", err)
	}
	current.SchemaVersion = InventorySchemaVersionWithSaveDomains
	if err := current.Validate(); err == nil {
		t.Fatal("schema 6 accepted schema 7 prepared copies")
	}
}
