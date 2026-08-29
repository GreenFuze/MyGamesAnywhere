package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/legacyretirement"
)

// LegacyRetirementRepository reads the retired MGA Client/device-agent tables
// so an owner can still export their recovery evidence during the retirement
// window. Every statement here is read-only.
//
// NO_MIGRATION_NEEDED for MGA-98. Retiring the local client removes Go
// packages, HTTP controllers, the device protocol module, and client packaging,
// but it does not add, drop, rename, or rewrite any table, column, index, or
// persisted JSON/configuration value. Migration 41 remains the latest applied
// version, the twelve classified legacy device tables are preserved read-only
// exactly as migration 38 left them, and the source-cache profile literal
// "device.files.v1" is unchanged (see core.FileDeliverySourceProfile), so an
// existing installation keeps the same stored representation and can still be
// rolled back to the pre-pivot checkpoint binary. Archiving or dropping these
// tables is deliberately deferred to a later ticket that must ship its own
// versioned migration and restorable-backup proof.
type LegacyRetirementRepository struct{ database core.Database }

var _ legacyretirement.Repository = (*LegacyRetirementRepository)(nil)

func NewLegacyRetirementRepository(database core.Database) (*LegacyRetirementRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &LegacyRetirementRepository{database: database}, nil
}

const profileLegacyEndpoints = `SELECT endpoint_id FROM device_grants WHERE profile_id=?
	UNION SELECT endpoint_id FROM device_game_installations WHERE profile_id=?
	UNION SELECT endpoint_id FROM device_storefront_products WHERE profile_id=?`

