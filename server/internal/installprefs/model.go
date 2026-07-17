package installprefs

import (
	"context"
	"time"
)

const ProfileSettingKey = "install_root_template"

type Repository interface {
	GetProfileRoot(ctx context.Context, profileID string) (string, error)
	SetProfileRoot(ctx context.Context, profileID, rootTemplate string, updatedAt time.Time) error
	GetEndpointRoot(ctx context.Context, endpointID string) (string, error)
	SetEndpointRoot(ctx context.Context, endpointID, rootTemplate, updatedByProfileID string, updatedAt time.Time) error
}

type Preference struct {
	ProfileRoot   string `json:"profile_root"`
	EndpointRoot  string `json:"endpoint_root,omitempty"`
	EffectiveRoot string `json:"effective_root"`
	Source        string `json:"source"`
}
