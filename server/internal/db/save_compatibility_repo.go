package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/savecompat"
)

type SaveCompatibilityRepository struct {
	database core.Database
	now      func() time.Time
}

func NewSaveCompatibilityRepository(database core.Database) *SaveCompatibilityRepository {
	return &SaveCompatibilityRepository{database: database, now: time.Now}
}

func (r *SaveCompatibilityRepository) UpsertConverter(ctx context.Context, definition savecompat.ConverterDefinition) error {
	if r == nil || r.database == nil || r.database.GetDB() == nil || r.now == nil {
		return fmt.Errorf("save compatibility repository is unavailable")
	}
	if err := definition.Validate(); err != nil {
		return err
	}
	now := r.now().UTC().Unix()
	_, err := r.database.GetDB().ExecContext(ctx, `INSERT INTO save_converter_registry
		(id, version, source_format_id, source_format_version, target_format_id, target_format_version, attribution, implementation_kind, reversible, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id, version) DO UPDATE SET
			source_format_id=excluded.source_format_id,
			source_format_version=excluded.source_format_version,
			target_format_id=excluded.target_format_id,
			target_format_version=excluded.target_format_version,
			attribution=excluded.attribution,
			implementation_kind=excluded.implementation_kind,
			reversible=excluded.reversible,
			enabled=excluded.enabled,
			updated_at=excluded.updated_at`,
		definition.ID, definition.Version,
		definition.Source.ID, definition.Source.Version,
		definition.Target.ID, definition.Target.Version,
		definition.Attribution, definition.ImplementationKind,
		boolInt(definition.Reversible), boolInt(definition.Enabled), now, now,
	)
	if err != nil {
		return fmt.Errorf("persist save converter: %w", err)
	}
	return nil
}

func (r *SaveCompatibilityRepository) GetConverter(ctx context.Context, id, version string) (*savecompat.ConverterDefinition, error) {
	if r == nil || r.database == nil || r.database.GetDB() == nil {
		return nil, fmt.Errorf("save compatibility repository is unavailable")
	}
	var result savecompat.ConverterDefinition
	var reversible, enabled int
	var createdAt, updatedAt int64
	err := r.database.GetDB().QueryRowContext(ctx, `SELECT id, version, source_format_id, source_format_version,
		target_format_id, target_format_version, attribution, implementation_kind, reversible, enabled, created_at, updated_at
		FROM save_converter_registry WHERE id=? AND version=?`, strings.TrimSpace(id), strings.TrimSpace(version)).
		Scan(&result.ID, &result.Version, &result.Source.ID, &result.Source.Version,
			&result.Target.ID, &result.Target.Version, &result.Attribution, &result.ImplementationKind,
			&reversible, &enabled, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load save converter: %w", err)
	}
	result.Reversible = reversible != 0
	result.Enabled = enabled != 0
	result.CreatedAt = time.Unix(createdAt, 0).UTC()
	result.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return &result, nil
}

func (r *SaveCompatibilityRepository) UpsertRule(ctx context.Context, rule savecompat.CompatibilityRule) error {
	if r == nil || r.database == nil || r.database.GetDB() == nil || r.now == nil {
		return fmt.Errorf("save compatibility repository is unavailable")
	}
	if err := rule.Validate(); err != nil {
		return err
	}
	now := r.now().UTC().Unix()
	var converterID, converterVersion any
	if rule.Relationship == savecompat.RelationshipConverter {
		converterID, converterVersion = rule.ConverterID, rule.ConverterVersion
	}
	_, err := r.database.GetDB().ExecContext(ctx, `INSERT INTO save_compatibility_rules
		(id, source_format_id, source_format_version, target_format_id, target_format_version, relationship,
		 converter_id, converter_version, evidence_source, evidence_version, evidence_json, reversible, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			source_format_id=excluded.source_format_id,
			source_format_version=excluded.source_format_version,
			target_format_id=excluded.target_format_id,
			target_format_version=excluded.target_format_version,
			relationship=excluded.relationship,
			converter_id=excluded.converter_id,
			converter_version=excluded.converter_version,
			evidence_source=excluded.evidence_source,
			evidence_version=excluded.evidence_version,
			evidence_json=excluded.evidence_json,
			reversible=excluded.reversible,
			enabled=excluded.enabled,
			updated_at=excluded.updated_at`,
		rule.ID, rule.Source.ID, rule.Source.Version, rule.Target.ID, rule.Target.Version, rule.Relationship,
		converterID, converterVersion, rule.EvidenceSource, rule.EvidenceVersion, rule.EvidenceJSON,
		boolInt(rule.Reversible), boolInt(rule.Enabled), now, now,
	)
	if err != nil {
		return fmt.Errorf("persist save compatibility rule: %w", err)
	}
	return nil
}

func (r *SaveCompatibilityRepository) FindRule(ctx context.Context, source, target savecompat.FormatRef) (*savecompat.CompatibilityRule, error) {
	if r == nil || r.database == nil || r.database.GetDB() == nil {
		return nil, fmt.Errorf("save compatibility repository is unavailable")
	}
	if err := source.Validate(); err != nil {
		return nil, err
	}
	if err := target.Validate(); err != nil {
		return nil, err
	}
	var result savecompat.CompatibilityRule
	var converterID, converterVersion sql.NullString
	var reversible, enabled int
	var createdAt, updatedAt int64
	err := r.database.GetDB().QueryRowContext(ctx, `SELECT id, source_format_id, source_format_version,
		target_format_id, target_format_version, relationship, converter_id, converter_version,
		evidence_source, evidence_version, evidence_json, reversible, enabled, created_at, updated_at
		FROM save_compatibility_rules
		WHERE source_format_id=? AND source_format_version=? AND target_format_id=? AND target_format_version=?`,
		source.ID, source.Version, target.ID, target.Version).
		Scan(&result.ID, &result.Source.ID, &result.Source.Version, &result.Target.ID, &result.Target.Version,
			&result.Relationship, &converterID, &converterVersion, &result.EvidenceSource,
			&result.EvidenceVersion, &result.EvidenceJSON, &reversible, &enabled, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load save compatibility rule: %w", err)
	}
	result.ConverterID = converterID.String
	result.ConverterVersion = converterVersion.String
	result.Reversible = reversible != 0
	result.Enabled = enabled != 0
	result.CreatedAt = time.Unix(createdAt, 0).UTC()
	result.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return &result, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
