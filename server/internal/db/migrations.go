package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const latestMigrationVersion = 9

type migration struct {
	Version int
	Name    string
	SQL     []string
	Run     func(context.Context, *sqliteDatabase) error
}

type appliedMigration struct {
	Version  int
	Name     string
	Checksum string
	Success  bool
}

func (s *sqliteDatabase) Migrate(options core.MigrationOptions) error {
	if s.db == nil {
		return fmt.Errorf("database not connected")
	}
	s.logger.Info("Ensuring database schema...")
	migrations := s.orderedMigrations()
	if err := s.ensureSchemaMigrationsTable(); err != nil {
		return err
	}
	applied, err := s.appliedMigrations()
	if err != nil {
		return err
	}
	if err := validateKnownMigrations(applied, migrations); err != nil {
		return err
	}
	pending := pendingMigrations(applied, migrations)
	if len(pending) == 0 {
		return nil
	}
	if options.BackupBeforeMigrate {
		if _, err := s.backupSQLiteFiles(options.BackupDir); err != nil {
			return fmt.Errorf("backup database before migration: %w", err)
		}
	}
	for _, m := range pending {
		if err := s.runMigration(context.Background(), m); err != nil {
			return err
		}
	}
	return nil
}

func (s *sqliteDatabase) orderedMigrations() []migration {
	return []migration{
		{
			Version: 1,
			Name:    "initial",
			SQL:     initialSchemaStatements(),
		},
		{
			Version: 2,
			Name:    "manual_review_columns",
			Run: func(_ context.Context, db *sqliteDatabase) error {
				return db.ensureManualReviewSchema()
			},
		},
		{
			Version: 3,
			Name:    "game_file_identity_columns",
			Run: func(_ context.Context, db *sqliteDatabase) error {
				return db.ensureGameFileIdentitySchema()
			},
		},
		{
			Version: 4,
			Name:    "media_download_state_columns",
			Run: func(_ context.Context, db *sqliteDatabase) error {
				return db.ensureMediaAssetDownloadStateSchema()
			},
		},
		{
			Version: 5,
			Name:    "profiles_and_profile_owned_data",
			Run: func(_ context.Context, db *sqliteDatabase) error {
				if err := db.ensureProfileSchema(); err != nil {
					return err
				}
				return db.ensureDefaultProfileForExistingData()
			},
		},
		{
			Version: 6,
			Name:    "canonical_id_backfills",
			Run: func(_ context.Context, db *sqliteDatabase) error {
				if err := db.backfillCanonicalGames(); err != nil {
					return err
				}
				return db.migrateLegacyCanonicalIDs()
			},
		},
		{
			Version: 7,
			Name:    "achievement_refresh_states",
			SQL: []string{
				`CREATE TABLE IF NOT EXISTS achievement_refresh_states (
					profile_id TEXT,
					source_game_id TEXT NOT NULL REFERENCES source_games(id) ON DELETE CASCADE,
					integration_id TEXT,
					plugin_id TEXT NOT NULL,
					external_game_id TEXT NOT NULL,
					status TEXT NOT NULL,
					last_attempted_at INTEGER,
					last_success_at INTEGER,
					last_error TEXT,
					PRIMARY KEY(source_game_id, plugin_id)
				);`,
				`CREATE INDEX IF NOT EXISTS idx_achievement_refresh_profile ON achievement_refresh_states(profile_id);`,
				`CREATE INDEX IF NOT EXISTS idx_achievement_refresh_status ON achievement_refresh_states(status);`,
			},
		},
		{
			Version: 8,
			Name:    "profile_scoped_favorites",
			Run: func(_ context.Context, db *sqliteDatabase) error {
				return db.migrateProfileScopedFavorites()
			},
		},
		{
			Version: 9,
			Name:    "canonical_source_pins",
			SQL: []string{
				`CREATE TABLE IF NOT EXISTS canonical_source_pins (
					profile_id TEXT NOT NULL,
					source_game_id TEXT NOT NULL REFERENCES source_games(id) ON DELETE CASCADE,
					canonical_id TEXT NOT NULL REFERENCES canonical_games(id) ON DELETE CASCADE,
					mode TEXT NOT NULL CHECK(mode IN ('split','merge')),
					note TEXT,
					created_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL,
					PRIMARY KEY(profile_id, source_game_id)
				);`,
				`CREATE INDEX IF NOT EXISTS idx_canonical_source_pins_canonical ON canonical_source_pins(canonical_id);`,
				`CREATE INDEX IF NOT EXISTS idx_canonical_source_pins_mode ON canonical_source_pins(mode);`,
			},
		},
	}
}

func (s *sqliteDatabase) ensureSchemaMigrationsTable() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	for _, column := range []struct {
		name string
		sql  string
	}{
		{"name", `ALTER TABLE schema_migrations ADD COLUMN name TEXT`},
		{"checksum", `ALTER TABLE schema_migrations ADD COLUMN checksum TEXT`},
		{"duration_ms", `ALTER TABLE schema_migrations ADD COLUMN duration_ms INTEGER NOT NULL DEFAULT 0`},
		{"success", `ALTER TABLE schema_migrations ADD COLUMN success INTEGER NOT NULL DEFAULT 1`},
	} {
		if err := s.ensureColumn("schema_migrations", column.name, column.sql); err != nil {
			return err
		}
	}
	return nil
}

