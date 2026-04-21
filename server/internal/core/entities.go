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
	Path     string
	Name     string
	IsDir    bool
	Size     int64
	ModTime  time.Time
	ObjectID string
	Revision string
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
	PlatformWindowsPC        Platform = "windows_pc"
	PlatformMSDOS            Platform = "ms_dos"
	PlatformArcade           Platform = "arcade"
	PlatformNES              Platform = "nes"
	PlatformSNES             Platform = "snes"
	PlatformGB               Platform = "gb"
	PlatformGBC              Platform = "gbc"
	PlatformGBA              Platform = "gba"
	PlatformN64              Platform = "n64"
	PlatformGenesis          Platform = "genesis"
	PlatformSegaMasterSystem Platform = "sega_master_system"
	PlatformGameGear         Platform = "game_gear"
	PlatformSegaCD           Platform = "sega_cd"
	PlatformSega32X          Platform = "sega_32x"
	PlatformPS1              Platform = "ps1"
	PlatformPS2              Platform = "ps2"
	PlatformPS3              Platform = "ps3"
	PlatformPSP              Platform = "psp"
	PlatformXbox360          Platform = "xbox_360"
	PlatformScummVM          Platform = "scummvm"
	PlatformUnknown          Platform = "unknown"
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
	// ManualSelection marks a sticky user-selected metadata match.
	ManualSelection bool `json:"manual_selection,omitempty"`

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

// AchievementSummary is a lightweight cached aggregate for a canonical game.
type AchievementSummary struct {
	SourceCount   int `json:"source_count"`
	TotalCount    int `json:"total_count"`
	UnlockedCount int `json:"unlocked_count"`
	TotalPoints   int `json:"total_points,omitempty"`
	EarnedPoints  int `json:"earned_points,omitempty"`
}

type CachedAchievementSystemSummary struct {
	Source        string `json:"source"`
	GameCount     int    `json:"game_count"`
	TotalCount    int    `json:"total_count"`
	UnlockedCount int    `json:"unlocked_count"`
	TotalPoints   int    `json:"total_points,omitempty"`
	EarnedPoints  int    `json:"earned_points,omitempty"`
}

type CachedAchievementGameSummary struct {
	Game    *CanonicalGame                   `json:"game"`
	Systems []CachedAchievementSystemSummary `json:"systems"`
}

type CachedAchievementsDashboard struct {
	Totals  AchievementSummary               `json:"totals"`
	Systems []CachedAchievementSystemSummary `json:"systems"`
	Games   []CachedAchievementGameSummary   `json:"games"`
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
	ReviewState   ManualReviewState
	LastSeenAt    *time.Time
	CreatedAt     time.Time
	ManualReview  *ManualReviewDecision

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
	ID          string // stable canonical game id from canonical_games/canonical_source_games_link
	SourceGames []*SourceGame

	// Unified fields (computed, not persisted).
	Title              string
	Platform           Platform
	Kind               GameKind
	Description        string
	ReleaseDate        string
	Genres             []string
	Developer          string
	Publisher          string
	Rating             float64
	MaxPlayers         int
	CompletionTime     *CompletionTime
	Media              []MediaRef
	CoverOverride      *MediaRef
	ExternalIDs        []ExternalID
	AchievementSummary *AchievementSummary

	// Xbox (and similar) flags merged from resolver matches.
	IsGamePass      bool
	XcloudAvailable bool
	StoreProductID  string
	XcloudURL       string
}

// ScanBatch holds everything produced by one scan cycle, validated in memory
// before being written to the DB in a single transaction.
type ScanBatch struct {
	IntegrationID        string
	SourceGames          []*SourceGame
	ResolverMatches      map[string][]ResolverMatch // keyed by source_game.ID
	MediaItems           map[string][]MediaRef      // keyed by source_game.ID
	FilesystemScope      *FilesystemScanScope
	SkipMissingReconcile bool // true for targeted refreshes that are not complete integration scans
}

type FilesystemIncludePath struct {
	Path      string
	Recursive bool
}

