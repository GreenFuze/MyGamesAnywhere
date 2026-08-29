package legacyretirement

import "time"

type Report struct {
	GeneratedAt     time.Time                   `json:"generated_at"`
	ProfileID       string                      `json:"profile_id"`
	SchemaVersion   int                         `json:"schema_version"`
	RetentionPolicy string                      `json:"retention_policy"`
	RowCounts       map[string]int              `json:"row_counts"`
	Endpoints       []EndpointObservation       `json:"endpoints"`
	Installations   []InstallationObservation   `json:"installations"`
	Preferences     []InstallPreferenceEvidence `json:"install_preferences"`
	Storefront      []StorefrontObservation     `json:"storefront_observations"`
}

type EndpointObservation struct {
	ID              string     `json:"id"`
	DisplayName     string     `json:"display_name"`
	HostName        string     `json:"host_name"`
	Platform        string     `json:"platform"`
	Architecture    string     `json:"architecture"`
	ClientVersion   string     `json:"client_version"`
	ProtocolVersion int        `json:"protocol_version"`
	Status          string     `json:"status"`
	LastSeenAt      *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type InstallationObservation struct {
	EndpointID      string     `json:"endpoint_id"`
	GameID          string     `json:"game_id"`
	SourceGameID    string     `json:"source_game_id"`
	InstallRoot     string     `json:"install_root"`
	InstallPath     string     `json:"install_path"`
	InstallKind     string     `json:"install_kind"`
	InstallerFamily string     `json:"installer_family,omitempty"`
	InstallState    string     `json:"install_state"`
	StateReason     string     `json:"state_reason,omitempty"`
	InstalledAt     time.Time  `json:"installed_at"`
	LastVerifiedAt  *time.Time `json:"last_verified_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type InstallPreferenceEvidence struct {
	EndpointID          string    `json:"endpoint_id"`
	InstallRootTemplate string    `json:"install_root_template"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type StorefrontObservation struct {
	EndpointID   string     `json:"endpoint_id"`
	GameID       string     `json:"game_id"`
	SourceGameID string     `json:"source_game_id"`
	Provider     string     `json:"provider"`
	ProductID    string     `json:"product_id"`
	Title        string     `json:"title"`
	InstallPath  string     `json:"install_path"`
	Installed    bool       `json:"installed"`
	ObservedAt   time.Time  `json:"observed_at"`
	UseGranted   bool       `json:"use_granted"`
	GrantedAt    *time.Time `json:"granted_at,omitempty"`
}
