package contentdelivery

import (
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestBuildManifestProducesStableOpaqueSortedFiles(t *testing.T) {
	modified := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	copy := &Copy{
		CanonicalGameID: "canonical-1",
		SourceGame: &core.SourceGame{
			ID:       "copy-1",
			RawTitle: "Game",
			Platform: core.PlatformWindowsPC,
			Files: []core.GameFile{
				{Path: `disc\\data.bin`, Role: core.GameFileRoleRequired, Size: 4, Revision: "rev-1", ModifiedAt: &modified},
				{Path: "game.exe", Role: core.GameFileRoleRoot, Size: 2, ObjectID: "sha256:" + strings.Repeat("a", 64)},
				{Path: "empty-dir", IsDir: true},
			},
		},
	}

	first, err := BuildManifest(copy, Delivery{Mode: core.SourceDeliveryModeDirect, Ready: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildManifest(copy, Delivery{Mode: core.SourceDeliveryModeDirect, Ready: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != second.Revision || first.ETag != second.ETag {
		t.Fatalf("manifest identity is not stable: %+v %+v", first, second)
	}
	if len(first.Files) != 2 || first.Files[0].RelativePath != "disc/data.bin" || first.Files[1].RelativePath != "game.exe" {
		t.Fatalf("unexpected files: %+v", first.Files)
	}
	if first.Files[1].Checksum == nil || first.Files[1].Checksum.Value != strings.Repeat("a", 64) {
		t.Fatalf("explicit checksum missing: %+v", first.Files[1])
	}
	if strings.Contains(first.Files[0].ID, "disc") || len(first.Files[0].ID) != 64 {
		t.Fatalf("file id is not opaque: %q", first.Files[0].ID)
	}

	copy.SourceGame.Files[0].Revision = "rev-2"
	changed, err := BuildManifest(copy, Delivery{Mode: core.SourceDeliveryModeDirect, Ready: true})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Revision == first.Revision || changed.Files[0].Revision == first.Files[0].Revision {
		t.Fatal("revision evidence change did not invalidate manifest")
	}
}

func TestNormalizeRelativePathFailsClosed(t *testing.T) {
	for _, value := range []string{"", "/absolute.bin", `C:\\absolute.bin`, "../escape.bin", "safe/../../escape.bin", "nul\x00.bin"} {
		if got, err := NormalizeRelativePath(value); err == nil {
			t.Fatalf("NormalizeRelativePath(%q) = %q, want error", value, got)
		}
	}
	if got, err := NormalizeRelativePath(`日本語\\game.rom`); err != nil || got != "日本語/game.rom" {
		t.Fatalf("unicode normalization = %q, %v", got, err)
	}
}

func TestBuildManifestRejectsNormalizationCollision(t *testing.T) {
	copy := &Copy{SourceGame: &core.SourceGame{ID: "copy", Files: []core.GameFile{
		{Path: "folder/game.bin"},
		{Path: "folder/./game.bin"},
	}}}
	if _, err := BuildManifest(copy, Delivery{}); err == nil {
		t.Fatal("expected normalized path collision")
	}
}

func TestBuildManifestRejectsNegativeFileSize(t *testing.T) {
	copy := &Copy{SourceGame: &core.SourceGame{ID: "copy", Files: []core.GameFile{{Path: "game.bin", Size: -1}}}}
	if _, err := BuildManifest(copy, Delivery{}); err == nil {
		t.Fatal("expected negative file size to fail closed")
	}
}

func TestValidFileIDRequiresCanonicalSHA256Shape(t *testing.T) {
	valid := FileID("copy", "game.bin")
	if !ValidFileID(valid) {
		t.Fatalf("generated file id is invalid: %q", valid)
	}
	for _, value := range []string{"", "game.bin", strings.ToUpper(valid), valid[:63], valid + "0"} {
		if ValidFileID(value) {
			t.Fatalf("ValidFileID(%q) = true", value)
		}
	}
}
