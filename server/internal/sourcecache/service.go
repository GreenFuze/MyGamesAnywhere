package sourcecache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/google/uuid"
)

const sourceFileMaterializeMethod = "source.file.materialize"
const sourceCacheMaterializeConcurrency = 10

type Service struct {
	store           core.SourceCacheStore
	integrationRepo core.IntegrationRepository
	pluginHost      plugins.PluginHost
	config          core.Configuration
	logger          core.Logger

	initOnce sync.Once
	initErr  error

	startMu sync.Mutex
}

func NewService(
	store core.SourceCacheStore,
	integrationRepo core.IntegrationRepository,
	pluginHost plugins.PluginHost,
	config core.Configuration,
	logger core.Logger,
) core.SourceCacheService {
	return &Service{
		store:           store,
		integrationRepo: integrationRepo,
		pluginHost:      pluginHost,
		config:          config,
		logger:          logger,
	}
}

func (s *Service) DescribeSourceGame(ctx context.Context, canonicalPlatform core.Platform, sourceGame *core.SourceGame) []core.SourceDeliveryProfile {
	if sourceGame == nil {
		return nil
	}
	profile, ok := core.BrowserPlayProfileForSourceGame(sourceGame.Platform, canonicalPlatform)
	if !ok {
		return nil
	}
	delivery := core.SourceDeliveryProfile{
		Profile: profile,
		Mode:    core.SourceDeliveryModeUnavailable,
	}
	switch {
	case supportsDirectSourceGame(sourceGame):
		delivery.Mode = core.SourceDeliveryModeDirect
		delivery.Ready = true
	case s.supportsMaterialization(sourceGame):
		delivery.Mode = core.SourceDeliveryModeMaterialized
		delivery.PrepareRequired = true
		entry, err := s.store.GetEntryBySourceProfile(ctx, sourceGame.ID, profile)
		if err == nil && entry != nil && entry.Status == "ready" {
			delivery.Ready = true
		}
	}
	if rootFile := selectRootFile(sourceGame); rootFile != nil {
		delivery.RootFilePath = rootFile.Path
	}
	return []core.SourceDeliveryProfile{delivery}
}

