package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type UserRepository interface {
	Create(ctx context.Context, user *core.User) error
	GetByUsername(ctx context.Context, username string) (*core.User, error)
	GetByID(ctx context.Context, id string) (*core.User, error)
	Update(ctx context.Context, user *core.User) error
}

type userRepository struct {
	db core.Database
}

func NewUserRepository(db core.Database) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, user *core.User) error {
	query := `INSERT INTO users (id, username, password_hash, role, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := r.db.GetDB().ExecContext(ctx, query, user.ID, user.Username, user.PasswordHash, user.Role, user.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

func (r *userRepository) GetByUsername(ctx context.Context, username string) (*core.User, error) {
	query := `SELECT id, username, password_hash, role, created_at, last_login_at FROM users WHERE username = ?`
	row := r.db.GetDB().QueryRowContext(ctx, query, username)

	var user core.User
	var createdAt int64
	var lastLoginAt sql.NullInt64
	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &createdAt, &lastLoginAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	user.CreatedAt = time.Unix(createdAt, 0)
	if lastLoginAt.Valid {
		user.LastLoginAt = time.Unix(lastLoginAt.Int64, 0)
	}
	return &user, nil
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*core.User, error) {
	query := `SELECT id, username, password_hash, role, created_at, last_login_at FROM users WHERE id = ?`
	row := r.db.GetDB().QueryRowContext(ctx, query, id)

	var user core.User
	var createdAt int64
	var lastLoginAt sql.NullInt64
	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &createdAt, &lastLoginAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	user.CreatedAt = time.Unix(createdAt, 0)
	if lastLoginAt.Valid {
		user.LastLoginAt = time.Unix(lastLoginAt.Int64, 0)
	}
	return &user, nil
}

func (r *userRepository) Update(ctx context.Context, user *core.User) error {
	query := `UPDATE users SET last_login_at = ? WHERE id = ?`
	_, err := r.db.GetDB().ExecContext(ctx, query, user.LastLoginAt.Unix(), user.ID)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}