type FilesystemScanScope struct {
	PluginID     string
	IncludePaths []FilesystemIncludePath
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

type AboutInfo struct {
	Version       string   `json:"version"`
	Commit        string   `json:"commit"`
	BuildDate     string   `json:"build_date"`
	AuthorCredits []string `json:"author_credits"`
}

type ScanJobProgress struct {
	Current       int    `json:"current"`
	Total         int    `json:"total,omitempty"`
	Unit          string `json:"unit,omitempty"`
	Indeterminate bool   `json:"indeterminate,omitempty"`
}

type ScanJobMetadataProviderStatus struct {
	IntegrationID string           `json:"integration_id"`
	Label         string           `json:"label,omitempty"`
	PluginID      string           `json:"plugin_id,omitempty"`
	Status        string           `json:"status"`
	Phase         string           `json:"phase,omitempty"`
	Progress      *ScanJobProgress `json:"progress,omitempty"`
	Reason        string           `json:"reason,omitempty"`
	Error         string           `json:"error,omitempty"`
}

type ScanJobIntegrationStatus struct {
	IntegrationID         string                          `json:"integration_id"`
	Label                 string                          `json:"label,omitempty"`
	PluginID              string                          `json:"plugin_id,omitempty"`
	Status                string                          `json:"status"`
	Phase                 string                          `json:"phase,omitempty"`
	GamesFound            int                             `json:"games_found,omitempty"`
	SourceProgress        *ScanJobProgress                `json:"source_progress,omitempty"`
	Reason                string                          `json:"reason,omitempty"`
	MetadataPhase         string                          `json:"metadata_phase,omitempty"`
	MetadataIntegrationID string                          `json:"metadata_integration_id,omitempty"`
	MetadataLabel         string                          `json:"metadata_label,omitempty"`
	MetadataPluginID      string                          `json:"metadata_plugin_id,omitempty"`
	MetadataProgress      *ScanJobProgress                `json:"metadata_progress,omitempty"`
	MetadataProviders     []ScanJobMetadataProviderStatus `json:"metadata_providers,omitempty"`
	Error                 string                          `json:"error,omitempty"`
}

type ScanJobRecentEvent struct {
	Type          string         `json:"type"`
	TS            string         `json:"ts,omitempty"`
	Message       string         `json:"message,omitempty"`
	IntegrationID string         `json:"integration_id,omitempty"`
	Label         string         `json:"label,omitempty"`
	Data          map[string]any `json:"data,omitempty"`
}

type ScanJobStatus struct {
	JobID                   string                     `json:"job_id"`
	Status                  string                     `json:"status"`
	MetadataOnly            bool                       `json:"metadata_only"`
	IntegrationIDs          []string                   `json:"integration_ids"`
	StartedAt               string                     `json:"started_at,omitempty"`
	FinishedAt              string                     `json:"finished_at,omitempty"`
	IntegrationCount        int                        `json:"integration_count"`
	IntegrationsCompleted   int                        `json:"integrations_completed"`
	CurrentPhase            string                     `json:"current_phase,omitempty"`
	CurrentIntegrationID    string                     `json:"current_integration_id,omitempty"`
	CurrentIntegrationLabel string                     `json:"current_integration_label,omitempty"`
	Integrations            []ScanJobIntegrationStatus `json:"integrations,omitempty"`
	RecentEvents            []ScanJobRecentEvent       `json:"recent_events,omitempty"`
	ReportID                string                     `json:"report_id,omitempty"`
	Error                   string                     `json:"error,omitempty"`
}

type SaveSyncSlotRef struct {
	CanonicalGameID string `json:"canonical_game_id"`
	SourceGameID    string `json:"source_game_id"`
	Runtime         string `json:"runtime"`
	SlotID          string `json:"slot_id"`
	IntegrationID   string `json:"integration_id"`
}

type SaveSyncListRequest struct {
	CanonicalGameID string `json:"canonical_game_id"`
	SourceGameID    string `json:"source_game_id"`
	Runtime         string `json:"runtime"`
	IntegrationID   string `json:"integration_id"`
}

type SaveSyncSnapshotFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Hash string `json:"hash"`
}

type SaveSyncSnapshot struct {
	ManifestHash    string                 `json:"manifest_hash"`
	CanonicalGameID string                 `json:"canonical_game_id"`
	SourceGameID    string                 `json:"source_game_id"`
	Runtime         string                 `json:"runtime"`
	SlotID          string                 `json:"slot_id"`
	UpdatedAt       time.Time              `json:"updated_at"`
	TotalSize       int64                  `json:"total_size"`
	FileCount       int                    `json:"file_count"`
	Files           []SaveSyncSnapshotFile `json:"files"`
	ArchiveBase64   string                 `json:"archive_base64,omitempty"`
}

