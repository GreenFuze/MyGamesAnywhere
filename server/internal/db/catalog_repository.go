package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/catalog"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/google/uuid"
)

type CatalogRepository struct {
	database core.Database
	newID    func() string
	now      func() time.Time
}

var _ catalog.Repository = (*CatalogRepository)(nil)

func NewCatalogRepository(database core.Database) *CatalogRepository {
	return &CatalogRepository{database: database, newID: uuid.NewString, now: time.Now}
}

type catalogObservationState struct {
	ID               string
	Availability     catalog.Availability
	CurrentVersionID string
	LatestVersionID  string
	ObservedAt       time.Time
}

func (r *CatalogRepository) RecordObservation(ctx context.Context, profileID string, command catalog.ObservationCommand) (*catalog.Offer, error) {
	if err := r.available(); err != nil {
		return nil, err
	}
	profileID = strings.TrimSpace(profileID)
	command.Normalize()
	if profileID == "" {
		return nil, catalog.ErrProfileRequired
	}
	if err := command.Validate(); err != nil {
		return nil, err
	}

	tx, err := r.database.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin catalog observation: %w", err)
	}
	defer tx.Rollback()

	if err := validateCatalogIdentity(ctx, tx, profileID, command); err != nil {
		return nil, err
	}
	offerID, err := r.upsertOffer(ctx, tx, profileID, command)
	if err != nil {
		return nil, err
	}
	currentVersionID, err := r.upsertPackageVersion(ctx, tx, profileID, offerID, command.CurrentVersion, command.ObservedAt)
	if err != nil {
		return nil, err
	}
	latestVersionID, err := r.upsertPackageVersion(ctx, tx, profileID, offerID, command.LatestVersion, command.ObservedAt)
	if err != nil {
		return nil, err
	}

	existingObservationID, err := findCatalogObservationID(ctx, tx, profileID, offerID, command.ObservationKey())
	if err != nil {
		return nil, err
	}
	if existingObservationID != "" {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit idempotent catalog observation: %w", err)
		}
		return r.GetOffer(ctx, profileID, offerID)
	}

	previous, err := latestCatalogObservation(ctx, tx, profileID, offerID)
	if err != nil {
		return nil, err
	}
	if previous != nil && !command.ObservedAt.After(previous.ObservedAt) {
		return nil, fmt.Errorf("catalog observation must be newer than the latest observation at %s", previous.ObservedAt.Format(time.RFC3339))
	}

	eventTypes, err := deriveCatalogEvents(ctx, tx, profileID, offerID, previous, command.Availability, currentVersionID, latestVersionID)
	if err != nil {
		return nil, err
	}

	observationID := r.newID()
	createdAt := r.now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_offer_observations
		(id, profile_id, offer_id, observation_key, entitlement, delivery, availability, current_package_version_id,
		 latest_package_version_id, evidence_source, evidence_json, observed_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		observationID, profileID, offerID, command.ObservationKey(), command.Entitlement, command.Delivery, command.Availability,
		nullEmpty(currentVersionID), nullEmpty(latestVersionID), command.EvidenceSource, string(command.EvidenceJSON),
		command.ObservedAt.UTC().Unix(), createdAt.Unix()); err != nil {
		return nil, fmt.Errorf("insert catalog observation: %w", err)
	}

	for _, eventType := range eventTypes {
		var previousAvailability any
		var previousCurrentVersionID any
		var previousLatestVersionID any
		if previous != nil {
			previousAvailability = string(previous.Availability)
			previousCurrentVersionID = nullEmpty(previous.CurrentVersionID)
			previousLatestVersionID = nullEmpty(previous.LatestVersionID)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_offer_events
			(id, profile_id, offer_id, observation_id, event_type, previous_availability, availability,
			 previous_current_package_version_id, current_package_version_id,
			 previous_latest_package_version_id, latest_package_version_id, occurred_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.newID(), profileID, offerID, observationID, eventType, previousAvailability, command.Availability,
			previousCurrentVersionID, nullEmpty(currentVersionID), previousLatestVersionID, nullEmpty(latestVersionID),
			command.ObservedAt.UTC().Unix(), createdAt.Unix()); err != nil {
			return nil, fmt.Errorf("insert catalog %s event: %w", eventType, err)
		}
	}
	if err := upsertCatalogRefreshSuccess(ctx, tx, profileID, catalog.RefreshScope{
		Provider: command.Provider, IntegrationID: command.IntegrationID, AttemptedAt: command.ObservedAt,
	}, createdAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit catalog observation: %w", err)
	}
	return r.GetOffer(ctx, profileID, offerID)
}

