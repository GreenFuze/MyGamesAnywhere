package core

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrManualReviewCandidateNotFound    = errors.New("manual review candidate not found")
	ErrManualReviewCandidateNotEligible = errors.New("manual review candidate is not eligible for re-detect")
	ErrManualReviewSelectionInvalid     = errors.New("manual review selection invalid")
	ErrMetadataProvidersUnavailable     = errors.New("metadata providers unavailable")
	ErrMetadataRefreshNoEligible        = errors.New("metadata refresh has no eligible source records")
	ErrSourceGameDeleteNotFound         = errors.New("source record not found")
	ErrSourceGameDeleteNotEligible      = errors.New("source record is not eligible for hard delete")
	ErrCanonicalGameNotFound            = errors.New("canonical game not found")
	ErrCoverOverrideMediaNotFound       = errors.New("cover override media asset is not linked to this game")
	ErrHoverOverrideMediaNotFound       = errors.New("hover override media asset is not linked to this game")
	ErrBackgroundOverrideMediaNotFound  = errors.New("background override media asset is not linked to this game")
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

	// PersistScanResults writes a scan batch: source games, files,
	// resolver matches, media links. Complete batches detect moves and
	// soft-delete missing games; targeted refresh batches skip that reconcile.
	PersistScanResults(ctx context.Context, batch *ScanBatch) error

	// CacheAchievements stores a fetched achievement set for a source game.
	// Replaces any existing cached set for the same (source_game, source).
	CacheAchievements(ctx context.Context, sourceGameID string, set *AchievementSet) error

	// UpdateMediaAsset marks a media asset as downloaded (sets local_path, hash).
	UpdateMediaAsset(ctx context.Context, assetID int, localPath, hash string) error
	// UpdateMediaAssetMetadata backfills width/height/mime_type for an existing media asset.
	UpdateMediaAssetMetadata(ctx context.Context, assetID, width, height int, mimeType string) error

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

	// GetCachedAchievementsDashboard returns cached achievement aggregates only.
	GetCachedAchievementsDashboard(ctx context.Context) (*CachedAchievementsDashboard, error)

	// GetCachedAchievementsExplorer returns cached achievement sets grouped by canonical game.
	GetCachedAchievementsExplorer(ctx context.Context) (*CachedAchievementsExplorer, error)

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

	// SaveRefreshedMetadataProviderResults persists resolver/media state for one
	// or more source records after a provider-scoped metadata refresh. The
	// supplied source records must already exist; files, source identity, and
	// manual-review state are preserved.
	SaveRefreshedMetadataProviderResults(ctx context.Context, sourceGames []*SourceGame) error

	// SetManualReviewState updates one source record's manual-review state and
	// recomputes canonical membership if its visibility changes.
	SetManualReviewState(ctx context.Context, candidateID string, state ManualReviewState) error

	// GetFoundSourceGames returns source games with status='found', optionally filtered
	// by integration IDs. Used by metadata-only refresh to re-enrich existing games.
	GetFoundSourceGames(ctx context.Context, integrationIDs []string) ([]*FoundSourceGame, error)

	// GetFoundSourceGameRecords returns full source game records with status='found',
	// optionally filtered by integration IDs. Used by refresh paths that must preserve
	// files, manual-review state, and source identity while replacing metadata state.
	GetFoundSourceGameRecords(ctx context.Context, integrationIDs []string) ([]*SourceGame, error)

	// DeleteGamesByIntegrationID removes all source games and related data for an integration.
	DeleteGamesByIntegrationID(ctx context.Context, integrationID string) error

	// DeleteSourceGameByID removes one source game and dependent rows, then recomputes canonical groups.
	DeleteSourceGameByID(ctx context.Context, sourceGameID string) error

	// SaveScanReport persists a completed scan report.
	SaveScanReport(ctx context.Context, report *ScanReport) error

	// GetScanReports returns the most recent N scan reports (newest first).
	GetScanReports(ctx context.Context, limit int) ([]*ScanReport, error)

	// GetScanReport returns a single scan report by ID.
	GetScanReport(ctx context.Context, id string) (*ScanReport, error)

	// GetSourceGameCountsByIntegration returns a map of integration_id → count of found source games.
	GetSourceGameCountsByIntegration(ctx context.Context) (map[string]int, error)

	// SetCanonicalCoverOverride pins one existing media asset as the canonical cover.
	SetCanonicalCoverOverride(ctx context.Context, canonicalID string, mediaAssetID int) error

	// ClearCanonicalCoverOverride removes a pinned canonical cover, restoring normal selection.
	ClearCanonicalCoverOverride(ctx context.Context, canonicalID string) error

	// SetCanonicalHoverOverride pins one existing media asset as the canonical hover image.
	SetCanonicalHoverOverride(ctx context.Context, canonicalID string, mediaAssetID int) error

	// SetCanonicalBackgroundOverride pins one existing media asset as the canonical background image.
	SetCanonicalBackgroundOverride(ctx context.Context, canonicalID string, mediaAssetID int) error

	// SetCanonicalFavorite marks one canonical game as a favorite.
	SetCanonicalFavorite(ctx context.Context, canonicalID string) error

	// ClearCanonicalFavorite removes one canonical game from favorites.
	ClearCanonicalFavorite(ctx context.Context, canonicalID string) error
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
	StartPrefetch(ctx context.Context, req SaveSyncPrefetchRequest) (*SaveSyncPrefetchStatus, error)
	GetPrefetchStatus(ctx context.Context, jobID string) (*SaveSyncPrefetchStatus, error)
	StartMigration(ctx context.Context, req SaveSyncMigrationRequest) (*SaveSyncMigrationStatus, error)
	GetMigrationStatus(ctx context.Context, jobID string) (*SaveSyncMigrationStatus, error)
}

