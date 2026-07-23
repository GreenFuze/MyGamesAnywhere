package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/auth"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func (s *AuthStore) CreateCredentialTicket(ctx context.Context, ticket auth.CredentialTicket, tokenHash string, now time.Time) error {
	tx, err := s.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE profile_credential_tickets SET revoked_at=?
		WHERE profile_id=? AND used_at IS NULL AND revoked_at IS NULL AND expires_at>?`,
		now.Unix(), ticket.ProfileID, now.Unix()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO profile_credential_tickets
		(id, profile_id, token_hash, created_by_profile_id, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`, ticket.ID, ticket.ProfileID, tokenHash, ticket.CreatedByProfileID,
		ticket.CreatedAt.Unix(), ticket.ExpiresAt.Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *AuthStore) GetActiveCredentialTicket(ctx context.Context, profileID string, now time.Time) (*auth.CredentialTicket, error) {
	row := s.db.GetDB().QueryRowContext(ctx, `SELECT id, profile_id, COALESCE(created_by_profile_id,''), created_at, expires_at
		FROM profile_credential_tickets
		WHERE profile_id=? AND used_at IS NULL AND revoked_at IS NULL AND expires_at>?
		ORDER BY created_at DESC LIMIT 1`, profileID, now.Unix())
	var ticket auth.CredentialTicket
	var createdAt, expiresAt int64
	if err := row.Scan(&ticket.ID, &ticket.ProfileID, &ticket.CreatedByProfileID, &createdAt, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	ticket.CreatedAt = time.Unix(createdAt, 0)
	ticket.ExpiresAt = time.Unix(expiresAt, 0)
	return &ticket, nil
}

func (s *AuthStore) RevokeCredentialTicket(ctx context.Context, profileID, ticketID string, now time.Time) error {
	result, err := s.db.GetDB().ExecContext(ctx, `UPDATE profile_credential_tickets SET revoked_at=?
		WHERE id=? AND profile_id=? AND used_at IS NULL AND revoked_at IS NULL AND expires_at>?`,
		now.Unix(), ticketID, profileID, now.Unix())
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return auth.ErrCredentialTicket
	}
	return nil
}

func (s *AuthStore) RedeemCredentialTicket(ctx context.Context, tokenHash, profileID string, now time.Time, credential auth.Credential) error {
	tx, err := s.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var ticketID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM profile_credential_tickets
		WHERE token_hash=? AND profile_id=? AND used_at IS NULL AND revoked_at IS NULL AND expires_at>?`,
		tokenHash, profileID, now.Unix()).Scan(&ticketID)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.ErrCredentialTicket
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO profile_credentials (profile_id, kind, hash, must_change, updated_at)
		VALUES (?, ?, ?, 0, ?)
		ON CONFLICT(profile_id) DO UPDATE SET kind=excluded.kind, hash=excluded.hash, must_change=0, updated_at=excluded.updated_at`,
		credential.ProfileID, string(credential.Kind), credential.Hash, credential.UpdatedAt.Unix()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM auth_sessions WHERE profile_id=?`, profileID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE profile_credential_tickets SET used_at=?
		WHERE id=? AND used_at IS NULL AND revoked_at IS NULL`, now.Unix(), ticketID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return auth.ErrCredentialTicket
	}
	return tx.Commit()
}

type AuthStore struct {
	db core.Database
}

func NewAuthStore(database core.Database) *AuthStore {
	return &AuthStore{db: database}
}

func (s *AuthStore) GetCredential(ctx context.Context, profileID string) (*auth.Credential, error) {
	row := s.db.GetDB().QueryRowContext(ctx, `SELECT profile_id, kind, hash, must_change, updated_at
		FROM profile_credentials WHERE profile_id=?`, profileID)
	var credential auth.Credential
	var kind string
	var mustChange int
	var updatedAt int64
	if err := row.Scan(&credential.ProfileID, &kind, &credential.Hash, &mustChange, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	credential.Kind = auth.CredentialKind(kind)
	credential.MustChange = mustChange != 0
	credential.UpdatedAt = time.Unix(updatedAt, 0)
	return &credential, nil
}

func (s *AuthStore) SetCredential(ctx context.Context, credential auth.Credential) error {
	_, err := s.db.GetDB().ExecContext(ctx, `INSERT INTO profile_credentials (profile_id, kind, hash, must_change, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(profile_id) DO UPDATE SET kind=excluded.kind, hash=excluded.hash, must_change=excluded.must_change, updated_at=excluded.updated_at`,
		credential.ProfileID, string(credential.Kind), credential.Hash, credential.MustChange, credential.UpdatedAt.Unix())
	return err
}

func (s *AuthStore) CreateCredential(ctx context.Context, credential auth.Credential) error {
	_, err := s.db.GetDB().ExecContext(ctx, `INSERT INTO profile_credentials
		(profile_id, kind, hash, must_change, updated_at) VALUES (?, ?, ?, ?, ?)`,
		credential.ProfileID, string(credential.Kind), credential.Hash, credential.MustChange, credential.UpdatedAt.Unix())
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return auth.ErrCredentialConfigured
	}
	return err
}

func (s *AuthStore) DeleteCredential(ctx context.Context, profileID string) error {
	_, err := s.db.GetDB().ExecContext(ctx, `DELETE FROM profile_credentials WHERE profile_id=?`, profileID)
	return err
}

func (s *AuthStore) CreateSession(ctx context.Context, session auth.Session, tokenHash string) error {
	_, err := s.db.GetDB().ExecContext(ctx, `INSERT INTO auth_sessions (id, token_hash, profile_id, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)`, session.ID, tokenHash, session.ProfileID, session.CreatedAt.Unix(), session.ExpiresAt.Unix())
	return err
}

func (s *AuthStore) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*auth.Session, error) {
	row := s.db.GetDB().QueryRowContext(ctx, `SELECT id, profile_id, created_at, expires_at FROM auth_sessions WHERE token_hash=?`, tokenHash)
	var session auth.Session
	var createdAt, expiresAt int64
	if err := row.Scan(&session.ID, &session.ProfileID, &createdAt, &expiresAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	session.CreatedAt = time.Unix(createdAt, 0)
	session.ExpiresAt = time.Unix(expiresAt, 0)
	return &session, nil
}

func (s *AuthStore) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := s.db.GetDB().ExecContext(ctx, `DELETE FROM auth_sessions WHERE token_hash=?`, tokenHash)
	return err
}

func (s *AuthStore) DeleteSessionsByProfile(ctx context.Context, profileID string) error {
	_, err := s.db.GetDB().ExecContext(ctx, `DELETE FROM auth_sessions WHERE profile_id=?`, profileID)
	return err
}

func (s *AuthStore) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.db.GetDB().ExecContext(ctx, `DELETE FROM auth_sessions WHERE expires_at<=?`, now.Unix())
	return err
}
