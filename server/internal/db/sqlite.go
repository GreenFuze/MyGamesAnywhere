package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type sqliteDatabase struct {
	db     *sql.DB
	logger core.Logger
	config core.Configuration
}

func NewSQLiteDatabase(logger core.Logger, config core.Configuration) core.Database {
	return &sqliteDatabase{
		logger: logger,
		config: config,
	}
}

func (s *sqliteDatabase) Connect() error {
	if s.db != nil {
		return nil
	}
	dbPath := s.config.Get("DB_PATH")
	s.logger.Info("Connecting to database", "path", dbPath)

	// Ensure the parent directory exists (SQLite creates the file but not directories).
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create database directory %s: %w", dir, err)
			}
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open sqlite database: %w", err)
	}
	// SQLite behaves more predictably for this app when all goroutines share
	// one pooled connection instead of opening multiple writer-capable handles
	// that race into SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	// Enable WAL mode for better concurrent read/write performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		s.logger.Warn("could not enable WAL mode", "error", err)
	}
	// Wait for a short window when another write transaction is active instead
	// of immediately surfacing SQLITE_BUSY to callers.
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		s.logger.Warn("could not enable busy timeout", "error", err)
	}
	// Enforce foreign key constraints.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		s.logger.Warn("could not enable foreign keys", "error", err)
	}

	s.db = db
	return nil
}

func (s *sqliteDatabase) Close() error {
	if s.db != nil {
		s.logger.Info("Closing database connection")
		err := s.db.Close()
		s.db = nil
		return err
	}
	return nil
}

