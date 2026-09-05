package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	mgaruntime "github.com/GreenFuze/MyGamesAnywhere/server/internal/runtime"
)

// fileBackedConfig stands in for the real configuration service: values as the
// running process loaded them, plus the path they were loaded from. The Path
// method is what makes config.PathOf resolve, exactly as it does in production.
type fileBackedConfig struct {
	values map[string]string
	path   string
}

func (c *fileBackedConfig) Get(key string) string { return c.values[strings.ToUpper(key)] }
func (c *fileBackedConfig) GetInt(key string) int {
	switch c.Get(key) {
	case "5":
		return 5
	default:
		return 0
	}
}
func (c *fileBackedConfig) GetBool(string) bool { return false }
func (c *fileBackedConfig) Validate() error     { return nil }
func (c *fileBackedConfig) Path() string        { return c.path }

func writeConfigFile(t *testing.T, values map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestServerSettingsReportsPendingValuesAndHidesUnknownKeyValues(t *testing.T) {
	// The file has moved on from what the process loaded, and carries a key
	// this server never reads whose value must not travel.
	path := writeConfigFile(t, map[string]string{
		"LISTEN_IP":       "0.0.0.0",
		"PORT":            "8900",
		"DB_PATH":         "./data/db.sqlite",
		"SOME_API_SECRET": "super-secret-value",
	})
	cfg := &fileBackedConfig{path: path, values: map[string]string{
		"LISTEN_IP": "127.0.0.1",
		"PORT":      "8900",
		"DB_PATH":   "./data/db.sqlite",
	}}
	controller := NewSystemController(cfg, nil, noopLogger{})

	response := httptest.NewRecorder()
	controller.GetSettings(response, httptest.NewRequest(http.MethodGet, "/api/server-settings", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "super-secret-value") {
		t.Fatal("a configuration key this server does not read had its value reported")
	}

	var payload serverSettingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.RestartRequired {
		t.Error("restart_required = false, but the file and the process disagree on LISTEN_IP")
	}
	settings := map[string]serverSettingDTO{}
	for _, setting := range payload.Settings {
		settings[setting.Key] = setting
	}
	if got := settings["LISTEN_IP"]; got.Value != "127.0.0.1" || got.PendingValue != "0.0.0.0" {
		t.Errorf("LISTEN_IP = %+v, want the running value with the file value pending", got)
	}
	if got := settings["PORT"]; got.PendingValue != "" {
		t.Errorf("PORT reported pending %q although the file agrees with the process", got.PendingValue)
	}
	if !settings["LISTEN_IP"].Editable || settings["DB_PATH"].Editable {
		t.Error("only the network settings are meant to be editable")
	}
	if len(payload.OtherKeys) != 1 || payload.OtherKeys[0] != "SOME_API_SECRET" {
		t.Errorf("other_keys = %v, want the unread key named once", payload.OtherKeys)
	}
}

func TestSetNetworkRejectsInvalidInputWithoutTouchingTheFile(t *testing.T) {
	original := map[string]string{"LISTEN_IP": "127.0.0.1", "PORT": "8900"}
	path := writeConfigFile(t, original)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &fileBackedConfig{path: path, values: original}
	controller := NewSystemController(cfg, nil, noopLogger{})

	// A valid address paired with a rejected port must not half-apply: this is
	// the case that would otherwise leave the server answering on an address
	// the owner did not agree to.
	for _, body := range []string{
		`{"listen_ip":"not-an-ip","port":"8900"}`,
		`{"listen_ip":"0.0.0.0","port":"99999"}`,
		`{"listen_ip":"0.0.0.0","port":"eight-nine-hundred"}`,
		`{"listen_ip":"","port":"8900"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/server-settings/network", strings.NewReader(body))
		response := httptest.NewRecorder()
		controller.SetNetwork(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", body, response.Code)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(before) {
			t.Fatalf("%s: the configuration file was changed by a rejected request", body)
		}
	}
}

func TestSetNetworkWritesTheFileAndReportsTheChangeAsPending(t *testing.T) {
	path := writeConfigFile(t, map[string]string{
		"LISTEN_IP":   "127.0.0.1",
		"PORT":        "8900",
		"PLUGINS_DIR": "./plugins",
	})
	cfg := &fileBackedConfig{path: path, values: map[string]string{
		"LISTEN_IP":   "127.0.0.1",
		"PORT":        "8900",
		"PLUGINS_DIR": "./plugins",
	}}
	controller := NewSystemController(cfg, nil, noopLogger{})

	request := httptest.NewRequest(http.MethodPost, "/api/server-settings/network",
		strings.NewReader(`{"listen_ip":"0.0.0.0","port":"9100"}`))
	response := httptest.NewRecorder()
	controller.SetNetwork(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}

	var payload serverSettingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.RestartRequired {
		t.Error("restart_required = false immediately after writing a value the process is not using")
	}

	written := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatal(err)
	}
	if written["LISTEN_IP"] != "0.0.0.0" || written["PORT"] != "9100" {
		t.Errorf("file = %v, want the new address and port", written)
	}
	if written["PLUGINS_DIR"] != "./plugins" {
		t.Error("writing the network settings dropped a key it was not asked to change")
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("no copy of the previous configuration was kept: %v", err)
	}
}

func TestGetLogReturnsTheEndOfTheFileAndCanDownloadItWhole(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "mga_server.log")
	lines := make([]string, 0, 500)
	for i := 0; i < 500; i++ {
		lines = append(lines, "line "+strings.Repeat("x", 20)+" "+strconv.Itoa(i))
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &fileBackedConfig{values: map[string]string{"LOG_FILE": logPath}}
	controller := NewSystemController(cfg, nil, noopLogger{})

	response := httptest.NewRecorder()
	controller.GetLog(response, httptest.NewRequest(http.MethodGet, "/api/diagnostics/log?lines=10", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	got := strings.Split(strings.TrimRight(response.Body.String(), "\n"), "\n")
	if len(got) != 10 {
		t.Fatalf("returned %d lines, want 10", len(got))
	}
	if !strings.HasSuffix(got[len(got)-1], " 499") {
		t.Errorf("last line = %q, want the end of the file", got[len(got)-1])
	}

	download := httptest.NewRecorder()
	controller.GetLog(download, httptest.NewRequest(http.MethodGet, "/api/diagnostics/log?download=1", nil))
	if download.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200", download.Code)
	}
	if !strings.Contains(download.Header().Get("Content-Disposition"), "attachment") {
		t.Error("the whole-file response is not offered as a download")
	}
	if !strings.Contains(download.Body.String(), "line") || !strings.HasPrefix(download.Body.String(), "line") {
		t.Error("the download did not start at the beginning of the file")
	}
}

func TestGetLogSaysSoWhenThereIsNoLogFile(t *testing.T) {
	cfg := &fileBackedConfig{values: map[string]string{"LOG_FILE": filepath.Join(t.TempDir(), "missing.log")}}
	controller := NewSystemController(cfg, nil, noopLogger{})
	response := httptest.NewRecorder()
	controller.GetLog(response, httptest.NewRequest(http.MethodGet, "/api/diagnostics/log", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

func TestTailLinesDropsThePartialFirstLineOfAMidFileChunk(t *testing.T) {
	// Reading the last N bytes of a log almost always lands mid-record. A
	// truncated first line is not a log line and must not be shown as one.
	text := "e 41 msg=partial\nline 42\nline 43\n"
	if got := tailLines(text, 10, true); got != "line 42\nline 43" {
		t.Errorf("tailLines kept a partial record: %q", got)
	}
	if got := tailLines(text, 10, false); !strings.HasPrefix(got, "e 41") {
		t.Errorf("tailLines dropped a line it was told was complete: %q", got)
	}
	if got := tailLines("a\nb\nc\n", 2, false); got != "b\nc" {
		t.Errorf("tailLines = %q, want the last two lines", got)
	}
}

func TestNormalizePortAcceptsOnlyRealPorts(t *testing.T) {
	for _, input := range []string{"", "0", "65536", "-1", "80x", " "} {
		if _, err := normalizePort(input); err == nil {
			t.Errorf("normalizePort(%q) was accepted", input)
		}
	}
	if got, err := normalizePort(" 8900 "); err != nil || got != "8900" {
		t.Errorf("normalizePort(\" 8900 \") = %q, %v", got, err)
	}
}

func TestTheLogFileFallsBackToTheLayoutWhenTheFileNeverNamedIt(t *testing.T) {
	// A configuration file written before LOG_FILE existed does not mention it,
	// and MGA still writes a log. Reporting nothing there would send someone
	// looking for a file this server is demonstrably filling.
	dir := t.TempDir()
	logPath := filepath.Join(dir, "logs", "mga_server.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("time=... msg=started\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &fileBackedConfig{values: map[string]string{}}
	layout := &mgaruntime.Layout{Mode: mgaruntime.ModePortable, LogFile: logPath}
	controller := NewSystemController(cfg, layout, noopLogger{})

	response := httptest.NewRecorder()
	controller.GetLog(response, httptest.NewRequest(http.MethodGet, "/api/diagnostics/log", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "started") {
		t.Errorf("body = %q, want the log contents", response.Body.String())
	}

	settings := httptest.NewRecorder()
	controller.GetSettings(settings, httptest.NewRequest(http.MethodGet, "/api/server-settings", nil))
	var payload serverSettingsResponse
	if err := json.Unmarshal(settings.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Log == nil || !payload.Log.Available || payload.Log.Path != logPath {
		t.Errorf("log = %+v, want the layout path reported as available", payload.Log)
	}
	if payload.Server.InstallType != string(mgaruntime.ModePortable) {
		t.Errorf("install_type = %q, want the layout mode", payload.Server.InstallType)
	}
	for _, setting := range payload.Settings {
		if setting.Key == "LOG_FILE" && setting.Source != settingSourceRuntime {
			t.Errorf("LOG_FILE source = %q, want %q", setting.Source, settingSourceRuntime)
		}
		if setting.Key == "MEDIA_ROOT" && setting.Source != settingSourceUnset {
			t.Errorf("MEDIA_ROOT source = %q; MGA must not claim a folder it did not choose here", setting.Source)
		}
	}
}

var _ core.Configuration = (*fileBackedConfig)(nil)
