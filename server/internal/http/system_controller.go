package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/buildinfo"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/config"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	mgaruntime "github.com/GreenFuze/MyGamesAnywhere/server/internal/runtime"
)

// SystemController answers what this server is running on, and produces the
// evidence someone needs when it misbehaves.
//
// It deliberately reports facts rather than sentences: the words a person reads
// belong to the frontend, because more than one frontend consumes this.
type SystemController struct {
	config core.Configuration
	// layout is what Resolve decided at startup. A configuration file written
	// before a key existed simply does not mention it, and the value in use
	// then comes from here rather than from the file.
	layout    *mgaruntime.Layout
	logger    core.Logger
	startedAt time.Time
}

func NewSystemController(cfg core.Configuration, layout *mgaruntime.Layout, logger core.Logger) *SystemController {
	return &SystemController{config: cfg, layout: layout, logger: logger, startedAt: time.Now()}
}

// Where a value came from. The distinction matters to anyone changing one:
// only a value in the file can be edited by editing the file.
const (
	settingSourceFile    = "file"
	settingSourceRuntime = "runtime"
	settingSourceUnset   = "unset"
)

// effectiveValue answers with the value actually in use.
//
// Only two keys are filled in from the layout, and only because the startup
// path does exactly the same: the log file falls back to layout.LogFile in
// main, and the install type is the layout mode. The remaining paths are each
// defaulted by the component that owns them, and those defaults disagree with
// the layout (the file cache and updates fall back to the user cache
// directory), so guessing on their behalf here would report a folder MGA is
// not using.
func (c *SystemController) effectiveValue(key string) (string, string) {
	if value := strings.TrimSpace(c.config.Get(key)); value != "" {
		return value, settingSourceFile
	}
	if c.layout != nil {
		switch key {
		case "LOG_FILE":
			if c.layout.LogFile != "" {
				return c.layout.LogFile, settingSourceRuntime
			}
		case "APP_INSTALL_TYPE":
			if c.layout.Mode != "" {
				return string(c.layout.Mode), settingSourceRuntime
			}
		}
	}
	return "", settingSourceUnset
}

// reportedConfigKeys are the configuration keys the running server actually
// reads. Anything else in the file is named but never valued, so a key added by
// hand, or a provider secret parked there, cannot leak through this endpoint.
var reportedConfigKeys = []string{
	"LISTEN_IP",
	"PORT",
	"APP_INSTALL_TYPE",
	"DB_PATH",
	"MEDIA_ROOT",
	"SOURCE_CACHE_ROOT",
	"PLUGINS_DIR",
	"FRONTEND_DIST",
	"UPDATES_DIR",
	"LOG_FILE",
	"LOG_MAX_SIZE_MB",
	"LOG_MAX_BACKUPS",
	"MEDIA_DOWNLOAD_CONCURRENCY",
	"UPDATE_MANIFEST_URL",
}

// editableConfigKeys are the two an owner has a reason to change from a
// browser: which addresses the server answers on, and which port. Every other
// key names a path this build chose at install time.
var editableConfigKeys = map[string]bool{"LISTEN_IP": true, "PORT": true}

type serverSettingDTO struct {
	Key string `json:"key"`
	// Value is what the running process is using.
	Value string `json:"value"`
	// PendingValue is what the file says, when the file has since been changed
	// and the server has not been restarted.
	PendingValue string `json:"pending_value,omitempty"`
	// Source is "file" when the configuration file names it, "runtime" when MGA
	// chose it at startup, and "unset" when nothing did.
	Source   string `json:"source"`
	Editable bool   `json:"editable"`
}

type serverIdentityDTO struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	BuildDate     string `json:"build_date"`
	InstallType   string `json:"install_type"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	GoVersion     string `json:"go_version"`
	StartedAt     string `json:"started_at"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

