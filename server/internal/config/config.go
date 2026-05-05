package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const defaultConfigPath = "config.json"
const defaultLocalAccessIP = "127.0.0.1"

var (
	currentMu sync.RWMutex
	current   core.Configuration
)

type configService struct {
	values map[string]any
	path   string
}

// NewConfigService loads configuration from a JSON file. If filePath is empty, "config.json" in the current working directory is used.
// Fail-fast: returns an error if the file cannot be read or parsed.
func NewConfigService(filePath string) (core.Configuration, error) {
	if filePath == "" {
		filePath = defaultConfigPath
	}
	path, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("config path: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file at expected path %s: %w", path, err)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}
	// Normalize keys to uppercase so Get("PORT") and Get("port") both work
	normalized := make(map[string]any, len(values))
	for k, v := range values {
		normalized[strings.ToUpper(k)] = v
	}
	cfg := &configService{values: normalized, path: path}
	SetCurrent(cfg)
	return cfg, nil
}

func (c *configService) getRaw(key string) (any, bool) {
	v, ok := c.values[strings.ToUpper(key)]
	return v, ok
}

// SetCurrent stores the process-wide configuration instance.
func SetCurrent(cfg core.Configuration) {
	currentMu.Lock()
	defer currentMu.Unlock()
	current = cfg
}

// Current returns the process-wide configuration instance loaded during startup.
func Current() core.Configuration {
	currentMu.RLock()
	defer currentMu.RUnlock()
	return current
}

func (c *configService) Get(key string) string {
	v, ok := c.getRaw(key)
	if !ok || v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(s)
	default:
		return fmt.Sprint(v)
	}
}

func (c *configService) GetInt(key string) int {
	v, ok := c.getRaw(key)
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		val, err := strconv.Atoi(n)
		if err != nil {
			return 0
		}
		return val
	default:
		return 0
	}
}

func (c *configService) GetBool(key string) bool {
	v, ok := c.getRaw(key)
	if !ok || v == nil {
		return false
	}
	switch b := v.(type) {
	case bool:
		return b
	case string:
		val, err := strconv.ParseBool(b)
		if err != nil {
			return false
		}
		return val
	default:
		return false
	}
}

func (c *configService) Validate() error {
	required := []string{"PORT", "LISTEN_IP", "DB_PATH"}
	for _, key := range required {
		if c.Get(key) == "" {
			return fmt.Errorf("required config missing: %s in %s", key, c.path)
		}
	}
	if _, err := NormalizeListenIP(c.Get("LISTEN_IP")); err != nil {
		return fmt.Errorf("invalid LISTEN_IP in %s: %w", c.path, err)
	}
	return nil
}

// NormalizeListenIP validates and normalizes the server listen IP.
func NormalizeListenIP(value string) (string, error) {
	listenIP := strings.TrimSpace(value)
	if strings.EqualFold(listenIP, "localhost") {
		return defaultLocalAccessIP, nil
	}
	if listenIP == "" {
		return "", errors.New("value is empty")
	}
	if net.ParseIP(listenIP) == nil {
		return "", fmt.Errorf("must be an IP address or localhost, got %q", value)
	}
	return listenIP, nil
}

// ListenIP returns the normalized LISTEN_IP value.
func ListenIP(cfg core.Configuration) (string, error) {
	if cfg == nil {
		return "", errors.New("configuration is not initialized")
	}
	return NormalizeListenIP(cfg.Get("LISTEN_IP"))
}

// Port returns the configured server port.
func Port(cfg core.Configuration) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Get("PORT"))
}

// ListenAddr returns the address the HTTP server should bind.
func ListenAddr(cfg core.Configuration) (string, error) {
	listenIP, err := ListenIP(cfg)
	if err != nil {
		return "", err
	}
	port := Port(cfg)
	if port == "" {
		return "", errors.New("PORT is required")
	}
	return net.JoinHostPort(listenIP, port), nil
}

// LocalAccessHost returns a browser/callback-safe host for server-generated local URLs.
func LocalAccessHost(cfg core.Configuration) (string, error) {
	listenIP, err := ListenIP(cfg)
	if err != nil {
		return "", err
	}
	if listenIP == "0.0.0.0" || listenIP == "::" {
		return defaultLocalAccessIP, nil
	}
	return listenIP, nil
}

// LocalBaseURL returns the local browser/callback-safe base URL for the server.
func LocalBaseURL(cfg core.Configuration) (string, error) {
	host, err := LocalAccessHost(cfg)
	if err != nil {
		return "", err
	}
	port := Port(cfg)
	if port == "" {
		return "", errors.New("PORT is required")
	}
	u := url.URL{Scheme: "http", Host: net.JoinHostPort(host, port)}
	return u.String(), nil
}

const (
	xboxPluginID                    = "game-source-xbox"
	xboxRegisteredOAuthPath         = "/api/auth/callback/game-source-xbox"
	googleRegisteredOAuthPathPrefix = "/auth/google/callback"
)

// OAuthCallbackURL returns the local callback URL for a plugin OAuth flow.
func OAuthCallbackURL(cfg core.Configuration, pluginID string) (string, error) {
	baseURL, err := LocalBaseURL(cfg)
	if err != nil {
		return "", err
	}
	escapedPluginID := url.PathEscape(pluginID)
	// Provider callback URLs are registered externally and must not drift:
	// - Google Drive uses /auth/google/callback/{plugin_id}.
	// - Xbox must keep /api/auth/callback/game-source-xbox. The Microsoft app
	//   registration currently allows:
	//   http://localhost:8900/api/auth/callback/game-source-xbox
	//   http://127.0.0.1:8900/api/auth/callback/game-source-xbox
	//   http://localhost:8900/auth/xbox/callback
	//   http://127.0.0.1:8900/auth/xbox/callback
	// Do not change these paths without updating the provider app registrations.
	if pluginID == xboxPluginID {
		return baseURL + xboxRegisteredOAuthPath, nil
	}
	if IsGoogleDrivePluginID(pluginID) {
		return fmt.Sprintf("%s%s/%s", baseURL, googleRegisteredOAuthPathPrefix, escapedPluginID), nil
	}
	return fmt.Sprintf("%s/api/auth/callback/%s", baseURL, escapedPluginID), nil
}

func IsGoogleDrivePluginID(pluginID string) bool {
	switch pluginID {
	case "game-source-google-drive", "save-sync-google-drive", "sync-settings-google-drive":
		return true
	default:
		return false
	}
}
