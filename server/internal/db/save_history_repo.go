package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/savehistory"
)

type SaveHistoryRepository struct {
	database core.Database
	now      func() time.Time
}

func NewSaveHistoryRepository(database core.Database) *SaveHistoryRepository {
	return &SaveHistoryRepository{database: database, now: time.Now}
}

func (r *SaveHistoryRepository) GetPolicy(ctx context.Context, profileID, domainID string) (savehistory.Policy, error) {
	policy := savehistory.DefaultPolicy(profileID, domainID)
	if err := policy.Validate(); err != nil {
		return savehistory.Policy{}, err
	}
	var updatedAt int64
	err := r.database.GetDB().QueryRowContext(ctx, `SELECT retain_versions, retain_days, updated_at
		FROM save_domain_policies WHERE profile_id=? AND domain_id=?`, profileID, domainID).
		Scan(&policy.RetainVersions, &policy.RetainDays, &updatedAt)
	if err == sql.ErrNoRows {
		return policy, nil
	}
	if err != nil {
		return savehistory.Policy{}, fmt.Errorf("load save history policy: %w", err)
	}
	policy.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return policy, nil
}

func (r *SaveHistoryRepository) UpsertPolicy(ctx context.Context, policy savehistory.Policy) ([]savehistory.Version, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	now := r.now().UTC()
	tx, err := r.database.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO save_domain_policies
		(profile_id, domain_id, retain_versions, retain_days, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(profile_id, domain_id) DO UPDATE SET
			retain_versions=excluded.retain_versions, retain_days=excluded.retain_days, updated_at=excluded.updated_at`,
		policy.ProfileID, policy.DomainID, policy.RetainVersions, policy.RetainDays, now.Unix())
	if err != nil {
		return nil, fmt.Errorf("persist save history policy: %w", err)
	}
	pruned, err := pruneSaveHistoryVersions(ctx, tx, policy, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return pruned, nil
}

func (r *SaveHistoryRepository) RecordVersion(ctx context.Context, version savehistory.Version, policy savehistory.Policy) ([]savehistory.Version, error) {
	if err := version.Validate(); err != nil {
		return nil, err
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if version.ProfileID != policy.ProfileID || version.DomainID != policy.DomainID {
		return nil, fmt.Errorf("save history policy does not own this version")
	}
	tx, err := r.database.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var reportedAt any
	if version.ReportedAt != nil {
		reportedAt = version.ReportedAt.UTC().Unix()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO save_domain_versions
		(id, profile_id, domain_id, canonical_game_id, source_game_id, runtime, slot_id, integration_id,
		 manifest_hash, origin_label, route_label, accepted_at, reported_at, file_count, total_size, payload_key, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		version.ID, version.ProfileID, version.DomainID, version.CanonicalGameID, version.SourceGameID,
		version.Runtime, version.SlotID, version.IntegrationID, version.ManifestHash, version.OriginLabel,
		version.RouteLabel, version.AcceptedAt.UTC().UnixNano(), reportedAt, version.FileCount, version.TotalSize,
		version.PayloadKey, r.now().UTC().Unix())
	if err != nil {
		return nil, fmt.Errorf("record save history version: %w", err)
	}
	pruned, err := pruneSaveHistoryVersions(ctx, tx, policy, r.now().UTC())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return pruned, nil
}

type saveHistoryTransaction interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func pruneSaveHistoryVersions(ctx context.Context, tx saveHistoryTransaction, policy savehistory.Policy, now time.Time) ([]savehistory.Version, error) {
	cutoff := now.AddDate(0, 0, -policy.RetainDays).UnixNano()
	rows, err := tx.QueryContext(ctx, `SELECT id, profile_id, domain_id, canonical_game_id, source_game_id,
		runtime, slot_id, integration_id, manifest_hash, origin_label, route_label, accepted_at, reported_at,
		file_count, total_size, payload_key, created_at
		FROM save_domain_versions WHERE profile_id=? AND domain_id=?
		ORDER BY accepted_at DESC, id DESC`, policy.ProfileID, policy.DomainID)
	if err != nil {
		return nil, err
	}
	var all []savehistory.Version
	for rows.Next() {
		item, err := scanSaveHistoryVersion(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, item)
	}
	rows.Close()
	var pruned []savehistory.Version
	for index, item := range all {
		if index >= policy.RetainVersions || item.AcceptedAt.UnixNano() < cutoff {
			if _, err := tx.ExecContext(ctx, `DELETE FROM save_domain_versions WHERE id=?`, item.ID); err != nil {
				return nil, err
			}
			pruned = append(pruned, item)
		}
	}
	return pruned, nil
}

func (r *SaveHistoryRepository) ListVersions(ctx context.Context, profileID, domainID string) ([]savehistory.Version, error) {
	policy := savehistory.DefaultPolicy(profileID, domainID)
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.database.GetDB().QueryContext(ctx, `SELECT id, profile_id, domain_id, canonical_game_id, source_game_id,
		runtime, slot_id, integration_id, manifest_hash, origin_label, route_label, accepted_at, reported_at,
		file_count, total_size, payload_key, created_at
		FROM save_domain_versions WHERE profile_id=? AND domain_id=?
		ORDER BY accepted_at DESC, id DESC`, profileID, domainID)
	if err != nil {
		return nil, fmt.Errorf("list save history: %w", err)
	}
	defer rows.Close()
	var result []savehistory.Version
	for rows.Next() {
		item, err := scanSaveHistoryVersion(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *SaveHistoryRepository) GetVersion(ctx context.Context, profileID, versionID string) (*savehistory.Version, error) {
	if err := savehistory.DefaultPolicy(profileID, versionID).Validate(); err != nil {
		return nil, err
	}
	row := r.database.GetDB().QueryRowContext(ctx, `SELECT id, profile_id, domain_id, canonical_game_id, source_game_id,
		runtime, slot_id, integration_id, manifest_hash, origin_label, route_label, accepted_at, reported_at,
		file_count, total_size, payload_key, created_at
		FROM save_domain_versions WHERE profile_id=? AND id=?`, profileID, versionID)
	item, err := scanSaveHistoryVersion(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load save history version: %w", err)
	}
	return &item, nil
}

type saveHistoryScanner interface{ Scan(...any) error }

func scanSaveHistoryVersion(scanner saveHistoryScanner) (savehistory.Version, error) {
	var result savehistory.Version
	var acceptedAt, createdAt int64
	var reportedAt sql.NullInt64
	err := scanner.Scan(&result.ID, &result.ProfileID, &result.DomainID, &result.CanonicalGameID,
		&result.SourceGameID, &result.Runtime, &result.SlotID, &result.IntegrationID, &result.ManifestHash,
		&result.OriginLabel, &result.RouteLabel, &acceptedAt, &reportedAt, &result.FileCount,
		&result.TotalSize, &result.PayloadKey, &createdAt)
	if err != nil {
		return savehistory.Version{}, err
	}
	result.AcceptedAt = time.Unix(0, acceptedAt).UTC()
	result.CreatedAt = time.Unix(createdAt, 0).UTC()
	if reportedAt.Valid {
		value := time.Unix(reportedAt.Int64, 0).UTC()
		result.ReportedAt = &value
	}
	return result, nil
}
