package gamesvc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
)

const sourceFilesystemListMethod = "source.filesystem.list"

var (
	ErrFileValidationNotSupported = errors.New("connection does not support file validation")
	ErrFileValidationSelection    = errors.New("invalid stale library record selection")
)

type fileValidationIntegrationRepository interface {
	GetByID(ctx context.Context, id string) (*core.Integration, error)
}

type fileValidationGameStore interface {
	GetFoundSourceGameRecords(ctx context.Context, integrationIDs []string) ([]*core.SourceGame, error)
	DeleteSourceGamesByID(ctx context.Context, sourceGameIDs []string) error
}

type FileValidationService interface {
	Validate(ctx context.Context, integrationID string) (*FileValidationReport, error)
	RemoveMissingRecords(ctx context.Context, integrationID string, sourceGameIDs []string) (*RemoveMissingRecordsResult, error)
}

type SourceFileValidationService struct {
	integrationRepo fileValidationIntegrationRepository
	gameStore       fileValidationGameStore
	pluginCaller    pluginCaller
}

type FileValidationFile struct {
	Path     string `json:"path"`
	ObjectID string `json:"object_id,omitempty"`
}

type FileValidationGame struct {
	ID           string               `json:"id"`
	Title        string               `json:"title"`
	RootPath     string               `json:"root_path,omitempty"`
	MissingFiles []FileValidationFile `json:"missing_files"`
}

type FileValidationFailure struct {
	SourceGameID string `json:"source_game_id,omitempty"`
	Title        string `json:"title,omitempty"`
	Message      string `json:"message"`
}

type FileValidationReport struct {
	IntegrationID    string                  `json:"integration_id"`
	IntegrationLabel string                  `json:"integration_label"`
	PluginID         string                  `json:"plugin_id"`
	TotalChecked     int                     `json:"total_checked"`
	FilesChecked     int                     `json:"files_checked"`
	MissingFileCount int                     `json:"missing_file_count"`
	Missing          []FileValidationGame    `json:"missing"`
	Failures         []FileValidationFailure `json:"failures"`
}

type RemoveMissingRecordsResult struct {
	RemovedSourceGameIDs []string `json:"removed_source_game_ids"`
	RemainingMissing     int      `json:"remaining_missing"`
}

type filesystemListResult struct {
	Files []struct {
		Path     string `json:"path"`
		ObjectID string `json:"object_id"`
	} `json:"files"`
}

func NewFileValidationService(
	integrationRepo fileValidationIntegrationRepository,
	gameStore fileValidationGameStore,
	pluginCaller pluginCaller,
) FileValidationService {
	return &SourceFileValidationService{
		integrationRepo: integrationRepo,
		gameStore:       gameStore,
		pluginCaller:    pluginCaller,
	}
}

