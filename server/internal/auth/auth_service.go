package auth

import (
	"context"
	"errors"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AuthService interface {
	Login(ctx context.Context, username, password string) (*core.Session, error)
	Logout(ctx context.Context, sessionID string) error
	ValidateSession(ctx context.Context, sessionID string) (*core.Session, error)
	CreateInitialAdmin(ctx context.Context, username, password string) error
}

type authService struct {
	userRepo UserRepository
	logger   core.Logger
	// In a real app, I'd have a SessionRepository too.
	// For now, I'll use a simple in-memory map for sessions to get things moving,
	// or implement a sessions table in Phase 2.
	// Actually, let's stick to the roadmap: "Define initial schema for users, sessions".
	// I'll add a simple sessions table.
}

func NewAuthService(userRepo UserRepository, logger core.Logger) AuthService {
	return &authService{
		userRepo: userRepo,
		logger:   logger,
	}
}

func (s *authService) Login(ctx context.Context, username, password string) (*core.Session, error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("invalid credentials")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	// Create session
	session := &core.Session{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour), // 24h for now
	}

	// Update last login
	user.LastLoginAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		s.logger.Error("update last login failed", err)
		// Continue; login still succeeds
	}

	return session, nil
}

func (s *authService) Logout(ctx context.Context, sessionID string) error {
	// TODO: Delete from session storage
	return nil
}

func (s *authService) ValidateSession(ctx context.Context, sessionID string) (*core.Session, error) {
	// TODO: Check session storage
	return nil, nil
}

func (s *authService) CreateInitialAdmin(ctx context.Context, username, password string) error {
	existing, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil // Already exists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := &core.User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: string(hash),
		Role:         "admin",
		CreatedAt:    time.Now(),
	}

	return s.userRepo.Create(ctx, user)
}
