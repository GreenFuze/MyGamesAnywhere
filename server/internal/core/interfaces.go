package core

import (
	"context"
	"database/sql"
	"errors"
)

var (
	ErrManualReviewCandidateNotFound = errors.New("manual review candidate not found")
	ErrManualReviewSelectionInvalid  = errors.New("manual review selection invalid")
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
	List(ctx context.Context) ([]*Setting, error)
}

type IntegrationRepository interface {
	Create(ctx context.Context, integration *Integration) error
	Update(ctx context.Context, integration *Integration) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*Integration, error)
	GetByID(ctx context.Context, id string) (*Integration, error)
	// ListByPluginID returns integrations for one plugin (for duplicate config checks).
	ListByPluginID(ctx context.Context, pluginID string) ([]*Integration, error)
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

	// GetCanonicalGamesByIDs loads merged views for the given canonical IDs (order preserved).
	GetCanonicalGamesByIDs(ctx context.Context, canonicalIDs []string) ([]*CanonicalGame, error)

	// CountVisibleCanonicalGames returns how many canonical games have at least one found source row.
	CountVisibleCanonicalGames(ctx context.Context) (int, error)

	// GetVisibleCanonicalIDs returns canonical IDs in stable order (for pagination).
	// limit <= 0 means no limit (all rows from offset).
	GetVisibleCanonicalIDs(ctx context.Context, offset, limit int) ([]string, error)

	// GetCanonicalGameByID returns one merged game view by stable canonical ID.
	GetCanonicalGameByID(ctx context.Context, canonicalID string) (*CanonicalGame, error)

	// GetMediaAssetByID returns a media_assets row by primary key, or nil if missing.
	GetMediaAssetByID(ctx context.Context, id int) (*MediaAsset, error)

	// GetSourceGamesForCanonical returns all source game records for a canonical game.
	GetSourceGamesForCanonical(ctx context.Context, canonicalID string) ([]*SourceGame, error)

	// GetPendingMediaDownloads returns media assets with no local_path.
	GetPendingMediaDownloads(ctx context.Context, limit int) ([]*MediaAsset, error)

	// GetCachedAchievements returns cached achievement data if it exists.
	GetCachedAchievements(ctx context.Context, sourceGameID, source string) (*AchievementSet, error)

	// GetExternalIDsForCanonical returns all external IDs across source games and resolver matches.
	GetExternalIDsForCanonical(ctx context.Context, canonicalID string) ([]ExternalID, error)

	// GetLibraryStats returns aggregate counts for the library (GET /api/stats).
	GetLibraryStats(ctx context.Context) (*LibraryStats, error)

	// GetGamesByIntegrationID returns canonical games discovered by a source integration.
	GetGamesByIntegrationID(ctx context.Context, integrationID string, limit int) ([]GameListItem, error)

	// GetEnrichedGamesByPluginID returns canonical games enriched by a metadata plugin.
	GetEnrichedGamesByPluginID(ctx context.Context, pluginID string, limit int) ([]GameListItem, error)

	// ListManualReviewCandidates returns source-record-backed candidates for the
	// requested review scope.
	ListManualReviewCandidates(ctx context.Context, scope ManualReviewScope, limit int) ([]*ManualReviewCandidate, error)

	// GetManualReviewCandidate returns one source-record-backed manual review candidate.
	// Unlike ListManualReviewCandidates, this may return a candidate that is not currently
	// in the review queue so direct reclassify entry can still open it.
	GetManualReviewCandidate(ctx context.Context, candidateID string) (*ManualReviewCandidate, error)

	// SaveManualReviewResult persists an inline manual-review decision and the
	// resulting resolver/media state for one source record without affecting
	// other source rows in the same integration.
	SaveManualReviewResult(ctx context.Context, sourceGame *SourceGame, resolverMatches []ResolverMatch, media []MediaRef) error

	// SetManualReviewState updates one source record's manual-review state and
	// recomputes canonical membership if its visibility changes.
	SetManualReviewState(ctx context.Context, candidateID string, state ManualReviewState) error

	// GetFoundSourceGames returns source games with status='found', optionally filtered
	// by integration IDs. Used by metadata-only refresh to re-enrich existing games.
	GetFoundSourceGames(ctx context.Context, integrationIDs []string) ([]*FoundSourceGame, error)

	// DeleteGamesByIntegrationID removes all source games and related data for an integration.
	DeleteGamesByIntegrationID(ctx context.Context, integrationID string) error

	// SaveScanReport persists a completed scan report.
	SaveScanReport(ctx context.Context, report *ScanReport) error

	// GetScanReports returns the most recent N scan reports (newest first).
	GetScanReports(ctx context.Context, limit int) ([]*ScanReport, error)

	// GetScanReport returns a single scan report by ID.
	GetScanReport(ctx context.Context, id string) (*ScanReport, error)

	// GetSourceGameCountsByIntegration returns a map of integration_id → count of found source games.
	GetSourceGameCountsByIntegration(ctx context.Context) (map[string]int, error)
}

// SyncService handles push/pull settings synchronisation to a remote store.
type SyncService interface {
	Push(ctx context.Context, passphrase string) (*PushResult, error)
	Pull(ctx context.Context, passphrase string) (*PullResult, error)
	Status(ctx context.Context) (*SyncStatus, error)
	StoreKey(passphrase string) error
	ClearKey() error
}

// SaveSyncService handles browser-runtime save snapshot storage and migration.
type SaveSyncService interface {
	ListSlots(ctx context.Context, req SaveSyncListRequest) ([]SaveSyncSlotSummary, error)
	GetSlot(ctx context.Context, req SaveSyncSlotRef) (*SaveSyncSnapshot, error)
	PutSlot(ctx context.Context, req SaveSyncPutRequest) (*SaveSyncPutResult, error)
	StartMigration(ctx context.Context, req SaveSyncMigrationRequest) (*SaveSyncMigrationStatus, error)
	GetMigrationStatus(ctx context.Context, jobID string) (*SaveSyncMigrationStatus, error)
}

// KeyStore persists the sync encryption key using OS-level protection.
type KeyStore interface {
	Store(passphrase string) error
	Load() (string, error)
	Clear() error
	HasKey() bool
}

// Server defines the interface for the HTTP server.
type Server interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// ManualReviewService handles inline manual metadata selection workflows.
type ManualReviewService interface {
	Apply(ctx context.Context, candidateID string, selection ManualReviewSelection) error
}
