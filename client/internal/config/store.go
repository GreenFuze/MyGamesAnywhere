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

	"github.com/google/uuid"
)

const (
	SchemaVersion         = 3
	BindingsSchemaVersion = 2
	LegacySchemaVersion   = 1
	MaxBindings           = 16
)

var ErrNotPaired = errors.New("MGA Client is not paired")

// Binding is one server-issued endpoint identity for this device/OS user.
type Binding struct {
	BindingID        string `json:"binding_id"`
	ServerURL        string `json:"server_url"`
	WebSocketURL     string `json:"websocket_url"`
	EndpointID       string `json:"endpoint_id"`
	ClientInstanceID string `json:"client_instance_id"`
	DisplayName      string `json:"display_name"`
	LegacyIdentity   bool   `json:"legacy_identity,omitempty"`
}

func (b Binding) Validate() error {
	for name, value := range map[string]string{
		"binding_id":         b.BindingID,
		"server_url":         b.ServerURL,
		"websocket_url":      b.WebSocketURL,
		"endpoint_id":        b.EndpointID,
		"client_instance_id": b.ClientInstanceID,
		"display_name":       b.DisplayName,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if _, err := uuid.Parse(b.BindingID); err != nil {
		return errors.New("binding_id must be a UUID")
	}
	return nil
}

// Document is the complete, versioned per-user client configuration.
type Document struct {
	SchemaVersion int       `json:"schema_version"`
	Bindings      []Binding `json:"bindings"`
}

func (d Document) Validate() error {
	if d.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported client config schema %d; expected %d", d.SchemaVersion, SchemaVersion)
	}
	if len(d.Bindings) == 0 {
		return errors.New("at least one server binding is required")
	}
	if len(d.Bindings) > MaxBindings {
		return fmt.Errorf("server binding count %d exceeds limit %d", len(d.Bindings), MaxBindings)
	}
	instances := make(map[string]struct{}, len(d.Bindings))
	bindingIDs := make(map[string]struct{}, len(d.Bindings))
	servers := make(map[string]struct{}, len(d.Bindings))
	for index, binding := range d.Bindings {
		if err := binding.Validate(); err != nil {
			return fmt.Errorf("binding %d: %w", index, err)
		}
		bindingID := strings.ToLower(strings.TrimSpace(binding.BindingID))
		if _, exists := bindingIDs[bindingID]; exists {
			return fmt.Errorf("duplicate binding_id %q", binding.BindingID)
		}
		bindingIDs[bindingID] = struct{}{}
		instance := strings.ToLower(strings.TrimSpace(binding.ClientInstanceID))
		if _, exists := instances[instance]; exists {
			return fmt.Errorf("duplicate client_instance_id %q", binding.ClientInstanceID)
		}
		instances[instance] = struct{}{}
		server := strings.ToLower(strings.TrimRight(strings.TrimSpace(binding.ServerURL), "/"))
		if _, exists := servers[server]; exists {
			return fmt.Errorf("duplicate server_url %q", binding.ServerURL)
		}
		servers[server] = struct{}{}
	}
	return nil
}

// LoadResult reports whether the caller must verify the legacy key before
// atomically persisting the in-memory schema-2 representation.
type LoadResult struct {
	Document      Document
	MigrationFrom int
}

type bindingsConfig struct {
	SchemaVersion int             `json:"schema_version"`
	Bindings      []legacyBinding `json:"bindings"`
}

type legacyBinding struct {
	ServerURL        string `json:"server_url"`
	WebSocketURL     string `json:"websocket_url"`
	EndpointID       string `json:"endpoint_id"`
	ClientInstanceID string `json:"client_instance_id"`
	DisplayName      string `json:"display_name"`
	LegacyIdentity   bool   `json:"legacy_identity,omitempty"`
}

type legacyConfig struct {
	SchemaVersion    int    `json:"schema_version"`
	ServerURL        string `json:"server_url"`
	WebSocketURL     string `json:"websocket_url"`
	EndpointID       string `json:"endpoint_id"`
	ClientInstanceID string `json:"client_instance_id"`
	DisplayName      string `json:"display_name"`
}

type schemaHeader struct {
	SchemaVersion int `json:"schema_version"`
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

func (s *Store) Load() (LoadResult, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LoadResult{}, ErrNotPaired
		}
		return LoadResult{}, fmt.Errorf("read client config: %w", err)
	}
	var header schemaHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return LoadResult{}, fmt.Errorf("decode client config header: %w", err)
	}
	switch header.SchemaVersion {
	case SchemaVersion:
		var document Document
		if err := decodeStrict(data, &document); err != nil {
			return LoadResult{}, fmt.Errorf("decode client config: %w", err)
		}
		if err := document.Validate(); err != nil {
			return LoadResult{}, err
		}
		return LoadResult{Document: document}, nil
	case BindingsSchemaVersion:
		var previous bindingsConfig
		if err := decodeStrict(data, &previous); err != nil {
			return LoadResult{}, fmt.Errorf("decode schema-%d client config: %w", BindingsSchemaVersion, err)
		}
		document := Document{SchemaVersion: SchemaVersion, Bindings: make([]Binding, 0, len(previous.Bindings))}
		for _, old := range previous.Bindings {
			document.Bindings = append(document.Bindings, Binding{
				BindingID: uuid.NewString(), ServerURL: old.ServerURL, WebSocketURL: old.WebSocketURL,
				EndpointID: old.EndpointID, ClientInstanceID: old.ClientInstanceID,
				DisplayName: old.DisplayName, LegacyIdentity: old.LegacyIdentity,
			})
		}
		if err := document.Validate(); err != nil {
			return LoadResult{}, fmt.Errorf("schema-%d client config: %w", BindingsSchemaVersion, err)
		}
		return LoadResult{Document: document, MigrationFrom: BindingsSchemaVersion}, nil
	case LegacySchemaVersion:
		var legacy legacyConfig
		if err := decodeStrict(data, &legacy); err != nil {
			return LoadResult{}, fmt.Errorf("decode legacy client config: %w", err)
		}
		binding := Binding{
			BindingID: uuid.NewString(),
			ServerURL: legacy.ServerURL, WebSocketURL: legacy.WebSocketURL,
			EndpointID: legacy.EndpointID, ClientInstanceID: legacy.ClientInstanceID,
			DisplayName: legacy.DisplayName, LegacyIdentity: true,
		}
		document := Document{SchemaVersion: SchemaVersion, Bindings: []Binding{binding}}
		if err := document.Validate(); err != nil {
			return LoadResult{}, fmt.Errorf("legacy client config: %w", err)
		}
		return LoadResult{Document: document, MigrationFrom: LegacySchemaVersion}, nil
	default:
		return LoadResult{}, fmt.Errorf("unsupported client config schema %d", header.SchemaVersion)
	}
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("client config contains trailing JSON")
	}
	return nil
}

func (s *Store) Save(document Document) error {
	if err := document.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(document, "", "  ")
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
