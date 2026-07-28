package sourcemove

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
	"github.com/google/uuid"
)

const (
	transferBeginMethod  = "source.transfer.begin"
	transferPutMethod    = "source.transfer.put"
	transferCommitMethod = "source.transfer.commit"
	transferAbortMethod  = "source.transfer.abort"
	materializeMethod    = "source.file.materialize"
)

type gameStore interface {
	GetCanonicalGameByID(ctx context.Context, canonicalID string) (*core.CanonicalGame, error)
	GetSourceGamesForCanonical(ctx context.Context, canonicalID string) ([]*core.SourceGame, error)
	IsSourceRootExclusive(ctx context.Context, integrationID, sourceGameID, rootPath string) (bool, error)
}

type pluginCaller interface {
	Call(ctx context.Context, pluginID string, method string, params any, result any) error
	GetPlugin(pluginID string) (*core.Plugin, bool)
}

type libraryScanner interface {
	RunScan(ctx context.Context, integrationIDs []string) ([]*core.CanonicalGame, error)
}

type Service struct {
	store           core.SourceMoveStore
	gameStore       gameStore
	integrationRepo core.IntegrationRepository
	plugins         pluginCaller
	deletion        core.GameDeletionService
	scanner         libraryScanner
	config          core.Configuration
	logger          core.Logger

	mu sync.Mutex
}

func NewService(
	store core.SourceMoveStore,
	gameStore gameStore,
	integrationRepo core.IntegrationRepository,
	plugins pluginCaller,
	deletion core.GameDeletionService,
	scanner libraryScanner,
	config core.Configuration,
	logger core.Logger,
) *Service {
	return &Service{
		store:           store,
		gameStore:       gameStore,
		integrationRepo: integrationRepo,
		plugins:         plugins,
		deletion:        deletion,
		scanner:         scanner,
		config:          config,
		logger:          logger,
	}
}

func (s *Service) Run(ctx context.Context) error {
	return s.store.MarkInFlightJobsInterrupted(ctx)
}

func (s *Service) ListDestinations(ctx context.Context) ([]core.SourceMoveDestination, error) {
	integrations, err := s.integrationRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]core.SourceMoveDestination, 0, len(integrations))
	for _, integration := range integrations {
		if integration == nil || strings.TrimSpace(integration.ID) == "" || !s.pluginCanReceive(integration.PluginID) {
			continue
		}
		suggestedRoot := ""
		if config, configErr := parseIntegrationConfig(integration); configErr == nil {
			includes := sourcescope.ReadIncludePaths(integration.PluginID, config)
			if len(includes) > 0 {
				suggestedRoot = sourcescope.NormalizeLogicalPath(includes[0].Path)
			}
		}
		result = append(result, core.SourceMoveDestination{
			IntegrationID: integration.ID,
			PluginID:      integration.PluginID,
			Label:         integration.Label,
			SuggestedRoot: suggestedRoot,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		return strings.ToLower(result[i].Label) < strings.ToLower(result[j].Label)
	})
	return result, nil
}

func (s *Service) Preview(ctx context.Context, req core.SourceMovePreviewRequest) (*core.SourceMovePreview, error) {
	if len(req.Items) == 0 {
		return nil, fmt.Errorf("at least one game copy is required")
	}
	if len(req.Items) > 100 {
		return nil, fmt.Errorf("at most 100 game copies can be moved at once")
	}
	result := &core.SourceMovePreview{Items: make([]core.SourceMovePreviewItem, 0, len(req.Items))}
	seen := make(map[string]bool, len(req.Items))
	for _, selection := range req.Items {
		key := strings.TrimSpace(selection.SourceGameID)
		if key == "" || seen[key] {
			return nil, fmt.Errorf("each source game must be selected exactly once")
		}
		seen[key] = true
		item, err := s.previewOne(ctx, selection)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *item)
	}
	return result, nil
}

