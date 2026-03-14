package core

import "time"

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
	GroupKindPacked        GroupKind = "packed"          // needs unpacking (installer or compressed archive)
	GroupKindExtras        GroupKind = "extras"          // non-game content (manuals, soundtracks)
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