func (s *Service) Prepare(ctx context.Context, req core.SourceCachePrepareRequest, canonicalPlatform core.Platform, sourceGame *core.SourceGame) (*core.SourceCacheJobStatus, bool, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, false, err
	}
	if sourceGame == nil {
		return nil, false, fmt.Errorf("source game is required")
	}
	req.SourceGameID = strings.TrimSpace(req.SourceGameID)
	req.Profile = strings.TrimSpace(req.Profile)
	if req.SourceGameID == "" || req.Profile == "" {
		return nil, false, fmt.Errorf("source_game_id and profile are required")
	}
	if sourceGame.ID != req.SourceGameID {
		return nil, false, fmt.Errorf("source game mismatch")
	}
	if supportsDirectSourceGame(sourceGame) {
		now := time.Now().UTC()
		return &core.SourceCacheJobStatus{
			JobID:           "direct:" + sourceGame.ID + ":" + req.Profile,
			CanonicalGameID: req.CanonicalGameID,
			CanonicalTitle:  req.CanonicalTitle,
			SourceGameID:    sourceGame.ID,
			SourceTitle:     sourceGame.RawTitle,
			IntegrationID:   sourceGame.IntegrationID,
			PluginID:        sourceGame.PluginID,
			Profile:         req.Profile,
			Status:          "completed",
			Message:         "direct source does not require preparation",
			CreatedAt:       now,
			UpdatedAt:       now,
			FinishedAt:      &now,
		}, true, nil
	}
	if !s.supportsMaterialization(sourceGame) {
		return nil, false, fmt.Errorf("source is not materializable")
	}

	files, sourcePath, err := filesForProfile(req.Profile, sourceGame)
	if err != nil {
		return nil, false, err
	}
	cacheKey := buildCacheKey(sourceGame, req.Profile, files)

	s.startMu.Lock()
	defer s.startMu.Unlock()

	activeJob, err := s.store.FindActiveJobByCacheKey(ctx, cacheKey)
	if err != nil {
		return nil, false, err
	}
	if activeJob != nil {
		return activeJob, false, nil
	}

	existingEntry, err := s.store.GetEntryBySourceProfile(ctx, sourceGame.ID, req.Profile)
	if err != nil {
		return nil, false, err
	}
	if existingEntry != nil && existingEntry.CacheKey == cacheKey && existingEntry.Status == "ready" {
		now := time.Now().UTC()
		_ = s.store.TouchEntry(ctx, existingEntry.ID, now)
		job := &core.SourceCacheJobStatus{
			CacheKey:        cacheKey,
			CanonicalGameID: req.CanonicalGameID,
			CanonicalTitle:  req.CanonicalTitle,
			SourceGameID:    sourceGame.ID,
			SourceTitle:     sourceGame.RawTitle,
			IntegrationID:   sourceGame.IntegrationID,
			PluginID:        sourceGame.PluginID,
			Profile:         req.Profile,
			Status:          "completed",
			Message:         "cache hit",
			EntryID:         existingEntry.ID,
			ProgressCurrent: len(files),
			ProgressTotal:   len(files),
			FinishedAt:      &now,
		}
		if err := s.store.CreateJob(ctx, job); err != nil {
			return nil, false, err
		}
		return job, true, nil
	}

	entryID := ""
	if existingEntry != nil {
		entryID = existingEntry.ID
	}
	if entryID == "" {
		entryID = newEntryID()
	}

	job := &core.SourceCacheJobStatus{
		CacheKey:        cacheKey,
		CanonicalGameID: req.CanonicalGameID,
		CanonicalTitle:  req.CanonicalTitle,
		SourceGameID:    sourceGame.ID,
		SourceTitle:     sourceGame.RawTitle,
		IntegrationID:   sourceGame.IntegrationID,
		PluginID:        sourceGame.PluginID,
		Profile:         req.Profile,
		Status:          "queued",
		Message:         "queued for materialization",
		EntryID:         entryID,
		ProgressTotal:   len(files),
	}
	if err := s.store.CreateJob(ctx, job); err != nil {
		return nil, false, err
	}

	entry := &core.SourceCacheEntry{
		ID:              entryID,
		CacheKey:        cacheKey,
		CanonicalGameID: req.CanonicalGameID,
		CanonicalTitle:  req.CanonicalTitle,
		SourceGameID:    sourceGame.ID,
		SourceTitle:     sourceGame.RawTitle,
		IntegrationID:   sourceGame.IntegrationID,
		PluginID:        sourceGame.PluginID,
		Profile:         req.Profile,
		Mode:            string(core.SourceDeliveryModeMaterialized),
		Status:          "preparing",
		SourcePath:      sourcePath,
		FileCount:       len(files),
	}
	if err := s.store.UpsertEntry(ctx, entry); err != nil {
		return nil, false, err
	}

	go s.runPrepare(job, entry, req.Profile, sourceGame, files)
	return job, false, nil
}

func (s *Service) GetJob(ctx context.Context, jobID string) (*core.SourceCacheJobStatus, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	return s.store.GetJob(ctx, jobID)
}

func (s *Service) ListJobs(ctx context.Context, limit int) ([]*core.SourceCacheJobStatus, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	return s.store.ListJobs(ctx, limit)
}

func (s *Service) ListEntries(ctx context.Context) ([]*core.SourceCacheEntry, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	return s.store.ListEntries(ctx)
}

func (s *Service) DeleteEntry(ctx context.Context, entryID string) error {
	if err := s.ensureInitialized(ctx); err != nil {
		return err
	}
	entry, err := s.findEntryByID(ctx, entryID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteEntry(ctx, entryID); err != nil {
		return err
	}
	if entry != nil {
		_ = os.RemoveAll(filepath.Join(s.cacheRoot(), filepath.FromSlash(entry.ID)))
	}
	return nil
}

func (s *Service) ClearEntries(ctx context.Context) error {
	if err := s.ensureInitialized(ctx); err != nil {
		return err
	}
	entries, err := s.store.ListEntries(ctx)
	if err != nil {
		return err
	}
	root := s.cacheRoot()
	if err := s.store.ClearEntries(ctx); err != nil {
		return err
	}
	if core.ProfileIDFromContext(ctx) != "" {
		for _, entry := range entries {
			if entry != nil && entry.ID != "" {
				_ = os.RemoveAll(filepath.Join(root, filepath.FromSlash(entry.ID)))
			}
		}
		return os.MkdirAll(root, 0o755)
	}
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	return os.MkdirAll(root, 0o755)
}

func (s *Service) ResolveCachedFile(ctx context.Context, sourceGameID, profile, filePath string) (*core.SourceCacheEntry, *core.SourceCacheEntryFile, string, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, nil, "", err
	}
	entry, file, err := s.store.GetEntryFileBySourceProfile(ctx, sourceGameID, profile, filePath)
	if err != nil || entry == nil || file == nil {
		return entry, file, "", err
	}
	now := time.Now().UTC()
	_ = s.store.TouchEntry(ctx, entry.ID, now)
	entry.LastAccessedAt = &now
	return entry, file, filepath.Join(s.cacheRoot(), filepath.FromSlash(file.LocalPath)), nil
}