func (s *Service) Start(ctx context.Context, req core.SourceMoveStartRequest) ([]*core.SourceMoveJob, error) {
	preview, err := s.Preview(ctx, core.SourceMovePreviewRequest{Items: req.Items})
	if err != nil {
		return nil, err
	}
	for _, item := range preview.Items {
		if !item.CanMove {
			return nil, fmt.Errorf("%s cannot be moved: %s", item.SourceTitle, item.Reason)
		}
	}

	owner, ok := core.ProfileFromContext(ctx)
	if !ok || owner == nil || strings.TrimSpace(owner.ID) == "" {
		return nil, core.ErrProfileRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := make([]*core.SourceMoveJob, 0, len(preview.Items))
	for _, item := range preview.Items {
		source, err := s.loadSource(ctx, item.CanonicalGameID, item.SourceGameID)
		if err != nil {
			s.rollbackQueuedJobs(ctx, jobs)
			return nil, err
		}
		files, err := moveFiles(source)
		if err != nil {
			s.rollbackQueuedJobs(ctx, jobs)
			return nil, err
		}
		job := &core.SourceMoveJob{
			ID:                       uuid.NewString(),
			TransferID:               uuid.NewString(),
			CanonicalGameID:          item.CanonicalGameID,
			CanonicalTitle:           item.CanonicalTitle,
			SourceGameID:             item.SourceGameID,
			SourceTitle:              item.SourceTitle,
			SourceIntegrationID:      item.SourceIntegrationID,
			SourcePluginID:           item.SourcePluginID,
			SourceRootPath:           item.SourceRootPath,
			DestinationIntegrationID: item.DestinationIntegrationID,
			DestinationPluginID:      item.DestinationPluginID,
			DestinationAuthority:     item.DestinationAuthority,
			DestinationLabel:         item.DestinationLabel,
			DestinationPath:          item.DestinationPath,
			Status:                   core.SourceMoveStatusQueued,
			Message:                  "Move queued",
			WholeDirectory:           item.WholeDirectory,
			ProgressTotal:            len(files),
			Files:                    files,
		}
		if err := s.store.CreateJob(ctx, job); err != nil {
			s.rollbackQueuedJobs(ctx, jobs)
			return nil, fmt.Errorf("reserve destination %q: %w", item.DestinationPath, err)
		}
		jobs = append(jobs, job)
	}

	profile := *owner
	for _, job := range jobs {
		jobCopy := cloneJob(job)
		go s.run(core.WithProfile(context.Background(), &profile), jobCopy)
	}
	return jobs, nil
}

func (s *Service) GetJob(ctx context.Context, jobID string) (*core.SourceMoveJob, error) {
	job, err := s.store.GetJob(ctx, strings.TrimSpace(jobID))
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, fmt.Errorf("source move job not found")
	}
	return job, nil
}

func (s *Service) ListJobs(ctx context.Context, limit int) ([]*core.SourceMoveJob, error) {
	return s.store.ListJobs(ctx, limit)
}

func (s *Service) Retry(ctx context.Context, jobID string) (*core.SourceMoveJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, err := s.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	switch job.Status {
	case core.SourceMoveStatusFailedBeforeCommit:
		job.Status = core.SourceMoveStatusQueued
		job.Message = "Retry queued"
		job.Error = ""
		job.RecoveryPhase = ""
		job.FinishedAt = nil
		job.ProgressCurrent = 0
		for index := range job.Files {
			job.Files[index].Status = "pending"
			job.Files[index].Error = ""
			job.Files[index].SHA256 = ""
		}
		if err := s.store.ReplaceFiles(ctx, job.ID, job.Files); err != nil {
			return nil, err
		}
		if err := s.store.UpdateJob(ctx, job); err != nil {
			return nil, err
		}
		owner, _ := core.ProfileFromContext(ctx)
		profile := *owner
		go s.run(core.WithProfile(context.Background(), &profile), cloneJob(job))
	case core.SourceMoveStatusSourceCleanupRequired:
		job.Status = core.SourceMoveStatusDestinationCommitted
		job.Message = "Recovery retry queued"
		job.Error = ""
		if err := s.store.UpdateJob(ctx, job); err != nil {
			return nil, err
		}
		owner, _ := core.ProfileFromContext(ctx)
		profile := *owner
		if job.RecoveryPhase == core.SourceMoveStatusDeletingSource {
			go s.deleteSourceAndComplete(core.WithProfile(context.Background(), &profile), cloneJob(job))
		} else {
			go s.finishAfterCommit(core.WithProfile(context.Background(), &profile), cloneJob(job))
		}
	case core.SourceMoveStatusInterrupted:
		owner, _ := core.ProfileFromContext(ctx)
		profile := *owner
		if interruptedAfterCommit(job.RecoveryPhase) {
			job.Status = core.SourceMoveStatusDestinationCommitted
			job.Message = "Destination was committed; source cleanup retry queued"
			job.Error = ""
			if err := s.store.UpdateJob(ctx, job); err != nil {
				return nil, err
			}
			if job.RecoveryPhase == core.SourceMoveStatusDeletingSource {
				go s.deleteSourceAndComplete(core.WithProfile(context.Background(), &profile), cloneJob(job))
			} else {
				go s.finishAfterCommit(core.WithProfile(context.Background(), &profile), cloneJob(job))
			}
		} else {
			if err := s.abortDestination(ctx, job); err != nil {
				return nil, fmt.Errorf("clean interrupted destination stage before retry: %w", err)
			}
			job.Status = core.SourceMoveStatusQueued
			job.Message = "Interrupted move retry queued"
			job.Error = ""
			job.RecoveryPhase = ""
			job.ProgressCurrent = 0
			for index := range job.Files {
				job.Files[index].Status = "pending"
				job.Files[index].Error = ""
				job.Files[index].SHA256 = ""
			}
			if err := s.store.ReplaceFiles(ctx, job.ID, job.Files); err != nil {
				return nil, err
			}
			if err := s.store.UpdateJob(ctx, job); err != nil {
				return nil, err
			}
			go s.run(core.WithProfile(context.Background(), &profile), cloneJob(job))
		}
	default:
		return nil, fmt.Errorf("move cannot be retried while it is %s", job.Status)
	}
	return job, nil
}