func (s *sqliteDatabase) appliedMigrations() (map[int]appliedMigration, error) {
	rows, err := s.db.Query(`SELECT version, COALESCE(name, ''), COALESCE(checksum, ''), COALESCE(success, 1) FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()
	applied := map[int]appliedMigration{}
	for rows.Next() {
		var m appliedMigration
		var success int
		if err := rows.Scan(&m.Version, &m.Name, &m.Checksum, &success); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		m.Success = success != 0
		applied[m.Version] = m
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}
	return applied, nil
}

func validateKnownMigrations(applied map[int]appliedMigration, migrations []migration) error {
	known := map[int]migration{}
	maxKnown := 0
	for _, m := range migrations {
		known[m.Version] = m
		if m.Version > maxKnown {
			maxKnown = m.Version
		}
	}
	for version, appliedMigration := range applied {
		if version > maxKnown {
			return fmt.Errorf("database schema version %d is newer than this MGA binary supports (%d)", version, maxKnown)
		}
		expected, ok := known[version]
		if !ok {
			return fmt.Errorf("database contains unknown migration version %d", version)
		}
		if !appliedMigration.Success {
			return fmt.Errorf("database contains failed migration version %d", version)
		}
		expectedChecksum := migrationChecksum(expected)
		if strings.TrimSpace(appliedMigration.Checksum) != "" && appliedMigration.Checksum != expectedChecksum {
			return fmt.Errorf("migration checksum mismatch for version %d (%s)", version, expected.Name)
		}
	}
	return nil
}

func pendingMigrations(applied map[int]appliedMigration, migrations []migration) []migration {
	pending := make([]migration, 0, len(migrations))
	for _, m := range migrations {
		if _, ok := applied[m.Version]; !ok {
			pending = append(pending, m)
		}
	}
	return pending
}

func (s *sqliteDatabase) runMigration(ctx context.Context, m migration) error {
	start := time.Now()
	checksum := migrationChecksum(m)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %03d %s: %w", m.Version, m.Name, err)
	}
	defer tx.Rollback()

	if len(m.SQL) > 0 {
		for _, statement := range m.SQL {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				_ = s.markMigrationFailed(m, checksum, time.Since(start))
				return fmt.Errorf("run SQL migration %03d %s: %w", m.Version, m.Name, err)
			}
		}
	}
	if m.Run != nil {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit SQL phase before Go migration %03d %s: %w", m.Version, m.Name, err)
		}
		if err := m.Run(ctx, s); err != nil {
			_ = s.markMigrationFailed(m, checksum, time.Since(start))
			return fmt.Errorf("run Go migration %03d %s: %w", m.Version, m.Name, err)
		}
		return s.markMigrationApplied(m, checksum, time.Since(start))
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO schema_migrations (version, name, checksum, applied_at, duration_ms, success) VALUES (?, ?, ?, ?, ?, 1)`,
		m.Version, m.Name, checksum, time.Now().Unix(), time.Since(start).Milliseconds()); err != nil {
		return fmt.Errorf("record migration %03d %s: %w", m.Version, m.Name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %03d %s: %w", m.Version, m.Name, err)
	}
	s.logger.Info("applied database migration", "version", m.Version, "name", m.Name)
	return nil
}

func (s *sqliteDatabase) markMigrationApplied(m migration, checksum string, duration time.Duration) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO schema_migrations (version, name, checksum, applied_at, duration_ms, success) VALUES (?, ?, ?, ?, ?, 1)`,
		m.Version, m.Name, checksum, time.Now().Unix(), duration.Milliseconds())
	if err != nil {
		return fmt.Errorf("record migration %03d %s: %w", m.Version, m.Name, err)
	}
	s.logger.Info("applied database migration", "version", m.Version, "name", m.Name)
	return nil
}

func (s *sqliteDatabase) markMigrationFailed(m migration, checksum string, duration time.Duration) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO schema_migrations (version, name, checksum, applied_at, duration_ms, success) VALUES (?, ?, ?, ?, ?, 0)`,
		m.Version, m.Name, checksum, time.Now().Unix(), duration.Milliseconds())
	return err
}

func migrationChecksum(m migration) string {
	h := sha256.New()
	_, _ = io.WriteString(h, fmt.Sprintf("%03d:%s\n", m.Version, m.Name))
	for _, statement := range m.SQL {
		_, _ = io.WriteString(h, strings.TrimSpace(statement))
		_, _ = io.WriteString(h, "\n")
	}
	if m.Run != nil {
		_, _ = io.WriteString(h, "go:")
		_, _ = io.WriteString(h, m.Name)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *sqliteDatabase) backupSQLiteFiles(backupDir string) (string, error) {
	dbPath := strings.TrimSpace(s.config.Get("DB_PATH"))
	if dbPath == "" || dbPath == ":memory:" || strings.HasPrefix(dbPath, "file::memory:") {
		return "", nil
	}
	if backupDir == "" {
		backupDir = filepath.Join(filepath.Dir(dbPath), "migration_backups")
	}
	targetDir := filepath.Join(backupDir, time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}
	for _, path := range sqliteFileTriplet(dbPath) {
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", err
		}
		if err := copyFile(path, filepath.Join(targetDir, filepath.Base(path))); err != nil {
			return "", err
		}
	}
	s.logger.Info("created database migration backup", "path", targetDir)
	return targetDir, nil
}

func sqliteFileTriplet(dbPath string) []string {
	return []string{dbPath, dbPath + "-wal", dbPath + "-shm"}
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