func (s *Service) runPrepare(job *core.SourceCacheJobStatus, entry *core.SourceCacheEntry, profile string, sourceGame *core.SourceGame, files []core.GameFile) {
	ctx := context.Background()
	job.Status = "running"
	job.Message = "materializing source files"
	if err := s.store.UpdateJob(ctx, job); err != nil {
		s.logger.Error("update cache job running", err, "job_id", job.JobID)
	}

	root := s.cacheRoot()
	entryDir := filepath.Join(root, filepath.FromSlash(entry.ID))
	if err := os.RemoveAll(entryDir); err != nil {
		s.failJob(ctx, job, entry, fmt.Errorf("clear cache entry dir: %w", err))
		return
	}
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		s.failJob(ctx, job, entry, fmt.Errorf("create cache entry dir: %w", err))
		return
	}

	integration, err := s.integrationRepo.GetByID(ctx, sourceGame.IntegrationID)
	if err != nil || integration == nil {
		if err == nil {
			err = fmt.Errorf("integration not found")
		}
		s.failJob(ctx, job, entry, err)
		return
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(integration.ConfigJSON), &config); err != nil {
		s.failJob(ctx, job, entry, fmt.Errorf("invalid integration config: %w", err))
		return
	}

	entryFiles, totalSize, err := s.materializeFiles(ctx, job, entry, profile, sourceGame, files, config)
	if err != nil {
		s.failJob(ctx, job, entry, err)
		return
	}

	now := time.Now().UTC()
	entry.Status = "ready"
	entry.FileCount = len(entryFiles)
	entry.Size = totalSize
	entry.LastAccessedAt = &now
	if err := s.store.UpsertEntry(ctx, entry); err != nil {
		s.failJob(ctx, job, entry, err)
		return
	}
	if err := s.store.ReplaceEntryFiles(ctx, entry.ID, entryFiles); err != nil {
		s.failJob(ctx, job, entry, err)
		return
	}

	job.Status = "completed"
	job.Message = "cache ready"
	job.ProgressCurrent = len(entryFiles)
	job.ProgressTotal = len(entryFiles)
	job.EntryID = entry.ID
	job.FinishedAt = &now
	if err := s.store.UpdateJob(ctx, job); err != nil {
		s.logger.Error("complete cache job", err, "job_id", job.JobID)
	}
}

type materializeFileResult struct {
	index int
	file  core.SourceCacheEntryFile
	size  int64
	err   error
}

