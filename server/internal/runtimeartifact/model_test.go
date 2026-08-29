package runtimeartifact

import (
	"strings"
	"testing"
)

func validArtifact() Artifact {
	return Artifact{ID: "retroarch-1.20-windows-amd64", PackageID: "retroarch", DisplayName: "RetroArch", Category: CategoryEmulator, Version: "1.20", Channel: "stable", OS: "windows", Architecture: "amd64", LicenseSPDX: "GPL-3.0-only", LicenseURL: "https://example.test/license", UpstreamURL: "https://example.test/retroarch.zip", AcquisitionMode: AcquisitionCached, Redistributable: true, ComplianceState: ComplianceApproved, SHA256: strings.Repeat("a", 64), SizeBytes: 42}
}

func TestArtifactValidationFailsClosed(t *testing.T) {
	if err := validArtifact().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Artifact){
		"firmware":           func(a *Artifact) { a.Category = "firmware" },
		"http":               func(a *Artifact) { a.UpstreamURL = "http://example.test/file" },
		"credential":         func(a *Artifact) { a.UpstreamURL = "https://user:secret@example.test/file" },
		"checksum":           func(a *Artifact) { a.SHA256 = "not-a-digest" },
		"unknown compliance": func(a *Artifact) { a.ComplianceState = "maybe" },
		"upstream bytes":     func(a *Artifact) { a.AcquisitionMode = AcquisitionUpstreamLink },
	} {
		t.Run(name, func(t *testing.T) {
			artifact := validArtifact()
			mutate(&artifact)
			if err := artifact.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