func (r *CatalogRepository) MarkRefreshFailed(ctx context.Context, profileID string, failure catalog.RefreshFailure) error {
	if err := r.available(); err != nil {
		return err
	}
	profileID = strings.TrimSpace(profileID)
	failure.RefreshScope.Normalize()
	if profileID == "" {
		return catalog.ErrProfileRequired
	}
	if err := failure.Validate(); err != nil {
		return err
	}
	tx, err := r.database.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin catalog refresh failure: %w", err)
	}
	defer tx.Rollback()
	if err := validateCatalogRefreshScope(ctx, tx, profileID, failure.RefreshScope); err != nil {
		return err
	}
	now := r.now().UTC().Unix()
	if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_refresh_states
		(profile_id, scope_key, provider, integration_id, last_attempted_at, last_error, stale_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(profile_id, scope_key) DO UPDATE SET
			provider=excluded.provider, integration_id=excluded.integration_id,
			last_attempted_at=MAX(catalog_refresh_states.last_attempted_at, excluded.last_attempted_at),
			last_error=excluded.last_error,
			stale_at=CASE WHEN catalog_refresh_states.stale_at IS NULL THEN excluded.stale_at ELSE MAX(catalog_refresh_states.stale_at, excluded.stale_at) END,
			updated_at=excluded.updated_at`,
		profileID, failure.Key(), failure.Provider, nullEmpty(failure.IntegrationID), failure.AttemptedAt.UTC().Unix(),
		strings.TrimSpace(failure.Error), failure.AttemptedAt.UTC().Unix(), now); err != nil {
		return fmt.Errorf("persist catalog refresh failure: %w", err)
	}
	query := `UPDATE catalog_offers SET stale_at=CASE WHEN stale_at IS NULL THEN ? ELSE MAX(stale_at, ?) END, updated_at=?
		WHERE profile_id=? AND provider=?`
	args := []any{failure.AttemptedAt.UTC().Unix(), failure.AttemptedAt.UTC().Unix(), now, profileID, failure.Provider}
	if failure.IntegrationID == "" {
		query += ` AND integration_id IS NULL`
	} else {
		query += ` AND integration_id=?`
		args = append(args, failure.IntegrationID)
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("mark catalog offers stale: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit catalog refresh failure: %w", err)
	}
	return nil
}

func (r *CatalogRepository) MarkRefreshSucceeded(ctx context.Context, profileID string, scope catalog.RefreshScope) error {
	if err := r.available(); err != nil {
		return err
	}
	profileID = strings.TrimSpace(profileID)
	scope.Normalize()
	if profileID == "" {
		return catalog.ErrProfileRequired
	}
	if err := scope.Validate(); err != nil {
		return err
	}
	tx, err := r.database.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin catalog refresh success: %w", err)
	}
	defer tx.Rollback()
	if err := validateCatalogRefreshScope(ctx, tx, profileID, scope); err != nil {
		return err
	}
	if err := upsertCatalogRefreshSuccess(ctx, tx, profileID, scope, r.now().UTC()); err != nil {
		return err
	}
	query := `UPDATE catalog_offers SET stale_at=NULL, last_success_at=MAX(last_success_at, ?), updated_at=?
		WHERE profile_id=? AND provider=?`
	args := []any{scope.AttemptedAt.UTC().Unix(), r.now().UTC().Unix(), profileID, scope.Provider}
	if scope.IntegrationID == "" {
		query += ` AND integration_id IS NULL`
	} else {
		query += ` AND integration_id=?`
		args = append(args, scope.IntegrationID)
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("clear catalog offer staleness: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit catalog refresh success: %w", err)
	}
	return nil
}

func (r *CatalogRepository) GetOffer(ctx context.Context, profileID, offerID string) (*catalog.Offer, error) {
	if err := r.available(); err != nil {
		return nil, err
	}
	row := r.database.GetDB().QueryRowContext(ctx, catalogOfferSelect+` WHERE o.profile_id=? AND o.id=?`, strings.TrimSpace(profileID), strings.TrimSpace(offerID))
	offer, err := scanCatalogOffer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, catalog.ErrOfferNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load catalog offer: %w", err)
	}
	return offer, nil
}

func (r *CatalogRepository) ListOffers(ctx context.Context, profileID string, filter catalog.OfferFilter) ([]catalog.Offer, error) {
	if err := r.available(); err != nil {
		return nil, err
	}
	query := catalogOfferSelect + ` WHERE o.profile_id=?`
	args := []any{strings.TrimSpace(profileID)}
	if filter.CanonicalGameID != "" {
		query += ` AND o.canonical_game_id=?`
		args = append(args, filter.CanonicalGameID)
	}
	if filter.Provider != "" {
		query += ` AND o.provider=?`
		args = append(args, filter.Provider)
	}
	if filter.Availability != "" {
		query += ` AND obs.availability=?`
		args = append(args, filter.Availability)
	}
	if filter.StaleOnly {
		query += ` AND o.stale_at IS NOT NULL`
	}
	query += ` ORDER BY o.provider, lower(o.sku), o.id`
	rows, err := r.database.GetDB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list catalog offers: %w", err)
	}
	defer rows.Close()
	offers := []catalog.Offer{}
	for rows.Next() {
		offer, err := scanCatalogOffer(rows)
		if err != nil {
			return nil, fmt.Errorf("scan catalog offer: %w", err)
		}
		offers = append(offers, *offer)
	}
	return offers, rows.Err()
}

func (r *CatalogRepository) ListHistory(ctx context.Context, profileID, offerID string, limit int) ([]catalog.HistoryEvent, error) {
	if err := r.available(); err != nil {
		return nil, err
	}
	var exists int
	if err := r.database.GetDB().QueryRowContext(ctx, `SELECT 1 FROM catalog_offers WHERE profile_id=? AND id=?`, profileID, offerID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return nil, catalog.ErrOfferNotFound
	} else if err != nil {
		return nil, fmt.Errorf("verify catalog offer history scope: %w", err)
	}
	rows, err := r.database.GetDB().QueryContext(ctx, `SELECT id, offer_id, observation_id, event_type,
		previous_availability, availability, previous_current_package_version_id, current_package_version_id,
		previous_latest_package_version_id, latest_package_version_id, occurred_at
		FROM catalog_offer_events WHERE profile_id=? AND offer_id=?
		ORDER BY occurred_at DESC, created_at DESC, id DESC LIMIT ?`, profileID, offerID, limit)
	if err != nil {
		return nil, fmt.Errorf("list catalog offer history: %w", err)
	}
	defer rows.Close()
	events := []catalog.HistoryEvent{}
	for rows.Next() {
		var event catalog.HistoryEvent
		var previousAvailability, previousCurrent, current, previousLatest, latest sql.NullString
		var occurredAt int64
		if err := rows.Scan(&event.ID, &event.OfferID, &event.ObservationID, &event.Type,
			&previousAvailability, &event.Availability, &previousCurrent, &current, &previousLatest, &latest, &occurredAt); err != nil {
			return nil, fmt.Errorf("scan catalog offer history: %w", err)
		}
		event.PreviousAvailability = catalog.Availability(previousAvailability.String)
		event.PreviousCurrentVersionID = previousCurrent.String
		event.CurrentVersionID = current.String
		event.PreviousLatestVersionID = previousLatest.String
		event.LatestVersionID = latest.String
		event.OccurredAt = time.Unix(occurredAt, 0).UTC()
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *CatalogRepository) available() error {
	if r == nil || r.database == nil || r.database.GetDB() == nil || r.newID == nil || r.now == nil {
		return errors.New("catalog repository is unavailable")
	}
	return nil
}

func validateCatalogIdentity(ctx context.Context, tx *sql.Tx, profileID string, command catalog.ObservationCommand) error {
	query := `SELECT 1 FROM canonical_source_games_link links
		JOIN source_games source ON source.id=links.source_game_id
		WHERE links.canonical_id=? AND source.profile_id=?`
	args := []any{command.CanonicalGameID, profileID}
	if command.SourceGameID != "" {
		query += ` AND source.id=?`
		args = append(args, command.SourceGameID)
	}
	query += ` LIMIT 1`
	var visible int
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&visible); errors.Is(err, sql.ErrNoRows) {
		return catalog.ErrCatalogIdentityNotVisible
	} else if err != nil {
		return fmt.Errorf("validate catalog game identity: %w", err)
	}
	if command.IntegrationID != "" {
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM integrations WHERE id=? AND profile_id=?`, command.IntegrationID, profileID).Scan(&visible); errors.Is(err, sql.ErrNoRows) {
			return catalog.ErrCatalogIdentityNotVisible
		} else if err != nil {
			return fmt.Errorf("validate catalog integration identity: %w", err)
		}
	}
	return nil
}

