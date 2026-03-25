package core

import (
	"fmt"
	"time"
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	LastLoginAt  time.Time `json:"last_login_at"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Integration struct {
	ID              string    `json:"id"`
	PluginID        string    `json:"plugin_id"`
	Label           string    `json:"label"`
	ConfigJSON      string    `json:"config_json"`
	IntegrationType string    `json:"integration_type"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Setting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FileEntry is a single entry from a filesystem listing.
type FileEntry struct {
	Path    string
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// GroupKind describes whether a game group is ready to play or needs action.
type GroupKind string

const (
	GroupKindSelfContained GroupKind = "self_contained" // files ARE the game, playable as-is
	GroupKindPacked        GroupKind = "packed"         // needs unpacking (installer or compressed archive)
	GroupKindExtras        GroupKind = "extras"         // non-game content (manuals, soundtracks)
	GroupKindUnknown       GroupKind = "unknown"
)

// Platform is the detected platform.
type Platform string

const (
	PlatformWindowsPC Platform = "windows_pc"
	PlatformMSDOS     Platform = "ms_dos"
	PlatformArcade    Platform = "arcade"
	PlatformGBA       Platform = "gba"
	PlatformPS1       Platform = "ps1"
	PlatformPS2       Platform = "ps2"
	PlatformPS3       Platform = "ps3"
	PlatformPSP       Platform = "psp"
	PlatformXbox360   Platform = "xbox_360"
	PlatformScummVM   Platform = "scummvm"
	PlatformUnknown   Platform = "unknown"
)

// GameKind is the kind of game (base, addon, dlc, etc.).
type GameKind string

const (
	GameKindBaseGame  GameKind = "base_game"
	GameKindAddon     GameKind = "addon"
	GameKindDLC       GameKind = "dlc"
	GameKindPatch     GameKind = "patch"
	GameKindExpansion GameKind = "expansion"
	GameKindExtras    GameKind = "extras"
	GameKindUnknown   GameKind = "unknown"
)

// ExternalID links a game to an entry in an external metadata database.
type ExternalID struct {
	Source     string // plugin ID that provided this match
	ExternalID string // the ID in that external system
	URL        string // optional deep link
}

// MediaType identifies the kind of media asset.
type MediaType string

const (
	MediaTypeCover      MediaType = "cover"
	MediaTypeScreenshot MediaType = "screenshot"
	MediaTypeArtwork    MediaType = "artwork"
	MediaTypeLogo       MediaType = "logo"
	MediaTypeVideo      MediaType = "video"
	MediaTypeBackground MediaType = "background"
	MediaTypeIcon       MediaType = "icon"
	MediaTypeCabinet    MediaType = "cabinet"
	MediaTypeMarquee    MediaType = "marquee"
	MediaTypeBanner     MediaType = "banner"
	MediaTypeBoxBack    MediaType = "box_back"
	MediaTypeBoxSide    MediaType = "box_side"
)

// MediaItem holds metadata for a single media asset.
// URL is always populated by resolvers/sources; LocalPath and Hash are
// filled later by a background downloader that caches media on disk.
type MediaItem struct {
	Type      MediaType `json:"type"`
	URL       string    `json:"url"`
	LocalPath string    `json:"local_path,omitempty"`
	Hash      string    `json:"hash,omitempty"`
	Width     int       `json:"width,omitempty"`
	Height    int       `json:"height,omitempty"`
	MimeType  string    `json:"mime_type,omitempty"`
	Source    string    `json:"source,omitempty"`
}

// CompletionTime holds "how long to beat" estimates in hours.
type CompletionTime struct {
	MainStory     float64 `json:"main_story,omitempty"`
	MainExtra     float64 `json:"main_extra,omitempty"`
	Completionist float64 `json:"completionist,omitempty"`
	Source        string  `json:"source,omitempty"`
}

// ResolverMatch stores one metadata resolver's raw match for a game.
// Every match from every resolver is preserved so we can audit decisions
// and recompute the unified view later.
type ResolverMatch struct {
	PluginID     string `json:"plugin_id"`
	Title        string `json:"title,omitempty"`
	Platform     string `json:"platform,omitempty"`
	Kind         string `json:"kind,omitempty"`
	ParentGameID string `json:"parent_game_id,omitempty"`
	ExternalID   string `json:"external_id"`
	URL          string `json:"url,omitempty"`
	Outvoted     bool   `json:"outvoted,omitempty"`

	Description    string          `json:"description,omitempty"`
	ReleaseDate    string          `json:"release_date,omitempty"`
	Genres         []string        `json:"genres,omitempty"`
	Developer      string          `json:"developer,omitempty"`
	Publisher      string          `json:"publisher,omitempty"`
	Media          []MediaItem     `json:"media,omitempty"`
	Rating         float64         `json:"rating,omitempty"`
	MaxPlayers     int             `json:"max_players,omitempty"`
	CompletionTime *CompletionTime `json:"completion_time,omitempty"`
	// Xbox / storefront extras (also mirrored in metadata_json for persistence).
	IsGamePass      bool   `json:"is_game_pass,omitempty"`
	XcloudAvailable bool   `json:"xcloud_available,omitempty"`
	StoreProductID  string `json:"store_product_id,omitempty"`
	XcloudURL       string `json:"xcloud_url,omitempty"`
	// MetadataJSON is the raw DB metadata_json blob (extra fields merged via parseMetadataJSON).
	MetadataJSON string `json:"metadata_json,omitempty"`
}

// Game is the persisted game entity.
type Game struct {
	ID              string
	Title           string
	RawTitle        string // original title from the scanner, before enrichment
	Platform        Platform
	Kind            GameKind
	ParentGameID    string
	GroupKind       GroupKind
	RootPath        string
	IntegrationID   string
	Status          string
	LastSeenAt      *time.Time
	Files           []GameFile
	ExternalIDs     []ExternalID
	ResolverMatches []ResolverMatch

	// Unified metadata: derived from the highest-priority non-outvoted resolver.
	Description    string
	ReleaseDate    string
	Genres         []string
	Developer      string
	Publisher      string
	Media          []MediaItem
	Rating         float64
	MaxPlayers     int
	CompletionTime *CompletionTime
}

// AchievementSet groups all achievements for a game from a single source.
type AchievementSet struct {
	GameID         string        `json:"game_id"`
	Source         string        `json:"source"`           // e.g. "steam", "xbox", "retroachievements"
	ExternalGameID string        `json:"external_game_id"` // ID in the source system
	TotalCount     int           `json:"total_count"`
	UnlockedCount  int           `json:"unlocked_count"`
	TotalPoints    int           `json:"total_points,omitempty"`
	EarnedPoints   int           `json:"earned_points,omitempty"`
	Achievements   []Achievement `json:"achievements"`
	FetchedAt      time.Time     `json:"fetched_at"`
}

// Achievement is a single achievement definition with optional user progress.
type Achievement struct {
	ExternalID   string    `json:"external_id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	LockedIcon   string    `json:"locked_icon,omitempty"`
	UnlockedIcon string    `json:"unlocked_icon,omitempty"`
	Points       int       `json:"points,omitempty"`
	Rarity       float64   `json:"rarity,omitempty"` // percentage of players who earned it
	Unlocked     bool      `json:"unlocked"`
	UnlockedAt   time.Time `json:"unlocked_at,omitempty"`
}

// SourceGame is a game record from a single source integration.
// "Dark Souls from Steam" and "Dark Souls from GOG" are separate SourceGames.
type SourceGame struct {
	ID            string
	IntegrationID string
	PluginID      string
	ExternalID    string // ID in the source system (steam appid, root path hash, etc.)
	RawTitle      string
	Platform      Platform
	Kind          GameKind
	GroupKind     GroupKind
	RootPath      string
	URL           string
	Status        string // "found", "not_found"
	LastSeenAt    *time.Time
	CreatedAt     time.Time

	Files           []GameFile
	ResolverMatches []ResolverMatch
	Media           []MediaRef
}

// MediaAsset is a globally deduplicated media file.
type MediaAsset struct {
	ID        int
	URL       string
	LocalPath string
	Hash      string
	Width     int
	Height    int
	MimeType  string
}

// MediaRef links a source game to a media asset.
type MediaRef struct {
	AssetID int // FK to MediaAsset.ID (0 if not yet persisted)
	Type    MediaType
	URL     string // carried for convenience; canonical URL is in MediaAsset
	Source  string // plugin_id that provided it
	Width   int
	Height  int
	// LocalPath and Hash come from media_assets when loaded for detail views.
	LocalPath string `json:"local_path,omitempty"`
	Hash      string `json:"hash,omitempty"`
	MimeType  string `json:"mime_type,omitempty"`
}

// CanonicalGame is the merged view of one logical game, computed from
// one or more SourceGames that share external IDs.
type CanonicalGame struct {
	ID          string // canonical_id from the link table
	SourceGames []*SourceGame

	// Unified fields (computed, not persisted).
	Title          string
	Platform       Platform
	Kind           GameKind
	Description    string
	ReleaseDate    string
	Genres         []string
	Developer      string
	Publisher      string
	Rating         float64
	MaxPlayers     int
	CompletionTime *CompletionTime
	Media          []MediaRef
	ExternalIDs    []ExternalID

	// Xbox (and similar) flags merged from resolver matches.
	IsGamePass      bool
	XcloudAvailable bool
	StoreProductID  string
	XcloudURL       string
}

// ScanBatch holds everything produced by one scan cycle, validated in memory
// before being written to the DB in a single transaction.
type ScanBatch struct {
	IntegrationID   string
	SourceGames     []*SourceGame
	ResolverMatches map[string][]ResolverMatch // keyed by source_game.ID
	MediaItems      map[string][]MediaRef      // keyed by source_game.ID
}

// Validate checks the batch for internal consistency.
func (b *ScanBatch) Validate() error {
	if b.IntegrationID == "" {
		return fmt.Errorf("integration_id is required")
	}
	for _, sg := range b.SourceGames {
		if sg.ID == "" {
			return fmt.Errorf("source game has empty ID")
		}
		if sg.RawTitle == "" {
			return fmt.Errorf("source game %q has empty raw_title", sg.ID)
		}
	}
	return nil
}

// SyncPayload is the JSON document exchanged during push/pull settings sync.
type SyncPayload struct {
	Version      int               `json:"version"`
	ExportedAt   time.Time         `json:"exported_at"`
	MGAVersion   string            `json:"mga_version"`
	Integrations []SyncIntegration `json:"integrations"`
	Settings     []Setting         `json:"settings"`
}

// SyncIntegration is an integration record within a sync payload.
// ConfigEncrypted holds the AES-256-GCM encrypted config_json (base64).
type SyncIntegration struct {
	PluginID        string    `json:"plugin_id"`
	Label           string    `json:"label"`
	IntegrationType string    `json:"integration_type"`
	ConfigEncrypted string    `json:"config_encrypted"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// PushResult is returned after a successful sync push.
type PushResult struct {
	ExportedAt     time.Time `json:"exported_at"`
	Integrations   int       `json:"integrations"`
	Settings       int       `json:"settings"`
	RemoteVersions int       `json:"remote_versions"`
}

// PullResult is returned after a successful sync pull (merge).
type PullResult struct {
	IntegrationsAdded   int       `json:"integrations_added"`
	IntegrationsUpdated int       `json:"integrations_updated"`
	IntegrationsSkipped int       `json:"integrations_skipped"`
	SettingsAdded       int       `json:"settings_added"`
	SettingsUpdated     int       `json:"settings_updated"`
	SettingsSkipped     int       `json:"settings_skipped"`
	RemoteExportedAt    time.Time `json:"remote_exported_at"`
}

// SyncStatus describes the current sync configuration state.
type SyncStatus struct {
	Configured   bool   `json:"configured"`
	HasStoredKey bool   `json:"has_stored_key"`
	LastPush     string `json:"last_push,omitempty"`
	LastPull     string `json:"last_pull,omitempty"`
}

// LibraryStats is the JSON body for GET /api/stats.
type LibraryStats struct {
	CanonicalGameCount         int            `json:"canonical_game_count"`
	SourceGameFoundCount       int            `json:"source_game_found_count"`
	SourceGameTotalCount       int            `json:"source_game_total_count"`
	ByPlatform                 map[string]int `json:"by_platform"`
	ByKind                     map[string]int `json:"by_kind"`
	ByIntegrationID            map[string]int `json:"by_integration_id"`
	ByPluginID                 map[string]int `json:"by_plugin_id"`
	ByMetadataPluginID         map[string]int `json:"by_metadata_plugin_id"`
	CanonicalWithResolverTitle int            `json:"canonical_with_resolver_title"`
	PercentWithResolverTitle   float64        `json:"percent_with_resolver_title"`
}

// GameListItem is a lightweight game reference returned by integration-scoped queries.
type GameListItem struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Platform Platform `json:"platform"`
}

// FoundSourceGame carries the fields needed for metadata re-enrichment
// without running the discovery phase again.
type FoundSourceGame struct {
	ID            string   `json:"id"`
	IntegrationID string   `json:"integration_id"`
	PluginID      string   `json:"plugin_id"`
	ExternalID    string   `json:"external_id"`
	RawTitle      string   `json:"raw_title"`
	Platform      Platform `json:"platform"`
	Kind          GameKind `json:"kind"`
	GroupKind     GroupKind `json:"group_kind"`
	RootPath      string   `json:"root_path"`
	URL           string   `json:"url,omitempty"`
}

// ScanReport stores the result and diff of a completed scan.
type ScanReport struct {
	ID             string                  `json:"id"`
	StartedAt      time.Time               `json:"started_at"`
	FinishedAt     time.Time               `json:"finished_at"`
	DurationMs     int64                   `json:"duration_ms"`
	MetadataOnly   bool                    `json:"metadata_only"`
	IntegrationIDs []string                `json:"integration_ids"`
	GamesAdded     int                     `json:"games_added"`
	GamesRemoved   int                     `json:"games_removed"`
	GamesUpdated   int                     `json:"games_updated"`
	TotalGames     int                     `json:"total_games"`
	Results        []ScanIntegrationResult `json:"integration_results"`
}

// ScanIntegrationResult is a per-integration breakdown within a ScanReport.
type ScanIntegrationResult struct {
	IntegrationID string `json:"integration_id"`
	Label         string `json:"label"`
	PluginID      string `json:"plugin_id"`
	GamesFound    int    `json:"games_found"`
	GamesAdded    int    `json:"games_added"`
	GamesRemoved  int    `json:"games_removed"`
	Error         string `json:"error,omitempty"`
}

// GameFileRole is the role of a file within a game package.
type GameFileRole string

const (
	GameFileRoleRoot     GameFileRole = "root"
	GameFileRoleRequired GameFileRole = "required"
	GameFileRoleOptional GameFileRole = "optional"
)

// GameFile is a file belonging to a game (persisted).
type GameFile struct {
	GameID   string
	Path     string
	FileName string
	Role     GameFileRole
	FileKind string
	Size     int64
	IsDir    bool
}
