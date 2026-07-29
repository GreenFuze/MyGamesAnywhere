package gamesvc

import (
	"context"
	"fmt"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type canonicalGroupingService struct {
	store core.GameStore
}

func NewCanonicalGroupingService(store core.GameStore) core.CanonicalGroupingService {
	return &canonicalGroupingService{store: store}
}

func (s *canonicalGroupingService) SplitSourceGame(ctx context.Context, canonicalID, sourceGameID string, decision core.CanonicalReviewDecision) (*core.CanonicalGroupingResult, error) {
	if s.store == nil {
		return nil, fmt.Errorf("game store is required")
	}
	if !decision.ValidFor(core.CanonicalSourcePinModeSplit) {
		return nil, fmt.Errorf("invalid separate-game review decision %q", decision)
	}
	return s.store.SplitSourceGameCanonical(ctx, strings.TrimSpace(canonicalID), strings.TrimSpace(sourceGameID), decision)
}

func (s *canonicalGroupingService) MergeSourceGame(ctx context.Context, canonicalID, sourceGameID, targetCanonicalID string, decision core.CanonicalReviewDecision) (*core.CanonicalGroupingResult, error) {
	if s.store == nil {
		return nil, fmt.Errorf("game store is required")
	}
	if !decision.ValidFor(core.CanonicalSourcePinModeMerge) {
		return nil, fmt.Errorf("invalid combined-game review decision %q", decision)
	}
	return s.store.MergeSourceGameCanonical(ctx, strings.TrimSpace(canonicalID), strings.TrimSpace(sourceGameID), strings.TrimSpace(targetCanonicalID), decision)
}

func (s *canonicalGroupingService) ClearSourceGamePin(ctx context.Context, canonicalID, sourceGameID string) (*core.CanonicalGroupingResult, error) {
	if s.store == nil {
		return nil, fmt.Errorf("game store is required")
	}
	return s.store.ClearSourceGameCanonicalPin(ctx, strings.TrimSpace(canonicalID), strings.TrimSpace(sourceGameID))
}
