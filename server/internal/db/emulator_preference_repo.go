package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type EmulatorPreferenceRepository struct {
	db core.Database
}

func NewEmulatorPreferenceRepository(database core.Database) *EmulatorPreferenceRepository {
	return &EmulatorPreferenceRepository{db: database}
}

func (r *EmulatorPreferenceRepository) ListDefaults(ctx context.Context, endpointID string) (map[core.Platform]string, error) {
	rows, err := r.db.GetDB().QueryContext(ctx, `SELECT platform, emulator_id FROM device_emulator_preferences WHERE endpoint_id=?`, endpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[core.Platform]string)
	for rows.Next() {
		var platform core.Platform
		var emulatorID string
		if err := rows.Scan(&platform, &emulatorID); err != nil {
			return nil, err
		}
		result[platform] = emulatorID
	}
	return result, rows.Err()
}

func (r *EmulatorPreferenceRepository) SetDefault(ctx context.Context, endpointID string, platform core.Platform, emulatorID, profileID string, updatedAt time.Time) error {
	if emulatorID == "" {
		_, err := r.db.GetDB().ExecContext(ctx, `DELETE FROM device_emulator_preferences WHERE endpoint_id=? AND platform=?`, endpointID, platform)
		return err
	}
	var profile any = profileID
	if profileID == "" {
		profile = sql.NullString{}
	}
	_, err := r.db.GetDB().ExecContext(ctx, `INSERT INTO device_emulator_preferences (endpoint_id, platform, emulator_id, updated_by_profile_id, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(endpoint_id, platform) DO UPDATE SET emulator_id=excluded.emulator_id, updated_by_profile_id=excluded.updated_by_profile_id, updated_at=excluded.updated_at`, endpointID, platform, emulatorID, profile, updatedAt.Unix())
	return err
}

func (r *EmulatorPreferenceRepository) ListCoreDefaults(ctx context.Context, endpointID string) (map[core.Platform]map[string]string, error) {
	rows, err := r.db.GetDB().QueryContext(ctx, `SELECT platform, emulator_id, core_id FROM device_emulator_core_preferences WHERE endpoint_id=?`, endpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[core.Platform]map[string]string)
	for rows.Next() {
		var platform core.Platform
		var emulatorID, coreID string
		if err := rows.Scan(&platform, &emulatorID, &coreID); err != nil {
			return nil, err
		}
		if result[platform] == nil {
			result[platform] = make(map[string]string)
		}
		result[platform][emulatorID] = coreID
	}
	return result, rows.Err()
}

func (r *EmulatorPreferenceRepository) SetCoreDefault(ctx context.Context, endpointID string, platform core.Platform, emulatorID, coreID, profileID string, updatedAt time.Time) error {
	if coreID == "" {
		_, err := r.db.GetDB().ExecContext(ctx, `DELETE FROM device_emulator_core_preferences WHERE endpoint_id=? AND platform=? AND emulator_id=?`, endpointID, platform, emulatorID)
		return err
	}
	var profile any = profileID
	if profileID == "" {
		profile = sql.NullString{}
	}
	_, err := r.db.GetDB().ExecContext(ctx, `INSERT INTO device_emulator_core_preferences (endpoint_id, platform, emulator_id, core_id, updated_by_profile_id, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(endpoint_id, platform, emulator_id) DO UPDATE SET core_id=excluded.core_id, updated_by_profile_id=excluded.updated_by_profile_id, updated_at=excluded.updated_at`, endpointID, platform, emulatorID, coreID, profile, updatedAt.Unix())
	return err
}