func (s *Service) CleanupStage(ctx context.Context, jobID string) (*core.SourceMoveJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.Status != core.SourceMoveStatusFailedBeforeCommit &&
		!(job.Status == core.SourceMoveStatusInterrupted && !interruptedAfterCommit(job.RecoveryPhase)) {
		return nil, fmt.Errorf("only an uncommitted failed move can be cleaned up")
	}
	if err := s.abortDestination(ctx, job); err != nil {
		return nil, err
	}
	s.removeTemp(job.ID)
	now := time.Now().UTC()
	job.Message = "Temporary destination files removed; source is unchanged"
	job.Error = ""
	job.FinishedAt = &now
	if err := s.store.UpdateJob(ctx, job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Service) KeepBoth(ctx context.Context, jobID string) (*core.SourceMoveJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.Status != core.SourceMoveStatusSourceCleanupRequired &&
		!(job.Status == core.SourceMoveStatusInterrupted && interruptedAfterCommit(job.RecoveryPhase)) {
		return nil, fmt.Errorf("keep both is available only after the destination was committed")
	}
	job.KeepBoth = true
	job.Status = core.SourceMoveStatusRefreshingLibrary
	job.Message = "Keeping both copies and refreshing the destination"
	job.Error = ""
	if err := s.store.UpdateJob(ctx, job); err != nil {
		return nil, err
	}
	owner, _ := core.ProfileFromContext(ctx)
	profile := *owner
	go s.refreshAndComplete(core.WithProfile(context.Background(), &profile), cloneJob(job))
	return job, nil
}

func (s *Service) previewOne(ctx context.Context, selection core.SourceMoveSelection) (*core.SourceMovePreviewItem, error) {
	selection.CanonicalGameID = strings.TrimSpace(selection.CanonicalGameID)
	selection.SourceGameID = strings.TrimSpace(selection.SourceGameID)
	selection.DestinationIntegrationID = strings.TrimSpace(selection.DestinationIntegrationID)
	rawDestinationPath := strings.TrimSpace(selection.DestinationPath)
	if logicalPathHasTraversal(rawDestinationPath) || filepath.IsAbs(rawDestinationPath) {
		return nil, fmt.Errorf("destination folder must be a safe connection-relative path")
	}
	selection.DestinationPath = sourcescope.NormalizeLogicalPath(rawDestinationPath)
	if selection.CanonicalGameID == "" || selection.SourceGameID == "" ||
		selection.DestinationIntegrationID == "" || selection.DestinationPath == "" {
		return nil, fmt.Errorf("game copy, destination connection, and destination folder are required")
	}
	game, err := s.gameStore.GetCanonicalGameByID(ctx, selection.CanonicalGameID)
	if err != nil {
		return nil, err
	}
	if game == nil {
		return nil, fmt.Errorf("game not found")
	}
	source, err := s.loadSource(ctx, selection.CanonicalGameID, selection.SourceGameID)
	if err != nil {
		return nil, err
	}
	destination, err := s.integrationRepo.GetByID(ctx, selection.DestinationIntegrationID)
	if err != nil {
		return nil, err
	}
	if destination == nil {
		return nil, fmt.Errorf("destination connection not found for this profile")
	}
	item := &core.SourceMovePreviewItem{
		CanonicalGameID:          selection.CanonicalGameID,
		CanonicalTitle:           game.Title,
		SourceGameID:             source.ID,
		SourceTitle:              source.RawTitle,
		SourceIntegrationID:      source.IntegrationID,
		SourcePluginID:           source.PluginID,
		SourceRootPath:           sourcescope.NormalizeLogicalPath(source.RootPath),
		DestinationIntegrationID: destination.ID,
		DestinationPluginID:      destination.PluginID,
		DestinationLabel:         destination.Label,
		DestinationPath:          selection.DestinationPath,
	}
	if !s.pluginCanReceive(destination.PluginID) {
		item.Reason = "This connection cannot receive game files yet"
		return item, nil
	}
	if source.IntegrationID == destination.ID {
		item.Reason = "Choose a different storage connection"
		return item, nil
	}
	if !s.pluginProvides(source.PluginID, materializeMethod) {
		item.Reason = "This source cannot provide files for a move yet"
		return item, nil
	}
	files, err := moveFiles(source)
	if err != nil {
		item.Reason = err.Error()
		return item, nil
	}
	item.FileCount = len(files)
	item.Files = append([]core.SourceMoveFile(nil), files...)
	for _, file := range files {
		item.TotalSize += file.Size
	}
	rootExclusive, err := s.gameStore.IsSourceRootExclusive(ctx, source.IntegrationID, source.ID, source.RootPath)
	if err != nil {
		return nil, err
	}
	deletePreview, err := s.deletion.PreviewDeleteSourceGame(ctx, selection.CanonicalGameID, selection.SourceGameID)
	if err != nil {
		item.Reason = "The original cannot be removed safely: " + err.Error()
		return item, nil
	}
	if deletePreview == nil || len(deletePreview.Items) == 0 {
		item.Reason = "The source did not provide a safe original-removal plan"
		return item, nil
	}
	item.SourceAction = deletePreview.Action
	item.SourceSummary = deletePreview.Summary
	item.Warnings = append(item.Warnings, deletePreview.Warnings...)
	item.WholeDirectory = rootExclusive && deletePreviewRemovesRoot(deletePreview, item.SourceRootPath)
	if !item.WholeDirectory {
		item.Warnings = append(item.Warnings, "MGA will move only this game copy's known files; it will not remove an unproven shared or incomplete folder.")
	}
	config, err := parseIntegrationConfig(destination)
	if err != nil {
		item.Reason = err.Error()
		return item, nil
	}
	item.DestinationAuthority, err = s.destinationAuthority(ctx, destination, config, item.DestinationPath)
	if err != nil {
		item.Reason = "MGA could not prove the destination storage identity: " + err.Error()
		return item, nil
	}
	var response transferResponse
	err = s.plugins.Call(ctx, destination.PluginID, transferBeginMethod, transferBeginRequest{
		Config:          config,
		TransferID:      "preview-" + uuid.NewString(),
		DestinationPath: item.DestinationPath,
		DryRun:          true,
		Files:           transferFiles(files),
	}, &response)
	if err != nil {
		item.Reason = friendlyPluginError(err)
		return item, nil
	}
	item.CanMove = true
	return item, nil
}

type pluginCheckResult struct {
	Status         string `json:"status"`
	Message        string `json:"message"`
	SourceIdentity string `json:"source_identity"`
}

func (s *Service) destinationAuthority(ctx context.Context, integration *core.Integration, config map[string]any, destinationPath string) (string, error) {
	var result pluginCheckResult
	if err := s.plugins.Call(ctx, integration.PluginID, "plugin.check_config", map[string]any{"config": config}, &result); err != nil {
		return "", err
	}
	if result.Status != "" && result.Status != "ok" {
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = result.Status
		}
		return "", fmt.Errorf("%s", message)
	}
	identity := strings.TrimSpace(result.SourceIdentity)
	if identity == "" {
		return "", fmt.Errorf("connection did not return a stable storage identity")
	}
	if integration.PluginID != "game-source-google-drive" {
		return identity, nil
	}
	selectedScope := ""
	selectedPathLength := -1
	for _, include := range sourcescope.ReadIncludePaths(integration.PluginID, config) {
		includePath := sourcescope.NormalizeLogicalPath(include.Path)
		if destinationPath != includePath && !strings.HasPrefix(destinationPath, includePath+"/") {
			continue
		}
		scope := includePath
		if objectID := strings.TrimSpace(include.ObjectID); objectID != "" {
			scope = "object:" + objectID
		}
		if len(includePath) > selectedPathLength {
			selectedScope = scope
			selectedPathLength = len(includePath)
		}
	}
	if selectedScope == "" {
		return "", fmt.Errorf("destination is outside its configured storage scope")
	}
	return identity + "|" + selectedScope, nil
}

