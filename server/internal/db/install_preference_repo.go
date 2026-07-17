package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/installprefs"
)

type InstallPreferenceRepository struct {
	db core.Database
}

func NewInstallPreferenceRepository(database core.Database) *InstallPreferenceRepository {
	return &InstallPreferenceRepository{db: database}
}

func (r *InstallPreferenceRepository) GetProfileRoot(ctx context.Context, profileID string) (string, error) {
	var root string
	err := r.db.GetDB().QueryRowContext(ctx, `SELECT value FROM profile_settings WHERE profile_id=? AND key=?`, profileID, installprefs.ProfileSettingKey).Scan(&root)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return root, err
}

func (r *InstallPreferenceRepository) SetProfileRoot(ctx context.Context, profileID, rootTemplate string, updatedAt time.Time) error {
	if rootTemplate == "" {
		_, err := r.db.GetDB().ExecContext(ctx, `DELETE FROM profile_settings WHERE profile_id=? AND key=?`, profileID, installprefs.ProfileSettingKey)
		return err
	}
	_, err := r.db.GetDB().ExecContext(ctx, `INSERT INTO profile_settings (profile_id, key, value, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(profile_id, key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, profileID, installprefs.ProfileSettingKey, rootTemplate, updatedAt.Unix())
	return err
}

func (r *InstallPreferenceRepository) GetEndpointRoot(ctx context.Context, endpointID string) (string, error) {
	var root string
	err := r.db.GetDB().QueryRowContext(ctx, `SELECT install_root_template FROM device_install_preferences WHERE endpoint_id=?`, endpointID).Scan(&root)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return root, err
}

func (r *InstallPreferenceRepository) SetEndpointRoot(ctx context.Context, endpointID, rootTemplate, updatedByProfileID string, updatedAt time.Time) error {
	if rootTemplate == "" {
		_, err := r.db.GetDB().ExecContext(ctx, `DELETE FROM device_install_preferences WHERE endpoint_id=?`, endpointID)
		return err
	}
	_, err := r.db.GetDB().ExecContext(ctx, `INSERT INTO device_install_preferences (endpoint_id, install_root_template, updated_by_profile_id, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(endpoint_id) DO UPDATE SET install_root_template=excluded.install_root_template, updated_by_profile_id=excluded.updated_by_profile_id, updated_at=excluded.updated_at`, endpointID, rootTemplate, updatedByProfileID, updatedAt.Unix())
	return err
}
