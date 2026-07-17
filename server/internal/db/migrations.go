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

const latestMigrationVersion = 25

var legacyMigrationChecksums = map[int]map[string]bool{
	// v0.0.9 installs recorded this initial migration checksum before the
	// fresh-schema definition was brought back in line with later migrations.
	1: {
		"3a75c8a20ded02e0892d85f70e29da25b2d65e3bcd2201f4d92cf5b430fe8a7d": true,
	},
}

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
		{
			Version: 10,
			Name:    "integration_needs_reauth",
			Run: func(_ context.Context, db *sqliteDatabase) error {
				if _, err := db.db.Exec(`ALTER TABLE integrations ADD COLUMN needs_reauth INTEGER NOT NULL DEFAULT 0`); err != nil {
					if !strings.Contains(err.Error(), "duplicate column") {
						return fmt.Errorf("add needs_reauth to integrations: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version: 11,
			Name:    "optional_profile_credentials",
			SQL: []string{
				`CREATE TABLE IF NOT EXISTS profile_credentials (
					profile_id TEXT PRIMARY KEY REFERENCES profiles(id) ON DELETE CASCADE,
					kind TEXT NOT NULL CHECK(kind IN ('password','pin')),
					hash TEXT NOT NULL,
					must_change INTEGER NOT NULL DEFAULT 0,
					updated_at INTEGER NOT NULL
				);`,
				`CREATE TABLE IF NOT EXISTS auth_sessions (
					id TEXT PRIMARY KEY,
					token_hash TEXT NOT NULL UNIQUE,
					profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
					created_at INTEGER NOT NULL,
					expires_at INTEGER NOT NULL
				);`,
				`CREATE INDEX IF NOT EXISTS idx_auth_sessions_profile ON auth_sessions(profile_id);`,
				`CREATE INDEX IF NOT EXISTS idx_auth_sessions_expiry ON auth_sessions(expires_at);`,
			},
		},
		{
			Version: 12,
			Name:    "device_endpoints_and_commands",
			SQL: []string{
				`CREATE TABLE IF NOT EXISTS device_endpoints (
					id TEXT PRIMARY KEY,
					client_instance_id TEXT NOT NULL UNIQUE,
					public_key TEXT NOT NULL,
					display_name TEXT NOT NULL,
					host_name TEXT NOT NULL,
					os_user TEXT NOT NULL,
					platform TEXT NOT NULL,
					arch TEXT NOT NULL,
					client_version TEXT NOT NULL,
					protocol_version INTEGER NOT NULL,
					capabilities_json TEXT NOT NULL,
					status TEXT NOT NULL,
					status_reason TEXT,
					last_seen_at INTEGER,
					created_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL
				);`,
				`CREATE TABLE IF NOT EXISTS device_grants (
					endpoint_id TEXT NOT NULL REFERENCES device_endpoints(id) ON DELETE CASCADE,
					profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
					access_level TEXT NOT NULL CHECK(access_level IN ('view','play','manage','owner')),
					created_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL,
					PRIMARY KEY(endpoint_id, profile_id)
				);`,
				`CREATE INDEX IF NOT EXISTS idx_device_grants_profile ON device_grants(profile_id);`,
				`CREATE TABLE IF NOT EXISTS device_pairing_challenges (
					id TEXT PRIMARY KEY,
					code_hash TEXT NOT NULL UNIQUE,
					profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
					created_at INTEGER NOT NULL,
					expires_at INTEGER NOT NULL,
					consumed_at INTEGER
				);`,
				`CREATE INDEX IF NOT EXISTS idx_device_pairing_expiry ON device_pairing_challenges(expires_at);`,
				`CREATE TABLE IF NOT EXISTS device_commands (
					id TEXT PRIMARY KEY,
					endpoint_id TEXT NOT NULL REFERENCES device_endpoints(id) ON DELETE CASCADE,
					profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
					name TEXT NOT NULL,
					schema_version INTEGER NOT NULL,
					idempotency_key TEXT NOT NULL UNIQUE,
					status TEXT NOT NULL,
					payload_json TEXT NOT NULL,
					result_json TEXT,
					error_code TEXT,
					error_message TEXT,
					created_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL,
					expires_at INTEGER NOT NULL
				);`,
				`CREATE INDEX IF NOT EXISTS idx_device_commands_endpoint ON device_commands(endpoint_id, created_at DESC);`,
				`CREATE INDEX IF NOT EXISTS idx_device_commands_profile ON device_commands(profile_id, created_at DESC);`,
			},
		},
		{
			Version: 13,
			Name:    "version_aware_game_identity",
			SQL: []string{
				`CREATE TABLE IF NOT EXISTS game_titles (
					id TEXT PRIMARY KEY,
					profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
					display_title TEXT NOT NULL,
					normalized_title TEXT NOT NULL,
					created_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL
				);`,
				`CREATE INDEX IF NOT EXISTS idx_game_titles_profile ON game_titles(profile_id);`,
				`CREATE INDEX IF NOT EXISTS idx_game_titles_normalized ON game_titles(profile_id, normalized_title);`,
				`CREATE TABLE IF NOT EXISTS game_editions (
					id TEXT PRIMARY KEY REFERENCES canonical_games(id) ON DELETE CASCADE,
					title_id TEXT NOT NULL REFERENCES game_titles(id) ON DELETE CASCADE,
					platform TEXT NOT NULL DEFAULT 'unknown',
					region TEXT,
					edition_label TEXT,
					kind TEXT NOT NULL DEFAULT 'unknown',
					identity_state TEXT NOT NULL CHECK(identity_state IN ('provider_confirmed','manual','unresolved','legacy_review')),
					created_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL
				);`,
				`CREATE INDEX IF NOT EXISTS idx_game_editions_title ON game_editions(title_id);`,
				`CREATE INDEX IF NOT EXISTS idx_game_editions_platform ON game_editions(platform);`,
				`CREATE TABLE IF NOT EXISTS game_title_external_ids (
					title_id TEXT NOT NULL REFERENCES game_titles(id) ON DELETE CASCADE,
					provider TEXT NOT NULL,
					external_id TEXT NOT NULL,
					PRIMARY KEY(title_id, provider, external_id)
				);`,
				`CREATE INDEX IF NOT EXISTS idx_game_title_external_lookup ON game_title_external_ids(provider, external_id);`,
			},
			Run: func(ctx context.Context, db *sqliteDatabase) error {
				return db.rebuildGameIdentity(ctx)
			},
		},
		{
			Version: 14,
			Name:    "device_inventory_snapshots",
			SQL: []string{
				`CREATE TABLE IF NOT EXISTS device_inventories (
					endpoint_id TEXT PRIMARY KEY REFERENCES device_endpoints(id) ON DELETE CASCADE,
					schema_version INTEGER NOT NULL,
					captured_at INTEGER NOT NULL,
					storage_json TEXT NOT NULL,
					runtimes_json TEXT NOT NULL,
					updated_at INTEGER NOT NULL
				);`,
			},
		},
		{
			Version: 15,
			Name:    "device_archive_installations",
			SQL: []string{
				`ALTER TABLE device_commands ADD COLUMN progress_sequence INTEGER NOT NULL DEFAULT 0;`,
				`ALTER TABLE device_commands ADD COLUMN progress_phase TEXT;`,
				`ALTER TABLE device_commands ADD COLUMN progress_percent INTEGER;`,
				`ALTER TABLE device_commands ADD COLUMN progress_message TEXT;`,
				`CREATE TABLE IF NOT EXISTS device_game_installations (
					endpoint_id TEXT NOT NULL REFERENCES device_endpoints(id) ON DELETE CASCADE,
					game_id TEXT NOT NULL REFERENCES canonical_games(id) ON DELETE CASCADE,
					source_game_id TEXT NOT NULL REFERENCES source_games(id) ON DELETE CASCADE,
					profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
					install_root TEXT NOT NULL,
					install_path TEXT NOT NULL,
					archive_sha256 TEXT NOT NULL,
					archive_bytes INTEGER NOT NULL,
					installed_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL,
					PRIMARY KEY(endpoint_id, game_id, source_game_id)
				);`,
				`CREATE INDEX IF NOT EXISTS idx_device_game_installations_game ON device_game_installations(game_id, endpoint_id);`,
				`CREATE INDEX IF NOT EXISTS idx_device_game_installations_profile ON device_game_installations(profile_id, updated_at DESC);`,
			},
		},
		{
			Version: 16,
			Name:    "device_install_launch_and_staged_progress",
			SQL: []string{
				`ALTER TABLE device_commands ADD COLUMN progress_stage TEXT;`,
				`ALTER TABLE device_commands ADD COLUMN progress_stage_percent INTEGER;`,
				`ALTER TABLE device_game_installations ADD COLUMN launch_target TEXT;`,
				`ALTER TABLE device_game_installations ADD COLUMN launch_candidates_json TEXT NOT NULL DEFAULT '[]';`,
			},
		},
		{
			Version: 17,
			Name:    "executable_installation_state",
			SQL: []string{
				`ALTER TABLE device_game_installations ADD COLUMN install_kind TEXT NOT NULL DEFAULT 'managed_archive';`,
				`ALTER TABLE device_game_installations ADD COLUMN installer_family TEXT;`,
				`ALTER TABLE device_game_installations ADD COLUMN installer_files_json TEXT NOT NULL DEFAULT '[]';`,
				`ALTER TABLE device_game_installations ADD COLUMN uninstall_target TEXT;`,
				`ALTER TABLE device_game_installations ADD COLUMN install_state TEXT NOT NULL DEFAULT 'installed';`,
				`ALTER TABLE device_game_installations ADD COLUMN state_reason TEXT;`,
				`ALTER TABLE device_game_installations ADD COLUMN last_verified_at INTEGER;`,
				`ALTER TABLE device_game_installations ADD COLUMN state_changed_at INTEGER;`,
			},
		},
		{
			Version: 18,
			Name:    "failed_install_cleanup",
			SQL: []string{
				`ALTER TABLE device_game_installations ADD COLUMN cleanup_marker_id TEXT;`,
				`ALTER TABLE device_game_installations ADD COLUMN cleanup_ignored_at INTEGER;`,
				`ALTER TABLE device_game_installations ADD COLUMN cleanup_ignored_by_profile_id TEXT REFERENCES profiles(id);`,
				`CREATE TABLE device_installation_events (
					id TEXT PRIMARY KEY,
					endpoint_id TEXT NOT NULL REFERENCES device_endpoints(id) ON DELETE CASCADE,
					game_id TEXT NOT NULL REFERENCES canonical_games(id) ON DELETE CASCADE,
					source_game_id TEXT NOT NULL REFERENCES source_games(id) ON DELETE CASCADE,
					actor_profile_id TEXT REFERENCES profiles(id) ON DELETE SET NULL,
					event_type TEXT NOT NULL CHECK(event_type IN ('failure_detected','post_success_crash_accepted','cleanup_started','cleanup_succeeded','cleanup_failed','failure_ignored','failure_reopened')),
					reason TEXT,
					details_json TEXT NOT NULL DEFAULT '{}',
					created_at INTEGER NOT NULL
				);`,
				`CREATE INDEX idx_device_installation_events_identity_time ON device_installation_events(endpoint_id, game_id, source_game_id, created_at DESC);`,
			},
		},
		{
			Version: 19,
			Name:    "device_endpoint_execution_mode",
			SQL: []string{
				`ALTER TABLE device_endpoints ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'standard' CHECK(execution_mode IN ('standard','elevated'));`,
			},
		},
		{
			Version: 20,
			Name:    "device_installation_reconciliation",
			SQL: []string{
				`ALTER TABLE device_game_installations ADD COLUMN verification_reason_code TEXT;`,
				`ALTER TABLE device_game_installations ADD COLUMN verification_details_json TEXT NOT NULL DEFAULT '{}';`,
				`CREATE TABLE device_installation_events_v20 (
					id TEXT PRIMARY KEY,
					endpoint_id TEXT NOT NULL REFERENCES device_endpoints(id) ON DELETE CASCADE,
					game_id TEXT NOT NULL REFERENCES canonical_games(id) ON DELETE CASCADE,
					source_game_id TEXT NOT NULL REFERENCES source_games(id) ON DELETE CASCADE,
					actor_profile_id TEXT REFERENCES profiles(id) ON DELETE SET NULL,
					event_type TEXT NOT NULL CHECK(event_type IN ('failure_detected','post_success_crash_accepted','cleanup_started','cleanup_succeeded','cleanup_failed','failure_ignored','failure_reopened','installation_missing','installation_needs_repair','installation_restored')),
					reason TEXT,
					details_json TEXT NOT NULL DEFAULT '{}',
					created_at INTEGER NOT NULL
				);`,
				`INSERT INTO device_installation_events_v20 (id, endpoint_id, game_id, source_game_id, actor_profile_id, event_type, reason, details_json, created_at)
				 SELECT id, endpoint_id, game_id, source_game_id, actor_profile_id, event_type, reason, details_json, created_at FROM device_installation_events;`,
				`DROP TABLE device_installation_events;`,
				`ALTER TABLE device_installation_events_v20 RENAME TO device_installation_events;`,
				`CREATE INDEX idx_device_installation_events_identity_time ON device_installation_events(endpoint_id, game_id, source_game_id, created_at DESC);`,
			},
		},
		{
			Version: 21,
			Name:    "device_install_preferences",
			SQL: []string{
				`CREATE TABLE device_install_preferences (
					endpoint_id TEXT PRIMARY KEY REFERENCES device_endpoints(id) ON DELETE CASCADE,
					install_root_template TEXT NOT NULL,
					updated_by_profile_id TEXT REFERENCES profiles(id) ON DELETE SET NULL,
					updated_at INTEGER NOT NULL
				);`,
			},
		},
		{
			Version: 22,
			Name:    "device_emulator_preferences",
			SQL: []string{
				`CREATE TABLE device_emulator_preferences (
					endpoint_id TEXT NOT NULL REFERENCES device_endpoints(id) ON DELETE CASCADE,
					platform TEXT NOT NULL CHECK(length(trim(platform)) > 0),
					emulator_id TEXT NOT NULL CHECK(length(trim(emulator_id)) > 0),
					updated_by_profile_id TEXT REFERENCES profiles(id) ON DELETE SET NULL,
					updated_at INTEGER NOT NULL,
					PRIMARY KEY(endpoint_id, platform)
				);`,
			},
		},
		{
			Version: 23,
			Name:    "device_emulator_core_preferences",
			SQL: []string{
				`CREATE TABLE device_emulator_core_preferences (
					endpoint_id TEXT NOT NULL REFERENCES device_endpoints(id) ON DELETE CASCADE,
					platform TEXT NOT NULL CHECK(length(trim(platform)) > 0),
					emulator_id TEXT NOT NULL CHECK(length(trim(emulator_id)) > 0),
					core_id TEXT NOT NULL CHECK(length(trim(core_id)) > 0),
					updated_by_profile_id TEXT REFERENCES profiles(id) ON DELETE SET NULL,
					updated_at INTEGER NOT NULL,
					PRIMARY KEY(endpoint_id, platform, emulator_id)
				);`,
			},
		},
		{
			Version: 24,
			Name:    "device_inventory_package_managers",
			SQL: []string{
				`ALTER TABLE device_inventories ADD COLUMN package_managers_json TEXT NOT NULL DEFAULT '[]';`,
			},
		},
		{
			Version: 25,
			Name:    "device_inventory_save_adapters",
			SQL: []string{
				`ALTER TABLE device_inventories ADD COLUMN save_adapters_json TEXT NOT NULL DEFAULT '[]';`,
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
		if strings.TrimSpace(appliedMigration.Checksum) != "" &&
			appliedMigration.Checksum != expectedChecksum &&
			!isAcceptedLegacyMigrationChecksum(version, appliedMigration.Checksum) {
			return fmt.Errorf("migration checksum mismatch for version %d (%s)", version, expected.Name)
		}
	}
	return nil
}

func isAcceptedLegacyMigrationChecksum(version int, checksum string) bool {
	allowed, ok := legacyMigrationChecksums[version]
	if !ok {
		return false
	}
	return allowed[checksum]
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
