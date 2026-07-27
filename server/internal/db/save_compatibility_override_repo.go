package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/savecompat"
)

const saveOverrideColumns = `id, owner_profile_id, source_domain_id, target_domain_id,
	source_format_id, source_format_version, target_format_id, target_format_version,
	relationship, converter_id, converter_version, reversible, origin, attribution,
	evidence_source, evidence_version, evidence_json, state, reviewed_by_profile_id,
	created_at, updated_at, reviewed_at, revoked_at`

type SaveCompatibilityOverrideRepository struct {
	database core.Database
}

func NewSaveCompatibilityOverrideRepository(database core.Database) *SaveCompatibilityOverrideRepository {
	return &SaveCompatibilityOverrideRepository{database: database}
}

func (r *SaveCompatibilityOverrideRepository) CreateOverride(ctx context.Context, override savecompat.CompatibilityOverride) error {
	db, err := r.db()
	if err != nil {
		return err
	}
	if override.State != savecompat.OverrideStatePending {
		return errors.New("new save compatibility override must be pending")
	}
	if err := override.Validate(); err != nil {
		return err
	}
	var converterID, converterVersion any
	if override.Relationship == savecompat.RelationshipConverter {
		converterID, converterVersion = override.ConverterID, override.ConverterVersion
	}
	_, err = db.ExecContext(ctx, `INSERT INTO save_compatibility_overrides (
		id, owner_profile_id, source_domain_id, target_domain_id,
		source_format_id, source_format_version, target_format_id, target_format_version,
		relationship, converter_id, converter_version, reversible, origin, attribution,
		evidence_source, evidence_version, evidence_json, state, reviewed_by_profile_id,
		created_at, updated_at, reviewed_at, revoked_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, NULL, NULL)`,
		override.ID, override.Scope.OwnerProfileID, override.Scope.SourceDomainID, override.Scope.TargetDomainID,
		override.Scope.Source.ID, override.Scope.Source.Version, override.Scope.Target.ID, override.Scope.Target.Version,
		override.Relationship, converterID, converterVersion, boolInt(override.Reversible), override.Origin, override.Attribution,
		override.EvidenceSource, override.EvidenceVersion, override.EvidenceJSON, override.State,
		override.CreatedAt.UTC().Unix(), override.UpdatedAt.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("persist save compatibility override: %w", err)
	}
	return nil
}

func (r *SaveCompatibilityOverrideRepository) GetOverride(ctx context.Context, id string) (*savecompat.CompatibilityOverride, error) {
	db, err := r.db()
	if err != nil {
		return nil, err
	}
	return loadSaveCompatibilityOverride(db.QueryRowContext(ctx,
		`SELECT `+saveOverrideColumns+` FROM save_compatibility_overrides WHERE id=?`, id))
}

func (r *SaveCompatibilityOverrideRepository) FindApprovedOverride(ctx context.Context, scope savecompat.OverrideScope) (*savecompat.CompatibilityOverride, error) {
	db, err := r.db()
	if err != nil {
		return nil, err
	}
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	return loadSaveCompatibilityOverride(db.QueryRowContext(ctx, `SELECT `+saveOverrideColumns+`
		FROM save_compatibility_overrides
		WHERE owner_profile_id=? AND source_domain_id=? AND target_domain_id=?
			AND source_format_id=? AND source_format_version=?
			AND target_format_id=? AND target_format_version=? AND state='approved'`,
		scope.OwnerProfileID, scope.SourceDomainID, scope.TargetDomainID,
		scope.Source.ID, scope.Source.Version, scope.Target.ID, scope.Target.Version))
}

