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
	_, err := r.db.GetDB().ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
	ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, s.Key, s.Value, s.UpdatedAt.Unix())
	return err
}

func (r *settingRepository) Get(ctx context.Context, key string) (*core.Setting, error) {
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
	_, err := r.db.GetDB().ExecContext(ctx, `INSERT INTO integrations (id, plugin_id, label, config_json, integration_type, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)`, a.ID, a.PluginID, a.Label, a.ConfigJSON, a.IntegrationType, a.CreatedAt.Unix(), a.UpdatedAt.Unix())
	return err
}

func (r *integrationRepository) Update(ctx context.Context, a *core.Integration) error {
	_, err := r.db.GetDB().ExecContext(ctx, `UPDATE integrations SET plugin_id=?, label=?, config_json=?, integration_type=?, updated_at=? WHERE id=?`,
		a.PluginID, a.Label, a.ConfigJSON, a.IntegrationType, a.UpdatedAt.Unix(), a.ID)
	return err
}

func (r *integrationRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.GetDB().ExecContext(ctx, "DELETE FROM integrations WHERE id = ?", id)
	return err
}

func (r *integrationRepository) List(ctx context.Context) ([]*core.Integration, error) {
	rows, err := r.db.GetDB().QueryContext(ctx, `SELECT id, plugin_id, label, config_json, integration_type, created_at, updated_at FROM integrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var integrations []*core.Integration
	for rows.Next() {
		var a core.Integration
		var created, updated int64
		if err := rows.Scan(&a.ID, &a.PluginID, &a.Label, &a.ConfigJSON, &a.IntegrationType, &created, &updated); err != nil {
			return nil, err
		}
		a.CreatedAt = time.Unix(created, 0)
		a.UpdatedAt = time.Unix(updated, 0)
		integrations = append(integrations, &a)
	}
	return integrations, nil
}

func (r *integrationRepository) GetByID(ctx context.Context, id string) (*core.Integration, error) {
	row := r.db.GetDB().QueryRowContext(ctx, `SELECT id, plugin_id, label, config_json, integration_type, created_at, updated_at FROM integrations WHERE id = ?`, id)
	var a core.Integration
	var created, updated int64
	if err := row.Scan(&a.ID, &a.PluginID, &a.Label, &a.ConfigJSON, &a.IntegrationType, &created, &updated); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	a.CreatedAt = time.Unix(created, 0)
	a.UpdatedAt = time.Unix(updated, 0)
	return &a, nil
}
