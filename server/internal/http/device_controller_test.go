package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDeviceControllerServesConfiguredClientInstaller(t *testing.T) {
	t.Parallel()

	installerPath := filepath.Join(t.TempDir(), "mga-client.exe")
	want := []byte("test-installer")
	if err := os.WriteFile(installerPath, want, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	controller := &DeviceController{clientInstallerPath: installerPath}

	downloadRecorder := httptest.NewRecorder()
	controller.ClientDownload(downloadRecorder, httptest.NewRequest(http.MethodGet, "/api/devices/client-download", nil))
	var download map[string]string
	if err := json.Unmarshal(downloadRecorder.Body.Bytes(), &download); err != nil {
		t.Fatalf("decode download response: %v", err)
	}
	if download["download_url"] != "/api/devices/client-installer" {
		t.Fatalf("download_url = %q", download["download_url"])
	}

	installerRecorder := httptest.NewRecorder()
	controller.ServeClientInstaller(installerRecorder, httptest.NewRequest(http.MethodGet, "/api/devices/client-installer", nil))
	if installerRecorder.Code != http.StatusOK {
		t.Fatalf("installer status = %d, want 200", installerRecorder.Code)
	}
	if got := installerRecorder.Body.Bytes(); string(got) != string(want) {
		t.Fatalf("installer body = %q, want %q", got, want)
	}
	headRecorder := httptest.NewRecorder()
	controller.ServeClientInstaller(headRecorder, httptest.NewRequest(http.MethodHead, "/api/devices/client-installer", nil))
	if headRecorder.Code != http.StatusOK || headRecorder.Header().Get("Content-Length") == "" {
		t.Fatalf("installer HEAD status = %d, length = %q", headRecorder.Code, headRecorder.Header().Get("Content-Length"))
	}
}

func TestDeviceControllerFallsBackToPublishedInstaller(t *testing.T) {
	t.Parallel()

	controller := &DeviceController{clientInstallerPath: "relative-installer.exe"}
	recorder := httptest.NewRecorder()
	controller.ClientDownload(recorder, httptest.NewRequest(http.MethodGet, "/api/devices/client-download", nil))
	var download map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &download); err != nil {
		t.Fatalf("decode download response: %v", err)
	}
	if download["download_url"] != "https://github.com/GreenFuze/MyGamesAnywhere/releases/latest/download/mga-client-windows-amd64-installer.exe" {
		t.Fatalf("download_url = %q", download["download_url"])
	}
}