func deletePreviewRemovesRoot(preview *core.DeleteSourceGamePreview, rootPath string) bool {
	rootPath = sourcescope.NormalizeLogicalPath(rootPath)
	if preview == nil || rootPath == "" {
		return false
	}
	for _, item := range preview.Items {
		if item.IsDir && sourcescope.NormalizeLogicalPath(item.Path) == rootPath {
			return true
		}
	}
	return false
}

func (s *Service) pluginCanReceive(pluginID string) bool {
	plugin, ok := s.plugins.GetPlugin(pluginID)
	if !ok || plugin == nil || !plugin.Enabled {
		return false
	}
	required := map[string]bool{
		transferBeginMethod: false, transferPutMethod: false, transferCommitMethod: false, transferAbortMethod: false,
	}
	for _, method := range plugin.Manifest.Provides {
		if _, ok := required[method]; ok {
			required[method] = true
		}
	}
	for _, present := range required {
		if !present {
			return false
		}
	}
	return true
}

func (s *Service) pluginProvides(pluginID, method string) bool {
	plugin, ok := s.plugins.GetPlugin(pluginID)
	if !ok || plugin == nil || !plugin.Enabled {
		return false
	}
	for _, provided := range plugin.Manifest.Provides {
		if provided == method {
			return true
		}
	}
	return false
}

