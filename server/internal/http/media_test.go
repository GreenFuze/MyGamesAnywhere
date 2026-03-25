package http

import (
	"path/filepath"
	"testing"
)

func TestResolveUnderMediaRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")

	got, err := resolveUnderMediaRoot(root, filepath.Join("covers", "1.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "covers", "1.jpg")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	for _, bad := range []string{"../secret", "..\\secret", "a/../../etc/passwd"} {
		if _, err := resolveUnderMediaRoot(root, bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}
