package keystore

import (
	"path/filepath"
	"testing"
)

func TestProfileKeyPathSeparatesOwnersAndRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	first, err := profileKeyPath(root, "profile-a", "sync_key.enc")
	if err != nil {
		t.Fatal(err)
	}
	second, err := profileKeyPath(root, "profile-b", "sync_key.enc")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two profiles resolved to the same protected-key path")
	}
	if first != filepath.Join(root, "profiles", "profile-a", "sync_key.enc") {
		t.Fatalf("profile key path = %q", first)
	}
	for _, unsafeID := range []string{"", "../profile-b", `profile\\b`, "profile/b"} {
		if _, err := profileKeyPath(root, unsafeID, "sync_key.enc"); err == nil {
			t.Fatalf("unsafe profile id %q was accepted", unsafeID)
		}
	}
}
