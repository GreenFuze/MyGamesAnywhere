package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
	"github.com/google/uuid"
)

type DeviceStore struct {
	db core.Database
}

func NewDeviceStore(database core.Database) *DeviceStore {
	return &DeviceStore{db: database}
}

func (s *DeviceStore) CreatePairingChallenge(ctx context.Context, challenge devices.PairingChallenge) error {
	_, err := s.db.GetDB().ExecContext(ctx, `INSERT INTO device_pairing_challenges
		(id, code_hash, profile_id, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		challenge.ID, challenge.CodeHash, challenge.ProfileID, challenge.CreatedAt.Unix(), challenge.ExpiresAt.Unix())
	return err
}

// NO_MIGRATION_NEEDED: adding another profile grant for an existing endpoint
// reuses the pairing-challenge, endpoint, and grant schema created by migration
// 12. Existing rows and client binding documents remain valid and unchanged.
func (s *DeviceStore) PairEndpoint(ctx context.Context, codeHash string, now time.Time, endpoint devices.Endpoint) (string, error) {
	if endpoint.ExecutionMode == "" {
		endpoint.ExecutionMode = devicev1.ClientExecutionModeStandard
	}
	if err := endpoint.ExecutionMode.Validate(); err != nil {
		return "", err
	}
	tx, err := s.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var profileID string
	err = tx.QueryRowContext(ctx, `SELECT profile_id FROM device_pairing_challenges
		WHERE code_hash=? AND consumed_at IS NULL AND expires_at>?`, codeHash, now.Unix()).Scan(&profileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", devices.ErrInvalidPairingCode
		}
		return "", err
	}
	var storedInstanceID, storedPublicKey string
	existingErr := tx.QueryRowContext(ctx, `SELECT client_instance_id, public_key FROM device_endpoints WHERE id=?`, endpoint.ID).
		Scan(&storedInstanceID, &storedPublicKey)
	switch {
	case existingErr == nil:
		if storedInstanceID != endpoint.ClientInstanceID || storedPublicKey != endpoint.PublicKey {
			return "", devices.ErrPairingIdentity
		}
	case !errors.Is(existingErr, sql.ErrNoRows):
		return "", existingErr
	default:
		capabilities, marshalErr := json.Marshal(endpoint.Capabilities)
		if marshalErr != nil {
			return "", fmt.Errorf("marshal endpoint capabilities: %w", marshalErr)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO device_endpoints
			(id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, client_version,
			 execution_mode, protocol_version, capabilities_json, status, status_reason, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
			endpoint.ID, endpoint.ClientInstanceID, endpoint.PublicKey, endpoint.DisplayName, endpoint.HostName, endpoint.OSUser,
			endpoint.Platform, endpoint.Arch, endpoint.ClientVersion, endpoint.ExecutionMode, endpoint.ProtocolVersion, string(capabilities),
			string(devicev1.EndpointOffline), now.Unix(), now.Unix())
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return "", devices.ErrClientAlreadyPaired
			}
			return "", err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO device_grants
		(endpoint_id, profile_id, access_level, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(endpoint_id, profile_id) DO UPDATE SET access_level=excluded.access_level, updated_at=excluded.updated_at`,
		endpoint.ID, profileID, string(devicev1.AccessOwner), now.Unix(), now.Unix()); err != nil {
		return "", err
	}
	result, err := tx.ExecContext(ctx, `UPDATE device_pairing_challenges SET consumed_at=?
		WHERE code_hash=? AND consumed_at IS NULL`, now.Unix(), codeHash)
	if err != nil {
		return "", err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return "", devices.ErrInvalidPairingCode
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return profileID, nil
}

func (s *DeviceStore) GetEndpoint(ctx context.Context, endpointID string) (*devices.Endpoint, error) {
	row := s.db.GetDB().QueryRowContext(ctx, endpointFields+` FROM device_endpoints e WHERE e.id=?`, endpointID)
	endpoint, err := scanEndpoint(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return endpoint, err
}

func (s *DeviceStore) ListEndpoints(ctx context.Context, profileID string) ([]devices.Endpoint, error) {
	rows, err := s.db.GetDB().QueryContext(ctx, endpointFields+`, g.access_level FROM device_endpoints e
		JOIN device_grants g ON g.endpoint_id=e.id WHERE g.profile_id=?
		ORDER BY e.display_name COLLATE NOCASE, e.created_at`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	endpoints := make([]devices.Endpoint, 0)
	for rows.Next() {
		endpoint, err := scanEndpointWithGrant(rows)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, *endpoint)
	}
	return endpoints, rows.Err()
}

func (s *DeviceStore) GetGrant(ctx context.Context, endpointID, profileID string) (devicev1.AccessLevel, error) {
	var level string
	err := s.db.GetDB().QueryRowContext(ctx, `SELECT access_level FROM device_grants WHERE endpoint_id=? AND profile_id=?`, endpointID, profileID).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) {
		return "", devices.ErrDeviceForbidden
	}
	return devicev1.AccessLevel(level), err
}

func (s *DeviceStore) ListGrants(ctx context.Context, endpointID string) ([]devices.Grant, error) {
	rows, err := s.db.GetDB().QueryContext(ctx, `SELECT g.endpoint_id, g.profile_id, p.display_name, p.role,
		g.access_level, g.created_at, g.updated_at
		FROM device_grants g JOIN profiles p ON p.id=g.profile_id
		WHERE g.endpoint_id=? ORDER BY p.display_name COLLATE NOCASE, g.profile_id`, endpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grants := make([]devices.Grant, 0)
	for rows.Next() {
		var grant devices.Grant
		var createdAt, updatedAt int64
		if err := rows.Scan(&grant.EndpointID, &grant.ProfileID, &grant.ProfileDisplayName, &grant.ProfileRole,
			&grant.AccessLevel, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		grant.CreatedAt = time.Unix(createdAt, 0)
		grant.UpdatedAt = time.Unix(updatedAt, 0)
		grants = append(grants, grant)
	}
	return grants, rows.Err()
}

func (s *DeviceStore) SetGrant(ctx context.Context, endpointID, profileID string, accessLevel devicev1.AccessLevel, now time.Time) error {
	tx, err := s.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentLevel string
	err = tx.QueryRowContext(ctx, `SELECT access_level FROM device_grants WHERE endpoint_id=? AND profile_id=?`, endpointID, profileID).Scan(&currentLevel)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if currentLevel == string(devicev1.AccessOwner) && accessLevel != devicev1.AccessOwner {
		if err := ensureAnotherOwner(ctx, tx, endpointID); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO device_grants (endpoint_id, profile_id, access_level, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(endpoint_id, profile_id) DO UPDATE SET access_level=excluded.access_level, updated_at=excluded.updated_at`,
		endpointID, profileID, string(accessLevel), now.Unix(), now.Unix())
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *DeviceStore) DeleteGrant(ctx context.Context, endpointID, profileID string) error {
	tx, err := s.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentLevel string
	if err := tx.QueryRowContext(ctx, `SELECT access_level FROM device_grants WHERE endpoint_id=? AND profile_id=?`, endpointID, profileID).Scan(&currentLevel); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return devices.ErrGrantNotFound
		}
		return err
	}
	if currentLevel == string(devicev1.AccessOwner) {
		if err := ensureAnotherOwner(ctx, tx, endpointID); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM device_grants WHERE endpoint_id=? AND profile_id=?`, endpointID, profileID)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return devices.ErrGrantNotFound
	}
	return tx.Commit()
}

func ensureAnotherOwner(ctx context.Context, tx *sql.Tx, endpointID string) error {
	var owners int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM device_grants WHERE endpoint_id=? AND access_level=?`,
		endpointID, string(devicev1.AccessOwner)).Scan(&owners); err != nil {
		return err
	}
	if owners <= 1 {
		return devices.ErrLastOwner
	}
	return nil
}

func (s *DeviceStore) UpdateEndpointConnection(ctx context.Context, endpoint devices.Endpoint) error {
	capabilities, err := json.Marshal(endpoint.Capabilities)
	if err != nil {
		return err
	}
	_, err = s.db.GetDB().ExecContext(ctx, `UPDATE device_endpoints SET
		host_name=?, os_user=?, platform=?, arch=?, execution_mode=?, client_version=?, protocol_version=?, capabilities_json=?,
		status=?, status_reason=?, last_seen_at=?, updated_at=? WHERE id=?`,
		endpoint.HostName, endpoint.OSUser, endpoint.Platform, endpoint.Arch, endpoint.ExecutionMode, endpoint.ClientVersion, endpoint.ProtocolVersion,
		string(capabilities), string(endpoint.Status), endpoint.StatusReason, unixOrNil(endpoint.LastSeenAt), endpoint.UpdatedAt.Unix(), endpoint.ID)
	return err
}

func (s *DeviceStore) SetEndpointStatus(ctx context.Context, endpointID string, status devicev1.EndpointState, reason string, seenAt time.Time) error {
	_, err := s.db.GetDB().ExecContext(ctx, `UPDATE device_endpoints SET status=?, status_reason=?, last_seen_at=?, updated_at=? WHERE id=?`,
		string(status), reason, seenAt.Unix(), seenAt.Unix(), endpointID)
	return err
}

func (s *DeviceStore) RenameEndpoint(ctx context.Context, endpointID, displayName string) error {
	result, err := s.db.GetDB().ExecContext(ctx, `UPDATE device_endpoints SET display_name=?, updated_at=? WHERE id=?`, displayName, time.Now().Unix(), endpointID)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return devices.ErrEndpointNotFound
	}
	return nil
}

func reconcileSharedInstallations(ctx context.Context, tx *sql.Tx, endpointID string, observations []devicev1.ManagedInstallationObservation, checkedAt time.Time) error {
	authorized := make(map[string]bool, len(observations))
	for _, observation := range observations {
		if observation.UseGranted || observation.State == "managed_here" {
			authorized[strings.ToLower(observation.LocalInstallationID)] = true
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT game_id, source_game_id, COALESCE(local_installation_id,''), install_state
		FROM device_game_installations WHERE endpoint_id=? AND authority_mode=?`, endpointID, devicev1.InstallationAuthorityShared)
	if err != nil {
		return fmt.Errorf("list shared installations for reconciliation: %w", err)
	}
	type sharedRow struct{ gameID, sourceID, localID, state string }
	var shared []sharedRow
	for rows.Next() {
		var row sharedRow
		if err := rows.Scan(&row.gameID, &row.sourceID, &row.localID, &row.state); err != nil {
			_ = rows.Close()
			return err
		}
		shared = append(shared, row)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, row := range shared {
		nextState, reason := devicev1.InstallStateMissing, "shared_grant_unavailable"
		if authorized[strings.ToLower(row.localID)] {
			nextState, reason = devicev1.InstallStateInstalled, ""
		}
		var stateChanged any
		if row.state != nextState {
			stateChanged = checkedAt.Unix()
		}
		if _, err := tx.ExecContext(ctx, `UPDATE device_game_installations SET install_state=?, state_reason=?, verification_reason_code=?,
			last_verified_at=?, state_changed_at=COALESCE(?, state_changed_at), updated_at=?
			WHERE endpoint_id=? AND game_id=? AND source_game_id=? AND authority_mode=?`, nextState, nullIfEmpty(reason), nullIfEmpty(reason),
			checkedAt.Unix(), stateChanged, checkedAt.Unix(), endpointID, row.gameID, row.sourceID, devicev1.InstallationAuthorityShared); err != nil {
			return fmt.Errorf("reconcile shared installation: %w", err)
		}
	}
	return nil
}

func (s *DeviceStore) DeleteEndpoint(ctx context.Context, endpointID string) error {
	result, err := s.db.GetDB().ExecContext(ctx, `DELETE FROM device_endpoints WHERE id=?`, endpointID)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return devices.ErrEndpointNotFound
	}
	return nil
}

func (s *DeviceStore) CreateCommand(ctx context.Context, command devices.Command) error {
	_, err := s.db.GetDB().ExecContext(ctx, `INSERT INTO device_commands
		(id, endpoint_id, profile_id, name, schema_version, idempotency_key, status, payload_json, created_at, updated_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, command.ID, command.EndpointID, command.ProfileID, command.Name,
		command.SchemaVersion, command.IdempotencyKey, string(command.Status), string(command.Payload), command.CreatedAt.Unix(), command.UpdatedAt.Unix(), command.ExpiresAt.Unix())
	return err
}

// FailInterruptedCommands closes commands that could not have survived an MGA
// Server process restart. Leaving them running would permanently disable the
// corresponding UI action even though no client is still executing it.
func (s *DeviceStore) FailInterruptedCommands(ctx context.Context, recoveredAt time.Time) (int64, error) {
	tx, err := s.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id, endpoint_id, profile_id, name, payload_json
		FROM device_commands
		WHERE status=? OR (name=? AND status=? AND error_code=?)`,
		string(devicev1.CommandRunning), devicev1.CapabilityGameCleanupGogInnoFailed,
		string(devicev1.CommandFailed), "command_interrupted")
	if err != nil {
		return 0, err
	}
	type interruptedCommand struct {
		id, endpointID, profileID, name, payload string
	}
	commands := make([]interruptedCommand, 0)
	for rows.Next() {
		var command interruptedCommand
		if err := rows.Scan(&command.id, &command.endpointID, &command.profileID, &command.name, &command.payload); err != nil {
			_ = rows.Close()
			return 0, err
		}
		commands = append(commands, command)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	const interruptedMessage = "MGA Server restarted before the client reported completion"
	for _, command := range commands {
		if command.name != devicev1.CapabilityGameCleanupGogInnoFailed {
			continue
		}
		var request devicev1.GogInnoFailedCleanupRequest
		if err := json.Unmarshal([]byte(command.payload), &request); err != nil {
			return 0, fmt.Errorf("decode interrupted cleanup command %s: %w", command.id, err)
		}
		result, err := tx.ExecContext(ctx, `UPDATE device_game_installations SET
			install_state=?, state_reason=?, state_changed_at=?, updated_at=?
			WHERE endpoint_id=? AND profile_id=? AND game_id=? AND source_game_id=?
				AND cleanup_marker_id=? AND install_state=?`,
			devicev1.InstallStateCleanupFailed, "command_interrupted: "+interruptedMessage,
			recoveredAt.Unix(), recoveredAt.Unix(), command.endpointID, command.profileID,
			request.GameID, request.SourceGameID, request.CleanupMarkerID, devicev1.InstallStateCleanupRunning)
		if err != nil {
			return 0, err
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		if updated == 1 {
			if err := insertInstallationEventTx(ctx, tx, devices.InstallationEvent{
				EndpointID: command.endpointID, GameID: request.GameID, SourceGameID: request.SourceGameID,
				ActorProfileID: command.profileID, EventType: "cleanup_failed",
				Reason:  "command_interrupted: " + interruptedMessage,
				Details: json.RawMessage(`{"family":"gog_inno"}`), CreatedAt: recoveredAt,
			}); err != nil {
				return 0, err
			}
		}
	}

	result, err := tx.ExecContext(ctx, `UPDATE device_commands SET
		status=?, result_json=NULL, error_code=?, error_message=?, updated_at=?
		WHERE status=?`, string(devicev1.CommandFailed), "command_interrupted",
		interruptedMessage, recoveredAt.Unix(), string(devicev1.CommandRunning))
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *DeviceStore) UpdateCommandStatus(ctx context.Context, endpointID, commandID string, status devicev1.CommandStatus, result json.RawMessage, protocolError *devicev1.ProtocolError, updatedAt time.Time) error {
	tx, err := s.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentStatus devicev1.CommandStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM device_commands WHERE id=? AND endpoint_id=?`, commandID, endpointID).Scan(&currentStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return devices.ErrCommandNotFound
		}
		return err
	}
	if err := devicev1.ValidateTransition(currentStatus, status); err != nil {
		return err
	}
	var errorCode, errorMessage any
	if protocolError != nil {
		errorCode = protocolError.Code
		errorMessage = protocolError.Message
	}
	var resultJSON any
	if len(result) > 0 {
		resultJSON = string(result)
	}
	update, err := tx.ExecContext(ctx, `UPDATE device_commands SET status=?, result_json=?, error_code=?, error_message=?, updated_at=?
		WHERE id=? AND endpoint_id=? AND status=?`,
		string(status), resultJSON, errorCode, errorMessage, updatedAt.Unix(), commandID, endpointID, string(currentStatus))
	if err != nil {
		return err
	}
	rows, err := update.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("device command status changed concurrently")
	}
	return tx.Commit()
}

func (s *DeviceStore) RecordCommandProgress(ctx context.Context, endpointID string, progress devicev1.CommandProgress, updatedAt time.Time) error {
	if err := progress.Validate(); err != nil {
		return err
	}
	percent := any(nil)
	if progress.Percent != nil {
		percent = int(*progress.Percent)
	}
	stagePercent := any(nil)
	if progress.StagePercent != nil {
		stagePercent = int(*progress.StagePercent)
	}
	result, err := s.db.GetDB().ExecContext(ctx, `UPDATE device_commands SET
		status=?, progress_sequence=?, progress_phase=?, progress_percent=?, progress_stage=?, progress_stage_percent=?, progress_message=?, updated_at=?
		WHERE id=? AND endpoint_id=? AND status IN (?, ?) AND progress_sequence < ?`,
		string(devicev1.CommandRunning), progress.Sequence, progress.Phase, percent, progress.Stage, stagePercent, progress.Message, updatedAt.Unix(),
		progress.CommandID, endpointID, string(devicev1.CommandAccepted), string(devicev1.CommandRunning), progress.Sequence)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("device command progress is stale or command is not running")
	}
	return nil
}

func (s *DeviceStore) CompleteCommand(ctx context.Context, endpointID string, result devicev1.CommandResult, updatedAt time.Time) error {
	tx, err := s.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentStatus devicev1.CommandStatus
	var commandName, commandPayload, profileID string
	if err := tx.QueryRowContext(ctx, `SELECT status, name, payload_json, profile_id FROM device_commands WHERE id=? AND endpoint_id=?`, result.CommandID, endpointID).Scan(&currentStatus, &commandName, &commandPayload, &profileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return devices.ErrCommandNotFound
		}
		return err
	}
	if err := devicev1.ValidateTransition(currentStatus, result.Status); err != nil {
		return err
	}
	if commandName == devicev1.CapabilityInventoryRefresh && result.Status == devicev1.CommandSucceeded {
		var inventory devicev1.DeviceInventory
		if err := json.Unmarshal(result.Payload, &inventory); err != nil {
			return fmt.Errorf("decode device inventory result: %w", err)
		}
		if err := inventory.Validate(); err != nil {
			return err
		}
		inventory = inventory.Normalize()
		if err := saveDeviceInventory(ctx, tx, endpointID, inventory, updatedAt); err != nil {
			return err
		}
	}
	if commandName == devicev1.CapabilityGameInstallArchive && result.Status == devicev1.CommandSucceeded {
		var request devicev1.ArchiveInstallRequest
		var installed devicev1.ArchiveInstallResult
		if err := json.Unmarshal([]byte(commandPayload), &request); err != nil {
			return fmt.Errorf("decode archive install command: %w", err)
		}
		if err := json.Unmarshal(result.Payload, &installed); err != nil {
			return fmt.Errorf("decode archive install result: %w", err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		if err := installed.Validate(); err != nil {
			return err
		}
		if installed.GameID != request.GameID || installed.SourceGameID != request.SourceGameID {
			return errors.New("archive install result does not match command")
		}
		if err := upsertGameInstallation(ctx, tx, endpointID, profileID, devices.GameInstallation{
			GameID: installed.GameID, SourceGameID: installed.SourceGameID, InstallRoot: installed.InstallRoot,
			InstallPath: installed.InstallPath, ArchiveSHA256: installed.ArchiveSHA256, ArchiveBytes: installed.ArchiveBytes,
			InstalledAt: installed.InstalledAt, LaunchTarget: installed.LaunchTarget, LaunchCandidates: installed.LaunchCandidates,
			InstallKind: devicev1.InstallKindManagedArchive, InstallState: devicev1.InstallStateInstalled,
		}, updatedAt); err != nil {
			return err
		}
	}
	if commandName == devicev1.CapabilityGameUseExisting && result.Status == devicev1.CommandSucceeded {
		var request devicev1.UseExistingInstallationRequest
		var shared devicev1.UseExistingInstallationResult
		if err := json.Unmarshal([]byte(commandPayload), &request); err != nil {
			return fmt.Errorf("decode use-existing command: %w", err)
		}
		if err := json.Unmarshal(result.Payload, &shared); err != nil {
			return fmt.Errorf("decode use-existing result: %w", err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		if err := shared.Validate(); err != nil {
			return err
		}
		if shared.LocalInstallationID != request.LocalInstallationID || shared.GameID != request.GameID || shared.SourceGameID != request.SourceGameID {
			return errors.New("use-existing result does not match command")
		}
		if err := upsertGameInstallation(ctx, tx, endpointID, profileID, devices.GameInstallation{
			GameID: shared.GameID, SourceGameID: shared.SourceGameID, InstallRoot: shared.InstallRoot, InstallPath: shared.InstallPath,
			ArchiveSHA256: "shared:" + shared.LocalInstallationID, InstalledAt: shared.GrantedAt,
			LaunchTarget: shared.LaunchTarget, LaunchCandidates: shared.LaunchCandidates,
			InstallKind: devicev1.InstallKindSharedExisting, InstallState: devicev1.InstallStateInstalled,
			LocalInstallationID: shared.LocalInstallationID, AuthorityMode: devicev1.InstallationAuthorityShared,
		}, updatedAt); err != nil {
			return err
		}
	}
	if commandName == devicev1.CapabilitySaveDomainClaim && result.Status == devicev1.CommandSucceeded {
		var request devicev1.SaveDomainClaimRequest
		var claimed devicev1.SaveDomainClaimResult
		if err := json.Unmarshal([]byte(commandPayload), &request); err != nil {
			return fmt.Errorf("decode save-domain claim command: %w", err)
		}
		if err := json.Unmarshal(result.Payload, &claimed); err != nil {
			return fmt.Errorf("decode save-domain claim result: %w", err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		if err := claimed.Validate(); err != nil {
			return err
		}
		if claimed.GameID != request.GameID || claimed.SourceGameID != request.SourceGameID || claimed.AdapterID != request.AdapterID || claimed.RouteFingerprint != request.RouteFingerprint || (request.LocalSaveDomainID != "" && request.LocalSaveDomainID != claimed.LocalSaveDomainID) {
			return errors.New("save-domain claim result does not match command")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO device_save_domain_links
			(endpoint_id, game_id, source_game_id, route_kind, emulator_id, local_save_domain_id, adapter_id, authority_state, created_by_profile_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(endpoint_id, game_id, source_game_id, route_kind, emulator_id) DO UPDATE SET
				local_save_domain_id=excluded.local_save_domain_id, adapter_id=excluded.adapter_id,
				authority_state=excluded.authority_state, created_by_profile_id=excluded.created_by_profile_id, updated_at=excluded.updated_at`,
			endpointID, request.GameID, request.SourceGameID, request.RouteKind, request.EmulatorID, claimed.LocalSaveDomainID, claimed.AdapterID, claimed.State, profileID, claimed.GrantedAt.Unix(), updatedAt.Unix()); err != nil {
			return fmt.Errorf("persist save-domain claim: %w", err)
		}
	}
	if commandName == devicev1.CapabilitySaveDomainRelease && result.Status == devicev1.CommandSucceeded {
		var request devicev1.SaveDomainReleaseRequest
		var released devicev1.SaveDomainReleaseResult
		if err := json.Unmarshal([]byte(commandPayload), &request); err != nil {
			return fmt.Errorf("decode save-domain release command: %w", err)
		}
		if err := json.Unmarshal(result.Payload, &released); err != nil {
			return fmt.Errorf("decode save-domain release result: %w", err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		if err := released.Validate(); err != nil {
			return err
		}
		if released.GameID != request.GameID || released.SourceGameID != request.SourceGameID || released.LocalSaveDomainID != request.LocalSaveDomainID {
			return errors.New("save-domain release result does not match command")
		}
		update, err := tx.ExecContext(ctx, `UPDATE device_save_domain_links SET authority_state='released', updated_at=?
			WHERE endpoint_id=? AND game_id=? AND source_game_id=? AND local_save_domain_id=? AND authority_state='owned_here'`,
			updatedAt.Unix(), endpointID, request.GameID, request.SourceGameID, request.LocalSaveDomainID)
		if err != nil {
			return fmt.Errorf("persist save-domain release: %w", err)
		}
		if rows, err := update.RowsAffected(); err != nil || rows != 1 {
			if err != nil {
				return err
			}
			return errors.New("save-domain release did not match an owned server link")
		}
	}
	if commandName == devicev1.CapabilitySaveDomainSnapshot {
		var request devicev1.SaveDomainSnapshotRequest
		if err := json.Unmarshal([]byte(commandPayload), &request); err != nil {
			return fmt.Errorf("decode save-domain snapshot command: %w", err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		syncState := "error"
		manifestHash := ""
		if result.Status == devicev1.CommandSucceeded {
			var snapshot devicev1.SaveDomainSnapshotResult
			if err := json.Unmarshal(result.Payload, &snapshot); err != nil {
				return fmt.Errorf("decode save-domain snapshot result: %w", err)
			}
			if err := snapshot.Validate(); err != nil {
				return err
			}
			if snapshot.GameID != request.GameID || snapshot.SourceGameID != request.SourceGameID || snapshot.LocalSaveDomainID != request.LocalSaveDomainID {
				return errors.New("save-domain snapshot result does not match command")
			}
			if snapshot.State == "stored" {
				syncState, manifestHash = "clean", snapshot.ManifestHash
			} else {
				syncState = "conflict"
			}
		}
		update, err := tx.ExecContext(ctx, `UPDATE device_save_domain_links SET sync_state=?,
			last_snapshot_manifest_hash=CASE WHEN ?<>'' THEN ? ELSE last_snapshot_manifest_hash END, updated_at=?
			WHERE endpoint_id=? AND game_id=? AND source_game_id=? AND local_save_domain_id=? AND authority_state='owned_here'`,
			syncState, manifestHash, manifestHash, updatedAt.Unix(), endpointID, request.GameID, request.SourceGameID, request.LocalSaveDomainID)
		if err != nil {
			return fmt.Errorf("persist save-domain snapshot result: %w", err)
		}
		if rows, err := update.RowsAffected(); err != nil || rows != 1 {
			return errors.New("save-domain snapshot did not match an owned server link")
		}
	}
	if commandName == devicev1.CapabilitySaveDomainRestore {
		var request devicev1.SaveDomainRestoreRequest
		if err := json.Unmarshal([]byte(commandPayload), &request); err != nil {
			return fmt.Errorf("decode save-domain restore command: %w", err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		syncState := "error"
		if result.Status == devicev1.CommandSucceeded {
			var restored devicev1.SaveDomainRestoreResult
			if err := json.Unmarshal(result.Payload, &restored); err != nil {
				return fmt.Errorf("decode save-domain restore result: %w", err)
			}
			if err := restored.Validate(); err != nil {
				return err
			}
			if restored.GameID != request.GameID || restored.SourceGameID != request.SourceGameID || restored.LocalSaveDomainID != request.LocalSaveDomainID || restored.ManifestHash != request.ManifestHash {
				return errors.New("save-domain restore result does not match command")
			}
			syncState = "clean"
		} else if result.Error != nil && result.Error.Code == "save_domain_local_conflict" {
			syncState = "conflict"
		}
		update, err := tx.ExecContext(ctx, `UPDATE device_save_domain_links SET sync_state=?, updated_at=?
			WHERE endpoint_id=? AND game_id=? AND source_game_id=? AND local_save_domain_id=? AND authority_state='owned_here'`,
			syncState, updatedAt.Unix(), endpointID, request.GameID, request.SourceGameID, request.LocalSaveDomainID)
		if err != nil {
			return fmt.Errorf("persist save-domain restore result: %w", err)
		}
		if rows, err := update.RowsAffected(); err != nil || rows != 1 {
			return errors.New("save-domain restore did not match an owned server link")
		}
	}
	if commandName == devicev1.CapabilitySaveDomainReconcile {
		var request devicev1.SaveDomainReconcileRequest
		if err := json.Unmarshal([]byte(commandPayload), &request); err != nil {
			return fmt.Errorf("decode save-domain reconciliation command: %w", err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		if result.Status != devicev1.CommandSucceeded {
			if _, err := tx.ExecContext(ctx, `UPDATE device_save_domain_links SET sync_state='error', updated_at=?
				WHERE endpoint_id=? AND game_id=? AND source_game_id=? AND local_save_domain_id=? AND authority_state='reconciliation_required'`,
				updatedAt.Unix(), endpointID, request.GameID, request.SourceGameID, request.LocalSaveDomainID); err != nil {
				return err
			}
		} else {
			var reconciled devicev1.SaveDomainReconcileResult
			if err := json.Unmarshal(result.Payload, &reconciled); err != nil {
				return fmt.Errorf("decode save-domain reconciliation result: %w", err)
			}
			if err := reconciled.Validate(); err != nil {
				return err
			}
			if reconciled.GameID != request.GameID || reconciled.SourceGameID != request.SourceGameID || reconciled.LocalSaveDomainID != request.LocalSaveDomainID || reconciled.Strategy != request.Strategy {
				return errors.New("save-domain reconciliation result does not match command")
			}
			update, err := tx.ExecContext(ctx, `UPDATE device_save_domain_links SET authority_state='owned_here', sync_state='clean',
				last_snapshot_manifest_hash=?, updated_at=? WHERE endpoint_id=? AND game_id=? AND source_game_id=?
				AND local_save_domain_id=? AND authority_state='reconciliation_required'`, reconciled.ManifestHash, updatedAt.Unix(), endpointID, request.GameID, request.SourceGameID, request.LocalSaveDomainID)
			if err != nil {
				return fmt.Errorf("persist save-domain reconciliation: %w", err)
			}
			if rows, err := update.RowsAffected(); err != nil || rows != 1 {
				return errors.New("save-domain reconciliation did not match its pending server link")
			}
		}
	}
	if commandName == devicev1.CapabilityGameInstallGogInno && (result.Status == devicev1.CommandSucceeded || result.Status == devicev1.CommandFailed) {
		var request devicev1.GogInnoInstallRequest
		if err := json.Unmarshal([]byte(commandPayload), &request); err != nil {
			return fmt.Errorf("decode gog inno install command: %w", err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		if len(result.Payload) == 0 {
			if result.Status == devicev1.CommandSucceeded {
				return errors.New("gog inno install result payload is required")
			}
		} else {
			var installed devicev1.GogInnoInstallResult
			if err := json.Unmarshal(result.Payload, &installed); err != nil {
				return fmt.Errorf("decode gog inno install result: %w", err)
			}
			if installed.GameID != request.GameID || installed.SourceGameID != request.SourceGameID {
				return errors.New("gog inno install result does not match command")
			}
			if result.Status == devicev1.CommandFailed {
				if err := installed.ValidateFailureEvidence(); err != nil {
					return fmt.Errorf("validate failed gog inno install evidence: %w", err)
				}
			}
			state := devicev1.InstallStateInstalled
			reason := ""
			if result.Status == devicev1.CommandFailed {
				if strings.TrimSpace(installed.CleanupMarkerID) != "" {
					state = devicev1.InstallStateCleanupRequired
				} else {
					state = devicev1.InstallStateAttentionRequired
				}
				if result.Error != nil {
					reason = strings.TrimSpace(result.Error.Code)
					if message := strings.TrimSpace(result.Error.Message); message != "" {
						if reason == "" {
							reason = message
						} else {
							reason = reason + ": " + message
						}
					}
				}
			} else if err := installed.Validate(); err != nil {
				return err
			}
			if strings.TrimSpace(installed.InstallPath) != "" {
				if err := upsertGameInstallation(ctx, tx, endpointID, profileID, devices.GameInstallation{
					GameID: installed.GameID, SourceGameID: installed.SourceGameID, InstallRoot: installed.InstallRoot,
					InstallPath: installed.InstallPath, ArchiveSHA256: installed.PrimarySHA256, ArchiveBytes: installed.TotalPackageBytes,
					InstalledAt: installed.InstalledAt, LaunchTarget: installed.LaunchTarget, LaunchCandidates: installed.LaunchCandidates,
					InstallKind: devicev1.InstallKindGogInno, InstallerFamily: devicev1.GogInnoInstallerFamily,
					InstallerFiles: installed.PackageFiles, UninstallTarget: installed.UninstallTarget,
					InstallState: state, StateReason: reason,
					CleanupMarkerID: installed.CleanupMarkerID,
				}, updatedAt); err != nil {
					return err
				}
				eventType := ""
				if result.Status == devicev1.CommandFailed {
					eventType = "failure_detected"
				} else if installed.CompletionBasis == devicev1.GogInnoCompletionValidatedPostSuccessCrash {
					eventType = "post_success_crash_accepted"
				}
				if eventType != "" {
					details, _ := json.Marshal(map[string]any{"family": installed.InstallerFamily, "completion_basis": installed.CompletionBasis, "exit_code": installed.ExitCode})
					if err := insertInstallationEventTx(ctx, tx, devices.InstallationEvent{
						EndpointID: endpointID, GameID: installed.GameID, SourceGameID: installed.SourceGameID,
						ActorProfileID: profileID, EventType: eventType, Reason: reason, Details: details, CreatedAt: updatedAt,
					}); err != nil {
						return err
					}
				}
			}
		}
	}
	if commandName == devicev1.CapabilityGameUninstall && result.Status == devicev1.CommandSucceeded {
		var request devicev1.GameUninstallRequest
		var removed devicev1.GameUninstallResult
		if err := json.Unmarshal([]byte(commandPayload), &request); err != nil {
			return fmt.Errorf("decode game uninstall command: %w", err)
		}
		if err := json.Unmarshal(result.Payload, &removed); err != nil {
			return fmt.Errorf("decode game uninstall result: %w", err)
		}
		if !removed.Removed || removed.GameID != request.GameID || removed.SourceGameID != request.SourceGameID {
			return errors.New("game uninstall result does not match command")
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM device_game_installations WHERE endpoint_id=? AND game_id=? AND source_game_id=?`, endpointID, request.GameID, request.SourceGameID); err != nil {
			return fmt.Errorf("remove device game installation: %w", err)
		}
	}
	if commandName == devicev1.CapabilityGameUninstallGogInno && result.Status == devicev1.CommandSucceeded {
		var request devicev1.GogInnoUninstallRequest
		var removed devicev1.GogInnoUninstallResult
		if err := json.Unmarshal([]byte(commandPayload), &request); err != nil {
			return fmt.Errorf("decode gog inno uninstall command: %w", err)
		}
		if err := json.Unmarshal(result.Payload, &removed); err != nil {
			return fmt.Errorf("decode gog inno uninstall result: %w", err)
		}
		if err := removed.Validate(); err != nil {
			return err
		}
		if !removed.Removed || removed.GameID != request.GameID || removed.SourceGameID != request.SourceGameID {
			return errors.New("gog inno uninstall result does not match command")
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM device_game_installations WHERE endpoint_id=? AND game_id=? AND source_game_id=?`, endpointID, request.GameID, request.SourceGameID); err != nil {
			return fmt.Errorf("remove device game installation: %w", err)
		}
	}
	if commandName == devicev1.CapabilityGameCleanupGogInnoFailed {
		var request devicev1.GogInnoFailedCleanupRequest
		if err := json.Unmarshal([]byte(commandPayload), &request); err != nil {
			return fmt.Errorf("decode gog inno cleanup command: %w", err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		if result.Status == devicev1.CommandSucceeded {
			var cleaned devicev1.GogInnoFailedCleanupResult
			if err := json.Unmarshal(result.Payload, &cleaned); err != nil {
				return fmt.Errorf("decode gog inno cleanup result: %w", err)
			}
			if err := cleaned.Validate(); err != nil {
				return err
			}
			if cleaned.GameID != request.GameID || cleaned.SourceGameID != request.SourceGameID {
				return errors.New("gog inno cleanup result does not match command")
			}
			details, _ := json.Marshal(map[string]any{
				"family": request.InstallerFamily, "publisher_uninstaller_used": cleaned.PublisherUninstallerUsed,
				"bounded_delete_used": cleaned.BoundedDeleteUsed, "leftover_directory": cleaned.LeftoverDirectory,
			})
			if err := insertInstallationEventTx(ctx, tx, devices.InstallationEvent{
				EndpointID: endpointID, GameID: request.GameID, SourceGameID: request.SourceGameID,
				ActorProfileID: profileID, EventType: "cleanup_succeeded", Details: details, CreatedAt: updatedAt,
			}); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM device_game_installations WHERE endpoint_id=? AND game_id=? AND source_game_id=?`, endpointID, request.GameID, request.SourceGameID); err != nil {
				return fmt.Errorf("remove cleaned failed installation: %w", err)
			}
		} else if result.Status == devicev1.CommandFailed {
			reason := "cleanup_failed"
			nextState := devicev1.InstallStateCleanupFailed
			if result.Error != nil {
				reason = strings.TrimSpace(result.Error.Code + ": " + result.Error.Message)
				if result.Error.Code == "local_confirmation_declined" || result.Error.Code == "local_confirmation_timeout" {
					nextState = devicev1.InstallStateCleanupRequired
				}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE device_game_installations SET install_state=?, state_reason=?, state_changed_at=?, updated_at=?
				WHERE endpoint_id=? AND game_id=? AND source_game_id=? AND cleanup_marker_id=?`,
				nextState, reason, updatedAt.Unix(), updatedAt.Unix(), endpointID, request.GameID, request.SourceGameID, request.CleanupMarkerID); err != nil {
				return err
			}
			if err := insertInstallationEventTx(ctx, tx, devices.InstallationEvent{
				EndpointID: endpointID, GameID: request.GameID, SourceGameID: request.SourceGameID,
				ActorProfileID: profileID, EventType: "cleanup_failed", Reason: reason, Details: json.RawMessage(`{"family":"gog_inno"}`), CreatedAt: updatedAt,
			}); err != nil {
				return err
			}
		}
	}
	if commandName == devicev1.CapabilityGameValidateInstallations && result.Status == devicev1.CommandSucceeded {
		var request devicev1.InstallationValidationRequest
		var validation devicev1.InstallationValidationResult
		if err := json.Unmarshal([]byte(commandPayload), &request); err != nil {
			return fmt.Errorf("decode installation validation command: %w", err)
		}
		if err := json.Unmarshal(result.Payload, &validation); err != nil {
			return fmt.Errorf("decode installation validation result: %w", err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		if err := validation.Validate(); err != nil {
			return err
		}
		changedMissing, changedRepair, restored, err := applyInstallationValidationResult(ctx, tx, endpointID, profileID, request, validation, updatedAt)
		if err != nil {
			return err
		}
		validation.ChangedMissing = changedMissing
		validation.ChangedNeedsRepair = changedRepair
		validation.Restored = restored
		result.Payload, err = json.Marshal(validation)
		if err != nil {
			return fmt.Errorf("encode installation validation summary: %w", err)
		}
	}
	if commandName == devicev1.CapabilityGameLaunch && result.Status == devicev1.CommandSucceeded {
		var request devicev1.GameLaunchRequest
		var launched devicev1.GameLaunchResult
		if err := json.Unmarshal([]byte(commandPayload), &request); err != nil {
			return fmt.Errorf("decode game launch command: %w", err)
		}
		if err := json.Unmarshal(result.Payload, &launched); err != nil {
			return fmt.Errorf("decode game launch result: %w", err)
		}
		if err := launched.Validate(); err != nil {
			return err
		}
		if launched.GameID != request.GameID || launched.SourceGameID != request.SourceGameID {
			return errors.New("game launch result does not match command")
		}
	}
	var errorCode, errorMessage any
	if result.Error != nil {
		errorCode, errorMessage = result.Error.Code, result.Error.Message
	}
	var resultJSON any
	if len(result.Payload) > 0 {
		resultJSON = string(result.Payload)
	}
	update, err := tx.ExecContext(ctx, `UPDATE device_commands SET status=?, result_json=?, error_code=?, error_message=?, updated_at=?
		WHERE id=? AND endpoint_id=? AND status=?`, string(result.Status), resultJSON, errorCode, errorMessage,
		updatedAt.Unix(), result.CommandID, endpointID, string(currentStatus))
	if err != nil {
		return err
	}
	if rows, err := update.RowsAffected(); err != nil || rows != 1 {
		if err != nil {
			return err
		}
		return errors.New("device command status changed concurrently")
	}
	return tx.Commit()
}

func applyInstallationValidationResult(ctx context.Context, tx *sql.Tx, endpointID, profileID string, request devicev1.InstallationValidationRequest, validation devicev1.InstallationValidationResult, updatedAt time.Time) (int, int, int, error) {
	requested := make(map[string]devicev1.InstallationValidationRequestItem, len(request.Items))
	for _, item := range request.Items {
		requested[item.GameID+"\x00"+item.SourceGameID] = item
	}
	if len(validation.Items) != len(requested) {
		return 0, 0, 0, errors.New("installation validation result omitted requested identities")
	}
	changedMissing, changedRepair, restored := 0, 0, 0
	for _, checked := range validation.Items {
		key := checked.GameID + "\x00" + checked.SourceGameID
		if _, ok := requested[key]; !ok {
			return 0, 0, 0, errors.New("installation validation result contains an unrequested identity")
		}
		var currentState string
		err := tx.QueryRowContext(ctx, `SELECT install_state FROM device_game_installations
			WHERE endpoint_id=? AND profile_id=? AND game_id=? AND source_game_id=?`, endpointID, profileID, checked.GameID, checked.SourceGameID).Scan(&currentState)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, 0, 0, devices.ErrInstallationNotFound
		}
		if err != nil {
			return 0, 0, 0, err
		}
		switch currentState {
		case devicev1.InstallStateInstalled, devicev1.InstallStateMissing, devicev1.InstallStateNeedsRepair:
		default:
			continue
		}
		details, err := json.Marshal(map[string]any{
			"reported_checked_at": checked.CheckedAt.UTC(), "manifest_schema": checked.ManifestSchema,
			"registered_program": checked.RegisteredProgram,
		})
		if err != nil {
			return 0, 0, 0, err
		}
		stateReason := any(nil)
		if checked.State != devicev1.InstallStateInstalled {
			stateReason = checked.ReasonCode
		}
		stateChangedAt := any(nil)
		if currentState != checked.State {
			stateChangedAt = updatedAt.Unix()
		}
		update, err := tx.ExecContext(ctx, `UPDATE device_game_installations SET
			install_state=?, state_reason=?, verification_reason_code=?, verification_details_json=?, last_verified_at=?,
			state_changed_at=COALESCE(?, state_changed_at)
			WHERE endpoint_id=? AND profile_id=? AND game_id=? AND source_game_id=?
			AND install_state IN ('installed','missing','needs_repair')`,
			checked.State, stateReason, checked.ReasonCode, string(details), updatedAt.Unix(), stateChangedAt,
			endpointID, profileID, checked.GameID, checked.SourceGameID)
		if err != nil {
			return 0, 0, 0, err
		}
		rows, err := update.RowsAffected()
		if err != nil {
			return 0, 0, 0, err
		}
		if rows != 1 {
			return 0, 0, 0, errors.New("installation changed while validation result was applied")
		}
		if currentState == checked.State {
			continue
		}
		eventType := ""
		switch checked.State {
		case devicev1.InstallStateMissing:
			changedMissing++
			eventType = "installation_missing"
		case devicev1.InstallStateNeedsRepair:
			changedRepair++
			eventType = "installation_needs_repair"
		case devicev1.InstallStateInstalled:
			restored++
			eventType = "installation_restored"
		}
		if err := insertInstallationEventTx(ctx, tx, devices.InstallationEvent{
			EndpointID: endpointID, GameID: checked.GameID, SourceGameID: checked.SourceGameID,
			ActorProfileID: profileID, EventType: eventType, Reason: checked.ReasonCode,
			Details: details, CreatedAt: updatedAt,
		}); err != nil {
			return 0, 0, 0, err
		}
	}
	return changedMissing, changedRepair, restored, nil
}

func (s *DeviceStore) SaveInventory(ctx context.Context, endpointID string, inventory devicev1.DeviceInventory, updatedAt time.Time) error {
	tx, err := s.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveDeviceInventory(ctx, tx, endpointID, inventory, updatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func saveDeviceInventory(ctx context.Context, tx *sql.Tx, endpointID string, inventory devicev1.DeviceInventory, updatedAt time.Time) error {
	if err := inventory.Validate(); err != nil {
		return err
	}
	storageJSON, err := json.Marshal(inventory.Storage)
	if err != nil {
		return fmt.Errorf("encode device storage inventory: %w", err)
	}
	runtimesJSON, err := json.Marshal(inventory.Runtimes)
	if err != nil {
		return fmt.Errorf("encode device runtime inventory: %w", err)
	}
	packageManagersJSON, err := json.Marshal(inventory.PackageManagers)
	if err != nil {
		return fmt.Errorf("encode device package manager inventory: %w", err)
	}
	saveAdaptersJSON, err := json.Marshal(inventory.SaveAdapters)
	if err != nil {
		return fmt.Errorf("encode device save adapter inventory: %w", err)
	}
	managedInstallationsJSON, err := json.Marshal(inventory.ManagedInstallations)
	if err != nil {
		return fmt.Errorf("encode managed installation inventory: %w", err)
	}
	saveDomainsJSON, err := json.Marshal(inventory.SaveDomains)
	if err != nil {
		return fmt.Errorf("encode save domain inventory: %w", err)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO device_inventories
		(endpoint_id, schema_version, captured_at, storage_json, runtimes_json, package_managers_json, save_adapters_json, managed_installations_json, save_domains_json, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(endpoint_id) DO UPDATE SET schema_version=excluded.schema_version,
			captured_at=excluded.captured_at, storage_json=excluded.storage_json,
			runtimes_json=excluded.runtimes_json, package_managers_json=excluded.package_managers_json,
			save_adapters_json=excluded.save_adapters_json, managed_installations_json=excluded.managed_installations_json,
			save_domains_json=excluded.save_domains_json,
			updated_at=excluded.updated_at`,
		endpointID, inventory.SchemaVersion, inventory.CapturedAt.Unix(), string(storageJSON), string(runtimesJSON), string(packageManagersJSON), string(saveAdaptersJSON), string(managedInstallationsJSON), string(saveDomainsJSON), updatedAt.Unix())
	if err != nil {
		return fmt.Errorf("persist device inventory: %w", err)
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		if err != nil {
			return err
		}
		return devices.ErrEndpointNotFound
	}
	if inventory.SchemaVersion >= devicev1.InventorySchemaVersionWithNativeProducts {
		if err := reconcileSharedInstallations(ctx, tx, endpointID, inventory.ManagedInstallations, updatedAt); err != nil {
			return err
		}
	}
	if inventory.SchemaVersion >= devicev1.InventorySchemaVersion {
		if err := reconcileSaveDomainLinks(ctx, tx, endpointID, inventory.SaveDomains, updatedAt); err != nil {
			return err
		}
	}
	return nil
}

func reconcileSaveDomainLinks(ctx context.Context, tx *sql.Tx, endpointID string, observations []devicev1.SaveDomainObservation, updatedAt time.Time) error {
	observed := make(map[string]devicev1.SaveDomainObservation, len(observations))
	for _, observation := range observations {
		observed[observation.LocalSaveDomainID] = observation
	}
	rows, err := tx.QueryContext(ctx, `SELECT local_save_domain_id FROM device_save_domain_links WHERE endpoint_id=?`, endpointID)
	if err != nil {
		return err
	}
	localIDs := make([]string, 0)
	for rows.Next() {
		var localID string
		if err := rows.Scan(&localID); err != nil {
			_ = rows.Close()
			return err
		}
		localIDs = append(localIDs, localID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, localID := range localIDs {
		state := "reconciliation_required"
		if observation, ok := observed[localID]; ok {
			state = observation.State
		}
		if _, err := tx.ExecContext(ctx, `UPDATE device_save_domain_links SET authority_state=?, updated_at=? WHERE endpoint_id=? AND local_save_domain_id=?`, state, updatedAt.Unix(), endpointID, localID); err != nil {
			return err
		}
	}
	return nil
}

func (s *DeviceStore) GetInventory(ctx context.Context, endpointID string) (*devicev1.DeviceInventory, error) {
	var schemaVersion uint16
	var capturedAt int64
	var storageJSON, runtimesJSON, packageManagersJSON, saveAdaptersJSON, managedInstallationsJSON, saveDomainsJSON string
	err := s.db.GetDB().QueryRowContext(ctx, `SELECT schema_version, captured_at, storage_json, runtimes_json, package_managers_json, save_adapters_json, managed_installations_json, save_domains_json
		FROM device_inventories WHERE endpoint_id=?`, endpointID).Scan(&schemaVersion, &capturedAt, &storageJSON, &runtimesJSON, &packageManagersJSON, &saveAdaptersJSON, &managedInstallationsJSON, &saveDomainsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	inventory := &devicev1.DeviceInventory{SchemaVersion: schemaVersion, CapturedAt: time.Unix(capturedAt, 0).UTC()}
	if err := json.Unmarshal([]byte(storageJSON), &inventory.Storage); err != nil {
		return nil, fmt.Errorf("decode device storage inventory: %w", err)
	}
	if err := json.Unmarshal([]byte(runtimesJSON), &inventory.Runtimes); err != nil {
		return nil, fmt.Errorf("decode device runtime inventory: %w", err)
	}
	if err := json.Unmarshal([]byte(packageManagersJSON), &inventory.PackageManagers); err != nil {
		return nil, fmt.Errorf("decode device package manager inventory: %w", err)
	}
	if err := json.Unmarshal([]byte(saveAdaptersJSON), &inventory.SaveAdapters); err != nil {
		return nil, fmt.Errorf("decode device save adapter inventory: %w", err)
	}
	if err := json.Unmarshal([]byte(managedInstallationsJSON), &inventory.ManagedInstallations); err != nil {
		return nil, fmt.Errorf("decode managed installation inventory: %w", err)
	}
	if err := json.Unmarshal([]byte(saveDomainsJSON), &inventory.SaveDomains); err != nil {
		return nil, fmt.Errorf("decode save domain inventory: %w", err)
	}
	if err := inventory.Validate(); err != nil {
		return nil, fmt.Errorf("validate persisted device inventory: %w", err)
	}
	return inventory, nil
}

func (s *DeviceStore) ListSaveDomainLinks(ctx context.Context, endpointID string) ([]devices.SaveDomainLink, error) {
	rows, err := s.db.GetDB().QueryContext(ctx, `SELECT endpoint_id, game_id, source_game_id, route_kind, emulator_id,
		local_save_domain_id, adapter_id, authority_state, sync_state, COALESCE(last_snapshot_manifest_hash,''),
		COALESCE(created_by_profile_id,''), created_at, updated_at
		FROM device_save_domain_links WHERE endpoint_id=?
		ORDER BY game_id, source_game_id, route_kind, emulator_id`, endpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := make([]devices.SaveDomainLink, 0)
	for rows.Next() {
		var link devices.SaveDomainLink
		var createdAt, updatedAt int64
		if err := rows.Scan(&link.EndpointID, &link.GameID, &link.SourceGameID, &link.RouteKind, &link.EmulatorID,
			&link.LocalSaveDomainID, &link.AdapterID, &link.AuthorityState, &link.SyncState, &link.LastSnapshotManifestHash,
			&link.CreatedByProfileID, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		link.CreatedAt = time.Unix(createdAt, 0).UTC()
		link.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *DeviceStore) ListCommands(ctx context.Context, endpointID, profileID string, limit int) ([]devices.Command, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.GetDB().QueryContext(ctx, `SELECT id, endpoint_id, profile_id, name, schema_version, idempotency_key,
		status, payload_json, COALESCE(result_json,''), COALESCE(error_code,''), COALESCE(error_message,''),
		progress_sequence, COALESCE(progress_phase,''), progress_percent, COALESCE(progress_stage,''), progress_stage_percent, COALESCE(progress_message,''), created_at, updated_at, expires_at
		FROM device_commands WHERE endpoint_id=? AND profile_id=? ORDER BY created_at DESC LIMIT ?`, endpointID, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	commands := make([]devices.Command, 0)
	for rows.Next() {
		var command devices.Command
		var status, payload, result string
		var createdAt, updatedAt, expiresAt int64
		var progressPercent, progressStagePercent sql.NullInt64
		if err := rows.Scan(&command.ID, &command.EndpointID, &command.ProfileID, &command.Name, &command.SchemaVersion,
			&command.IdempotencyKey, &status, &payload, &result, &command.ErrorCode, &command.ErrorMessage,
			&command.ProgressSequence, &command.ProgressPhase, &progressPercent, &command.ProgressStage, &progressStagePercent, &command.ProgressMessage,
			&createdAt, &updatedAt, &expiresAt); err != nil {
			return nil, err
		}
		command.Status = devicev1.CommandStatus(status)
		command.Payload = json.RawMessage(payload)
		if result != "" {
			command.Result = json.RawMessage(result)
		}
		if progressPercent.Valid {
			value := uint8(progressPercent.Int64)
			command.ProgressPercent = &value
		}
		if progressStagePercent.Valid {
			value := uint8(progressStagePercent.Int64)
			command.ProgressStagePercent = &value
		}
		command.CreatedAt = time.Unix(createdAt, 0)
		command.UpdatedAt = time.Unix(updatedAt, 0)
		command.ExpiresAt = time.Unix(expiresAt, 0)
		commands = append(commands, command)
	}
	return commands, rows.Err()
}

func (s *DeviceStore) GetCommand(ctx context.Context, endpointID, commandID string) (*devices.Command, error) {
	row := s.db.GetDB().QueryRowContext(ctx, `SELECT id, endpoint_id, profile_id, name, schema_version, idempotency_key,
		status, payload_json, COALESCE(result_json,''), COALESCE(error_code,''), COALESCE(error_message,''),
		progress_sequence, COALESCE(progress_phase,''), progress_percent, COALESCE(progress_stage,''), progress_stage_percent, COALESCE(progress_message,''), created_at, updated_at, expires_at
		FROM device_commands WHERE endpoint_id=? AND id=?`, endpointID, commandID)
	var command devices.Command
	var status, payload, result string
	var createdAt, updatedAt, expiresAt int64
	var progressPercent, progressStagePercent sql.NullInt64
	if err := row.Scan(&command.ID, &command.EndpointID, &command.ProfileID, &command.Name, &command.SchemaVersion,
		&command.IdempotencyKey, &status, &payload, &result, &command.ErrorCode, &command.ErrorMessage,
		&command.ProgressSequence, &command.ProgressPhase, &progressPercent, &command.ProgressStage, &progressStagePercent,
		&command.ProgressMessage, &createdAt, &updatedAt, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, devices.ErrCommandNotFound
		}
		return nil, err
	}
	command.Status = devicev1.CommandStatus(status)
	command.Payload = json.RawMessage(payload)
	if result != "" {
		command.Result = json.RawMessage(result)
	}
	if progressPercent.Valid {
		value := uint8(progressPercent.Int64)
		command.ProgressPercent = &value
	}
	if progressStagePercent.Valid {
		value := uint8(progressStagePercent.Int64)
		command.ProgressStagePercent = &value
	}
	command.CreatedAt = time.Unix(createdAt, 0).UTC()
	command.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	command.ExpiresAt = time.Unix(expiresAt, 0).UTC()
	return &command, nil
}

func (s *DeviceStore) ListInstallations(ctx context.Context, endpointID, profileID string) ([]devices.GameInstallation, error) {
	rows, err := s.db.GetDB().QueryContext(ctx, `SELECT endpoint_id, game_id, source_game_id, profile_id,
		install_root, install_path, archive_sha256, archive_bytes, installed_at, updated_at, COALESCE(launch_target,''), launch_candidates_json,
		COALESCE(install_kind, 'managed_archive'), COALESCE(installer_family,''), COALESCE(installer_files_json,'[]'),
		COALESCE(uninstall_target,''), COALESCE(install_state, 'installed'), COALESCE(state_reason,''), last_verified_at, state_changed_at
		, COALESCE(cleanup_marker_id,''), cleanup_ignored_at, COALESCE(cleanup_ignored_by_profile_id,''),
		COALESCE(verification_reason_code,''), COALESCE(verification_details_json,'{}')
		, COALESCE(local_installation_id,''), COALESCE(authority_mode,'managed')
		FROM device_game_installations WHERE endpoint_id=? AND profile_id=? ORDER BY installed_at DESC`, endpointID, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	installations := make([]devices.GameInstallation, 0)
	for rows.Next() {
		var installation devices.GameInstallation
		var installedAt, updatedAt int64
		var launchCandidates, installerFiles, verificationDetails string
		var lastVerifiedAt, stateChangedAt, cleanupIgnoredAt sql.NullInt64
		if err := rows.Scan(&installation.EndpointID, &installation.GameID, &installation.SourceGameID, &installation.ProfileID,
			&installation.InstallRoot, &installation.InstallPath, &installation.ArchiveSHA256, &installation.ArchiveBytes,
			&installedAt, &updatedAt, &installation.LaunchTarget, &launchCandidates,
			&installation.InstallKind, &installation.InstallerFamily, &installerFiles,
			&installation.UninstallTarget, &installation.InstallState, &installation.StateReason, &lastVerifiedAt, &stateChangedAt,
			&installation.CleanupMarkerID, &cleanupIgnoredAt, &installation.CleanupIgnoredByProfileID,
			&installation.VerificationReasonCode, &verificationDetails, &installation.LocalInstallationID, &installation.AuthorityMode); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(launchCandidates), &installation.LaunchCandidates); err != nil {
			return nil, fmt.Errorf("decode launch candidates: %w", err)
		}
		if err := json.Unmarshal([]byte(installerFiles), &installation.InstallerFiles); err != nil {
			return nil, fmt.Errorf("decode installer files: %w", err)
		}
		installation.VerificationDetails = json.RawMessage(verificationDetails)
		installation.InstalledAt = time.Unix(installedAt, 0).UTC()
		installation.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		if lastVerifiedAt.Valid {
			value := time.Unix(lastVerifiedAt.Int64, 0).UTC()
			installation.LastVerifiedAt = &value
		}
		if stateChangedAt.Valid {
			value := time.Unix(stateChangedAt.Int64, 0).UTC()
			installation.StateChangedAt = &value
		}
		if cleanupIgnoredAt.Valid {
			value := time.Unix(cleanupIgnoredAt.Int64, 0).UTC()
			installation.CleanupIgnoredAt = &value
		}
		installations = append(installations, installation)
	}
	return installations, rows.Err()
}

func upsertGameInstallation(ctx context.Context, tx *sql.Tx, endpointID, profileID string, installation devices.GameInstallation, updatedAt time.Time) error {
	switch installation.InstallKind {
	case devicev1.InstallKindManagedArchive, devicev1.InstallKindGogInno, devicev1.InstallKindSharedExisting:
	default:
		return fmt.Errorf("unsupported install_kind %q", installation.InstallKind)
	}
	if installation.AuthorityMode == "" {
		installation.AuthorityMode = devicev1.InstallationAuthorityManaged
	}
	if installation.AuthorityMode != devicev1.InstallationAuthorityManaged && installation.AuthorityMode != devicev1.InstallationAuthorityShared {
		return fmt.Errorf("unsupported installation authority %q", installation.AuthorityMode)
	}
	if installation.AuthorityMode == devicev1.InstallationAuthorityShared && (installation.InstallKind != devicev1.InstallKindSharedExisting || strings.TrimSpace(installation.LocalInstallationID) == "") {
		return errors.New("shared installation requires shared kind and local installation ID")
	}
	switch installation.InstallState {
	case devicev1.InstallStateInstalled, devicev1.InstallStateMissing, devicev1.InstallStateNeedsRepair,
		devicev1.InstallStateAttentionRequired, devicev1.InstallStateCleanupRequired,
		devicev1.InstallStateCleanupRunning, devicev1.InstallStateCleanupFailed, devicev1.InstallStateIgnoredFailure:
	default:
		return fmt.Errorf("unsupported install_state %q", installation.InstallState)
	}
	if installation.LaunchCandidates == nil {
		installation.LaunchCandidates = []string{}
	}
	if installation.InstallerFiles == nil {
		installation.InstallerFiles = []devicev1.GogInnoPackageFile{}
	}
	launchCandidates, err := json.Marshal(installation.LaunchCandidates)
	if err != nil {
		return fmt.Errorf("encode launch candidates: %w", err)
	}
	installerFiles, err := json.Marshal(installation.InstallerFiles)
	if err != nil {
		return fmt.Errorf("encode installer files: %w", err)
	}
	installedAt := installation.InstalledAt
	if installedAt.IsZero() {
		installedAt = updatedAt
	}
	stateChangedAt := updatedAt.Unix()
	if _, err := tx.ExecContext(ctx, `INSERT INTO device_game_installations
		(endpoint_id, game_id, source_game_id, profile_id, install_root, install_path, archive_sha256, archive_bytes, installed_at, updated_at,
		 launch_target, launch_candidates_json, install_kind, installer_family, installer_files_json, uninstall_target, install_state, state_reason, state_changed_at,
		 cleanup_marker_id, cleanup_ignored_at, cleanup_ignored_by_profile_id, verification_reason_code, verification_details_json,
		 local_installation_id, authority_mode)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(endpoint_id, game_id, source_game_id) DO UPDATE SET
			profile_id=excluded.profile_id, install_root=excluded.install_root, install_path=excluded.install_path,
			archive_sha256=excluded.archive_sha256, archive_bytes=excluded.archive_bytes,
			installed_at=excluded.installed_at, updated_at=excluded.updated_at,
			launch_target=excluded.launch_target, launch_candidates_json=excluded.launch_candidates_json,
			install_kind=excluded.install_kind, installer_family=excluded.installer_family, installer_files_json=excluded.installer_files_json,
			uninstall_target=excluded.uninstall_target, install_state=excluded.install_state, state_reason=excluded.state_reason,
			state_changed_at=excluded.state_changed_at, cleanup_marker_id=excluded.cleanup_marker_id,
			cleanup_ignored_at=excluded.cleanup_ignored_at, cleanup_ignored_by_profile_id=excluded.cleanup_ignored_by_profile_id,
			verification_reason_code=NULL, verification_details_json='{}', local_installation_id=excluded.local_installation_id,
			authority_mode=excluded.authority_mode`,
		endpointID, installation.GameID, installation.SourceGameID, profileID, installation.InstallRoot, installation.InstallPath,
		installation.ArchiveSHA256, installation.ArchiveBytes, installedAt.Unix(), updatedAt.Unix(), installation.LaunchTarget, string(launchCandidates),
		installation.InstallKind, nullIfEmpty(installation.InstallerFamily), string(installerFiles), nullIfEmpty(installation.UninstallTarget),
		installation.InstallState, nullIfEmpty(installation.StateReason), stateChangedAt, nullIfEmpty(installation.CleanupMarkerID),
		unixOrNil(installation.CleanupIgnoredAt), nullIfEmpty(installation.CleanupIgnoredByProfileID), nil, `{}`,
		nullIfEmpty(installation.LocalInstallationID), installation.AuthorityMode); err != nil {
		return fmt.Errorf("persist device game installation: %w", err)
	}
	return nil
}

func (s *DeviceStore) SetInstallationFailureState(ctx context.Context, endpointID, gameID, sourceGameID, profileID, state, reason, markerID string, ignoredAt *time.Time, ignoredByProfileID, eventType string, details json.RawMessage, updatedAt time.Time) error {
	switch state {
	case devicev1.InstallStateAttentionRequired, devicev1.InstallStateCleanupRequired, devicev1.InstallStateCleanupRunning,
		devicev1.InstallStateCleanupFailed, devicev1.InstallStateIgnoredFailure:
	default:
		return fmt.Errorf("unsupported failed installation state %q", state)
	}
	if len(details) == 0 {
		details = json.RawMessage(`{}`)
	}
	if !json.Valid(details) {
		return errors.New("installation event details must be valid JSON")
	}
	tx, err := s.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE device_game_installations SET install_state=?, state_reason=?, cleanup_marker_id=?,
		cleanup_ignored_at=?, cleanup_ignored_by_profile_id=?, state_changed_at=?, updated_at=?
		WHERE endpoint_id=? AND game_id=? AND source_game_id=? AND profile_id=?`, state, nullIfEmpty(reason), nullIfEmpty(markerID),
		unixOrNil(ignoredAt), nullIfEmpty(ignoredByProfileID), updatedAt.Unix(), updatedAt.Unix(), endpointID, gameID, sourceGameID, profileID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return devices.ErrInstallationNotFound
	}
	if err := insertInstallationEventTx(ctx, tx, devices.InstallationEvent{
		EndpointID: endpointID, GameID: gameID, SourceGameID: sourceGameID, ActorProfileID: profileID,
		EventType: eventType, Reason: reason, Details: details, CreatedAt: updatedAt,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func insertInstallationEventTx(ctx context.Context, tx *sql.Tx, event devices.InstallationEvent) error {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if len(event.Details) == 0 {
		event.Details = json.RawMessage(`{}`)
	}
	if !json.Valid(event.Details) || event.CreatedAt.IsZero() {
		return errors.New("valid installation event details and created_at are required")
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO device_installation_events
		(id, endpoint_id, game_id, source_game_id, actor_profile_id, event_type, reason, details_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.ID, event.EndpointID, event.GameID, event.SourceGameID,
		nullIfEmpty(event.ActorProfileID), event.EventType, nullIfEmpty(event.Reason), string(event.Details), event.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("persist installation event: %w", err)
	}
	return nil
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func (s *DeviceStore) UpdateInstallationLaunchTarget(ctx context.Context, endpointID, gameID, sourceGameID, profileID, launchTarget string, updatedAt time.Time) error {
	result, err := s.db.GetDB().ExecContext(ctx, `UPDATE device_game_installations SET launch_target=?, updated_at=?
		WHERE endpoint_id=? AND game_id=? AND source_game_id=? AND profile_id=?`, launchTarget, updatedAt.Unix(), endpointID, gameID, sourceGameID, profileID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return devices.ErrEndpointNotFound
	}
	return nil
}

const endpointFields = `SELECT e.id, e.client_instance_id, e.public_key, e.display_name, e.host_name, e.os_user,
	e.platform, e.arch, e.execution_mode, e.client_version, e.protocol_version, e.capabilities_json, e.status,
	COALESCE(e.status_reason,''), e.last_seen_at, e.created_at, e.updated_at`

type deviceScanner interface {
	Scan(dest ...any) error
}

func scanEndpoint(row deviceScanner) (*devices.Endpoint, error) {
	return scanEndpointFields(row, false)
}

func scanEndpointWithGrant(row deviceScanner) (*devices.Endpoint, error) {
	return scanEndpointFields(row, true)
}

func scanEndpointFields(row deviceScanner, withGrant bool) (*devices.Endpoint, error) {
	var endpoint devices.Endpoint
	var protocolVersion uint16
	var capabilities, status string
	var lastSeen sql.NullInt64
	var createdAt, updatedAt int64
	dest := []any{
		&endpoint.ID, &endpoint.ClientInstanceID, &endpoint.PublicKey, &endpoint.DisplayName, &endpoint.HostName, &endpoint.OSUser,
		&endpoint.Platform, &endpoint.Arch, &endpoint.ExecutionMode, &endpoint.ClientVersion, &protocolVersion, &capabilities, &status,
		&endpoint.StatusReason, &lastSeen, &createdAt, &updatedAt,
	}
	if withGrant {
		dest = append(dest, &endpoint.AccessLevel)
	}
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	endpoint.ProtocolVersion = devicev1.ProtocolVersion(protocolVersion)
	endpoint.Status = devicev1.EndpointState(status)
	if err := json.Unmarshal([]byte(capabilities), &endpoint.Capabilities); err != nil {
		return nil, fmt.Errorf("decode endpoint capabilities: %w", err)
	}
	if lastSeen.Valid {
		seen := time.Unix(lastSeen.Int64, 0)
		endpoint.LastSeenAt = &seen
	}
	endpoint.CreatedAt = time.Unix(createdAt, 0)
	endpoint.UpdatedAt = time.Unix(updatedAt, 0)
	return &endpoint, nil
}

func unixOrNil(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.Unix()
}
