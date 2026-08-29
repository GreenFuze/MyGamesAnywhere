package legacyretirement

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrProfileRequired = errors.New("profile is required for legacy retirement report")

type Repository interface {
	BuildReport(context.Context, string, time.Time) (*Report, error)
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("legacy retirement repository is required")
	}
	return &Service{repository: repository, now: time.Now}, nil
}

func (s *Service) Report(ctx context.Context, profileID string) (*Report, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return nil, ErrProfileRequired
	}
	return s.repository.BuildReport(ctx, profileID, s.now().UTC().Truncate(time.Second))
}