func (s *Service) materializeFiles(
	ctx context.Context,
	job *core.SourceCacheJobStatus,
	entry *core.SourceCacheEntry,
	profile string,
	sourceGame *core.SourceGame,
	files []core.GameFile,
	config map[string]any,
) ([]core.SourceCacheEntryFile, int64, error) {
	if len(files) == 0 {
		return nil, 0, nil
	}

	workerCount := min(sourceCacheMaterializeConcurrency, len(files))
	workCh := make(chan int)
	resultCh := make(chan materializeFileResult, len(files))
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(workerCount)
	for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
		go func() {
			defer wg.Done()
			for index := range workCh {
				cacheFile, size, err := s.materializeFile(runCtx, job, entry, profile, sourceGame, files[index], config)
				resultCh <- materializeFileResult{index: index, file: cacheFile, size: size, err: err}
			}
		}()
	}
	go func() {
		defer close(workCh)
		for index := range files {
			select {
			case <-runCtx.Done():
				return
			case workCh <- index:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	entryFiles := make([]core.SourceCacheEntryFile, len(files))
	var totalSize int64
	completed := 0
	for result := range resultCh {
		if result.err != nil {
			cancel()
			return nil, 0, result.err
		}
		entryFiles[result.index] = result.file
		totalSize += result.size
		completed++
		job.ProgressCurrent = completed
		job.Message = fmt.Sprintf("materialized %d/%d files", completed, len(files))
		if err := s.store.UpdateJob(ctx, job); err != nil {
			s.logger.Error("update cache job progress", err, "job_id", job.JobID)
		}
	}

	return entryFiles, totalSize, nil
}

func (s *Service) materializeFile(
	ctx context.Context,
	job *core.SourceCacheJobStatus,
	entry *core.SourceCacheEntry,
	profile string,
	sourceGame *core.SourceGame,
	file core.GameFile,
	config map[string]any,
) (core.SourceCacheEntryFile, int64, error) {
	relativePath, err := safeCacheRelativePath(entry.ID, file.Path)
	if err != nil {
		return core.SourceCacheEntryFile{}, 0, err
	}
	fullPath := filepath.Join(s.cacheRoot(), filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return core.SourceCacheEntryFile{}, 0, fmt.Errorf("create cache file dir: %w", err)
	}
	tempPath := fullPath + ".tmp-" + job.JobID
	_ = os.Remove(tempPath)

	var result core.SourceMaterializeResult
	if err := s.pluginHost.Call(ctx, sourceGame.PluginID, sourceFileMaterializeMethod, core.SourceMaterializeRequest{
		Config:   config,
		Path:     file.Path,
		ObjectID: file.ObjectID,
		Revision: file.Revision,
		Profile:  profile,
		DestPath: tempPath,
	}, &result); err != nil {
		_ = os.Remove(tempPath)
		return core.SourceCacheEntryFile{}, 0, err
	}

	if err := os.Rename(tempPath, fullPath); err != nil {
		_ = os.Remove(tempPath)
		return core.SourceCacheEntryFile{}, 0, fmt.Errorf("rename cache file: %w", err)
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return core.SourceCacheEntryFile{}, 0, fmt.Errorf("stat cache file: %w", err)
	}

	size := info.Size()
	if result.Size > 0 {
		size = result.Size
	}

	cacheFile := core.SourceCacheEntryFile{
		EntryID:   entry.ID,
		Path:      file.Path,
		LocalPath: filepath.ToSlash(relativePath),
		ObjectID:  file.ObjectID,
		Revision:  file.Revision,
		Size:      size,
	}
	if result.Revision != "" {
		cacheFile.Revision = result.Revision
	}
	if result.ModTime != "" {
		if parsed, err := time.Parse(time.RFC3339, result.ModTime); err == nil {
			parsed = parsed.UTC()
			cacheFile.ModifiedAt = &parsed
		}
	} else if file.ModifiedAt != nil {
		cacheFile.ModifiedAt = file.ModifiedAt
	}
	return cacheFile, size, nil
}

func (s *Service) failJob(ctx context.Context, job *core.SourceCacheJobStatus, entry *core.SourceCacheEntry, err error) {
	s.logger.Error("cache prepare failed", err, "job_id", job.JobID, "source_game_id", job.SourceGameID, "profile", job.Profile)
	if entry != nil {
		entry.Status = "failed"
		_ = s.store.UpsertEntry(ctx, entry)
		_ = s.store.ReplaceEntryFiles(ctx, entry.ID, nil)
		_ = os.RemoveAll(filepath.Join(s.cacheRoot(), filepath.FromSlash(entry.ID)))
	}
	now := time.Now().UTC()
	job.Status = "failed"
	job.Error = err.Error()
	job.Message = err.Error()
	job.FinishedAt = &now
	_ = s.store.UpdateJob(ctx, job)
}

func (s *Service) ensureInitialized(ctx context.Context) error {
	s.initOnce.Do(func() {
		s.initErr = s.store.MarkInFlightJobsInterrupted(ctx)
		if s.initErr == nil {
			s.initErr = os.MkdirAll(s.cacheRoot(), 0o755)
		}
	})
	return s.initErr
}

func (s *Service) supportsMaterialization(sourceGame *core.SourceGame) bool {
	if sourceGame == nil {
		return false
	}
	plugin, ok := s.pluginHost.GetPlugin(sourceGame.PluginID)
	if !ok || plugin == nil {
		return false
	}
	for _, provided := range plugin.Manifest.Provides {
		if provided == sourceFileMaterializeMethod {
			return true
		}
	}
	return false
}

func (s *Service) cacheRoot() string {
	root := strings.TrimSpace(s.config.Get("SOURCE_CACHE_ROOT"))
	if root != "" {
		if filepath.IsAbs(root) {
			return root
		}
		if wd, err := os.Getwd(); err == nil {
			return filepath.Join(wd, root)
		}
		return root
	}
	if dir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, "MyGamesAnywhere", "source-cache")
	}
	return filepath.Join(".", "source-cache")
}