func (s *SourceFileValidationService) Validate(ctx context.Context, integrationID string) (*FileValidationReport, error) {
	integrationID = strings.TrimSpace(integrationID)
	if integrationID == "" {
		return nil, fmt.Errorf("%w: connection is required", ErrFileValidationSelection)
	}
	if s.integrationRepo == nil || s.gameStore == nil || s.pluginCaller == nil {
		return nil, errors.New("file validation service is unavailable")
	}

	integration, err := s.integrationRepo.GetByID(ctx, integrationID)
	if err != nil {
		return nil, fmt.Errorf("load connection: %w", err)
	}
	if integration == nil {
		return nil, core.ErrSourceGameDeleteNotFound
	}
	if !sourcescope.IsFilesystemBackedPlugin(integration.PluginID) {
		return nil, fmt.Errorf("%w: %s", ErrFileValidationNotSupported, integration.PluginID)
	}

	config, err := parseConfigJSON(integration.ConfigJSON)
	if err != nil {
		return nil, fmt.Errorf("read connection settings: %w", err)
	}
	config = sourcescope.NormalizeConfig(integration.PluginID, config)

	var listing filesystemListResult
	if err := s.pluginCaller.Call(ctx, integration.PluginID, sourceFilesystemListMethod, config, &listing); err != nil {
		return nil, fmt.Errorf("check files through %s: %w", playerFacingIntegrationLabel(integration), err)
	}

	sourceGames, err := s.gameStore.GetFoundSourceGameRecords(ctx, []string{integrationID})
	if err != nil {
		return nil, fmt.Errorf("load games for connection: %w", err)
	}

	report := &FileValidationReport{
		IntegrationID:    integration.ID,
		IntegrationLabel: playerFacingIntegrationLabel(integration),
		PluginID:         integration.PluginID,
		TotalChecked:     len(sourceGames),
		Missing:          []FileValidationGame{},
		Failures:         []FileValidationFailure{},
	}
	liveObjectIDs, livePaths := indexLiveFiles(listing)

	for _, sourceGame := range sourceGames {
		if sourceGame == nil {
			continue
		}
		if len(sourceGame.Files) == 0 {
			report.Failures = append(report.Failures, FileValidationFailure{
				SourceGameID: sourceGame.ID,
				Title:        playerFacingSourceTitle(sourceGame),
				Message:      "MGA has no stored file list for this game. Rescan this connection first.",
			})
			continue
		}

		missingFiles := make([]FileValidationFile, 0)
		for _, file := range sourceGame.Files {
			report.FilesChecked++
			path := sourcescope.NormalizeLogicalPath(file.Path)
			objectID := strings.TrimSpace(file.ObjectID)
			if objectID == "" && path == "" {
				report.Failures = append(report.Failures, FileValidationFailure{
					SourceGameID: sourceGame.ID,
					Title:        playerFacingSourceTitle(sourceGame),
					Message:      "One stored file has no provider ID or path. Rescan this connection.",
				})
				continue
			}
			if fileExistsInListing(objectID, path, liveObjectIDs, livePaths) {
				continue
			}
			missingFiles = append(missingFiles, FileValidationFile{Path: file.Path, ObjectID: objectID})
		}
		if len(missingFiles) == 0 {
			continue
		}

		report.MissingFileCount += len(missingFiles)
		report.Missing = append(report.Missing, FileValidationGame{
			ID:           sourceGame.ID,
			Title:        playerFacingSourceTitle(sourceGame),
			RootPath:     sourceGame.RootPath,
			MissingFiles: missingFiles,
		})
	}

	return report, nil
}

func (s *SourceFileValidationService) RemoveMissingRecords(
	ctx context.Context,
	integrationID string,
	sourceGameIDs []string,
) (*RemoveMissingRecordsResult, error) {
	selected := normalizedUniqueIDs(sourceGameIDs)
	if len(selected) == 0 {
		return nil, fmt.Errorf("%w: select at least one game", ErrFileValidationSelection)
	}

	report, err := s.Validate(ctx, integrationID)
	if err != nil {
		return nil, err
	}
	missing := make(map[string]struct{}, len(report.Missing))
	for _, game := range report.Missing {
		missing[game.ID] = struct{}{}
	}
	for _, sourceGameID := range selected {
		if _, ok := missing[sourceGameID]; !ok {
			return nil, fmt.Errorf("%w: game %s is no longer missing", ErrFileValidationSelection, sourceGameID)
		}
	}

	if err := s.gameStore.DeleteSourceGamesByID(ctx, selected); err != nil {
		return nil, fmt.Errorf("remove stale library records: %w", err)
	}
	return &RemoveMissingRecordsResult{
		RemovedSourceGameIDs: selected,
		RemainingMissing:     len(report.Missing) - len(selected),
	}, nil
}

func indexLiveFiles(listing filesystemListResult) (map[string]struct{}, map[string]struct{}) {
	objectIDs := make(map[string]struct{}, len(listing.Files))
	paths := make(map[string]struct{}, len(listing.Files))
	for _, file := range listing.Files {
		if objectID := strings.TrimSpace(file.ObjectID); objectID != "" {
			objectIDs[objectID] = struct{}{}
		}
		if path := sourcescope.NormalizeLogicalPath(file.Path); path != "" {
			paths[path] = struct{}{}
		}
	}
	return objectIDs, paths
}

func fileExistsInListing(objectID, path string, objectIDs, paths map[string]struct{}) bool {
	if objectID != "" {
		if _, ok := objectIDs[objectID]; ok {
			return true
		}
	}
	if path != "" {
		_, ok := paths[path]
		return ok
	}
	return false
}

func normalizedUniqueIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func playerFacingIntegrationLabel(integration *core.Integration) string {
	if integration == nil {
		return "this connection"
	}
	if label := strings.TrimSpace(integration.Label); label != "" {
		return label
	}
	return integration.PluginID
}

func playerFacingSourceTitle(sourceGame *core.SourceGame) string {
	if sourceGame == nil {
		return "Unknown game"
	}
	if title := strings.TrimSpace(sourceGame.RawTitle); title != "" {
		return title
	}
	return sourceGame.ID
}
