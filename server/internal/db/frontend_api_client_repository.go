package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/frontendauth"
)

type FrontendAPIClientRepository struct {
	database core.Database
}

var _ frontendauth.Repository = (*FrontendAPIClientRepository)(nil)

func NewFrontendAPIClientRepository(database core.Database) (*FrontendAPIClientRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &FrontendAPIClientRepository{database: database}, nil
}

func (r *FrontendAPIClientRepository) Create(ctx context.Context, client frontendauth.Client) error {
	scopes, err := json.Marshal(client.Scopes)
	if err != nil {
		return fmt.Errorf("encode frontend API client scopes: %w", err)
	}
	_, err = r.db().ExecContext(ctx, `INSERT INTO frontend_api_clients
		(id, profile_id, name, secret_hash, scopes_json, created_at, last_used_at, expires_at, revoked_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NULL, ?, NULL, ?)`, client.ID, client.ProfileID, client.Name, client.SecretHash,
		string(scopes), client.CreatedAt.Unix(), unixNullable(client.ExpiresAt), client.UpdatedAt.Unix())
	if err != nil {
		return fmt.Errorf("insert frontend API client: %w", err)
	}
	return nil
}

func (r *FrontendAPIClientRepository) ListByProfile(ctx context.Context, profileID string) ([]frontendauth.Client, error) {
	rows, err := r.db().QueryContext(ctx, frontendAPIClientSelect+` WHERE profile_id=? ORDER BY created_at DESC, id`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list frontend API clients: %w", err)
	}
	defer rows.Close()
	clients := make([]frontendauth.Client, 0)
	for rows.Next() {
		client, err := scanFrontendAPIClient(rows)
		if err != nil {
			return nil, err
		}
		clients = append(clients, *client)
	}
	return clients, rows.Err()
}

func (r *FrontendAPIClientRepository) GetByID(ctx context.Context, clientID string) (*frontendauth.Client, error) {
	client, err := scanFrontendAPIClient(r.db().QueryRowContext(ctx, frontendAPIClientSelect+` WHERE id=?`, clientID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return client, err
}

func (r *FrontendAPIClientRepository) Rotate(ctx context.Context, profileID, clientID, secretHash string, now time.Time) (*frontendauth.Client, error) {
	result, err := r.db().ExecContext(ctx, `UPDATE frontend_api_clients SET secret_hash=?, revoked_at=NULL, last_used_at=NULL, updated_at=? WHERE id=? AND profile_id=?`,
		secretHash, now.Unix(), clientID, profileID)
	if err != nil {
		return nil, fmt.Errorf("rotate frontend API client: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return nil, frontendauth.ErrNotFound
	}
	return r.getOwned(ctx, profileID, clientID)
}

func (r *FrontendAPIClientRepository) Revoke(ctx context.Context, profileID, clientID string, now time.Time) (*frontendauth.Client, error) {
	result, err := r.db().ExecContext(ctx, `UPDATE frontend_api_clients SET revoked_at=COALESCE(revoked_at, ?), updated_at=? WHERE id=? AND profile_id=?`,
		now.Unix(), now.Unix(), clientID, profileID)
	if err != nil {
		return nil, fmt.Errorf("revoke frontend API client: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return nil, frontendauth.ErrNotFound
	}
	return r.getOwned(ctx, profileID, clientID)
}

func (r *FrontendAPIClientRepository) TouchLastUsed(ctx context.Context, clientID string, now time.Time) error {
	result, err := r.db().ExecContext(ctx, `UPDATE frontend_api_clients SET last_used_at=? WHERE id=? AND revoked_at IS NULL`, now.Unix(), clientID)
	if err != nil {
		return fmt.Errorf("touch frontend API client: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return frontendauth.ErrNotFound
	}
	return nil
}

func (r *FrontendAPIClientRepository) RecordAudit(ctx context.Context, event frontendauth.AuditEvent) error {
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := r.db().ExecContext(ctx, `INSERT INTO frontend_api_client_audit
		(profile_id, client_id, action, outcome, reason, request_id, remote_ip, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, nullableString(event.ProfileID), nullableString(event.ClientID),
		event.Action, event.Outcome, nullableString(event.Reason), nullableString(event.RequestID), nullableString(event.RemoteIP), createdAt.Unix())
	if err != nil {
		return fmt.Errorf("record frontend API client audit: %w", err)
	}
	return nil
}

func (r *FrontendAPIClientRepository) getOwned(ctx context.Context, profileID, clientID string) (*frontendauth.Client, error) {
	client, err := scanFrontendAPIClient(r.db().QueryRowContext(ctx, frontendAPIClientSelect+` WHERE id=? AND profile_id=?`, clientID, profileID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, frontendauth.ErrNotFound
	}
	return client, err
}

func (r *FrontendAPIClientRepository) db() *sql.DB { return r.database.GetDB() }

const frontendAPIClientSelect = `SELECT id, profile_id, name, secret_hash, scopes_json, created_at, last_used_at, expires_at, revoked_at, updated_at FROM frontend_api_clients`

type frontendAPIClientScanner interface{ Scan(...any) error }

func scanFrontendAPIClient(scanner frontendAPIClientScanner) (*frontendauth.Client, error) {
	var client frontendauth.Client
	var scopesJSON string
	var createdAt, updatedAt int64
	var lastUsedAt, expiresAt, revokedAt sql.NullInt64
	if err := scanner.Scan(&client.ID, &client.ProfileID, &client.Name, &client.SecretHash, &scopesJSON, &createdAt, &lastUsedAt, &expiresAt, &revokedAt, &updatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(scopesJSON), &client.Scopes); err != nil {
		return nil, fmt.Errorf("decode frontend API client scopes: %w", err)
	}
	client.CreatedAt, client.UpdatedAt = time.Unix(createdAt, 0).UTC(), time.Unix(updatedAt, 0).UTC()
	client.LastUsedAt = timeFromNull(lastUsedAt)
	client.ExpiresAt = timeFromNull(expiresAt)
	client.RevokedAt = timeFromNull(revokedAt)
	return &client, nil
}

func timeFromNull(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	result := time.Unix(value.Int64, 0).UTC()
	return &result
}

func unixNullable(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Unix()
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}
