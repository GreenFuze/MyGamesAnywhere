package db

import (
	"database/sql"
	"fmt"

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

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open sqlite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping sqlite database: %w", err)
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

func (s *sqliteDatabase) Migrate() error {
	s.logger.Info("Running migrations...")

	creates := []string{
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
		`CREATE TABLE IF NOT EXISTS games (
			id TEXT PRIMARY KEY,
			title TEXT,
			platform TEXT NOT NULL,
			kind TEXT NOT NULL,
			parent_game_id TEXT,
			package_kind TEXT NOT NULL,
			root_path TEXT,
			integration_id TEXT,
			confidence TEXT,
			status TEXT NOT NULL DEFAULT 'found',
			last_seen_at INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS game_files (
			game_id TEXT NOT NULL,
			path TEXT NOT NULL,
			file_name TEXT NOT NULL,
			role TEXT NOT NULL,
			file_kind TEXT,
			size INTEGER NOT NULL,
			is_dir INTEGER NOT NULL,
			PRIMARY KEY(game_id, path),
			FOREIGN KEY(game_id) REFERENCES games(id)
		);`,
	}
	for _, q := range creates {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migration create failed: %w", err)
		}
	}
	return nil
}

func (s *sqliteDatabase) GetDB() *sql.DB {
	return s.db
}