type SaveSyncSlotSummary struct {
	SlotID       string `json:"slot_id"`
	Exists       bool   `json:"exists"`
	ManifestHash string `json:"manifest_hash,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	FileCount    int    `json:"file_count,omitempty"`
	TotalSize    int64  `json:"total_size,omitempty"`
}

type SaveSyncConflict struct {
	SlotID             string `json:"slot_id"`
	Message            string `json:"message"`
	RemoteManifestHash string `json:"remote_manifest_hash"`
	RemoteUpdatedAt    string `json:"remote_updated_at"`
	RemoteFileCount    int    `json:"remote_file_count"`
	RemoteTotalSize    int64  `json:"remote_total_size"`
}

type SaveSyncPutRequest struct {
	SaveSyncSlotRef
	BaseManifestHash string           `json:"base_manifest_hash,omitempty"`
	Force            bool             `json:"force"`
	Snapshot         SaveSyncSnapshot `json:"snapshot"`
}

type SaveSyncPutResult struct {
	OK       bool                `json:"ok"`
	Summary  SaveSyncSlotSummary `json:"summary"`
	Conflict *SaveSyncConflict   `json:"conflict,omitempty"`
}

type SaveSyncMigrationScope string

const (
	SaveSyncMigrationScopeAll  SaveSyncMigrationScope = "all"
	SaveSyncMigrationScopeGame SaveSyncMigrationScope = "game"
)

type SaveSyncMigrationRequest struct {
	SourceIntegrationID      string                 `json:"source_integration_id"`
	TargetIntegrationID      string                 `json:"target_integration_id"`
	Scope                    SaveSyncMigrationScope `json:"scope"`
	CanonicalGameID          string                 `json:"canonical_game_id,omitempty"`
	DeleteSourceAfterSuccess bool                   `json:"delete_source_after_success"`
}

type SaveSyncMigrationStatus struct {
	JobID               string                 `json:"job_id"`
	Status              string                 `json:"status"`
	Scope               SaveSyncMigrationScope `json:"scope"`
	SourceIntegrationID string                 `json:"source_integration_id"`
	TargetIntegrationID string                 `json:"target_integration_id"`
	CanonicalGameID     string                 `json:"canonical_game_id,omitempty"`
	StartedAt           string                 `json:"started_at,omitempty"`
	FinishedAt          string                 `json:"finished_at,omitempty"`
	ItemsTotal          int                    `json:"items_total"`
	ItemsCompleted      int                    `json:"items_completed"`
	SlotsMigrated       int                    `json:"slots_migrated"`
	SlotsSkipped        int                    `json:"slots_skipped"`
	Error               string                 `json:"error,omitempty"`
}

// LibraryStats is the JSON body for GET /api/stats.
type LibraryStats struct {
	CanonicalGameCount         int            `json:"canonical_game_count"`
	SourceGameFoundCount       int            `json:"source_game_found_count"`
	SourceGameTotalCount       int            `json:"source_game_total_count"`
	ByPlatform                 map[string]int `json:"by_platform"`
	ByDecade                   map[string]int `json:"by_decade"`
	ByKind                     map[string]int `json:"by_kind"`
	TopGenres                  map[string]int `json:"top_genres"`
	ByIntegrationID            map[string]int `json:"by_integration_id"`
	ByPluginID                 map[string]int `json:"by_plugin_id"`
	ByMetadataPluginID         map[string]int `json:"by_metadata_plugin_id"`
	CanonicalWithResolverTitle int            `json:"canonical_with_resolver_title"`
	PercentWithResolverTitle   float64        `json:"percent_with_resolver_title"`
	GamesWithDescription       int            `json:"games_with_description"`
	PercentWithDescription     float64        `json:"percent_with_description"`
	GamesWithMedia             int            `json:"games_with_media"`
	GamesWithAchievements      int            `json:"games_with_achievements"`
	PercentWithMedia           float64        `json:"percent_with_media"`
	PercentWithAchievements    float64        `json:"percent_with_achievements"`
}

// GameListItem is a lightweight game reference returned by integration-scoped queries.
type GameListItem struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Platform Platform `json:"platform"`
}

type ManualReviewState string

const (
	ManualReviewStatePending  ManualReviewState = "pending"
	ManualReviewStateMatched  ManualReviewState = "matched"
	ManualReviewStateNotAGame ManualReviewState = "not_a_game"
)

type ManualReviewScope string

const (
	ManualReviewScopeActive  ManualReviewScope = "active"
	ManualReviewScopeArchive ManualReviewScope = "archive"
)

type ManualReviewSelection struct {
	ProviderIntegrationID string   `json:"provider_integration_id"`
	ProviderLabel         string   `json:"provider_label,omitempty"`
	ProviderPluginID      string   `json:"provider_plugin_id"`
	Title                 string   `json:"title"`
	Platform              string   `json:"platform,omitempty"`
	Kind                  string   `json:"kind,omitempty"`
	ParentGameID          string   `json:"parent_game_id,omitempty"`
	ExternalID            string   `json:"external_id"`
	URL                   string   `json:"url,omitempty"`
	Description           string   `json:"description,omitempty"`
	ReleaseDate           string   `json:"release_date,omitempty"`
	Genres                []string `json:"genres,omitempty"`
	Developer             string   `json:"developer,omitempty"`
	Publisher             string   `json:"publisher,omitempty"`
	Rating                float64  `json:"rating,omitempty"`
	MaxPlayers            int      `json:"max_players,omitempty"`
	ImageURL              string   `json:"image_url,omitempty"`
}

type ManualReviewDecision struct {
	State    ManualReviewState      `json:"state"`
	Selected *ManualReviewSelection `json:"selected,omitempty"`
}

type ManualReviewRedetectStatus string

const (
	ManualReviewRedetectStatusMatched      ManualReviewRedetectStatus = "matched"
	ManualReviewRedetectStatusPending      ManualReviewRedetectStatus = "pending"
	ManualReviewRedetectStatusUnidentified ManualReviewRedetectStatus = "unidentified"
)

type ManualReviewRedetectResult struct {
	CandidateID   string                     `json:"candidate_id"`
	Status        ManualReviewRedetectStatus `json:"status"`
	MatchCount    int                        `json:"match_count"`
	ProviderCount int                        `json:"provider_count"`
}

type ManualReviewRedetectBatchResult struct {
	Attempted         int                          `json:"attempted"`
	Matched           int                          `json:"matched"`
	Unidentified      int                          `json:"unidentified"`
	FailedCandidateID string                       `json:"failed_candidate_id,omitempty"`
	Error             string                       `json:"error,omitempty"`
	Results           []ManualReviewRedetectResult `json:"results"`
}

// ManualReviewCandidate is a read-only server-owned review candidate used by
// the Undetected Games and Reclassify flows.
type ManualReviewCandidate struct {
	ID                 string            `json:"id"`
	CanonicalGameID    string            `json:"canonical_game_id,omitempty"`
	CurrentTitle       string            `json:"current_title"`
	RawTitle           string            `json:"raw_title"`
	Platform           Platform          `json:"platform"`
	Kind               GameKind          `json:"kind"`
	GroupKind          GroupKind         `json:"group_kind"`
	IntegrationID      string            `json:"integration_id"`
	PluginID           string            `json:"plugin_id"`
	ExternalID         string            `json:"external_id"`
	RootPath           string            `json:"root_path,omitempty"`
	URL                string            `json:"url,omitempty"`
	Status             string            `json:"status"`
	ReviewState        ManualReviewState `json:"review_state"`
	FileCount          int               `json:"file_count"`
	ResolverMatchCount int               `json:"resolver_match_count"`
	ReviewReasons      []string          `json:"review_reasons,omitempty"`
	Files              []GameFile        `json:"files,omitempty"`
	ResolverMatches    []ResolverMatch   `json:"resolver_matches,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	LastSeenAt         *time.Time        `json:"last_seen_at,omitempty"`
}