func (s *Service) findEntryByID(ctx context.Context, entryID string) (*core.SourceCacheEntry, error) {
	entries, err := s.store.ListEntries(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry != nil && entry.ID == entryID {
			return entry, nil
		}
	}
	return nil, nil
}

func supportsDirectSourceGame(sourceGame *core.SourceGame) bool {
	if sourceGame == nil {
		return false
	}
	rootPath := strings.TrimSpace(sourceGame.RootPath)
	return rootPath != "" && filepath.IsAbs(rootPath)
}

func selectRootFile(sourceGame *core.SourceGame) *core.GameFile {
	if sourceGame == nil {
		return nil
	}
	for i := range sourceGame.Files {
		if sourceGame.Files[i].Role == core.GameFileRoleRoot {
			return &sourceGame.Files[i]
		}
	}
	return nil
}

func filesForProfile(profile string, sourceGame *core.SourceGame) ([]core.GameFile, string, error) {
	if sourceGame == nil {
		return nil, "", fmt.Errorf("source game is required")
	}
	nonDirs := make([]core.GameFile, 0, len(sourceGame.Files))
	for _, file := range sourceGame.Files {
		if !file.IsDir {
			nonDirs = append(nonDirs, file)
		}
	}
	switch profile {
	case core.BrowserProfileEmulatorJS:
		rootFile := selectRootFile(sourceGame)
		if rootFile == nil {
			return nil, "", fmt.Errorf("profile %s requires a root file", profile)
		}
		return []core.GameFile{*rootFile}, rootFile.Path, nil
	case core.BrowserProfileJSDOS:
		rootFile := selectRootFile(sourceGame)
		if rootFile == nil {
			return nil, "", fmt.Errorf("profile %s requires a root file", profile)
		}
		lowerPath := strings.ToLower(rootFile.Path)
		if strings.HasSuffix(lowerPath, ".jsdos") || strings.HasSuffix(lowerPath, ".zip") {
			return []core.GameFile{*rootFile}, rootFile.Path, nil
		}
		if len(nonDirs) == 0 {
			return nil, "", fmt.Errorf("profile %s requires source files", profile)
		}
		return nonDirs, rootFile.Path, nil
	case core.BrowserProfileScummVM:
		if len(nonDirs) == 0 {
			return nil, "", fmt.Errorf("profile %s requires source files", profile)
		}
		return nonDirs, commonDirectoryPath(nonDirs), nil
	default:
		return nil, "", fmt.Errorf("unsupported profile %q", profile)
	}
}

func buildCacheKey(sourceGame *core.SourceGame, profile string, files []core.GameFile) string {
	parts := []string{
		"integration=" + sourceGame.IntegrationID,
		"plugin=" + sourceGame.PluginID,
		"profile=" + profile,
	}
	fileParts := make([]string, 0, len(files))
	for _, file := range files {
		modified := ""
		if file.ModifiedAt != nil {
			modified = file.ModifiedAt.UTC().Format(time.RFC3339Nano)
		}
		fileParts = append(fileParts, strings.Join([]string{
			file.Path,
			file.ObjectID,
			file.Revision,
			modified,
			fmt.Sprintf("%d", file.Size),
		}, "|"))
	}
	sort.Strings(fileParts)
	parts = append(parts, fileParts...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func safeCacheRelativePath(entryID, filePath string) (string, error) {
	cleaned := strings.ReplaceAll(strings.TrimSpace(filePath), "\\", "/")
	cleaned = strings.TrimPrefix(cleaned, "./")
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = strings.ReplaceAll(cleaned, ":", "_")
	cleaned = path.Clean(cleaned)
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("invalid cache file path %q", filePath)
	}
	return path.Join(entryID, cleaned), nil
}

func commonDirectoryPath(files []core.GameFile) string {
	if len(files) == 0 {
		return ""
	}
	parts := make([][]string, 0, len(files))
	for _, file := range files {
		dir := path.Dir(strings.ReplaceAll(file.Path, "\\", "/"))
		if dir == "." {
			dir = ""
		}
		parts = append(parts, strings.Split(strings.Trim(dir, "/"), "/"))
	}
	prefix := append([]string(nil), parts[0]...)
	for _, current := range parts[1:] {
		shared := 0
		for shared < len(prefix) && shared < len(current) && prefix[shared] == current[shared] {
			shared++
		}
		prefix = prefix[:shared]
		if len(prefix) == 0 {
			break
		}
	}
	return strings.Join(prefix, "/")
}

func newEntryID() string {
	return uuid.NewString()
}