func (s *Service) run(ctx context.Context, job *core.SourceMoveJob) {
	if err := s.materialize(ctx, job); err != nil {
		s.failBeforeCommit(ctx, job, err)
		return
	}
	destination, err := s.integrationRepo.GetByID(ctx, job.DestinationIntegrationID)
	if err != nil || destination == nil {
		if err == nil {
			err = fmt.Errorf("destination connection no longer exists")
		}
		s.failBeforeCommit(ctx, job, err)
		return
	}
	config, err := parseIntegrationConfig(destination)
	if err != nil {
		s.failBeforeCommit(ctx, job, err)
		return
	}
	job.Status = core.SourceMoveStatusStagingDestination
	job.Message = "Copying files to " + job.DestinationLabel
	job.Error = ""
	if err := s.store.UpdateJob(ctx, job); err != nil {
		s.failBeforeCommit(ctx, job, err)
		return
	}
	var response transferResponse
	if err := s.plugins.Call(ctx, job.DestinationPluginID, transferBeginMethod, transferBeginRequest{
		Config: config, TransferID: job.TransferID, DestinationPath: job.DestinationPath, Files: transferFiles(job.Files),
	}, &response); err != nil {
		s.failBeforeCommit(ctx, job, err)
		return
	}
	for index := range job.Files {
		file := &job.Files[index]
		if err := s.plugins.Call(ctx, job.DestinationPluginID, transferPutMethod, transferPutRequest{
			Config: config, TransferID: job.TransferID, DestinationPath: job.DestinationPath,
			RelativePath: file.RelativePath, SourcePath: s.tempFile(job.ID, file.Ordinal),
			Size: file.Size, SHA256: file.SHA256,
		}, &response); err != nil {
			file.Error = err.Error()
			_ = s.store.UpdateFile(ctx, job.ID, *file)
			s.failBeforeCommit(ctx, job, err)
			return
		}
		file.Status = "staged"
		file.Error = ""
		job.ProgressCurrent = index + 1
		job.Message = fmt.Sprintf("Copied %d of %d files to %s", index+1, len(job.Files), job.DestinationLabel)
		_ = s.store.UpdateFile(ctx, job.ID, *file)
		_ = s.store.UpdateJob(ctx, job)
	}
	if err := s.plugins.Call(ctx, job.DestinationPluginID, transferCommitMethod, transferCommitRequest{
		Config: config, TransferID: job.TransferID, DestinationPath: job.DestinationPath, Files: transferFiles(job.Files),
	}, &response); err != nil {
		s.failBeforeCommit(ctx, job, err)
		return
	}
	for index := range job.Files {
		job.Files[index].Status = "committed"
		_ = s.store.UpdateFile(ctx, job.ID, job.Files[index])
	}
	job.Status = core.SourceMoveStatusDestinationCommitted
	job.RecoveryPhase = core.SourceMoveStatusDestinationCommitted
	job.Message = "Destination verified; adding the new copy to your library"
	job.Error = ""
	if err := s.store.UpdateJob(ctx, job); err != nil {
		s.logger.Error("persist committed source move", err, "job_id", job.ID)
		return
	}
	s.removeTemp(job.ID)
	s.finishAfterCommit(ctx, job)
}

