package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const defaultConfigPath = "config.json"

type configService struct {
	values map[string]any
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
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}
	// Normalize keys to uppercase so Get("PORT") and Get("port") both work
	normalized := make(map[string]any, len(values))
	for k, v := range values {
		normalized[strings.ToUpper(k)] = v
	}
	return &configService{values: normalized}, nil
}

func (c *configService) getRaw(key string) (any, bool) {
	v, ok := c.values[strings.ToUpper(key)]
	return v, ok
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
	required := []string{"PORT", "DB_PATH"}
	for _, key := range required {
		if c.Get(key) == "" {
			return errors.New("required config missing: " + key)
		}
	}
	return nil
}
