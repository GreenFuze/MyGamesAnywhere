package gamesvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcegames"
)

const sourceFilesystemDeleteMethod = "source.filesystem.delete"

type pluginCaller interface {
	Call(ctx context.Context, pluginID string, method string, params any, result any) error
}

type sourceDeleteFile struct {
	Path       string `json:"path"`
	IsDir      bool   `json:"is_dir,omitempty"`
	Size       int64  `json:"size,omitempty"`
	ObjectID   string `json:"object_id,omitempty"`
	Revision   string `json:"revision,omitempty"`
	ModifiedAt string `json:"modified_at,omitempty"`
}

type sourceDeletionPlan struct {
	CanonicalID   string
	SourceGameID  string
	IntegrationID string
	PluginID      string
	RootPath      string
	Config        map[string]any
	Files         []sourceDeleteFile
}

type sourceDeletePluginResponse struct {
	DeletedCount int                                `json:"deleted_count,omitempty"`
	SourceGameID string                             `json:"source_game_id,omitempty"`
	PluginID     string                             `json:"plugin_id,omitempty"`
	Action       string                             `json:"action,omitempty"`
	Summary      string                             `json:"summary,omitempty"`
	Items        []core.DeleteSourceGamePreviewItem `json:"items,omitempty"`
	Warnings     []string                           `json:"warnings,omitempty"`
}

type DeletionService struct {
	gameStore       core.GameStore
	integrationRepo core.IntegrationRepository
	pluginCaller    pluginCaller
	logger          core.Logger
}

func NewDeletionService(
	gameStore core.GameStore,
	integrationRepo core.IntegrationRepository,
	pluginCaller pluginCaller,
	logger core.Logger,
) core.GameDeletionService {
	return &DeletionService{
		gameStore:       gameStore,
		integrationRepo: integrationRepo,
		pluginCaller:    pluginCaller,
		logger:          logger,
	}
}

func (s *DeletionService) PreviewDeleteSourceGame(ctx context.Context, canonicalID, sourceGameID string) (*core.DeleteSourceGamePreview, error) {
	if strings.TrimSpace(canonicalID) == "" || strings.TrimSpace(sourceGameID) == "" {
		return nil, core.ErrSourceGameDeleteNotFound
	}

	sourceGames, err := s.gameStore.GetSourceGamesForCanonical(ctx, canonicalID)
	if err != nil {
		return nil, fmt.Errorf("get source games for canonical %s: %w", canonicalID, err)
	}

	target := findSourceGame(sourceGames, sourceGameID)
	if target == nil {
		return nil, core.ErrSourceGameDeleteNotFound
	}

	plan, err := s.buildDeletionPlan(ctx, canonicalID, target)
	if err != nil {
		return nil, err
	}
	return s.previewDeletionPlan(ctx, plan)
}

func (s *DeletionService) DeleteSourceGame(ctx context.Context, canonicalID, sourceGameID string) (*core.DeleteSourceGameResult, error) {
	if strings.TrimSpace(canonicalID) == "" || strings.TrimSpace(sourceGameID) == "" {
		return nil, core.ErrSourceGameDeleteNotFound
	}

	sourceGames, err := s.gameStore.GetSourceGamesForCanonical(ctx, canonicalID)
	if err != nil {
		return nil, fmt.Errorf("get source games for canonical %s: %w", canonicalID, err)
	}

	target := findSourceGame(sourceGames, sourceGameID)
	if target == nil {
		return nil, core.ErrSourceGameDeleteNotFound
	}

	plan, err := s.buildDeletionPlan(ctx, canonicalID, target)
	if err != nil {
		return nil, err
	}
	if err := s.executeDeletionPlan(ctx, plan); err != nil {
		return nil, err
	}
	if err := s.gameStore.DeleteSourceGameByID(ctx, sourceGameID); err != nil {
		return nil, err
	}

	updatedCanonical, err := s.gameStore.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		return nil, fmt.Errorf("load canonical game after delete: %w", err)
	}

	return &core.DeleteSourceGameResult{
		DeletedSourceGameID: sourceGameID,
		CanonicalExists:     updatedCanonical != nil,
		CanonicalGame:       updatedCanonical,
	}, nil
}

