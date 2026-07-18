package desktop

import (
	"context"
	"errors"
	"strings"
)

// AgentRunner is the blocking MGA Client agent function hosted by the desktop
// notification-area lifecycle.
type AgentRunner func(context.Context) error

// BindingOption describes one independently removable MGA Server binding.
type BindingOption struct {
	ServerURL string
	Unpair    func() error
}

// Options describes the per-user desktop host without exposing platform UI
// details to the device agent.
type Options struct {
	DisplayName string
	LogPath     string
	Version     string
	Bindings    []BindingOption
}

// Host owns the platform notification-area lifecycle for one client instance.
type Host struct {
	options Options
}

// NewHost validates and constructs a desktop host.
func NewHost(options Options) (*Host, error) {
	if strings.TrimSpace(options.DisplayName) == "" {
		return nil, errors.New("desktop host display name is required")
	}
	if strings.TrimSpace(options.LogPath) == "" {
		return nil, errors.New("desktop host log path is required")
	}
	if strings.TrimSpace(options.Version) == "" {
		return nil, errors.New("desktop host version is required")
	}
	if len(options.Bindings) == 0 {
		return nil, errors.New("desktop host requires at least one server binding")
	}
	for index, binding := range options.Bindings {
		if strings.TrimSpace(binding.ServerURL) == "" {
			return nil, errors.New("desktop host binding server URL is required")
		}
		if binding.Unpair == nil {
			return nil, errors.New("desktop host binding unpair callback is required")
		}
		for previous := 0; previous < index; previous++ {
			if strings.EqualFold(strings.TrimRight(options.Bindings[previous].ServerURL, "/"), strings.TrimRight(binding.ServerURL, "/")) {
				return nil, errors.New("desktop host server bindings must be unique")
			}
		}
	}
	return &Host{options: options}, nil
}
