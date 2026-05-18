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

func (s *canonicalGroupingService) SplitSourceGame(ctx context.Context, canonicalID, sourceGameID string) (*core.CanonicalGroupingResult, error) {
	if s.store == nil {
		return nil, fmt.Errorf("game store is required")
	}
	return s.store.SplitSourceGameCanonical(ctx, strings.TrimSpace(canonicalID), strings.TrimSpace(sourceGameID))
}

func (s *canonicalGroupingService) MergeSourceGame(ctx context.Context, canonicalID, sourceGameID, targetCanonicalID string) (*core.CanonicalGroupingResult, error) {
	if s.store == nil {
		return nil, fmt.Errorf("game store is required")
	}
	return s.store.MergeSourceGameCanonical(ctx, strings.TrimSpace(canonicalID), strings.TrimSpace(sourceGameID), strings.TrimSpace(targetCanonicalID))
}

func (s *canonicalGroupingService) ClearSourceGamePin(ctx context.Context, canonicalID, sourceGameID string) (*core.CanonicalGroupingResult, error) {
	if s.store == nil {
		return nil, fmt.Errorf("game store is required")
	}
	return s.store.ClearSourceGameCanonicalPin(ctx, strings.TrimSpace(canonicalID), strings.TrimSpace(sourceGameID))
}
