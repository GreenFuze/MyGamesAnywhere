package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/runtimeartifact"
)

type RuntimeArtifactRepository struct {
	database core.Database
	now      func() time.Time
}

var _ runtimeartifact.Repository = (*RuntimeArtifactRepository)(nil)

func NewRuntimeArtifactRepository(database core.Database) *RuntimeArtifactRepository {
	return &RuntimeArtifactRepository{database: database, now: time.Now}
}

func (r *RuntimeArtifactRepository) List(ctx context.Context) ([]runtimeartifact.Artifact, error) {
	if err := r.available(); err != nil {
		return nil, err
	}
	rows, err := r.database.GetDB().QueryContext(ctx, runtimeArtifactSelect+` ORDER BY display_name, version, os, architecture`)
	if err != nil {
		return nil, fmt.Errorf("list runtime artifacts: %w", err)
	}
	defer rows.Close()
	artifacts := make([]runtimeartifact.Artifact, 0)
	for rows.Next() {
		artifact, err := scanRuntimeArtifact(rows)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, *artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime artifacts: %w", err)
	}
	return artifacts, nil
}

func (r *RuntimeArtifactRepository) Get(ctx context.Context, artifactID string) (*runtimeartifact.Artifact, error) {
	if err := r.available(); err != nil {
		return nil, err
	}
	artifact, err := scanRuntimeArtifact(r.database.GetDB().QueryRowContext(ctx, runtimeArtifactSelect+` WHERE id=?`, strings.TrimSpace(artifactID)))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return artifact, nil
}

func (r *RuntimeArtifactRepository) Upsert(ctx context.Context, artifact runtimeartifact.Artifact) (*runtimeartifact.Artifact, error) {
	if err := r.available(); err != nil {
		return nil, err
	}
	artifact.Normalize()
	if err := artifact.Validate(); err != nil {
		return nil, err
	}
	now := r.now().UTC().Truncate(time.Second)
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = now
	}
	artifact.UpdatedAt = now
	var releaseAt any
	if artifact.ReleaseObservedAt != nil {
		releaseAt = artifact.ReleaseObservedAt.UTC().Unix()
	}
	_, err := r.database.GetDB().ExecContext(ctx, `INSERT INTO runtime_artifacts
		(id, package_id, display_name, category, version, channel, os, architecture, compatibility_json,
		 license_spdx, license_url, notices, upstream_url, acquisition_mode, redistributable, compliance_state,
		 sha256, signature, release_observed_at, size_bytes, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		 package_id=excluded.package_id, display_name=excluded.display_name, category=excluded.category,
		 version=excluded.version, channel=excluded.channel, os=excluded.os, architecture=excluded.architecture,
		 compatibility_json=excluded.compatibility_json, license_spdx=excluded.license_spdx,
		 license_url=excluded.license_url, notices=excluded.notices, upstream_url=excluded.upstream_url,
		 acquisition_mode=excluded.acquisition_mode, redistributable=excluded.redistributable,
		 compliance_state=excluded.compliance_state, sha256=excluded.sha256, signature=excluded.signature,
		 release_observed_at=excluded.release_observed_at, size_bytes=excluded.size_bytes, updated_at=excluded.updated_at`,
		artifact.ID, artifact.PackageID, artifact.DisplayName, artifact.Category, artifact.Version, artifact.Channel,
		artifact.OS, artifact.Architecture, string(artifact.Compatibility), artifact.LicenseSPDX, nullEmpty(artifact.LicenseURL),
		nullEmpty(artifact.Notices), artifact.UpstreamURL, artifact.AcquisitionMode, boolInt(artifact.Redistributable),
		artifact.ComplianceState, nullEmpty(artifact.SHA256), nullEmpty(artifact.Signature), releaseAt, artifact.SizeBytes,
		artifact.CreatedAt.UTC().Unix(), artifact.UpdatedAt.Unix())
	if err != nil {
		return nil, fmt.Errorf("upsert runtime artifact: %w", err)
	}
	return r.Get(ctx, artifact.ID)
}

const runtimeArtifactSelect = `SELECT id, package_id, display_name, category, version, channel, os, architecture,
	compatibility_json, license_spdx, COALESCE(license_url,''), COALESCE(notices,''), upstream_url,
	acquisition_mode, redistributable, compliance_state, COALESCE(sha256,''), COALESCE(signature,''),
	release_observed_at, size_bytes, created_at, updated_at FROM runtime_artifacts`

type runtimeArtifactScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeArtifact(scanner runtimeArtifactScanner) (*runtimeartifact.Artifact, error) {
	var artifact runtimeartifact.Artifact
	var compatibility string
	var redistributable int
	var releaseAt sql.NullInt64
	var createdAt, updatedAt int64
	err := scanner.Scan(&artifact.ID, &artifact.PackageID, &artifact.DisplayName, &artifact.Category, &artifact.Version,
		&artifact.Channel, &artifact.OS, &artifact.Architecture, &compatibility, &artifact.LicenseSPDX,
		&artifact.LicenseURL, &artifact.Notices, &artifact.UpstreamURL, &artifact.AcquisitionMode, &redistributable,
		&artifact.ComplianceState, &artifact.SHA256, &artifact.Signature, &releaseAt, &artifact.SizeBytes, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("scan runtime artifact: %w", err)
	}
	artifact.Compatibility = []byte(compatibility)
	artifact.Redistributable = redistributable != 0
	artifact.CreatedAt = time.Unix(createdAt, 0).UTC()
	artifact.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	if releaseAt.Valid {
		value := time.Unix(releaseAt.Int64, 0).UTC()
		artifact.ReleaseObservedAt = &value
	}
	return &artifact, nil
}

func (r *RuntimeArtifactRepository) available() error {
	if r == nil || r.database == nil || r.database.GetDB() == nil {
		return fmt.Errorf("runtime artifact database is required")
	}
	return nil
}