func (r *CatalogRepository) upsertOffer(ctx context.Context, tx *sql.Tx, profileID string, command catalog.ObservationCommand) (string, error) {
	var offerID, canonicalGameID, sourceGameID string
	var lastObservedAt int64
	err := tx.QueryRowContext(ctx, `SELECT id, canonical_game_id, COALESCE(source_game_id,''), last_observed_at
		FROM catalog_offers WHERE profile_id=? AND offer_key=?`, profileID, command.OfferKey()).
		Scan(&offerID, &canonicalGameID, &sourceGameID, &lastObservedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("load catalog offer identity: %w", err)
	}
	now := r.now().UTC().Unix()
	observedAt := command.ObservedAt.UTC().Unix()
	if errors.Is(err, sql.ErrNoRows) {
		offerID = r.newID()
		if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_offers
			(id, profile_id, offer_key, canonical_game_id, source_game_id, integration_id, provider, sku, platform, region,
			 entitlement, delivery, evidence_source, evidence_json, first_observed_at, last_observed_at, last_success_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			offerID, profileID, command.OfferKey(), command.CanonicalGameID, nullEmpty(command.SourceGameID), nullEmpty(command.IntegrationID),
			command.Provider, command.SKU, command.Platform, command.Region, command.Entitlement, command.Delivery,
			command.EvidenceSource, string(command.EvidenceJSON), observedAt, observedAt, observedAt, now, now); err != nil {
			return "", fmt.Errorf("insert catalog offer: %w", err)
		}
		return offerID, nil
	}
	if canonicalGameID != command.CanonicalGameID || (sourceGameID != "" && command.SourceGameID != "" && sourceGameID != command.SourceGameID) {
		return "", errors.New("catalog offer identity conflicts with an existing canonical or source game")
	}
	if observedAt < lastObservedAt {
		// An exact historical retry is identified after this lookup. A new
		// out-of-order observation is rejected later in the same transaction,
		// so the current offer projection must not be changed here.
		return offerID, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE catalog_offers SET
		source_game_id=COALESCE(source_game_id, ?), entitlement=?, delivery=?, evidence_source=?, evidence_json=?,
		first_observed_at=MIN(first_observed_at, ?), last_observed_at=MAX(last_observed_at, ?),
		last_success_at=MAX(last_success_at, ?),
		stale_at=CASE WHEN stale_at IS NOT NULL AND stale_at>? THEN stale_at ELSE NULL END,
		updated_at=? WHERE id=? AND profile_id=?`,
		nullEmpty(command.SourceGameID), command.Entitlement, command.Delivery, command.EvidenceSource, string(command.EvidenceJSON),
		observedAt, observedAt, observedAt, observedAt, now, offerID, profileID); err != nil {
		return "", fmt.Errorf("update catalog offer: %w", err)
	}
	return offerID, nil
}

func (r *CatalogRepository) upsertPackageVersion(ctx context.Context, tx *sql.Tx, profileID, offerID string, version *catalog.PackageVersion, observedAt time.Time) (string, error) {
	if version == nil {
		return "", nil
	}
	versionKey := version.IdentityKey()
	var id string
	var existingSize int64
	var existingReleasedAt sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT id FROM catalog_package_versions WHERE profile_id=? AND offer_id=? AND version_key=?`,
		profileID, offerID, versionKey).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("load catalog package version: %w", err)
	}
	now := r.now().UTC().Unix()
	observedUnix := observedAt.UTC().Unix()
	var releasedAt any
	if !version.ReleasedAt.IsZero() {
		releasedAt = version.ReleasedAt.UTC().Unix()
	}
	if errors.Is(err, sql.ErrNoRows) {
		id = r.newID()
		if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_package_versions
			(id, profile_id, offer_id, version_key, version, build_id, channel, source_revision, sha256, size_bytes,
			 released_at, first_observed_at, last_observed_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, profileID, offerID, versionKey, nullEmpty(version.Version), nullEmpty(version.BuildID), nullEmpty(version.Channel),
			nullEmpty(version.SourceRevision), nullEmpty(version.SHA256), version.SizeBytes, releasedAt,
			observedUnix, observedUnix, now, now); err != nil {
			return "", fmt.Errorf("insert catalog package version: %w", err)
		}
		return id, nil
	}
	if err := tx.QueryRowContext(ctx, `SELECT size_bytes, released_at FROM catalog_package_versions WHERE id=? AND profile_id=?`, id, profileID).
		Scan(&existingSize, &existingReleasedAt); err != nil {
		return "", fmt.Errorf("load catalog package version evidence: %w", err)
	}
	if existingSize > 0 && version.SizeBytes > 0 && existingSize != version.SizeBytes {
		return "", errors.New("catalog package version size conflicts with existing evidence")
	}
	if existingReleasedAt.Valid && releasedAt != nil && existingReleasedAt.Int64 != releasedAt.(int64) {
		return "", errors.New("catalog package release time conflicts with existing evidence")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE catalog_package_versions SET
		last_observed_at=MAX(last_observed_at, ?), size_bytes=MAX(size_bytes, ?),
		released_at=COALESCE(released_at, ?), updated_at=? WHERE id=? AND profile_id=?`,
		observedUnix, version.SizeBytes, releasedAt, now, id, profileID); err != nil {
		return "", fmt.Errorf("update catalog package version: %w", err)
	}
	return id, nil
}

func findCatalogObservationID(ctx context.Context, tx *sql.Tx, profileID, offerID, observationKey string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM catalog_offer_observations WHERE profile_id=? AND offer_id=? AND observation_key=?`,
		profileID, offerID, observationKey).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find catalog observation: %w", err)
	}
	return id, nil
}

