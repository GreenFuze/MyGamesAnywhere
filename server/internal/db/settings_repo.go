package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type settingRepository struct {
	db core.Database
}

func NewSettingRepository(db core.Database) core.SettingRepository {
	return &settingRepository{db: db}
}

func (r *settingRepository) Upsert(ctx context.Context, s *core.Setting) error {
	if profileID := scopedSettingProfileID(ctx, s.Key); profileID != "" {
		s.ProfileID = profileID
		_, err := r.db.GetDB().ExecContext(ctx, `INSERT INTO profile_settings (profile_id, key, value, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(profile_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			profileID, s.Key, s.Value, s.UpdatedAt.Unix())
		return err
	}
	_, err := r.db.GetDB().ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
	ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, s.Key, s.Value, s.UpdatedAt.Unix())
	return err
}

func (r *settingRepository) Get(ctx context.Context, key string) (*core.Setting, error) {
	if profileID := scopedSettingProfileID(ctx, key); profileID != "" {
		row := r.db.GetDB().QueryRowContext(ctx, `SELECT profile_id, key, value, updated_at FROM profile_settings WHERE profile_id = ? AND key = ?`, profileID, key)
		var s core.Setting
		var updatedAt int64
		if err := row.Scan(&s.ProfileID, &s.Key, &s.Value, &updatedAt); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}
		s.UpdatedAt = time.Unix(updatedAt, 0)
		return &s, nil
	}
	row := r.db.GetDB().QueryRowContext(ctx, `SELECT key, value, updated_at FROM settings WHERE key = ?`, key)
	var s core.Setting
	var updatedAt int64
	if err := row.Scan(&s.Key, &s.Value, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	s.UpdatedAt = time.Unix(updatedAt, 0)
	return &s, nil
}

func (r *settingRepository) List(ctx context.Context) ([]*core.Setting, error) {
	if profileID := core.ProfileIDFromContext(ctx); profileID != "" {
		rows, err := r.db.GetDB().QueryContext(ctx, `SELECT profile_id, key, value, updated_at FROM profile_settings WHERE profile_id = ?`, profileID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var settings []*core.Setting
		for rows.Next() {
			var s core.Setting
			var updatedAt int64
			if err := rows.Scan(&s.ProfileID, &s.Key, &s.Value, &updatedAt); err != nil {
				return nil, err
			}
			s.UpdatedAt = time.Unix(updatedAt, 0)
			settings = append(settings, &s)
		}
		return settings, rows.Err()
	}
	rows, err := r.db.GetDB().QueryContext(ctx, `SELECT key, value, updated_at FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var settings []*core.Setting
	for rows.Next() {
		var s core.Setting
		var updatedAt int64
		if err := rows.Scan(&s.Key, &s.Value, &updatedAt); err != nil {
			return nil, err
		}
		s.UpdatedAt = time.Unix(updatedAt, 0)
		settings = append(settings, &s)
	}
	return settings, nil
}

type integrationRepository struct {
	db core.Database
}

func NewIntegrationRepository(db core.Database) core.IntegrationRepository {
	return &integrationRepository{db: db}
}

func (r *integrationRepository) Create(ctx context.Context, a *core.Integration) error {
	if a.ProfileID == "" {
		a.ProfileID = core.ProfileIDFromContext(ctx)
	}
	_, err := r.db.GetDB().ExecContext(ctx, `INSERT INTO integrations (id, profile_id, plugin_id, label, config_json, integration_type, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, a.ID, a.ProfileID, a.PluginID, a.Label, a.ConfigJSON, a.IntegrationType, a.CreatedAt.Unix(), a.UpdatedAt.Unix())
	return err
}

func (r *integrationRepository) Update(ctx context.Context, a *core.Integration) error {
	if a.ProfileID == "" {
		a.ProfileID = core.ProfileIDFromContext(ctx)
	}
	query := `UPDATE integrations SET profile_id=?, plugin_id=?, label=?, config_json=?, integration_type=?, updated_at=? WHERE id=?`
	args := []any{a.ProfileID, a.PluginID, a.Label, a.ConfigJSON, a.IntegrationType, a.UpdatedAt.Unix(), a.ID}
	if profileID := core.ProfileIDFromContext(ctx); profileID != "" {
		query += ` AND profile_id=?`
		args = append(args, profileID)
	}
	_, err := r.db.GetDB().ExecContext(ctx, query, args...)
	return err
}

func (r *integrationRepository) Delete(ctx context.Context, id string) error {
	if profileID := core.ProfileIDFromContext(ctx); profileID != "" {
		_, err := r.db.GetDB().ExecContext(ctx, "DELETE FROM integrations WHERE id = ? AND profile_id = ?", id, profileID)
		return err
	}
	_, err := r.db.GetDB().ExecContext(ctx, "DELETE FROM integrations WHERE id = ?", id)
	return err
}

func (r *integrationRepository) List(ctx context.Context) ([]*core.Integration, error) {
	query := `SELECT id, COALESCE(profile_id,''), plugin_id, label, config_json, integration_type, created_at, updated_at FROM integrations`
	args := []any{}
	if profileID := core.ProfileIDFromContext(ctx); profileID != "" {
		query += ` WHERE profile_id = ?`
		args = append(args, profileID)
	}
	rows, err := r.db.GetDB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var integrations []*core.Integration
	for rows.Next() {
		var a core.Integration
		var created, updated int64
		if err := rows.Scan(&a.ID, &a.ProfileID, &a.PluginID, &a.Label, &a.ConfigJSON, &a.IntegrationType, &created, &updated); err != nil {
			return nil, err
		}
		a.CreatedAt = time.Unix(created, 0)
		a.UpdatedAt = time.Unix(updated, 0)
		integrations = append(integrations, &a)
	}
	return integrations, nil
}

func (r *integrationRepository) GetByID(ctx context.Context, id string) (*core.Integration, error) {
	query := `SELECT id, COALESCE(profile_id,''), plugin_id, label, config_json, integration_type, created_at, updated_at FROM integrations WHERE id = ?`
	args := []any{id}
	if profileID := core.ProfileIDFromContext(ctx); profileID != "" {
		query += ` AND profile_id = ?`
		args = append(args, profileID)
	}
	row := r.db.GetDB().QueryRowContext(ctx, query, args...)
	var a core.Integration
	var created, updated int64
	if err := row.Scan(&a.ID, &a.ProfileID, &a.PluginID, &a.Label, &a.ConfigJSON, &a.IntegrationType, &created, &updated); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	a.CreatedAt = time.Unix(created, 0)
	a.UpdatedAt = time.Unix(updated, 0)
	return &a, nil
}

func (r *integrationRepository) ListByPluginID(ctx context.Context, pluginID string) ([]*core.Integration, error) {
	query := `SELECT id, COALESCE(profile_id,''), plugin_id, label, config_json, integration_type, created_at, updated_at FROM integrations WHERE plugin_id=?`
	args := []any{pluginID}
	if profileID := core.ProfileIDFromContext(ctx); profileID != "" {
		query += ` AND profile_id = ?`
		args = append(args, profileID)
	}
	rows, err := r.db.GetDB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var integrations []*core.Integration
	for rows.Next() {
		var a core.Integration
		var created, updated int64
		if err := rows.Scan(&a.ID, &a.ProfileID, &a.PluginID, &a.Label, &a.ConfigJSON, &a.IntegrationType, &created, &updated); err != nil {
			return nil, err
		}
		a.CreatedAt = time.Unix(created, 0)
		a.UpdatedAt = time.Unix(updated, 0)
		integrations = append(integrations, &a)
	}
	return integrations, rows.Err()
}

func scopedSettingProfileID(ctx context.Context, key string) string {
	switch key {
	case "frontend", "last_sync_push", "last_sync_pull":
		return core.ProfileIDFromContext(ctx)
	default:
		return ""
	}
}