func (s *sqliteDatabase) EnsureSchema() error {
	s.logger.Info("Ensuring database schema...")

	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS profiles (
			id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			avatar_key TEXT,
			role TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS profile_settings (
			profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
			key TEXT NOT NULL,
			value TEXT,
			updated_at INTEGER,
			PRIMARY KEY(profile_id, key)
		);`,
		`CREATE TABLE IF NOT EXISTS integrations (
			id TEXT PRIMARY KEY,
			profile_id TEXT,
			plugin_id TEXT NOT NULL,
			label TEXT NOT NULL,
			config_json TEXT,
			integration_type TEXT NOT NULL,
			created_at INTEGER,
			updated_at INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS source_games (
			id TEXT PRIMARY KEY,
			profile_id TEXT,
			integration_id TEXT NOT NULL,
			plugin_id TEXT NOT NULL,
			external_id TEXT NOT NULL,
			raw_title TEXT NOT NULL,
			platform TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT 'base_game',
			group_kind TEXT NOT NULL DEFAULT 'unknown',
			root_path TEXT,
			url TEXT,
			status TEXT NOT NULL DEFAULT 'found',
			last_seen_at INTEGER,
			created_at INTEGER NOT NULL,
			UNIQUE(integration_id, plugin_id, external_id)
		);`,
		`CREATE TABLE IF NOT EXISTS canonical_source_games_link (
			canonical_id TEXT NOT NULL,
			source_game_id TEXT NOT NULL REFERENCES source_games(id),
			PRIMARY KEY(canonical_id, source_game_id)
		);`,
		`CREATE TABLE IF NOT EXISTS canonical_games (
			id TEXT PRIMARY KEY,
			created_at INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_csgl_source ON canonical_source_games_link(source_game_id);`,
		`CREATE INDEX IF NOT EXISTS idx_csgl_canonical ON canonical_source_games_link(canonical_id);`,
		`CREATE TABLE IF NOT EXISTS game_files (
			source_game_id TEXT NOT NULL REFERENCES source_games(id),
			path TEXT NOT NULL,
			file_name TEXT NOT NULL,
			role TEXT NOT NULL,
			file_kind TEXT,
			size INTEGER NOT NULL,
			is_dir INTEGER NOT NULL,
			PRIMARY KEY(source_game_id, path)
		);`,
		`CREATE TABLE IF NOT EXISTS metadata_resolver_matches (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_game_id TEXT NOT NULL REFERENCES source_games(id),
			plugin_id TEXT NOT NULL,
			external_id TEXT NOT NULL,
			title TEXT,
			platform TEXT,
			url TEXT,
			outvoted INTEGER NOT NULL DEFAULT 0,
			developer TEXT,
			publisher TEXT,
			release_date TEXT,
			rating REAL,
			metadata_json TEXT,
			created_at INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_mrm_source ON metadata_resolver_matches(source_game_id);`,
		`CREATE INDEX IF NOT EXISTS idx_mrm_external ON metadata_resolver_matches(plugin_id, external_id);`,
		`CREATE TABLE IF NOT EXISTS media_assets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL UNIQUE,
			local_path TEXT,
			hash TEXT,
			width INTEGER,
			height INTEGER,
			mime_type TEXT,
			download_attempts INTEGER NOT NULL DEFAULT 0,
			download_failed_at INTEGER,
			download_last_error TEXT,
			download_permanent_failure INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS source_game_media (
			source_game_id TEXT NOT NULL REFERENCES source_games(id),
			media_asset_id INTEGER NOT NULL REFERENCES media_assets(id),
			type TEXT NOT NULL,
			source TEXT,
			PRIMARY KEY(source_game_id, media_asset_id, type)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sgm_source ON source_game_media(source_game_id);`,
		`CREATE TABLE IF NOT EXISTS canonical_game_cover_overrides (
			canonical_id TEXT PRIMARY KEY REFERENCES canonical_games(id) ON DELETE CASCADE,
			media_asset_id INTEGER NOT NULL REFERENCES media_assets(id),
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS canonical_game_cover_override_clears (
			canonical_id TEXT PRIMARY KEY REFERENCES canonical_games(id) ON DELETE CASCADE,
			cleared_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS canonical_game_hover_overrides (
			canonical_id TEXT PRIMARY KEY REFERENCES canonical_games(id) ON DELETE CASCADE,
			media_asset_id INTEGER NOT NULL REFERENCES media_assets(id),
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS canonical_game_background_overrides (
			canonical_id TEXT PRIMARY KEY REFERENCES canonical_games(id) ON DELETE CASCADE,
			media_asset_id INTEGER NOT NULL REFERENCES media_assets(id),
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS canonical_game_favorites (
			canonical_id TEXT PRIMARY KEY REFERENCES canonical_games(id) ON DELETE CASCADE,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS achievement_sets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_game_id TEXT NOT NULL REFERENCES source_games(id),
			source TEXT NOT NULL,
			external_game_id TEXT NOT NULL,
			total_count INTEGER NOT NULL,
			unlocked_count INTEGER NOT NULL,
			total_points INTEGER,
			earned_points INTEGER,
			fetched_at INTEGER NOT NULL,
			UNIQUE(source_game_id, source)
		);`,
		`CREATE TABLE IF NOT EXISTS achievements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			set_id INTEGER NOT NULL REFERENCES achievement_sets(id),
			external_id TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT,
			locked_icon TEXT,
			unlocked_icon TEXT,
			points INTEGER,
			rarity REAL,
			unlocked INTEGER NOT NULL DEFAULT 0,
			unlocked_at INTEGER
		);`,
		`CREATE INDEX IF NOT EXISTS idx_ach_set ON achievements(set_id);`,
		`CREATE TABLE IF NOT EXISTS scan_reports (
			id TEXT PRIMARY KEY,
			profile_id TEXT,
			started_at INTEGER NOT NULL,
			finished_at INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			metadata_only INTEGER NOT NULL DEFAULT 0,
			report_json TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS source_cache_entries (
			id TEXT PRIMARY KEY,
			cache_key TEXT NOT NULL,
			canonical_game_id TEXT,
			canonical_title TEXT,
			source_game_id TEXT NOT NULL REFERENCES source_games(id),
			source_title TEXT,
			integration_id TEXT NOT NULL,
			plugin_id TEXT NOT NULL,
			profile TEXT NOT NULL,
			mode TEXT NOT NULL,
			status TEXT NOT NULL,
			source_path TEXT,
			file_count INTEGER NOT NULL DEFAULT 0,
			size INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			last_accessed_at INTEGER,
			UNIQUE(source_game_id, profile)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_source_cache_entries_cache_key ON source_cache_entries(cache_key);`,
		`CREATE TABLE IF NOT EXISTS source_cache_entry_files (
			entry_id TEXT NOT NULL REFERENCES source_cache_entries(id) ON DELETE CASCADE,
			path TEXT NOT NULL,
			local_path TEXT NOT NULL,
			object_id TEXT,
			revision TEXT,
			modified_at INTEGER,
			size INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(entry_id, path)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_source_cache_entry_files_entry ON source_cache_entry_files(entry_id);`,
		`CREATE TABLE IF NOT EXISTS source_cache_jobs (
			job_id TEXT PRIMARY KEY,
			cache_key TEXT,
			canonical_game_id TEXT,
			canonical_title TEXT,
			source_game_id TEXT NOT NULL REFERENCES source_games(id),
			source_title TEXT,
			integration_id TEXT,
			plugin_id TEXT,
			profile TEXT NOT NULL,
			status TEXT NOT NULL,
			message TEXT,
			error TEXT,
			entry_id TEXT,
			progress_current INTEGER NOT NULL DEFAULT 0,
			progress_total INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			finished_at INTEGER
		);`,
		`CREATE INDEX IF NOT EXISTS idx_source_cache_jobs_cache_key ON source_cache_jobs(cache_key);`,
		`CREATE INDEX IF NOT EXISTS idx_source_cache_jobs_updated_at ON source_cache_jobs(updated_at DESC);`,
	}

	for _, q := range statements {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("schema creation failed: %w", err)
		}
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (?, ?)`, 1, time.Now().Unix()); err != nil {
		return fmt.Errorf("schema migration marker failed: %w", err)
	}
	if err := s.ensureManualReviewSchema(); err != nil {
		return err
	}
	if err := s.ensureGameFileIdentitySchema(); err != nil {
		return err
	}
	if err := s.ensureMediaAssetDownloadStateSchema(); err != nil {
		return err
	}
	if err := s.ensureProfileSchema(); err != nil {
		return err
	}
	if err := s.ensureDefaultProfileForExistingData(); err != nil {
		return err
	}
	if err := s.backfillCanonicalGames(); err != nil {
		return err
	}
	if err := s.migrateLegacyCanonicalIDs(); err != nil {
		return err
	}
	return nil
}

func (s *sqliteDatabase) ensureProfileSchema() error {
	if s.db == nil {
		return fmt.Errorf("database not connected")
	}
	if err := s.ensureColumn("integrations", "profile_id",
		`ALTER TABLE integrations ADD COLUMN profile_id TEXT`); err != nil {
		return err
	}
	if err := s.ensureColumn("source_games", "profile_id",
		`ALTER TABLE source_games ADD COLUMN profile_id TEXT`); err != nil {
		return err
	}
	if err := s.ensureColumn("scan_reports", "profile_id",
		`ALTER TABLE scan_reports ADD COLUMN profile_id TEXT`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_integrations_profile ON integrations(profile_id)`); err != nil {
		return fmt.Errorf("create integrations profile index: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_source_games_profile ON source_games(profile_id)`); err != nil {
		return fmt.Errorf("create source games profile index: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_scan_reports_profile ON scan_reports(profile_id)`); err != nil {
		return fmt.Errorf("create scan reports profile index: %w", err)
	}
	if _, err := s.db.Exec(`UPDATE integrations SET integration_type='sync' WHERE plugin_id='sync-settings-google-drive' AND integration_type='storage'`); err != nil {
		return fmt.Errorf("normalize settings sync integration type: %w", err)
	}
	return nil
}

func (s *sqliteDatabase) ensureDefaultProfileForExistingData() error {
	if s.db == nil {
		return fmt.Errorf("database not connected")
	}
	var profileCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM profiles`).Scan(&profileCount); err != nil {
		return fmt.Errorf("count profiles: %w", err)
	}
	if profileCount > 0 {
		return nil
	}

	var dataRows int
	if err := s.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM integrations) +
			(SELECT COUNT(*) FROM source_games) +
			(SELECT COUNT(*) FROM scan_reports) +
			(SELECT COUNT(*) FROM source_cache_entries)
	`).Scan(&dataRows); err != nil {
		return fmt.Errorf("count existing profile-owned data: %w", err)
	}
	if dataRows == 0 {
		return nil
	}

	profileID := uuid.NewString()
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin default profile migration: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO profiles (id, display_name, avatar_key, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, profileID, defaultProfileName, "player-1", string(core.ProfileRoleAdminPlayer), now, now); err != nil {
		return fmt.Errorf("create default profile: %w", err)
	}
	for _, q := range []string{
		`UPDATE integrations SET profile_id=? WHERE profile_id IS NULL OR profile_id=''`,
		`UPDATE source_games SET profile_id=? WHERE profile_id IS NULL OR profile_id=''`,
		`UPDATE scan_reports SET profile_id=? WHERE profile_id IS NULL OR profile_id=''`,
	} {
		if _, err := tx.Exec(q, profileID); err != nil {
			return fmt.Errorf("assign existing rows to default profile: %w", err)
		}
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO profile_settings (profile_id, key, value, updated_at)
		SELECT ?, key, value, updated_at FROM settings WHERE key IN ('frontend', 'last_sync_push', 'last_sync_pull')`, profileID); err != nil {
		return fmt.Errorf("copy profile settings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit default profile migration: %w", err)
	}
	s.logger.Info("created default admin profile for existing data", "profile_id", profileID)
	return nil
}

func (s *sqliteDatabase) GetDB() *sql.DB {
	return s.db
}

func (s *sqliteDatabase) backfillCanonicalGames() error {
	if s.db == nil {
		return fmt.Errorf("database not connected")
	}

	now := time.Now().Unix()
	if _, err := s.db.Exec(
		`INSERT OR IGNORE INTO canonical_games (id, created_at)
		 SELECT
		   l.canonical_id,
		   COALESCE(MIN(sg.created_at), ?)
		 FROM canonical_source_games_link l
		 LEFT JOIN source_games sg ON sg.id = l.source_game_id
		 GROUP BY l.canonical_id`,
		now,
	); err != nil {
		return fmt.Errorf("canonical game backfill failed: %w", err)
	}

	return nil
}

func (s *sqliteDatabase) migrateLegacyCanonicalIDs() error {
	if s.db == nil {
		return fmt.Errorf("database not connected")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin canonical id migration: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT l.canonical_id, COALESCE(cg.created_at, MIN(sg.created_at), ?)
		FROM canonical_source_games_link l
		LEFT JOIN canonical_games cg ON cg.id = l.canonical_id
		LEFT JOIN source_games sg ON sg.id = l.source_game_id
		WHERE l.canonical_id LIKE 'scan:%'
		GROUP BY l.canonical_id, cg.created_at
	`, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("query legacy canonical ids: %w", err)
	}
	defer rows.Close()

	type legacyCanonicalID struct {
		id        string
		createdAt int64
	}

	var legacyIDs []legacyCanonicalID
	for rows.Next() {
		var item legacyCanonicalID
		if err := rows.Scan(&item.id, &item.createdAt); err != nil {
			return fmt.Errorf("scan legacy canonical id: %w", err)
		}
		legacyIDs = append(legacyIDs, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate legacy canonical ids: %w", err)
	}

	for _, legacy := range legacyIDs {
		newID := uuid.NewString()
		if _, err := tx.Exec(`INSERT INTO canonical_games (id, created_at) VALUES (?, ?)`, newID, legacy.createdAt); err != nil {
			return fmt.Errorf("insert migrated canonical game %s: %w", newID, err)
		}
		if _, err := tx.Exec(`UPDATE canonical_source_games_link SET canonical_id = ? WHERE canonical_id = ?`, newID, legacy.id); err != nil {
			return fmt.Errorf("update canonical links %s -> %s: %w", legacy.id, newID, err)
		}
		if _, err := tx.Exec(`DELETE FROM canonical_games WHERE id = ?`, legacy.id); err != nil {
			return fmt.Errorf("delete legacy canonical game %s: %w", legacy.id, err)
		}
	}

	if _, err := tx.Exec(`
		DELETE FROM canonical_games
		WHERE id LIKE 'scan:%'
		  AND id NOT IN (SELECT DISTINCT canonical_id FROM canonical_source_games_link)
	`); err != nil {
		return fmt.Errorf("cleanup orphaned legacy canonical ids: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit canonical id migration: %w", err)
	}

	if len(legacyIDs) > 0 {
		s.logger.Info("migrated legacy canonical ids", "count", len(legacyIDs))
	}
	return nil
}

func (s *sqliteDatabase) ensureManualReviewSchema() error {
	if s.db == nil {
		return fmt.Errorf("database not connected")
	}
	if err := s.ensureColumn("source_games", "review_state",
		`ALTER TABLE source_games ADD COLUMN review_state TEXT NOT NULL DEFAULT 'pending'`); err != nil {
		return err
	}
	if err := s.ensureColumn("source_games", "manual_review_json",
		`ALTER TABLE source_games ADD COLUMN manual_review_json TEXT`); err != nil {
		return err
	}
	if err := s.ensureColumn("metadata_resolver_matches", "manual_selection",
		`ALTER TABLE metadata_resolver_matches ADD COLUMN manual_selection INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	return nil
}

func (s *sqliteDatabase) ensureGameFileIdentitySchema() error {
	if s.db == nil {
		return fmt.Errorf("database not connected")
	}
	if err := s.ensureColumn("game_files", "object_id",
		`ALTER TABLE game_files ADD COLUMN object_id TEXT`); err != nil {
		return err
	}
	if err := s.ensureColumn("game_files", "revision",
		`ALTER TABLE game_files ADD COLUMN revision TEXT`); err != nil {
		return err
	}
	if err := s.ensureColumn("game_files", "modified_at",
		`ALTER TABLE game_files ADD COLUMN modified_at INTEGER`); err != nil {
		return err
	}
	return nil
}

func (s *sqliteDatabase) ensureMediaAssetDownloadStateSchema() error {
	if s.db == nil {
		return fmt.Errorf("database not connected")
	}
	if err := s.ensureColumn("media_assets", "download_attempts",
		`ALTER TABLE media_assets ADD COLUMN download_attempts INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn("media_assets", "download_failed_at",
		`ALTER TABLE media_assets ADD COLUMN download_failed_at INTEGER`); err != nil {
		return err
	}
	if err := s.ensureColumn("media_assets", "download_last_error",
		`ALTER TABLE media_assets ADD COLUMN download_last_error TEXT`); err != nil {
		return err
	}
	if err := s.ensureColumn("media_assets", "download_permanent_failure",
		`ALTER TABLE media_assets ADD COLUMN download_permanent_failure INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	return nil
}

func (s *sqliteDatabase) ensureColumn(tableName, columnName, alterSQL string) error {
	ok, err := s.hasColumn(tableName, columnName)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if _, err := s.db.Exec(alterSQL); err != nil {
		return fmt.Errorf("add column %s.%s: %w", tableName, columnName, err)
	}
	return nil
}

func (s *sqliteDatabase) hasColumn(tableName, columnName string) (bool, error) {
	rows, err := s.db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, tableName))
	if err != nil {
		return false, fmt.Errorf("inspect table %s: %w", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scan table info %s: %w", tableName, err)
		}
		if name == columnName {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate table info %s: %w", tableName, err)
	}
	return false, nil
}
