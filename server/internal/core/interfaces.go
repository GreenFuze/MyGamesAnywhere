package core

import (
	"context"
	"database/sql"
)

// Logger defines the interface for structured logging.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, err error, args ...any)
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
}

// Configuration defines the interface for application settings.
type Configuration interface {
	Get(key string) string
	GetInt(key string) int
	GetBool(key string) bool
	Validate() error
}

// Database defines the interface for database operations.
type Database interface {
	Connect() error
	Close() error
	EnsureSchema() error
	GetDB() *sql.DB
}

type SettingRepository interface {
	Upsert(ctx context.Context, setting *Setting) error
	Get(ctx context.Context, key string) (*Setting, error)
}

type IntegrationRepository interface {
	Create(ctx context.Context, integration *Integration) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*Integration, error)
	GetByID(ctx context.Context, id string) (*Integration, error)
}

// GameStore is the single persistence layer for all game-related data.
// All write methods are transactional — if any step fails, the entire
// operation rolls back. Read methods compute canonical views on the fly.
type GameStore interface {
	// ── Writes (all transactional) ──

	// PersistScanResults writes a complete scan batch: source games, files,
	// resolver matches, media links. Detects moves, soft-deletes missing
	// games, assigns canonical groupings.
	PersistScanResults(ctx context.Context, batch *ScanBatch) error

	// CacheAchievements stores a fetched achievement set for a source game.
	// Replaces any existing cached set for the same (source_game, source).
	CacheAchievements(ctx context.Context, sourceGameID string, set *AchievementSet) error

	// UpdateMediaAsset marks a media asset as downloaded (sets local_path, hash).
	UpdateMediaAsset(ctx context.Context, assetID int, localPath, hash string) error

	// DeleteAllGames removes all source games, files, matches, media links.
	DeleteAllGames(ctx context.Context) error

	// ── Reads ──

	// GetCanonicalGames returns merged views of all visible games.
	GetCanonicalGames(ctx context.Context) ([]*CanonicalGame, error)

	// GetCanonicalGameByID returns one merged game view by canonical ID.
	GetCanonicalGameByID(ctx context.Context, canonicalID string) (*CanonicalGame, error)

	// GetSourceGamesForCanonical returns all source game records for a canonical game.
	GetSourceGamesForCanonical(ctx context.Context, canonicalID string) ([]*SourceGame, error)

	// GetPendingMediaDownloads returns media assets with no local_path.
	GetPendingMediaDownloads(ctx context.Context, limit int) ([]*MediaAsset, error)

	// GetCachedAchievements returns cached achievement data if it exists.
	GetCachedAchievements(ctx context.Context, sourceGameID, source string) (*AchievementSet, error)

	// GetExternalIDsForCanonical returns all external IDs across source games and resolver matches.
	GetExternalIDsForCanonical(ctx context.Context, canonicalID string) ([]ExternalID, error)
}

// SettingsSyncProvider backs up and restores server state to/from remote storage.
type SettingsSyncProvider interface {
	Backup(ctx context.Context, config map[string]any) error
	Restore(ctx context.Context, config map[string]any) error
}

// Server defines the interface for the HTTP server.
type Server interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