func (s *DeletionService) PreviewDeleteReviewCandidateFiles(ctx context.Context, candidateID string) (*core.DeleteSourceGamePreview, error) {
	if strings.TrimSpace(candidateID) == "" {
		return nil, core.ErrManualReviewCandidateNotFound
	}
	candidate, err := s.gameStore.GetManualReviewCandidate(ctx, candidateID)
	if err != nil {
		return nil, fmt.Errorf("get manual review candidate %s: %w", candidateID, err)
	}
	if candidate == nil {
		return nil, core.ErrManualReviewCandidateNotFound
	}

	sourceGame := sourceGameFromReviewCandidate(candidate)
	plan, err := s.buildDeletionPlan(ctx, strings.TrimSpace(candidate.CanonicalGameID), sourceGame)
	if err != nil {
		return nil, err
	}
	return s.previewDeletionPlan(ctx, plan)
}

func (s *DeletionService) DeleteReviewCandidateFiles(ctx context.Context, candidateID string) (*core.DeleteSourceGameResult, error) {
	if strings.TrimSpace(candidateID) == "" {
		return nil, core.ErrManualReviewCandidateNotFound
	}
	candidate, err := s.gameStore.GetManualReviewCandidate(ctx, candidateID)
	if err != nil {
		return nil, fmt.Errorf("get manual review candidate %s: %w", candidateID, err)
	}
	if candidate == nil {
		return nil, core.ErrManualReviewCandidateNotFound
	}

	sourceGame := sourceGameFromReviewCandidate(candidate)
	canonicalID := strings.TrimSpace(candidate.CanonicalGameID)
	plan, err := s.buildDeletionPlan(ctx, canonicalID, sourceGame)
	if err != nil {
		return nil, err
	}
	if err := s.executeDeletionPlan(ctx, plan); err != nil {
		return nil, err
	}
	if err := s.gameStore.DeleteSourceGameByID(ctx, candidate.ID); err != nil {
		return nil, err
	}

	result := &core.DeleteSourceGameResult{
		DeletedSourceGameID: candidate.ID,
		CanonicalExists:     false,
	}
	if canonicalID != "" {
		updatedCanonical, err := s.gameStore.GetCanonicalGameByID(ctx, canonicalID)
		if err != nil {
			return nil, fmt.Errorf("load canonical game after candidate delete: %w", err)
		}
		result.CanonicalExists = updatedCanonical != nil
		result.CanonicalGame = updatedCanonical
	}
	return result, nil
}

func (s *DeletionService) buildDeletionPlan(ctx context.Context, canonicalID string, sourceGame *core.SourceGame) (*sourceDeletionPlan, error) {
	eligible, reason := sourcegames.HardDeleteEligibility(sourceGame)
	if !eligible {
		return nil, fmt.Errorf("%w: %s", core.ErrSourceGameDeleteNotEligible, reason)
	}

	integration, err := s.integrationRepo.GetByID(ctx, sourceGame.IntegrationID)
	if err != nil {
		return nil, fmt.Errorf("load integration %s: %w", sourceGame.IntegrationID, err)
	}
	if integration == nil {
		return nil, fmt.Errorf("%w: integration %s no longer exists", core.ErrSourceGameDeleteNotEligible, sourceGame.IntegrationID)
	}

	config, err := parseConfigJSON(integration.ConfigJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid integration config: %v", core.ErrSourceGameDeleteNotEligible, err)
	}

	files := make([]sourceDeleteFile, 0, len(sourceGame.Files))
	for _, file := range sourceGame.Files {
		deletionFile := sourceDeleteFile{
			Path:     file.Path,
			IsDir:    file.IsDir,
			Size:     file.Size,
			ObjectID: file.ObjectID,
			Revision: file.Revision,
		}
		if file.ModifiedAt != nil {
			deletionFile.ModifiedAt = file.ModifiedAt.UTC().Format(time.RFC3339Nano)
		}
		files = append(files, deletionFile)
	}

	return &sourceDeletionPlan{
		CanonicalID:   canonicalID,
		SourceGameID:  sourceGame.ID,
		IntegrationID: sourceGame.IntegrationID,
		PluginID:      sourceGame.PluginID,
		RootPath:      sourceGame.RootPath,
		Config:        config,
		Files:         files,
	}, nil
}