// FoundSourceGame carries the fields needed for metadata re-enrichment
// without running the discovery phase again.
type FoundSourceGame struct {
	ID            string    `json:"id"`
	IntegrationID string    `json:"integration_id"`
	PluginID      string    `json:"plugin_id"`
	ExternalID    string    `json:"external_id"`
	RawTitle      string    `json:"raw_title"`
	Platform      Platform  `json:"platform"`
	Kind          GameKind  `json:"kind"`
	GroupKind     GroupKind `json:"group_kind"`
	RootPath      string    `json:"root_path"`
	URL           string    `json:"url,omitempty"`
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
	GameID     string
	Path       string
	FileName   string
	Role       GameFileRole
	FileKind   string
	Size       int64
	IsDir      bool
	ObjectID   string
	Revision   string
	ModifiedAt *time.Time
}

type SourceDeliveryMode string

const (
	SourceDeliveryModeDirect       SourceDeliveryMode = "direct"
	SourceDeliveryModeMaterialized SourceDeliveryMode = "materialized"
	SourceDeliveryModeUnavailable  SourceDeliveryMode = "unavailable"
)

type SourceDeliveryProfile struct {
	Profile         string             `json:"profile"`
	Mode            SourceDeliveryMode `json:"mode"`
	PrepareRequired bool               `json:"prepare_required,omitempty"`
	Ready           bool               `json:"ready,omitempty"`
	RootFilePath    string             `json:"root_file_path,omitempty"`
}

