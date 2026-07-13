package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const SchemaVersion = 1

var ErrNotPaired = errors.New("MGA Client is not paired")

type Config struct {
	SchemaVersion    int    `json:"schema_version"`
	ServerURL        string `json:"server_url"`
	WebSocketURL     string `json:"websocket_url"`
	EndpointID       string `json:"endpoint_id"`
	ClientInstanceID string `json:"client_instance_id"`
	DisplayName      string `json:"display_name"`
}

func (c Config) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported client config schema %d; expected %d", c.SchemaVersion, SchemaVersion)
	}
	for name, value := range map[string]string{
		"server_url":         c.ServerURL,
		"websocket_url":      c.WebSocketURL,
		"endpoint_id":        c.EndpointID,
		"client_instance_id": c.ClientInstanceID,
		"display_name":       c.DisplayName,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	return nil
}

type Store struct {
	path string
}

func NewStore(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("config path is required")
	}
	return &Store{path: path}, nil
}

func (s *Store) Load() (Config, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, ErrNotPaired
		}
		return Config{}, fmt.Errorf("read client config: %w", err)
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode client config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("client config contains trailing JSON")
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (s *Store) Save(config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, s.path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace client config: %w", err)
	}
	return nil
}

func (s *Store) Clear() error {
	err := os.Remove(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
