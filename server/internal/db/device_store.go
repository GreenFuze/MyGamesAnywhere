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

func (s *DeviceStore) PairEndpoint(ctx context.Context, codeHash string, now time.Time, endpoint devices.Endpoint) (string, error) {
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
	capabilities, err := json.Marshal(endpoint.Capabilities)
	if err != nil {
		return "", fmt.Errorf("marshal endpoint capabilities: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO device_endpoints
		(id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, client_version,
		 protocol_version, capabilities_json, status, status_reason, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		endpoint.ID, endpoint.ClientInstanceID, endpoint.PublicKey, endpoint.DisplayName, endpoint.HostName, endpoint.OSUser,
		endpoint.Platform, endpoint.Arch, endpoint.ClientVersion, endpoint.ProtocolVersion, string(capabilities),
		string(devicev1.EndpointOffline), now.Unix(), now.Unix())
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return "", devices.ErrClientAlreadyPaired
		}
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO device_grants
		(endpoint_id, profile_id, access_level, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
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
		host_name=?, os_user=?, platform=?, arch=?, client_version=?, protocol_version=?, capabilities_json=?,
		status=?, status_reason=?, last_seen_at=?, updated_at=? WHERE id=?`,
		endpoint.HostName, endpoint.OSUser, endpoint.Platform, endpoint.Arch, endpoint.ClientVersion, endpoint.ProtocolVersion,
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

func (s *DeviceStore) ListCommands(ctx context.Context, endpointID, profileID string, limit int) ([]devices.Command, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.GetDB().QueryContext(ctx, `SELECT id, endpoint_id, profile_id, name, schema_version, idempotency_key,
		status, payload_json, COALESCE(result_json,''), COALESCE(error_code,''), COALESCE(error_message,''), created_at, updated_at, expires_at
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
		if err := rows.Scan(&command.ID, &command.EndpointID, &command.ProfileID, &command.Name, &command.SchemaVersion,
			&command.IdempotencyKey, &status, &payload, &result, &command.ErrorCode, &command.ErrorMessage,
			&createdAt, &updatedAt, &expiresAt); err != nil {
			return nil, err
		}
		command.Status = devicev1.CommandStatus(status)
		command.Payload = json.RawMessage(payload)
		if result != "" {
			command.Result = json.RawMessage(result)
		}
		command.CreatedAt = time.Unix(createdAt, 0)
		command.UpdatedAt = time.Unix(updatedAt, 0)
		command.ExpiresAt = time.Unix(expiresAt, 0)
		commands = append(commands, command)
	}
	return commands, rows.Err()
}

const endpointFields = `SELECT e.id, e.client_instance_id, e.public_key, e.display_name, e.host_name, e.os_user,
	e.platform, e.arch, e.client_version, e.protocol_version, e.capabilities_json, e.status,
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
		&endpoint.Platform, &endpoint.Arch, &endpoint.ClientVersion, &protocolVersion, &capabilities, &status,
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