func latestCatalogObservation(ctx context.Context, tx *sql.Tx, profileID, offerID string) (*catalogObservationState, error) {
	var state catalogObservationState
	var currentVersionID, latestVersionID sql.NullString
	var observedAt int64
	err := tx.QueryRowContext(ctx, `SELECT id, availability, current_package_version_id, latest_package_version_id, observed_at
		FROM catalog_offer_observations WHERE profile_id=? AND offer_id=?
		ORDER BY observed_at DESC, created_at DESC, id DESC LIMIT 1`, profileID, offerID).
		Scan(&state.ID, &state.Availability, &currentVersionID, &latestVersionID, &observedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load latest catalog observation: %w", err)
	}
	state.CurrentVersionID = currentVersionID.String
	state.LatestVersionID = latestVersionID.String
	state.ObservedAt = time.Unix(observedAt, 0).UTC()
	return &state, nil
}

func deriveCatalogEvents(ctx context.Context, tx *sql.Tx, profileID, offerID string, previous *catalogObservationState, availability catalog.Availability, currentVersionID, latestVersionID string) ([]catalog.EventType, error) {
	events := []catalog.EventType{}
	if availability.Active() && (previous == nil || !previous.Availability.Active()) {
		var priorActive int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM catalog_offer_observations
			WHERE profile_id=? AND offer_id=? AND availability IN ('available','leaving_soon')`, profileID, offerID).Scan(&priorActive); err != nil {
			return nil, fmt.Errorf("count prior active catalog observations: %w", err)
		}
		if priorActive == 0 {
			events = append(events, catalog.EventAdded)
		} else {
			events = append(events, catalog.EventReturned)
		}
	}
	if previous != nil && previous.Availability.Active() && availability == catalog.AvailabilityUnavailable {
		events = append(events, catalog.EventRemoved)
	}
	if availability == catalog.AvailabilityLeavingSoon && (previous == nil || previous.Availability != catalog.AvailabilityLeavingSoon) {
		events = append(events, catalog.EventLeavingSoon)
	}
	if previous != nil && (previous.CurrentVersionID != currentVersionID || previous.LatestVersionID != latestVersionID) {
		events = append(events, catalog.EventVersionChanged)
	}
	return events, nil
}

func upsertCatalogRefreshSuccess(ctx context.Context, tx *sql.Tx, profileID string, scope catalog.RefreshScope, updatedAt time.Time) error {
	scope.Normalize()
	if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_refresh_states
		(profile_id, scope_key, provider, integration_id, last_attempted_at, last_success_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(profile_id, scope_key) DO UPDATE SET
			provider=excluded.provider, integration_id=excluded.integration_id,
			last_attempted_at=MAX(catalog_refresh_states.last_attempted_at, excluded.last_attempted_at),
			last_success_at=CASE WHEN catalog_refresh_states.last_success_at IS NULL THEN excluded.last_success_at ELSE MAX(catalog_refresh_states.last_success_at, excluded.last_success_at) END,
			last_error=NULL, stale_at=NULL, updated_at=excluded.updated_at`,
		profileID, scope.Key(), scope.Provider, nullEmpty(scope.IntegrationID), scope.AttemptedAt.UTC().Unix(),
		scope.AttemptedAt.UTC().Unix(), updatedAt.UTC().Unix()); err != nil {
		return fmt.Errorf("persist catalog refresh success: %w", err)
	}
	return nil
}

