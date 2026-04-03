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

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	// Enable WAL mode for better concurrent read/write performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		s.logger.Warn("could not enable WAL mode", "error", err)
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
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS integrations (
			id TEXT PRIMARY KEY,
			plugin_id TEXT NOT NULL,
			label TEXT NOT NULL,
			config_json TEXT,
			integration_type TEXT NOT NULL,
			created_at INTEGER,
			updated_at INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS source_games (
			id TEXT PRIMARY KEY,
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
			mime_type TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS source_game_media (
			source_game_id TEXT NOT NULL REFERENCES source_games(id),
			media_asset_id INTEGER NOT NULL REFERENCES media_assets(id),
			type TEXT NOT NULL,
			source TEXT,
			PRIMARY KEY(source_game_id, media_asset_id, type)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sgm_source ON source_game_media(source_game_id);`,
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
	if err := s.ensureManualReviewSchema(); err != nil {
		return err
	}
	if err := s.ensureGameFileIdentitySchema(); err != nil {
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
