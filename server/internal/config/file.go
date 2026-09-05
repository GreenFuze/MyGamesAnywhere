package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// Path returns the file this configuration was loaded from.
func (c *configService) Path() string { return c.path }

// pathed is implemented by configurations that came from a file on disk.
type pathed interface{ Path() string }

// PathOf returns the file a configuration was loaded from, or "" for a
// configuration that has no file behind it (tests use in-memory stubs).
func PathOf(cfg core.Configuration) string {
	if p, ok := cfg.(pathed); ok {
		return p.Path()
	}
	return ""
}

// LoadValues reads the raw configuration file as flat string values, with keys
// normalized the same way NewConfigService normalizes them. It reports what is
// on disk right now, which is not necessarily what the running process loaded
// at startup.
func LoadValues(path string) (map[string]string, error) {
	raw, err := readValues(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		out[key] = formatValue(value)
	}
	return out, nil
}

// SaveValues merges updates into the configuration file, leaving every key it
// does not mention untouched, and replaces the file in one atomic step.
//
// A copy of the previous file is kept alongside it. A configuration that stops
// the server from starting can only be repaired from outside MGA, so the
// previous known-good file is worth the few hundred bytes.
func SaveValues(path string, updates map[string]string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("configuration path is empty")
	}
	if len(updates) == 0 {
		return fmt.Errorf("no configuration values to write")
	}

	values, err := readValues(path)
	if err != nil {
		return err
	}
	for key, value := range updates {
		values[strings.ToUpper(strings.TrimSpace(key))] = value
	}

	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	data = append(data, '\n')

	if previous, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", previous, 0o600); err != nil {
			return fmt.Errorf("keep a copy of the previous configuration: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read configuration %s: %w", path, err)
	}

	// Written beside the target so the rename stays on one volume, then moved
	// into place: a half-written configuration file is never observable.
	temp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return fmt.Errorf("write configuration: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("replace configuration %s: %w", path, err)
	}
	return nil
}

func readValues(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read configuration %s: %w", path, err)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse configuration %s: %w", path, err)
	}
	normalized := make(map[string]any, len(parsed))
	for key, value := range parsed {
		normalized[strings.ToUpper(key)] = value
	}
	return normalized, nil
}

func formatValue(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(value)
	default:
		return fmt.Sprint(value)
	}
}