// BackgroundService starts long-lived background work that stops when ctx is cancelled.
type BackgroundService interface {
	Start(ctx context.Context) error
}

// MediaDownloadQueue schedules pending media_assets rows for background download.
type MediaDownloadQueue interface {
	EnqueuePending(ctx context.Context) error
}

type MediaDownloadService interface {
	BackgroundService
	MediaDownloadQueue
}

type SourceCacheStore interface {
	MarkInFlightJobsInterrupted(ctx context.Context) error
	GetEntryBySourceProfile(ctx context.Context, sourceGameID, profile string) (*SourceCacheEntry, error)
	GetEntryFileBySourceProfile(ctx context.Context, sourceGameID, profile, path string) (*SourceCacheEntry, *SourceCacheEntryFile, error)
	UpsertEntry(ctx context.Context, entry *SourceCacheEntry) error
	ReplaceEntryFiles(ctx context.Context, entryID string, files []SourceCacheEntryFile) error
	TouchEntry(ctx context.Context, entryID string, at time.Time) error
	ListEntries(ctx context.Context) ([]*SourceCacheEntry, error)
	DeleteEntry(ctx context.Context, entryID string) error
	ClearEntries(ctx context.Context) error
	CreateJob(ctx context.Context, job *SourceCacheJobStatus) error
	UpdateJob(ctx context.Context, job *SourceCacheJobStatus) error
	GetJob(ctx context.Context, jobID string) (*SourceCacheJobStatus, error)
	ListJobs(ctx context.Context, limit int) ([]*SourceCacheJobStatus, error)
	FindActiveJobByCacheKey(ctx context.Context, cacheKey string) (*SourceCacheJobStatus, error)
}

type SourceCacheService interface {
	DescribeSourceGame(ctx context.Context, canonicalPlatform Platform, sourceGame *SourceGame) []SourceDeliveryProfile
	Prepare(ctx context.Context, req SourceCachePrepareRequest, canonicalPlatform Platform, sourceGame *SourceGame) (*SourceCacheJobStatus, bool, error)
	GetJob(ctx context.Context, jobID string) (*SourceCacheJobStatus, error)
	ListJobs(ctx context.Context, limit int) ([]*SourceCacheJobStatus, error)
	ListEntries(ctx context.Context) ([]*SourceCacheEntry, error)
	DeleteEntry(ctx context.Context, entryID string) error
	ClearEntries(ctx context.Context) error
	ResolveCachedFile(ctx context.Context, sourceGameID, profile, path string) (*SourceCacheEntry, *SourceCacheEntryFile, string, error)
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
	Redetect(ctx context.Context, candidateID string) (*ManualReviewRedetectResult, error)
	RedetectActive(ctx context.Context) (*ManualReviewRedetectBatchResult, error)
}

// GameMetadataRefreshService handles explicit metadata/media refresh for one canonical game.
type GameMetadataRefreshService interface {
	RefreshGameMetadata(ctx context.Context, canonicalID string) (*CanonicalGame, error)
}

type DeleteSourceGameResult struct {
	DeletedSourceGameID string
	CanonicalExists     bool
	CanonicalGame       *CanonicalGame
}

// GameDeletionService handles destructive, source-scoped file-backed deletions.
type GameDeletionService interface {
	DeleteSourceGame(ctx context.Context, canonicalID, sourceGameID string) (*DeleteSourceGameResult, error)
	DeleteReviewCandidateFiles(ctx context.Context, candidateID string) (*DeleteSourceGameResult, error)
}