type SourceCacheEntryFile struct {
	EntryID    string     `json:"entry_id"`
	Path       string     `json:"path"`
	LocalPath  string     `json:"local_path"`
	ObjectID   string     `json:"object_id,omitempty"`
	Revision   string     `json:"revision,omitempty"`
	ModifiedAt *time.Time `json:"modified_at,omitempty"`
	Size       int64      `json:"size"`
}

type SourceCacheEntry struct {
	ID              string                 `json:"id"`
	CacheKey        string                 `json:"cache_key"`
	CanonicalGameID string                 `json:"canonical_game_id,omitempty"`
	CanonicalTitle  string                 `json:"canonical_title,omitempty"`
	SourceGameID    string                 `json:"source_game_id"`
	SourceTitle     string                 `json:"source_title,omitempty"`
	IntegrationID   string                 `json:"integration_id"`
	PluginID        string                 `json:"plugin_id"`
	Profile         string                 `json:"profile"`
	Mode            string                 `json:"mode"`
	Status          string                 `json:"status"`
	SourcePath      string                 `json:"source_path,omitempty"`
	FileCount       int                    `json:"file_count"`
	Size            int64                  `json:"size"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	LastAccessedAt  *time.Time             `json:"last_accessed_at,omitempty"`
	Files           []SourceCacheEntryFile `json:"files,omitempty"`
}

type SourceCacheJobStatus struct {
	JobID           string     `json:"job_id"`
	CacheKey        string     `json:"cache_key,omitempty"`
	CanonicalGameID string     `json:"canonical_game_id,omitempty"`
	CanonicalTitle  string     `json:"canonical_title,omitempty"`
	SourceGameID    string     `json:"source_game_id"`
	SourceTitle     string     `json:"source_title,omitempty"`
	IntegrationID   string     `json:"integration_id,omitempty"`
	PluginID        string     `json:"plugin_id,omitempty"`
	Profile         string     `json:"profile"`
	Status          string     `json:"status"`
	Message         string     `json:"message,omitempty"`
	Error           string     `json:"error,omitempty"`
	EntryID         string     `json:"entry_id,omitempty"`
	ProgressCurrent int        `json:"progress_current,omitempty"`
	ProgressTotal   int        `json:"progress_total,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

type SourceCachePrepareRequest struct {
	CanonicalGameID string `json:"canonical_game_id"`
	CanonicalTitle  string `json:"canonical_title,omitempty"`
	SourceGameID    string `json:"source_game_id"`
	Profile         string `json:"profile"`
}

type SourceMaterializeRequest struct {
	Config   map[string]any `json:"config"`
	Path     string         `json:"path"`
	ObjectID string         `json:"object_id,omitempty"`
	Revision string         `json:"revision,omitempty"`
	Profile  string         `json:"profile,omitempty"`
	DestPath string         `json:"dest_path"`
}

type SourceMaterializeResult struct {
	Size     int64  `json:"size,omitempty"`
	Revision string `json:"revision,omitempty"`
	ModTime  string `json:"mod_time,omitempty"`
}