func (s *DeletionService) executeDeletionPlan(ctx context.Context, plan *sourceDeletionPlan) error {
	if plan == nil {
		return fmt.Errorf("%w: deletion plan is required", core.ErrSourceGameDeleteNotEligible)
	}

	var result sourceDeletePluginResponse
	params := map[string]any{
		"config":         plan.Config,
		"root_path":      plan.RootPath,
		"source_game_id": plan.SourceGameID,
		"files":          plan.Files,
		"dry_run":        false,
	}
	if err := s.pluginCaller.Call(ctx, plan.PluginID, sourceFilesystemDeleteMethod, params, &result); err != nil {
		return fmt.Errorf("delete source files for %s via %s: %w", plan.SourceGameID, plan.PluginID, err)
	}
	s.logger.Info("deleted source record backing files", "canonical_id", plan.CanonicalID, "source_game_id", plan.SourceGameID, "plugin_id", plan.PluginID, "deleted_count", result.DeletedCount)
	return nil
}

func (s *DeletionService) previewDeletionPlan(ctx context.Context, plan *sourceDeletionPlan) (*core.DeleteSourceGamePreview, error) {
	if plan == nil {
		return nil, fmt.Errorf("%w: deletion plan is required", core.ErrSourceGameDeleteNotEligible)
	}

	var result sourceDeletePluginResponse
	params := map[string]any{
		"config":         plan.Config,
		"root_path":      plan.RootPath,
		"source_game_id": plan.SourceGameID,
		"files":          plan.Files,
		"dry_run":        true,
	}
	if err := s.pluginCaller.Call(ctx, plan.PluginID, sourceFilesystemDeleteMethod, params, &result); err != nil {
		return nil, fmt.Errorf("preview source file deletion for %s via %s: %w", plan.SourceGameID, plan.PluginID, err)
	}

	preview := &core.DeleteSourceGamePreview{
		SourceGameID: plan.SourceGameID,
		PluginID:     plan.PluginID,
		Action:       strings.TrimSpace(result.Action),
		Summary:      strings.TrimSpace(result.Summary),
		Items:        append([]core.DeleteSourceGamePreviewItem(nil), result.Items...),
		Warnings:     append([]string(nil), result.Warnings...),
	}
	if strings.TrimSpace(result.SourceGameID) != "" {
		preview.SourceGameID = strings.TrimSpace(result.SourceGameID)
	}
	if strings.TrimSpace(result.PluginID) != "" {
		preview.PluginID = strings.TrimSpace(result.PluginID)
	}
	if preview.Action == "" && len(preview.Items) > 0 {
		preview.Action = preview.Items[0].Action
	}
	if preview.Summary == "" {
		preview.Summary = fmt.Sprintf("%d item(s) will be affected", len(preview.Items))
	}
	if len(preview.Items) == 0 {
		return nil, fmt.Errorf("%w: delete preview returned no items", core.ErrSourceGameDeleteNotEligible)
	}
	return preview, nil
}

func findSourceGame(sourceGames []*core.SourceGame, sourceGameID string) *core.SourceGame {
	for _, sourceGame := range sourceGames {
		if sourceGame != nil && sourceGame.ID == sourceGameID {
			return sourceGame
		}
	}
	return nil
}

func parseConfigJSON(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, err
	}
	return config, nil
}

func sourceGameFromReviewCandidate(candidate *core.ManualReviewCandidate) *core.SourceGame {
	if candidate == nil {
		return nil
	}
	return &core.SourceGame{
		ID:            candidate.ID,
		IntegrationID: candidate.IntegrationID,
		PluginID:      candidate.PluginID,
		ExternalID:    candidate.ExternalID,
		RawTitle:      candidate.RawTitle,
		Platform:      candidate.Platform,
		Kind:          candidate.Kind,
		GroupKind:     candidate.GroupKind,
		RootPath:      candidate.RootPath,
		URL:           candidate.URL,
		Status:        candidate.Status,
		ReviewState:   candidate.ReviewState,
		LastSeenAt:    candidate.LastSeenAt,
		CreatedAt:     candidate.CreatedAt,
		Files:         append([]core.GameFile(nil), candidate.Files...),
	}
}