func (r *SaveCompatibilityOverrideRepository) ApproveOverride(
	ctx context.Context,
	id string,
	reviewerProfileID string,
	reviewedAt time.Time,
	resolveConflict bool,
) (*savecompat.CompatibilityOverride, error) {
	db, err := r.db()
	if err != nil {
		return nil, err
	}
	if reviewedAt.IsZero() {
		return nil, errors.New("override review time is required")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin save compatibility override review: %w", err)
	}
	defer tx.Rollback()

	target, err := loadSaveCompatibilityOverride(tx.QueryRowContext(ctx,
		`SELECT `+saveOverrideColumns+` FROM save_compatibility_overrides WHERE id=?`, id))
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, errors.New("save compatibility override not found")
	}
	if target.State == savecompat.OverrideStateRevoked {
		return nil, errors.New("revoked save compatibility override cannot be approved")
	}
	if target.State == savecompat.OverrideStateConflict && !resolveConflict {
		return nil, savecompat.ErrOverrideResolutionRequired
	}

	existing, err := findApprovedOverrideTx(ctx, tx, target.Scope, target.ID)
	if err != nil {
		return nil, err
	}
	now := reviewedAt.UTC().Unix()
	if existing != nil && !target.DecisionEqual(*existing) && !resolveConflict {
		if _, err := tx.ExecContext(ctx, `UPDATE save_compatibility_overrides
			SET state='conflict', reviewed_by_profile_id=?, reviewed_at=?, revoked_at=NULL, updated_at=?
			WHERE id IN (?, ?)`, reviewerProfileID, now, now, target.ID, existing.ID); err != nil {
			return nil, fmt.Errorf("mark save compatibility override conflict: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit save compatibility override conflict: %w", err)
		}
		conflicted, loadErr := r.GetOverride(ctx, target.ID)
		if loadErr != nil {
			return nil, loadErr
		}
		return conflicted, savecompat.ErrOverrideConflict
	}

	if existing != nil || resolveConflict {
		if _, err := tx.ExecContext(ctx, `UPDATE save_compatibility_overrides
			SET state='revoked', reviewed_by_profile_id=?, reviewed_at=COALESCE(reviewed_at, ?),
				revoked_at=?, updated_at=?
			WHERE owner_profile_id=? AND source_domain_id=? AND target_domain_id=?
				AND source_format_id=? AND source_format_version=?
				AND target_format_id=? AND target_format_version=?
				AND id<>? AND state IN ('pending', 'approved', 'conflict')`,
			reviewerProfileID, now, now, now,
			target.Scope.OwnerProfileID, target.Scope.SourceDomainID, target.Scope.TargetDomainID,
			target.Scope.Source.ID, target.Scope.Source.Version, target.Scope.Target.ID, target.Scope.Target.Version,
			target.ID); err != nil {
			return nil, fmt.Errorf("revoke competing save compatibility overrides: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE save_compatibility_overrides
		SET state='approved', reviewed_by_profile_id=?, reviewed_at=?, revoked_at=NULL, updated_at=?
		WHERE id=?`, reviewerProfileID, now, now, target.ID); err != nil {
		return nil, fmt.Errorf("approve save compatibility override: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit save compatibility override approval: %w", err)
	}
	return r.GetOverride(ctx, target.ID)
}

func (r *SaveCompatibilityOverrideRepository) RevokeOverride(
	ctx context.Context,
	id string,
	actorProfileID string,
	revokedAt time.Time,
	actorIsAdmin bool,
) (*savecompat.CompatibilityOverride, error) {
	db, err := r.db()
	if err != nil {
		return nil, err
	}
	current, err := r.GetOverride(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, errors.New("save compatibility override not found")
	}
	if !actorIsAdmin && current.Scope.OwnerProfileID != actorProfileID {
		return nil, errors.New("only the owning profile or an admin can revoke this save compatibility override")
	}
	if current.State == savecompat.OverrideStateRevoked {
		return current, nil
	}
	now := revokedAt.UTC().Unix()
	if _, err := db.ExecContext(ctx, `UPDATE save_compatibility_overrides
		SET state='revoked', reviewed_by_profile_id=?, reviewed_at=COALESCE(reviewed_at, ?),
			revoked_at=?, updated_at=? WHERE id=?`,
		actorProfileID, now, now, now, id); err != nil {
		return nil, fmt.Errorf("revoke save compatibility override: %w", err)
	}
	return r.GetOverride(ctx, id)
}

func (r *SaveCompatibilityOverrideRepository) db() (*sql.DB, error) {
	if r == nil || r.database == nil || r.database.GetDB() == nil {
		return nil, errors.New("save compatibility override repository is unavailable")
	}
	return r.database.GetDB(), nil
}

type overrideScanner interface {
	Scan(...any) error
}

func loadSaveCompatibilityOverride(scanner overrideScanner) (*savecompat.CompatibilityOverride, error) {
	var result savecompat.CompatibilityOverride
	var converterID, converterVersion, reviewer sql.NullString
	var reversible int
	var createdAt, updatedAt int64
	var reviewedAt, revokedAt sql.NullInt64
	err := scanner.Scan(
		&result.ID, &result.Scope.OwnerProfileID, &result.Scope.SourceDomainID, &result.Scope.TargetDomainID,
		&result.Scope.Source.ID, &result.Scope.Source.Version, &result.Scope.Target.ID, &result.Scope.Target.Version,
		&result.Relationship, &converterID, &converterVersion, &reversible, &result.Origin, &result.Attribution,
		&result.EvidenceSource, &result.EvidenceVersion, &result.EvidenceJSON, &result.State, &reviewer,
		&createdAt, &updatedAt, &reviewedAt, &revokedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load save compatibility override: %w", err)
	}
	result.ConverterID = converterID.String
	result.ConverterVersion = converterVersion.String
	result.Reversible = reversible != 0
	result.ReviewedBy = reviewer.String
	result.CreatedAt = time.Unix(createdAt, 0).UTC()
	result.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	if reviewedAt.Valid {
		value := time.Unix(reviewedAt.Int64, 0).UTC()
		result.ReviewedAt = &value
	}
	if revokedAt.Valid {
		value := time.Unix(revokedAt.Int64, 0).UTC()
		result.RevokedAt = &value
	}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("stored save compatibility override is invalid: %w", err)
	}
	return &result, nil
}

func findApprovedOverrideTx(ctx context.Context, tx *sql.Tx, scope savecompat.OverrideScope, excludeID string) (*savecompat.CompatibilityOverride, error) {
	return loadSaveCompatibilityOverride(tx.QueryRowContext(ctx, `SELECT `+saveOverrideColumns+`
		FROM save_compatibility_overrides
		WHERE owner_profile_id=? AND source_domain_id=? AND target_domain_id=?
			AND source_format_id=? AND source_format_version=?
			AND target_format_id=? AND target_format_version=?
			AND state='approved' AND id<>?`,
		scope.OwnerProfileID, scope.SourceDomainID, scope.TargetDomainID,
		scope.Source.ID, scope.Source.Version, scope.Target.ID, scope.Target.Version, excludeID))
}
