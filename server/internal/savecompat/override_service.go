package savecompat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OverrideService is the only application-facing entry point for untrusted
// player/community compatibility evidence. It generates identity and lifecycle
// state rather than accepting either from imported data.
type OverrideService struct {
	repository OverrideRepository
	now        func() time.Time
	newID      func() string
}

func NewOverrideService(repository OverrideRepository) (*OverrideService, error) {
	if repository == nil {
		return nil, errors.New("save compatibility override repository is required")
	}
	return &OverrideService{repository: repository, now: time.Now, newID: uuid.NewString}, nil
}

func (s *OverrideService) Submit(ctx context.Context, submission OverrideSubmission) (*CompatibilityOverride, error) {
	if s == nil || s.repository == nil || s.now == nil || s.newID == nil {
		return nil, errors.New("save compatibility override service is unavailable")
	}
	now := s.now().UTC()
	override := CompatibilityOverride{
		ID:               s.newID(),
		Scope:            submission.Scope,
		Relationship:     submission.Relationship,
		ConverterID:      submission.ConverterID,
		ConverterVersion: submission.ConverterVersion,
		Reversible:       submission.Reversible,
		Origin:           submission.Origin,
		Attribution:      submission.Attribution,
		EvidenceSource:   submission.EvidenceSource,
		EvidenceVersion:  submission.EvidenceVersion,
		EvidenceJSON:     submission.EvidenceJSON,
		State:            OverrideStatePending,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := override.Validate(); err != nil {
		return nil, fmt.Errorf("validate save compatibility override submission: %w", err)
	}
	if err := s.repository.CreateOverride(ctx, override); err != nil {
		return nil, fmt.Errorf("create save compatibility override: %w", err)
	}
	return &override, nil
}

// ReviewAsAdmin assumes the HTTP/application authorization layer has already
// established that reviewerProfileID belongs to an admin profile.
func (s *OverrideService) ReviewAsAdmin(ctx context.Context, overrideID, reviewerProfileID string, resolveConflict bool) (*CompatibilityOverride, error) {
	if s == nil || s.repository == nil || s.now == nil {
		return nil, errors.New("save compatibility override service is unavailable")
	}
	if err := validateSingleLine("override ID", overrideID, maxIDLength); err != nil {
		return nil, err
	}
	if err := validateSingleLine("reviewer profile ID", reviewerProfileID, maxIDLength); err != nil {
		return nil, err
	}
	return s.repository.ApproveOverride(ctx, overrideID, reviewerProfileID, s.now().UTC(), resolveConflict)
}

func (s *OverrideService) Revoke(ctx context.Context, overrideID, actorProfileID string, actorIsAdmin bool) (*CompatibilityOverride, error) {
	if s == nil || s.repository == nil || s.now == nil {
		return nil, errors.New("save compatibility override service is unavailable")
	}
	if err := validateSingleLine("override ID", overrideID, maxIDLength); err != nil {
		return nil, err
	}
	if err := validateSingleLine("actor profile ID", actorProfileID, maxIDLength); err != nil {
		return nil, err
	}
	return s.repository.RevokeOverride(ctx, overrideID, actorProfileID, s.now().UTC(), actorIsAdmin)
}

func (s *OverrideService) FindApproved(ctx context.Context, scope OverrideScope) (*CompatibilityOverride, error) {
	if s == nil || s.repository == nil {
		return nil, errors.New("save compatibility override service is unavailable")
	}
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	return s.repository.FindApprovedOverride(ctx, scope)
}
