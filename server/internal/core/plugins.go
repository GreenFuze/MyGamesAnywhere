package core

type PluginManifest struct {
	ID             string         `json:"plugin_id"`
	Version        string         `json:"plugin_version"`
	APIMajor       int            `json:"api_major"`
	Capabilities   []string       `json:"capabilities"` // "source", "storage"
	Kinds          []string       `json:"kinds"`        // Deprecated, use Capabilities
	Provides       []string       `json:"provides"`
	Exec           string         `json:"exec"`
	DefaultTimeout int            `json:"default_timeout_ms"`
	MaxConcurrency int            `json:"max_concurrency"`
	ConfigSchema   map[string]any `json:"config"`
}

type Plugin struct {
	Manifest PluginManifest
	Path     string
	Enabled  bool
}