type serverSettingsResponse struct {
	Server     serverIdentityDTO  `json:"server"`
	ConfigPath string             `json:"config_path"`
	Settings   []serverSettingDTO `json:"settings"`
	// OtherKeys names configuration keys present in the file that this server
	// does not read. Names only: a value here is not ours to hand out.
	OtherKeys []string `json:"other_keys"`
	// RestartRequired is true when the file and the running process disagree.
	RestartRequired bool         `json:"restart_required"`
	Log             *logFileDTO  `json:"log,omitempty"`
	Storage         []storageDTO `json:"storage"`
}

type logFileDTO struct {
	Path       string `json:"path"`
	SizeBytes  int64  `json:"size_bytes"`
	ModifiedAt string `json:"modified_at,omitempty"`
	Backups    int    `json:"backups"`
	Available  bool   `json:"available"`
}

type storageDTO struct {
	Key       string `json:"key"`
	Path      string `json:"path"`
	Source    string `json:"source"`
	Exists    bool   `json:"exists"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

// GetSettings reports the effective configuration (GET /api/server-settings).
func (c *SystemController) GetSettings(w http.ResponseWriter, r *http.Request) {
	response, err := c.settings()
	if err != nil {
		c.logger.Error("read server settings", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeSystemJSON(w, response)
}

// SetNetwork changes the address and port the server answers on
// (POST /api/server-settings/network). It writes the configuration file and
// takes effect the next time MGA starts; nothing about the running listener
// changes underneath the caller.
func (c *SystemController) SetNetwork(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ListenIP string `json:"listen_ip"`
		Port     string `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "body must be a JSON object with listen_ip and port", http.StatusBadRequest)
		return
	}

	// Both values are validated before either is written, so a rejected port
	// cannot leave a half-applied address behind.
	listenIP, err := config.NormalizeListenIP(body.ListenIP)
	if err != nil {
		http.Error(w, fmt.Sprintf("listen_ip: %v", err), http.StatusBadRequest)
		return
	}
	port, err := normalizePort(body.Port)
	if err != nil {
		http.Error(w, fmt.Sprintf("port: %v", err), http.StatusBadRequest)
		return
	}

	path := config.PathOf(c.config)
	if path == "" {
		http.Error(w, "this server has no configuration file to change", http.StatusConflict)
		return
	}
	if err := config.SaveValues(path, map[string]string{"LISTEN_IP": listenIP, "PORT": port}); err != nil {
		c.logger.Error("save server network settings", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.logger.Info("server network settings changed", "listen_ip", listenIP, "port", port)

	response, err := c.settings()
	if err != nil {
		c.logger.Error("read server settings", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeSystemJSON(w, response)
}

func normalizePort(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("value is empty")
	}
	number, err := strconv.Atoi(trimmed)
	if err != nil {
		return "", fmt.Errorf("must be a number, got %q", value)
	}
	if number < 1 || number > 65535 {
		return "", fmt.Errorf("must be between 1 and 65535, got %d", number)
	}
	return strconv.Itoa(number), nil
}

func (c *SystemController) settings() (*serverSettingsResponse, error) {
	path := config.PathOf(c.config)
	onDisk := map[string]string{}
	if path != "" {
		values, err := config.LoadValues(path)
		if err != nil {
			return nil, err
		}
		onDisk = values
	}

	reported := make(map[string]bool, len(reportedConfigKeys))
	settings := make([]serverSettingDTO, 0, len(reportedConfigKeys))
	restartRequired := false
	for _, key := range reportedConfigKeys {
		reported[key] = true
		value, source := c.effectiveValue(key)
		setting := serverSettingDTO{
			Key:      key,
			Value:    value,
			Source:   source,
			Editable: editableConfigKeys[key],
		}
		if pending, ok := onDisk[key]; ok && pending != setting.Value {
			setting.PendingValue = pending
			restartRequired = true
		}
		settings = append(settings, setting)
	}

	other := make([]string, 0)
	for key := range onDisk {
		if !reported[key] {
			other = append(other, key)
		}
	}
	sort.Strings(other)

	now := time.Now()
	return &serverSettingsResponse{
		Server: serverIdentityDTO{
			Version:       buildinfo.Version,
			Commit:        buildinfo.Commit,
			BuildDate:     buildinfo.BuildDate,
			InstallType:   firstValue(c.effectiveValue("APP_INSTALL_TYPE")),
			OS:            runtime.GOOS,
			Arch:          runtime.GOARCH,
			GoVersion:     runtime.Version(),
			StartedAt:     c.startedAt.UTC().Format(time.RFC3339),
			UptimeSeconds: int64(now.Sub(c.startedAt).Seconds()),
		},
		ConfigPath:      path,
		Settings:        settings,
		OtherKeys:       other,
		RestartRequired: restartRequired,
		Log:             c.logFile(),
		Storage:         c.storage(),
	}, nil
}

func firstValue(value string, _ string) string { return value }

func (c *SystemController) logFile() *logFileDTO {
	path, _ := c.effectiveValue("LOG_FILE")
	if path == "" {
		return nil
	}
	dto := &logFileDTO{Path: path, Backups: c.config.GetInt("LOG_MAX_BACKUPS")}
	info, err := os.Stat(path)
	if err != nil {
		return dto
	}
	dto.Available = true
	dto.SizeBytes = info.Size()
	dto.ModifiedAt = info.ModTime().UTC().Format(time.RFC3339)
	return dto
}

// storage reports where things live and how large the two single files are.
// Directory totals are deliberately not walked here: the media root holds tens
// of thousands of files, and an admin page must not stall on a disk walk.
func (c *SystemController) storage() []storageDTO {
	keys := []string{"DB_PATH", "MEDIA_ROOT", "SOURCE_CACHE_ROOT", "PLUGINS_DIR", "UPDATES_DIR"}
	out := make([]storageDTO, 0, len(keys))
	for _, key := range keys {
		path, source := c.effectiveValue(key)
		entry := storageDTO{Key: key, Path: path, Source: source}
		if info, err := os.Stat(path); err == nil {
			entry.Exists = true
			if !info.IsDir() {
				entry.SizeBytes = info.Size()
			}
		}
		out = append(out, entry)
	}
	return out
}

const (
	defaultLogTailLines = 200
	maxLogTailLines     = 2000
	// The tail is taken from the end of the file rather than by reading all of
	// it: this log is capped at 50 MB by rotation and routinely reaches double
	// digits of megabytes.
	maxLogTailBytes = 512 * 1024
)

// GetLog returns the end of the server log as text, or the whole file as a
// download (GET /api/diagnostics/log). The log records paths, identifiers and
// provider error text, so it is administrator-only like the rest of this
// controller.
func (c *SystemController) GetLog(w http.ResponseWriter, r *http.Request) {
	path, _ := c.effectiveValue("LOG_FILE")
	if path == "" {
		http.Error(w, "this server is not writing a log file", http.StatusNotFound)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "no log file has been written yet", http.StatusNotFound)
			return
		}
		c.logger.Error("open server log", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		c.logger.Error("stat server log", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if r.URL.Query().Get("download") == "1" {
		name := fmt.Sprintf("mga-server-log-%s.txt", time.Now().Format("2006-01-02"))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		http.ServeContent(w, r, name, info.ModTime(), file)
		return
	}

	lines := defaultLogTailLines
	if raw := strings.TrimSpace(r.URL.Query().Get("lines")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			http.Error(w, "lines must be a positive number", http.StatusBadRequest)
			return
		}
		lines = parsed
	}
	if lines > maxLogTailLines {
		lines = maxLogTailLines
	}

	offset := int64(0)
	length := info.Size()
	if length > maxLogTailBytes {
		offset = length - maxLogTailBytes
		length = maxLogTailBytes
	}
	buffer := make([]byte, length)
	if length > 0 {
		if _, err := file.ReadAt(buffer, offset); err != nil {
			c.logger.Error("read server log", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(tailLines(string(buffer), lines, offset > 0)))
}

// tailLines returns the last n complete lines of text. A chunk read from the
// middle of a file almost always starts mid-line, so that first partial line is
// dropped rather than shown as though it were a whole record.
func tailLines(text string, n int, partialStart bool) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(strings.TrimRight(normalized, "\n"), "\n")
	if partialStart && len(lines) > 1 {
		lines = lines[1:]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func writeSystemJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
