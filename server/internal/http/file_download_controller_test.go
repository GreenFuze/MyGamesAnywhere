package http

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestPreparedSourcePathAndHashAreSafe(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "data", "game.bin")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("game"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := &core.SourceGame{RootPath: root}
	relative, err := safePreparedRelativePath(source, filePath)
	if err != nil || relative != "data/game.bin" {
		t.Fatalf("relative path = %q, error = %v", relative, err)
	}
	hash, size, err := hashPreparedSourceFile(filePath)
	if err != nil || size != 4 || len(hash) != 64 {
		t.Fatalf("hash = %q, size = %d, error = %v", hash, size, err)
	}
	if _, err := safePreparedRelativePath(source, "../outside.bin"); err == nil {
		t.Fatal("traversal path was accepted")
	}
	if _, _, err := hashPreparedSourceFile(root); err == nil {
		t.Fatal("directory was accepted as a source file")
	}
}

func TestPreparedSourceHashAcceptsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.bin")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	hash, size, err := hashPreparedSourceFile(path)
	if err != nil || size != 0 || hash != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("empty hash = %q, size = %d, error = %v", hash, size, err)
	}
}