func validateCatalogRefreshScope(ctx context.Context, tx *sql.Tx, profileID string, scope catalog.RefreshScope) error {
	if scope.IntegrationID == "" {
		return nil
	}
	var visible int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM integrations WHERE id=? AND profile_id=?`, scope.IntegrationID, profileID).Scan(&visible); errors.Is(err, sql.ErrNoRows) {
		return catalog.ErrCatalogIdentityNotVisible
	} else if err != nil {
		return fmt.Errorf("validate catalog refresh scope: %w", err)
	}
	return nil
}

const catalogOfferSelect = `SELECT o.id, o.profile_id, o.canonical_game_id, COALESCE(o.source_game_id,''),
	COALESCE(o.integration_id,''), o.provider, o.sku, o.platform, o.region, obs.entitlement, obs.delivery,
	obs.availability, obs.evidence_source, obs.evidence_json, obs.observed_at,
	o.last_success_at, o.stale_at, o.created_at, o.updated_at,
	current.id, current.version, current.build_id, current.channel, current.source_revision, current.sha256,
	current.size_bytes, current.released_at, current.first_observed_at, current.last_observed_at,
	latest.id, latest.version, latest.build_id, latest.channel, latest.source_revision, latest.sha256,
	latest.size_bytes, latest.released_at, latest.first_observed_at, latest.last_observed_at
	FROM catalog_offers o
	JOIN catalog_offer_observations obs ON obs.id=(
		SELECT candidate.id FROM catalog_offer_observations candidate
		WHERE candidate.profile_id=o.profile_id AND candidate.offer_id=o.id
		ORDER BY candidate.observed_at DESC, candidate.created_at DESC, candidate.id DESC LIMIT 1
	)
	LEFT JOIN catalog_package_versions current ON current.id=obs.current_package_version_id AND current.profile_id=o.profile_id
	LEFT JOIN catalog_package_versions latest ON latest.id=obs.latest_package_version_id AND latest.profile_id=o.profile_id`

type catalogOfferScanner interface {
	Scan(dest ...any) error
}

func scanCatalogOffer(scanner catalogOfferScanner) (*catalog.Offer, error) {
	var offer catalog.Offer
	var evidenceJSON string
	var observedAt, lastSuccessAt, createdAt, updatedAt int64
	var staleAt sql.NullInt64
	current := nullableCatalogPackage{}
	latest := nullableCatalogPackage{}
	if err := scanner.Scan(
		&offer.ID, &offer.ProfileID, &offer.CanonicalGameID, &offer.SourceGameID, &offer.IntegrationID,
		&offer.Provider, &offer.SKU, &offer.Platform, &offer.Region, &offer.Entitlement, &offer.Delivery,
		&offer.Availability, &offer.EvidenceSource, &evidenceJSON, &observedAt,
		&lastSuccessAt, &staleAt, &createdAt, &updatedAt,
		&current.ID, &current.Version, &current.BuildID, &current.Channel, &current.SourceRevision, &current.SHA256,
		&current.SizeBytes, &current.ReleasedAt, &current.FirstObservedAt, &current.LastObservedAt,
		&latest.ID, &latest.Version, &latest.BuildID, &latest.Channel, &latest.SourceRevision, &latest.SHA256,
		&latest.SizeBytes, &latest.ReleasedAt, &latest.FirstObservedAt, &latest.LastObservedAt,
	); err != nil {
		return nil, err
	}
	offer.EvidenceJSON = []byte(evidenceJSON)
	offer.ObservedAt = time.Unix(observedAt, 0).UTC()
	offer.LastSuccessAt = time.Unix(lastSuccessAt, 0).UTC()
	if staleAt.Valid {
		stale := time.Unix(staleAt.Int64, 0).UTC()
		offer.StaleAt = &stale
	}
	offer.CreatedAt = time.Unix(createdAt, 0).UTC()
	offer.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	offer.CurrentVersion = current.value(offer.ID)
	offer.LatestVersion = latest.value(offer.ID)
	return &offer, nil
}

type nullableCatalogPackage struct {
	ID              sql.NullString
	Version         sql.NullString
	BuildID         sql.NullString
	Channel         sql.NullString
	SourceRevision  sql.NullString
	SHA256          sql.NullString
	SizeBytes       sql.NullInt64
	ReleasedAt      sql.NullInt64
	FirstObservedAt sql.NullInt64
	LastObservedAt  sql.NullInt64
}

func (p nullableCatalogPackage) value(offerID string) *catalog.PackageVersion {
	if !p.ID.Valid {
		return nil
	}
	result := &catalog.PackageVersion{
		ID: p.ID.String, OfferID: offerID, Version: p.Version.String, BuildID: p.BuildID.String,
		Channel: p.Channel.String, SourceRevision: p.SourceRevision.String, SHA256: p.SHA256.String,
		SizeBytes: p.SizeBytes.Int64,
	}
	if p.ReleasedAt.Valid {
		result.ReleasedAt = time.Unix(p.ReleasedAt.Int64, 0).UTC()
	}
	if p.FirstObservedAt.Valid {
		result.FirstObservedAt = time.Unix(p.FirstObservedAt.Int64, 0).UTC()
	}
	if p.LastObservedAt.Valid {
		result.LastObservedAt = time.Unix(p.LastObservedAt.Int64, 0).UTC()
	}
	return result
}
