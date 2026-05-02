package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/google/uuid"
)

const defaultProfileName = "Admin Player"

type profileRepository struct {
	db core.Database
}

func NewProfileRepository(db core.Database) core.ProfileRepository {
	return &profileRepository{db: db}
}

func (r *profileRepository) Create(ctx context.Context, profile *core.Profile) error {
	if profile == nil {
		return fmt.Errorf("profile is required")
	}
	normalizeProfile(profile, time.Now())
	if err := validateProfile(profile); err != nil {
		return err
	}
	_, err := r.db.GetDB().ExecContext(ctx, `INSERT INTO profiles (id, display_name, avatar_key, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		profile.ID, profile.DisplayName, profile.AvatarKey, string(profile.Role), profile.CreatedAt.Unix(), profile.UpdatedAt.Unix())
	return err
}

func (r *profileRepository) Update(ctx context.Context, profile *core.Profile) error {
	if profile == nil {
		return fmt.Errorf("profile is required")
	}
	normalizeProfile(profile, time.Now())
	if err := validateProfile(profile); err != nil {
		return err
	}

	existing, err := r.GetByID(ctx, profile.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return sql.ErrNoRows
	}
	if existing.Role == core.ProfileRoleAdminPlayer && profile.Role != core.ProfileRoleAdminPlayer {
		admins, err := r.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if admins <= 1 {
			return fmt.Errorf("cannot demote the last admin player")
		}
	}

	res, err := r.db.GetDB().ExecContext(ctx, `UPDATE profiles
		SET display_name=?, avatar_key=?, role=?, updated_at=?
		WHERE id=?`,
		profile.DisplayName, profile.AvatarKey, string(profile.Role), profile.UpdatedAt.Unix(), profile.ID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *profileRepository) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("profile id is required")
	}
	existing, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return sql.ErrNoRows
	}
	if existing.Role == core.ProfileRoleAdminPlayer {
		admins, err := r.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if admins <= 1 {
			return fmt.Errorf("cannot delete the last admin player")
		}
	}
	res, err := r.db.GetDB().ExecContext(ctx, `DELETE FROM profiles WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *profileRepository) List(ctx context.Context) ([]*core.Profile, error) {
	rows, err := r.db.GetDB().QueryContext(ctx, `SELECT id, display_name, COALESCE(avatar_key,''), role, created_at, updated_at
		FROM profiles ORDER BY created_at ASC, display_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var profiles []*core.Profile
	for rows.Next() {
		profile, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (r *profileRepository) GetByID(ctx context.Context, id string) (*core.Profile, error) {
	row := r.db.GetDB().QueryRowContext(ctx, `SELECT id, display_name, COALESCE(avatar_key,''), role, created_at, updated_at
		FROM profiles WHERE id=?`, id)
	profile, err := scanProfile(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return profile, nil
}

func (r *profileRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles`).Scan(&count)
	return count, err
}

func (r *profileRepository) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := r.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE role=?`, string(core.ProfileRoleAdminPlayer)).Scan(&count)
	return count, err
}

func (r *profileRepository) EnsureDefaultForExistingData(ctx context.Context) (*core.Profile, error) {
	count, err := r.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, nil
	}

	var dataRows int
	if err := r.db.GetDB().QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM integrations) +
			(SELECT COUNT(*) FROM source_games) +
			(SELECT COUNT(*) FROM scan_reports) +
			(SELECT COUNT(*) FROM settings)
	`).Scan(&dataRows); err != nil {
		return nil, err
	}
	if dataRows == 0 {
		return nil, nil
	}

	now := time.Now()
	profile := &core.Profile{
		ID:          uuid.NewString(),
		DisplayName: defaultProfileName,
		AvatarKey:   "player-1",
		Role:        core.ProfileRoleAdminPlayer,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	tx, err := r.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `INSERT INTO profiles (id, display_name, avatar_key, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, profile.ID, profile.DisplayName, profile.AvatarKey, string(profile.Role), now.Unix(), now.Unix()); err != nil {
		return nil, err
	}
	for _, q := range []string{
		`UPDATE integrations SET profile_id=? WHERE profile_id IS NULL OR profile_id=''`,
		`UPDATE source_games SET profile_id=? WHERE profile_id IS NULL OR profile_id=''`,
		`UPDATE scan_reports SET profile_id=? WHERE profile_id IS NULL OR profile_id=''`,
	} {
		if _, err := tx.ExecContext(ctx, q, profile.ID); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO profile_settings (profile_id, key, value, updated_at)
		SELECT ?, key, value, updated_at FROM settings WHERE key IN ('frontend', 'last_sync_push', 'last_sync_pull')`, profile.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return profile, nil
}

type profileScanner interface {
	Scan(dest ...any) error
}

func scanProfile(row profileScanner) (*core.Profile, error) {
	var profile core.Profile
	var role string
	var createdAt, updatedAt int64
	if err := row.Scan(&profile.ID, &profile.DisplayName, &profile.AvatarKey, &role, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	profile.Role = core.ProfileRole(role)
	profile.CreatedAt = time.Unix(createdAt, 0)
	profile.UpdatedAt = time.Unix(updatedAt, 0)
	return &profile, nil
}

func normalizeProfile(profile *core.Profile, now time.Time) {
	profile.ID = strings.TrimSpace(profile.ID)
	if profile.ID == "" {
		profile.ID = uuid.NewString()
	}
	profile.DisplayName = strings.TrimSpace(profile.DisplayName)
	profile.AvatarKey = strings.TrimSpace(profile.AvatarKey)
	if profile.AvatarKey == "" {
		profile.AvatarKey = "player-1"
	}
	if profile.Role == "" {
		profile.Role = core.ProfileRolePlayer
	}
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}
	if profile.UpdatedAt.IsZero() || profile.UpdatedAt.Before(profile.CreatedAt) {
		profile.UpdatedAt = now
	}
}

func validateProfile(profile *core.Profile) error {
	if profile.ID == "" {
		return fmt.Errorf("profile id is required")
	}
	if profile.DisplayName == "" {
		return fmt.Errorf("display_name is required")
	}
	switch profile.Role {
	case core.ProfileRoleAdminPlayer, core.ProfileRolePlayer:
		return nil
	default:
		return fmt.Errorf("invalid profile role: %s", profile.Role)
	}
}
