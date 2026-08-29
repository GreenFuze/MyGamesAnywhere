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

	// Runtime, save-domain, and prepared-copy evidence complete the recovery
	// picture required by the accepted retirement plan. Every entry is a
	// historical observation: MGA never acts on these paths again.
	EmulatorPreferences     []EmulatorPreferenceEvidence     `json:"emulator_preferences"`
	EmulatorCorePreferences []EmulatorCorePreferenceEvidence `json:"emulator_core_preferences"`
	SaveDomainLinks         []SaveDomainLinkObservation      `json:"save_domain_links"`
	Runtimes                []RuntimeObservation             `json:"runtimes"`
	PreparedCopies          []PreparedCopyObservation        `json:"prepared_copies"`

	// ExcludedSensitiveMaterial names the legacy material this report
	// deliberately never contains, so an owner knows the raw SQLite backup —
	// not this export — is the lossless recovery artifact.
	ExcludedSensitiveMaterial []string `json:"excluded_sensitive_material"`
}

// SensitiveExclusions lists the legacy material that must never appear in the
// human-readable retirement export. Pairing and grant material is reusable
// authentication evidence, and command payloads may embed local paths and
// arguments from retired install/launch work.
func SensitiveExclusions() []string {
	return []string{
		"device_pairing_challenges challenge and verifier hashes",
		"device_grants token hashes and grant secrets",
		"device_endpoints public keys and client key material",
		"device_commands request payloads, results, and error details",
	}
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

// EmulatorPreferenceEvidence records which emulator a retired device had
// chosen for one platform. It is recovery evidence only: MGA no longer
// configures or launches emulators on a device.
type EmulatorPreferenceEvidence struct {
	EndpointID string    `json:"endpoint_id"`
	Platform   string    `json:"platform"`
	EmulatorID string    `json:"emulator_id"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// EmulatorCorePreferenceEvidence records the core selected for one emulator.
type EmulatorCorePreferenceEvidence struct {
	EndpointID string    `json:"endpoint_id"`
	Platform   string    `json:"platform"`
	EmulatorID string    `json:"emulator_id"`
	CoreID     string    `json:"core_id"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SaveDomainLinkObservation is the retired device-local save authority record.
// The referenced saves are user-owned and are never deleted or moved by MGA.
type SaveDomainLinkObservation struct {
	EndpointID               string    `json:"endpoint_id"`
	GameID                   string    `json:"game_id"`
	SourceGameID             string    `json:"source_game_id"`
	RouteKind                string    `json:"route_kind"`
	EmulatorID               string    `json:"emulator_id,omitempty"`
	LocalSaveDomainID        string    `json:"local_save_domain_id"`
	AdapterID                string    `json:"adapter_id"`
	AuthorityState           string    `json:"authority_state"`
	SyncState                string    `json:"sync_state"`
	LastSnapshotManifestHash string    `json:"last_snapshot_manifest_hash,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// RuntimeObservation is one runtime/emulator that a retired device reported in
// its last inventory. It documents what the owner installed locally; it does
// not authorize MGA to deliver or execute that runtime.
type RuntimeObservation struct {
	EndpointID         string    `json:"endpoint_id"`
	RuntimeID          string    `json:"runtime_id"`
	Name               string    `json:"name,omitempty"`
	Version            string    `json:"version,omitempty"`
	Path               string    `json:"path,omitempty"`
	CoreProbeState     string    `json:"core_probe_state,omitempty"`
	FirmwareProbeState string    `json:"firmware_probe_state,omitempty"`
	CapturedAt         time.Time `json:"captured_at"`
}

// PreparedCopyObservation is a retired device-local prepared game copy. The
// referenced directories are user-owned content and are never deleted by MGA.
type PreparedCopyObservation struct {
	EndpointID          string    `json:"endpoint_id"`
	LocalPreparedCopyID string    `json:"local_prepared_copy_id"`
	GameID              string    `json:"game_id,omitempty"`
	SourceGameID        string    `json:"source_game_id,omitempty"`
	Title               string    `json:"title,omitempty"`
	PreparedPath        string    `json:"prepared_path"`
	FileCount           int       `json:"file_count"`
	TotalBytes          uint64    `json:"total_bytes"`
	PreparedAt          time.Time `json:"prepared_at"`
	CapturedAt          time.Time `json:"captured_at"`
}
