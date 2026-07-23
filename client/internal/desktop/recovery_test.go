package desktop

import (
	"path/filepath"
	"testing"
)

func TestSaveDomainClaimDetailsValidate(t *testing.T) {
	valid := SaveDomainClaimDetails{
		Title:       "The Secret of Monkey Island",
		Server:      "http://mga:8900",
		Adapter:     "ScummVM",
		ExactTarget: "scumm:monkey",
		SaveKind:    "ScummVM save files",
		LocalPath:   filepath.Join(t.TempDir(), "saves"),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid details: %v", err)
	}

	unsafe := valid
	unsafe.Title = "Trusted game\nFake server: http://attacker"
	if err := unsafe.Validate(); err == nil {
		t.Fatal("multiline game title was accepted")
	}

	relative := valid
	relative.LocalPath = filepath.Join("relative", "saves")
	if err := relative.Validate(); err == nil {
		t.Fatal("relative save path was accepted")
	}
}