func (s *Service) materialize(ctx context.Context, job *core.SourceMoveJob) error {
	source, err := s.loadSource(ctx, job.CanonicalGameID, job.SourceGameID)
	if err != nil {
		return err
	}
	integration, err := s.integrationRepo.GetByID(ctx, source.IntegrationID)
	if err != nil || integration == nil {
		if err == nil {
			err = fmt.Errorf("source connection no longer exists")
		}
		return err
	}
	config, err := parseIntegrationConfig(integration)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(s.tempDir(job.ID)); err != nil {
		return err
	}
	if err := os.MkdirAll(s.tempDir(job.ID), 0o700); err != nil {
		return err
	}
	job.Status = core.SourceMoveStatusMaterializingSource
	job.Message = "Reading source files"
	job.Error = ""
	job.ProgressCurrent = 0
	if err := s.store.UpdateJob(ctx, job); err != nil {
		return err
	}
	for index := range job.Files {
		file := &job.Files[index]
		target := s.tempFile(job.ID, file.Ordinal)
		var result core.SourceMaterializeResult
		if err := s.plugins.Call(ctx, job.SourcePluginID, materializeMethod, core.SourceMaterializeRequest{
			Config: config, Path: file.SourcePath, ObjectID: file.ObjectID,
			Revision: file.Revision, DestPath: target,
		}, &result); err != nil {
			file.Error = err.Error()
			_ = s.store.UpdateFile(ctx, job.ID, *file)
			return err
		}
		hash, size, err := hashFile(target)
		if err != nil {
			return err
		}
		if file.Size > 0 && size != file.Size {
			return fmt.Errorf("source file %q changed size during move", file.SourcePath)
		}
		file.Size = size
		file.SHA256 = hash
		file.Status = "materialized"
		file.Error = ""
		job.ProgressCurrent = index + 1
		job.Message = fmt.Sprintf("Read %d of %d source files", index+1, len(job.Files))
		if err := s.store.UpdateFile(ctx, job.ID, *file); err != nil {
			return err
		}
		if err := s.store.UpdateJob(ctx, job); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) finishAfterCommit(ctx context.Context, job *core.SourceMoveJob) {
	job.Status = core.SourceMoveStatusRefreshingLibrary
	job.RecoveryPhase = core.SourceMoveStatusRefreshingLibrary
	job.Message = "Adding the verified copy to your library"
	job.Error = ""
	if err := s.store.UpdateJob(ctx, job); err != nil {
		s.logger.Error("update source move refresh status", err, "job_id", job.ID)
		return
	}
	if _, err := s.scanner.RunScan(ctx, []string{job.DestinationIntegrationID}); err != nil {
		job.Status = core.SourceMoveStatusSourceCleanupRequired
		job.Message = "The new copy is safe, but MGA could not add it to your library yet"
		job.Error = err.Error()
		if updateErr := s.store.UpdateJob(ctx, job); updateErr != nil {
			s.logger.Error("persist source move refresh failure", updateErr, "job_id", job.ID)
		}
		return
	}
	s.deleteSourceAndComplete(ctx, job)
}

func (s *Service) deleteSourceAndComplete(ctx context.Context, job *core.SourceMoveJob) {
	job.Status = core.SourceMoveStatusDeletingSource
	job.RecoveryPhase = core.SourceMoveStatusDeletingSource
	job.Message = "Removing the original copy"
	job.Error = ""
	if err := s.store.UpdateJob(ctx, job); err != nil {
		s.logger.Error("update source move cleanup status", err, "job_id", job.ID)
		return
	}
	if _, err := s.deletion.DeleteSourceGame(ctx, job.CanonicalGameID, job.SourceGameID); err != nil {
		job.Status = core.SourceMoveStatusSourceCleanupRequired
		job.Message = "The new copy is safe, but the original still needs attention"
		job.Error = err.Error()
		if updateErr := s.store.UpdateJob(ctx, job); updateErr != nil {
			s.logger.Error("persist source move cleanup failure", updateErr, "job_id", job.ID)
		}
		return
	}
	s.complete(ctx, job)
}

func (s *Service) refreshAndComplete(ctx context.Context, job *core.SourceMoveJob) {
	job.Status = core.SourceMoveStatusRefreshingLibrary
	job.Message = "Refreshing the library"
	job.Error = ""
	if err := s.store.UpdateJob(ctx, job); err != nil {
		s.logger.Error("update source move refresh status", err, "job_id", job.ID)
		return
	}
	if _, err := s.scanner.RunScan(ctx, []string{job.DestinationIntegrationID}); err != nil {
		job.Status = core.SourceMoveStatusSourceCleanupRequired
		job.RecoveryPhase = core.SourceMoveStatusRefreshingLibrary
		job.Message = "Files are safe, but MGA could not refresh the destination"
		job.Error = err.Error()
		if updateErr := s.store.UpdateJob(ctx, job); updateErr != nil {
			s.logger.Error("persist source move refresh failure", updateErr, "job_id", job.ID)
		}
		return
	}
	s.complete(ctx, job)
}

func (s *Service) complete(ctx context.Context, job *core.SourceMoveJob) {
	now := time.Now().UTC()
	job.Status = core.SourceMoveStatusCompleted
	job.RecoveryPhase = ""
	if job.KeepBoth {
		job.Message = "Both copies are available"
	} else {
		job.Message = "Move completed"
	}
	job.Error = ""
	job.ProgressCurrent = job.ProgressTotal
	job.FinishedAt = &now
	if err := s.store.UpdateJob(ctx, job); err != nil {
		s.logger.Error("complete source move", err, "job_id", job.ID)
	}
}

func (s *Service) failBeforeCommit(ctx context.Context, job *core.SourceMoveJob, err error) {
	job.Status = core.SourceMoveStatusFailedBeforeCommit
	job.Message = "Move stopped before changing the original"
	job.Error = err.Error()
	if updateErr := s.store.UpdateJob(ctx, job); updateErr != nil {
		s.logger.Error("persist source move failure", updateErr, "job_id", job.ID)
	}
}

func (s *Service) abortDestination(ctx context.Context, job *core.SourceMoveJob) error {
	integration, err := s.integrationRepo.GetByID(ctx, job.DestinationIntegrationID)
	if err != nil {
		return err
	}
	if integration == nil {
		return fmt.Errorf("destination connection no longer exists")
	}
	config, err := parseIntegrationConfig(integration)
	if err != nil {
		return err
	}
	var response transferResponse
	return s.plugins.Call(ctx, job.DestinationPluginID, transferAbortMethod, transferAbortRequest{
		Config: config, TransferID: job.TransferID, DestinationPath: job.DestinationPath,
	}, &response)
}

func (s *Service) loadSource(ctx context.Context, canonicalID, sourceGameID string) (*core.SourceGame, error) {
	sources, err := s.gameStore.GetSourceGamesForCanonical(ctx, canonicalID)
	if err != nil {
		return nil, err
	}
	for _, source := range sources {
		if source != nil && source.ID == sourceGameID {
			return source, nil
		}
	}
	return nil, fmt.Errorf("source game not found for this profile")
}

func (s *Service) rollbackQueuedJobs(ctx context.Context, jobs []*core.SourceMoveJob) {
	for _, job := range jobs {
		if err := s.store.DeleteJob(ctx, job.ID); err != nil {
			s.logger.Warn("rollback queued source move failed", "job_id", job.ID, "error", err)
		}
	}
}

func (s *Service) moveRoot() string {
	dbPath := strings.TrimSpace(s.config.Get("DB_PATH"))
	if dbPath == "" || dbPath == ":memory:" || strings.Contains(dbPath, "mode=memory") {
		return filepath.Join(os.TempDir(), "mga-source-moves")
	}
	return filepath.Join(filepath.Dir(dbPath), "source-moves")
}

func (s *Service) tempDir(jobID string) string {
	return filepath.Join(s.moveRoot(), jobID)
}

func (s *Service) tempFile(jobID string, ordinal int) string {
	return filepath.Join(s.tempDir(jobID), fmt.Sprintf("%06d.bin", ordinal))
}

func (s *Service) removeTemp(jobID string) {
	if err := os.RemoveAll(s.tempDir(jobID)); err != nil {
		s.logger.Warn("remove source move temp files failed", "job_id", jobID, "error", err)
	}
}

type transferFile struct {
	RelativePath string `json:"relative_path"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256,omitempty"`
}

type transferBeginRequest struct {
	Config          map[string]any `json:"config"`
	TransferID      string         `json:"transfer_id"`
	DestinationPath string         `json:"destination_path"`
	DryRun          bool           `json:"dry_run,omitempty"`
	Files           []transferFile `json:"files"`
}

type transferPutRequest struct {
	Config          map[string]any `json:"config"`
	TransferID      string         `json:"transfer_id"`
	DestinationPath string         `json:"destination_path"`
	RelativePath    string         `json:"relative_path"`
	SourcePath      string         `json:"source_path"`
	Size            int64          `json:"size"`
	SHA256          string         `json:"sha256"`
}

type transferCommitRequest struct {
	Config          map[string]any `json:"config"`
	TransferID      string         `json:"transfer_id"`
	DestinationPath string         `json:"destination_path"`
	Files           []transferFile `json:"files"`
}

type transferAbortRequest struct {
	Config          map[string]any `json:"config"`
	TransferID      string         `json:"transfer_id"`
	DestinationPath string         `json:"destination_path"`
}

type transferResponse struct {
	Status          string `json:"status,omitempty"`
	DestinationPath string `json:"destination_path,omitempty"`
	Summary         string `json:"summary,omitempty"`
}

func moveFiles(source *core.SourceGame) ([]core.SourceMoveFile, error) {
	if source == nil {
		return nil, fmt.Errorf("source game is required")
	}
	root := sourcescope.NormalizeLogicalPath(source.RootPath)
	if root == "" {
		return nil, fmt.Errorf("this copy has no file root")
	}
	files := make([]core.SourceMoveFile, 0, len(source.Files))
	for _, sourceFile := range source.Files {
		if sourceFile.IsDir {
			continue
		}
		if logicalPathHasTraversal(sourceFile.Path) || filepath.IsAbs(sourceFile.Path) {
			return nil, fmt.Errorf("file %q has an unsafe source path", sourceFile.Path)
		}
		sourcePath := sourcescope.NormalizeLogicalPath(sourceFile.Path)
		if sourcePath == "" || (sourcePath != root && !strings.HasPrefix(sourcePath, root+"/")) {
			return nil, fmt.Errorf("file %q is outside the game folder", sourceFile.Path)
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(sourcePath, root), "/")
		if relative == "" {
			relative = filepath.Base(filepath.FromSlash(sourcePath))
		}
		relative = sourcescope.NormalizeLogicalPath(relative)
		if relative == "" || relative == "." || strings.HasPrefix(relative, "../") {
			return nil, fmt.Errorf("file %q has an unsafe relative path", sourceFile.Path)
		}
		files = append(files, core.SourceMoveFile{
			SourcePath: sourcePath, RelativePath: relative, Size: max(sourceFile.Size, 0),
			ObjectID: sourceFile.ObjectID, Revision: sourceFile.Revision, Status: "pending",
		})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("this copy has no movable files")
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].RelativePath < files[j].RelativePath })
	for index := range files {
		files[index].Ordinal = index
	}
	return files, nil
}

