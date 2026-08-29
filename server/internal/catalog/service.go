package catalog

import (
	"context"
	"errors"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type Repository interface {
	RecordObservation(ctx context.Context, profileID string, command ObservationCommand) (*Offer, error)
	MarkRefreshFailed(ctx context.Context, profileID string, failure RefreshFailure) error
	MarkRefreshSucceeded(ctx context.Context, profileID string, scope RefreshScope) error
	GetOffer(ctx context.Context, profileID, offerID string) (*Offer, error)
	ListOffers(ctx context.Context, profileID string, filter OfferFilter) ([]Offer, error)
	ListHistory(ctx context.Context, profileID, offerID string, limit int) ([]HistoryEvent, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("catalog repository is required")
	}
	return &Service{repository: repository}, nil
}

func (s *Service) Observe(ctx context.Context, command ObservationCommand) (*Offer, error) {
	profileID, err := requireProfile(ctx)
	if err != nil {
		return nil, err
	}
	command.Normalize()
	if err := command.Validate(); err != nil {
		return nil, err
	}
	return s.repository.RecordObservation(ctx, profileID, command)
}

func (s *Service) MarkRefreshFailed(ctx context.Context, failure RefreshFailure) error {
	profileID, err := requireProfile(ctx)
	if err != nil {
		return err
	}
	failure.RefreshScope.Normalize()
	failure.Error = strings.TrimSpace(failure.Error)
	if err := failure.Validate(); err != nil {
		return err
	}
	return s.repository.MarkRefreshFailed(ctx, profileID, failure)
}

func (s *Service) MarkRefreshSucceeded(ctx context.Context, scope RefreshScope) error {
	profileID, err := requireProfile(ctx)
	if err != nil {
		return err
	}
	scope.Normalize()
	if err := scope.Validate(); err != nil {
		return err
	}
	return s.repository.MarkRefreshSucceeded(ctx, profileID, scope)
}

func (s *Service) GetOffer(ctx context.Context, offerID string) (*Offer, error) {
	profileID, err := requireProfile(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(offerID) == "" {
		return nil, ErrOfferNotFound
	}
	return s.repository.GetOffer(ctx, profileID, strings.TrimSpace(offerID))
}

func (s *Service) ListOffers(ctx context.Context, filter OfferFilter) ([]Offer, error) {
	profileID, err := requireProfile(ctx)
	if err != nil {
		return nil, err
	}
	filter.CanonicalGameID = strings.TrimSpace(filter.CanonicalGameID)
	filter.Provider = strings.ToLower(strings.TrimSpace(filter.Provider))
	if filter.Availability != "" && !filter.Availability.Valid() {
		return nil, errors.New("unsupported catalog availability filter")
	}
	return s.repository.ListOffers(ctx, profileID, filter)
}

func (s *Service) ListHistory(ctx context.Context, offerID string, limit int) ([]HistoryEvent, error) {
	profileID, err := requireProfile(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(offerID) == "" {
		return nil, ErrOfferNotFound
	}
	if limit < 0 || limit > 1000 {
		return nil, errors.New("catalog history limit must be between 0 and 1000")
	}
	if limit == 0 {
		limit = 100
	}
	return s.repository.ListHistory(ctx, profileID, strings.TrimSpace(offerID), limit)
}

func requireProfile(ctx context.Context) (string, error) {
	profileID := strings.TrimSpace(core.ProfileIDFromContext(ctx))
	if profileID == "" {
		return "", ErrProfileRequired
	}
	return profileID, nil
}
