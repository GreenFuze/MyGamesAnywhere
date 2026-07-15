package desktop

import (
	"context"
	"errors"
	"strings"
)

// AgentRunner is the blocking MGA Client agent function hosted by the desktop
// notification-area lifecycle.
type AgentRunner func(context.Context) error

// Options describes the per-user desktop host without exposing platform UI
// details to the device agent.
type Options struct {
	DisplayName string
	LogPath     string
	Version     string
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
	return &Host{options: options}, nil
}