func logicalPathHasTraversal(value string) bool {
	for _, component := range strings.Split(strings.ReplaceAll(value, `\`, "/"), "/") {
		if strings.TrimSpace(component) == ".." {
			return true
		}
	}
	return false
}

func transferFiles(files []core.SourceMoveFile) []transferFile {
	result := make([]transferFile, 0, len(files))
	for _, file := range files {
		result = append(result, transferFile{RelativePath: file.RelativePath, Size: file.Size, SHA256: file.SHA256})
	}
	return result
}

func parseIntegrationConfig(integration *core.Integration) (map[string]any, error) {
	if integration == nil {
		return nil, fmt.Errorf("connection is required")
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(integration.ConfigJSON), &config); err != nil {
		return nil, fmt.Errorf("connection settings are invalid: %w", err)
	}
	return sourcescope.NormalizeConfig(integration.PluginID, config), nil
}

func hashFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func friendlyPluginError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if strings.Contains(strings.ToLower(message), "not supported") || strings.Contains(message, "UNKNOWN_METHOD") {
		return "This connection cannot receive game files yet"
	}
	return message
}

func interruptedAfterCommit(status string) bool {
	switch strings.TrimSpace(status) {
	case core.SourceMoveStatusDestinationCommitted,
		core.SourceMoveStatusDeletingSource,
		core.SourceMoveStatusRefreshingLibrary:
		return true
	default:
		return false
	}
}

func cloneJob(job *core.SourceMoveJob) *core.SourceMoveJob {
	if job == nil {
		return nil
	}
	copyJob := *job
	copyJob.Files = append([]core.SourceMoveFile(nil), job.Files...)
	return &copyJob
}

var _ core.SourceMoveService = (*Service)(nil)
var _ core.StartupTask = (*Service)(nil)
