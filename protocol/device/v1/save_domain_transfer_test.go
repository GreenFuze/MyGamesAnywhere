package v1

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestSaveDomainSnapshotValidation(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot := SaveDomainSnapshot{LocalFingerprint: strings.Repeat("a", 64), CapturedAt: time.Now(), Files: []SaveDomainSnapshotFile{}, ArchiveBase64: base64.StdEncoding.EncodeToString(archive.Bytes())}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	snapshot.Files = []SaveDomainSnapshotFile{{Path: "../outside.sav", Hash: strings.Repeat("b", 64)}}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("unsafe snapshot path accepted")
	}
}

func TestSaveDomainTransferRequestsStayOnBoundedEndpoint(t *testing.T) {
	request := SaveDomainSnapshotRequest{GameID: "game", SourceGameID: "source", Title: "Title", LocalSaveDomainID: "local", UploadURL: "/api/device-transfers/save-domain", UploadToken: "token"}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.UploadURL = "https://example.com/upload"
	if err := request.Validate(); err == nil {
		t.Fatal("external upload URL accepted")
	}
	for _, unsafe := range []string{
		"/api/device-transfers/save-domain-other",
		"/api/device-transfers/save-domain?token=leak",
		"//other-host/api/device-transfers/save-domain",
	} {
		request.UploadURL = unsafe
		if err := request.Validate(); err == nil {
			t.Fatalf("unsafe transfer URL %q accepted", unsafe)
		}
	}
}
