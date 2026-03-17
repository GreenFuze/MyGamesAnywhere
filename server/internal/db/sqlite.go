package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
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
	}

	for _, q := range statements {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("schema creation failed: %w", err)
		}
	}
	return nil
}

func (s *sqliteDatabase) GetDB() *sql.DB {
	return s.db
}