func (r *LegacyRetirementRepository) BuildReport(ctx context.Context, profileID string, generatedAt time.Time) (*legacyretirement.Report, error) {
	report := &legacyretirement.Report{
		GeneratedAt: generatedAt, ProfileID: profileID,
		RetentionPolicy: "read-only compatibility evidence; two stable releases and at least 90 days",
		RowCounts:       map[string]int{}, Endpoints: []legacyretirement.EndpointObservation{},
		Installations: []legacyretirement.InstallationObservation{}, Preferences: []legacyretirement.InstallPreferenceEvidence{}, Storefront: []legacyretirement.StorefrontObservation{},
		EmulatorPreferences: []legacyretirement.EmulatorPreferenceEvidence{}, EmulatorCorePreferences: []legacyretirement.EmulatorCorePreferenceEvidence{},
		SaveDomainLinks: []legacyretirement.SaveDomainLinkObservation{}, Runtimes: []legacyretirement.RuntimeObservation{},
		PreparedCopies:            []legacyretirement.PreparedCopyObservation{},
		ExcludedSensitiveMaterial: legacyretirement.SensitiveExclusions(),
	}
	if err := r.db().QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations WHERE success=1`).Scan(&report.SchemaVersion); err != nil {
		return nil, fmt.Errorf("read schema version for retirement report: %w", err)
	}
	counts := map[string]struct {
		query string
		args  []any
	}{
		"device_endpoints":                 {`SELECT COUNT(*) FROM device_endpoints WHERE id IN (` + profileLegacyEndpoints + `)`, []any{profileID, profileID, profileID}},
		"device_grants":                    {`SELECT COUNT(*) FROM device_grants WHERE profile_id=?`, []any{profileID}},
		"device_pairing_challenges":        {`SELECT COUNT(*) FROM device_pairing_challenges WHERE profile_id=?`, []any{profileID}},
		"device_commands":                  {`SELECT COUNT(*) FROM device_commands WHERE profile_id=?`, []any{profileID}},
		"device_inventories":               {`SELECT COUNT(*) FROM device_inventories WHERE endpoint_id IN (` + profileLegacyEndpoints + `)`, []any{profileID, profileID, profileID}},
		"device_game_installations":        {`SELECT COUNT(*) FROM device_game_installations WHERE profile_id=?`, []any{profileID}},
		"device_installation_events":       {`SELECT COUNT(*) FROM device_installation_events WHERE endpoint_id IN (` + profileLegacyEndpoints + `)`, []any{profileID, profileID, profileID}},
		"device_install_preferences":       {`SELECT COUNT(*) FROM device_install_preferences WHERE endpoint_id IN (` + profileLegacyEndpoints + `)`, []any{profileID, profileID, profileID}},
		"device_emulator_preferences":      {`SELECT COUNT(*) FROM device_emulator_preferences WHERE endpoint_id IN (` + profileLegacyEndpoints + `)`, []any{profileID, profileID, profileID}},
		"device_emulator_core_preferences": {`SELECT COUNT(*) FROM device_emulator_core_preferences WHERE endpoint_id IN (` + profileLegacyEndpoints + `)`, []any{profileID, profileID, profileID}},
		"device_save_domain_links":         {`SELECT COUNT(*) FROM device_save_domain_links WHERE endpoint_id IN (` + profileLegacyEndpoints + `)`, []any{profileID, profileID, profileID}},
		"device_storefront_products":       {`SELECT COUNT(*) FROM device_storefront_products WHERE profile_id=?`, []any{profileID}},
	}
	for name, statement := range counts {
		var count int
		if err := r.db().QueryRowContext(ctx, statement.query, statement.args...).Scan(&count); err != nil {
			return nil, fmt.Errorf("count %s for retirement report: %w", name, err)
		}
		report.RowCounts[name] = count
	}
	if err := r.readEndpoints(ctx, report); err != nil {
		return nil, err
	}
	if err := r.readInstallations(ctx, report); err != nil {
		return nil, err
	}
	if err := r.readPreferences(ctx, report); err != nil {
		return nil, err
	}
	if err := r.readStorefront(ctx, report); err != nil {
		return nil, err
	}
	if err := r.readEmulatorPreferences(ctx, report); err != nil {
		return nil, err
	}
	if err := r.readSaveDomainLinks(ctx, report); err != nil {
		return nil, err
	}
	if err := r.readInventoryEvidence(ctx, report); err != nil {
		return nil, err
	}
	return report, nil
}

func (r *LegacyRetirementRepository) readEndpoints(ctx context.Context, report *legacyretirement.Report) error {
	rows, err := r.db().QueryContext(ctx, `SELECT id, display_name, host_name, platform, arch, client_version, protocol_version, status, last_seen_at, created_at FROM device_endpoints WHERE id IN (`+profileLegacyEndpoints+`) ORDER BY display_name, id`, report.ProfileID, report.ProfileID, report.ProfileID)
	if err != nil {
		return fmt.Errorf("read legacy endpoints: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item legacyretirement.EndpointObservation
		var last sql.NullInt64
		var created int64
		if err := rows.Scan(&item.ID, &item.DisplayName, &item.HostName, &item.Platform, &item.Architecture, &item.ClientVersion, &item.ProtocolVersion, &item.Status, &last, &created); err != nil {
			return err
		}
		item.LastSeenAt, item.CreatedAt = retirementTime(last), time.Unix(created, 0).UTC()
		report.Endpoints = append(report.Endpoints, item)
	}
	return rows.Err()
}

func (r *LegacyRetirementRepository) readInstallations(ctx context.Context, report *legacyretirement.Report) error {
	rows, err := r.db().QueryContext(ctx, `SELECT endpoint_id, game_id, source_game_id, install_root, install_path, install_kind, COALESCE(installer_family,''), install_state, COALESCE(state_reason,''), installed_at, last_verified_at, updated_at FROM device_game_installations WHERE profile_id=? ORDER BY updated_at DESC`, report.ProfileID)
	if err != nil {
		return fmt.Errorf("read legacy installations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item legacyretirement.InstallationObservation
		var verified sql.NullInt64
		var installed, updated int64
		if err := rows.Scan(&item.EndpointID, &item.GameID, &item.SourceGameID, &item.InstallRoot, &item.InstallPath, &item.InstallKind, &item.InstallerFamily, &item.InstallState, &item.StateReason, &installed, &verified, &updated); err != nil {
			return err
		}
		item.InstalledAt, item.LastVerifiedAt, item.UpdatedAt = time.Unix(installed, 0).UTC(), retirementTime(verified), time.Unix(updated, 0).UTC()
		report.Installations = append(report.Installations, item)
	}
	return rows.Err()
}

func (r *LegacyRetirementRepository) readPreferences(ctx context.Context, report *legacyretirement.Report) error {
	rows, err := r.db().QueryContext(ctx, `SELECT endpoint_id, install_root_template, updated_at FROM device_install_preferences WHERE endpoint_id IN (`+profileLegacyEndpoints+`) ORDER BY endpoint_id`, report.ProfileID, report.ProfileID, report.ProfileID)
	if err != nil {
		return fmt.Errorf("read legacy install preferences: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item legacyretirement.InstallPreferenceEvidence
		var updated int64
		if err := rows.Scan(&item.EndpointID, &item.InstallRootTemplate, &updated); err != nil {
			return err
		}
		item.UpdatedAt = time.Unix(updated, 0).UTC()
		report.Preferences = append(report.Preferences, item)
	}
	return rows.Err()
}

func (r *LegacyRetirementRepository) readStorefront(ctx context.Context, report *legacyretirement.Report) error {
	rows, err := r.db().QueryContext(ctx, `SELECT endpoint_id, game_id, source_game_id, provider, product_id, title, install_path, installed, observed_at, use_granted, granted_at FROM device_storefront_products WHERE profile_id=? ORDER BY observed_at DESC`, report.ProfileID)
	if err != nil {
		return fmt.Errorf("read legacy storefront observations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item legacyretirement.StorefrontObservation
		var installed, useGranted int
		var observed int64
		var granted sql.NullInt64
		if err := rows.Scan(&item.EndpointID, &item.GameID, &item.SourceGameID, &item.Provider, &item.ProductID, &item.Title, &item.InstallPath, &installed, &observed, &useGranted, &granted); err != nil {
			return err
		}
		item.Installed = installed != 0
		item.ObservedAt = time.Unix(observed, 0).UTC()
		item.UseGranted = useGranted != 0
		item.GrantedAt = retirementTime(granted)
		report.Storefront = append(report.Storefront, item)
	}
	return rows.Err()
}

// readEmulatorPreferences exports the retired per-device emulator and core
// selections. They are historical configuration evidence; MGA no longer
// configures or launches an emulator on any device.
func (r *LegacyRetirementRepository) readEmulatorPreferences(ctx context.Context, report *legacyretirement.Report) error {
	rows, err := r.db().QueryContext(ctx, `SELECT endpoint_id, platform, emulator_id, updated_at FROM device_emulator_preferences WHERE endpoint_id IN (`+profileLegacyEndpoints+`) ORDER BY endpoint_id, platform`, report.ProfileID, report.ProfileID, report.ProfileID)
	if err != nil {
		return fmt.Errorf("read legacy emulator preferences: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item legacyretirement.EmulatorPreferenceEvidence
		var updated int64
		if err := rows.Scan(&item.EndpointID, &item.Platform, &item.EmulatorID, &updated); err != nil {
			return err
		}
		item.UpdatedAt = time.Unix(updated, 0).UTC()
		report.EmulatorPreferences = append(report.EmulatorPreferences, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	coreRows, err := r.db().QueryContext(ctx, `SELECT endpoint_id, platform, emulator_id, core_id, updated_at FROM device_emulator_core_preferences WHERE endpoint_id IN (`+profileLegacyEndpoints+`) ORDER BY endpoint_id, platform, emulator_id`, report.ProfileID, report.ProfileID, report.ProfileID)
	if err != nil {
		return fmt.Errorf("read legacy emulator core preferences: %w", err)
	}
	defer coreRows.Close()
	for coreRows.Next() {
		var item legacyretirement.EmulatorCorePreferenceEvidence
		var updated int64
		if err := coreRows.Scan(&item.EndpointID, &item.Platform, &item.EmulatorID, &item.CoreID, &updated); err != nil {
			return err
		}
		item.UpdatedAt = time.Unix(updated, 0).UTC()
		report.EmulatorCorePreferences = append(report.EmulatorCorePreferences, item)
	}
	return coreRows.Err()
}

// readSaveDomainLinks exports the retired device-local save authority records.
// The referenced save data is user-owned and is never deleted or relocated.
func (r *LegacyRetirementRepository) readSaveDomainLinks(ctx context.Context, report *legacyretirement.Report) error {
	rows, err := r.db().QueryContext(ctx, `SELECT endpoint_id, game_id, source_game_id, route_kind, emulator_id, local_save_domain_id, adapter_id, authority_state, sync_state, COALESCE(last_snapshot_manifest_hash,''), created_at, updated_at FROM device_save_domain_links WHERE endpoint_id IN (`+profileLegacyEndpoints+`) ORDER BY updated_at DESC`, report.ProfileID, report.ProfileID, report.ProfileID)
	if err != nil {
		return fmt.Errorf("read legacy save domain links: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item legacyretirement.SaveDomainLinkObservation
		var created, updated int64
		if err := rows.Scan(&item.EndpointID, &item.GameID, &item.SourceGameID, &item.RouteKind, &item.EmulatorID, &item.LocalSaveDomainID, &item.AdapterID, &item.AuthorityState, &item.SyncState, &item.LastSnapshotManifestHash, &created, &updated); err != nil {
			return err
		}
		item.CreatedAt, item.UpdatedAt = time.Unix(created, 0).UTC(), time.Unix(updated, 0).UTC()
		report.SaveDomainLinks = append(report.SaveDomainLinks, item)
	}
	return rows.Err()
}

// readInventoryEvidence decodes the runtime and prepared-copy observations kept
// inside device_inventories so the export stays complete without depending on
// the retired device protocol module.
func (r *LegacyRetirementRepository) readInventoryEvidence(ctx context.Context, report *legacyretirement.Report) error {
	rows, err := r.db().QueryContext(ctx, `SELECT endpoint_id, captured_at, runtimes_json, prepared_copies_json FROM device_inventories WHERE endpoint_id IN (`+profileLegacyEndpoints+`) ORDER BY endpoint_id`, report.ProfileID, report.ProfileID, report.ProfileID)
	if err != nil {
		return fmt.Errorf("read legacy device inventories: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var endpointID, runtimesJSON, preparedJSON string
		var captured int64
		if err := rows.Scan(&endpointID, &captured, &runtimesJSON, &preparedJSON); err != nil {
			return err
		}
		snapshot, err := legacyretirement.DecodeInventory(endpointID, time.Unix(captured, 0).UTC(), runtimesJSON, preparedJSON)
		if err != nil {
			return err
		}
		report.Runtimes = append(report.Runtimes, snapshot.Runtimes...)
		report.PreparedCopies = append(report.PreparedCopies, snapshot.Prepared...)
	}
	return rows.Err()
}

func retirementTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	result := time.Unix(value.Int64, 0).UTC()
	return &result
}
func (r *LegacyRetirementRepository) db() *sql.DB { return r.database.GetDB() }
