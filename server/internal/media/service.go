package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const (
	defaultMediaRoot            = "media"
	defaultDownloadConcurrency  = 4
	defaultPendingFetchLimit    = 100000
	defaultQueueBufferPerWorker = 4
	defaultBusyRetryAttempts    = 8
	defaultBusyRetryDelay       = 100 * time.Millisecond
	defaultBrowserUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

type mediaDownloadError struct {
	statusCode int
	message    string
}

func (e mediaDownloadError) Error() string {
	return e.message
}

func (e mediaDownloadError) Permanent() bool {
	return e.statusCode == http.StatusNotFound || e.statusCode == http.StatusGone
}

type Service struct {
	store  core.GameStore
	config core.Configuration
	logger core.Logger
	client *http.Client
	policy mediaRequestPolicy

	workerCount int

	startOnce sync.Once
	startErr  error

	mu       sync.Mutex
	started  bool
	queue    chan *core.MediaAsset
	inFlight map[int]struct{}
}

func NewService(store core.GameStore, config core.Configuration, logger core.Logger) core.MediaDownloadService {
	workerCount := config.GetInt("MEDIA_DOWNLOAD_CONCURRENCY")
	if workerCount <= 0 {
		workerCount = defaultDownloadConcurrency
	}
	return &Service{
		store:       store,
		config:      config,
		logger:      logger,
		client:      &http.Client{Timeout: 2 * time.Minute},
		policy:      mediaRequestPolicyRegistry{policies: []mediaRequestPolicyMatcher{hltbImageRequestPolicy{}, retroAchievementsImageRequestPolicy{}}},
		workerCount: workerCount,
		inFlight:    make(map[int]struct{}),
	}
}

func (s *Service) Start(ctx context.Context) error {
	s.startOnce.Do(func() {
		rootAbs, err := mediaRootAbs(s.config)
		if err != nil {
			s.startErr = err
			return
		}
		if err := os.MkdirAll(rootAbs, 0o755); err != nil {
			s.startErr = fmt.Errorf("create media root: %w", err)
			return
		}

		s.mu.Lock()
		s.queue = make(chan *core.MediaAsset, s.workerCount*defaultQueueBufferPerWorker)
		s.started = true
		s.mu.Unlock()

		for i := 0; i < s.workerCount; i++ {
			go s.worker(ctx, rootAbs)
		}
		go func() {
			if err := s.EnqueuePending(ctx); err != nil && ctx.Err() == nil {
				s.logger.Warn("media downloader startup enqueue failed", "error", err)
			}
		}()
	})
	return s.startErr
}

func (s *Service) EnqueuePending(ctx context.Context) error {
	assets, err := s.store.GetPendingMediaDownloads(ctx, defaultPendingFetchLimit)
	if err != nil {
		return err
	}
	for _, asset := range assets {
		if err := s.enqueueAsset(ctx, asset); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Status(ctx context.Context) (*core.MediaDownloadStatus, error) {
	status, err := s.store.GetMediaDownloadStatus(ctx)
	if err != nil {
		return nil, err
	}
	queued, downloading := s.queueStats()
	status.Queued = queued
	status.Downloading = downloading
	return status, nil
}

func (s *Service) RetryFailed(ctx context.Context) (*core.MediaDownloadStatus, error) {
	if err := s.store.ResetRetryableMediaDownloadFailures(ctx); err != nil {
		return nil, err
	}
	s.enqueuePendingAsync("retry failed media")
	return s.Status(ctx)
}

func (s *Service) ClearCache(ctx context.Context) (*core.MediaDownloadStatus, error) {
	rootAbs, err := mediaRootAbs(s.config)
	if err != nil {
		return nil, fmt.Errorf("resolve media root: %w", err)
	}
	if err := clearMediaRootContents(rootAbs); err != nil {
		return nil, err
	}
	if err := s.store.ClearMediaDownloadState(ctx); err != nil {
		return nil, err
	}
	s.enqueuePendingAsync("clear media cache")
	return s.Status(ctx)
}

func (s *Service) MarkLocalFileMissing(ctx context.Context, assetID int) error {
	if assetID <= 0 {
		return fmt.Errorf("assetID must be positive")
	}
	if err := s.store.ResetMediaAssetDownloadState(ctx, assetID); err != nil {
		return err
	}
	s.forgetInFlight(assetID)
	s.enqueuePendingAsync("missing local media")
	return nil
}

func (s *Service) queueStats() (queued int, downloading int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.queue != nil {
		queued = len(s.queue)
	}
	inFlight := len(s.inFlight)
	downloading = inFlight - queued
	if downloading < 0 {
		downloading = 0
	}
	return queued, downloading
}

func (s *Service) enqueuePendingAsync(reason string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.EnqueuePending(ctx); err != nil {
			s.logger.Warn("media downloader enqueue failed", "reason", reason, "error", err)
		}
	}()
}

func (s *Service) enqueueAsset(ctx context.Context, asset *core.MediaAsset) error {
	if asset == nil || asset.ID <= 0 || strings.TrimSpace(asset.LocalPath) != "" {
		return nil
	}

	s.mu.Lock()
	if !s.started || s.queue == nil {
		s.mu.Unlock()
		return fmt.Errorf("media downloader not started")
	}
	if _, exists := s.inFlight[asset.ID]; exists {
		s.mu.Unlock()
		return nil
	}
	s.inFlight[asset.ID] = struct{}{}
	queue := s.queue
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		s.forgetInFlight(asset.ID)
		return ctx.Err()
	case queue <- asset:
		return nil
	}
}

func (s *Service) worker(ctx context.Context, rootAbs string) {
	for {
		select {
		case <-ctx.Done():
			return
		case asset := <-s.queue:
			if asset == nil {
				continue
			}
			if err := s.downloadAsset(ctx, rootAbs, asset); err != nil && ctx.Err() == nil {
				if markErr := retrySQLiteBusy(ctx, func() error {
					return s.store.MarkMediaAssetDownloadFailed(ctx, asset.ID, err.Error(), permanentMediaDownloadFailure(err))
				}); markErr != nil {
					s.logger.Warn("record media download failure failed", "asset_id", asset.ID, "url", asset.URL, "error", markErr)
				}
				s.logger.Warn("media download failed", "asset_id", asset.ID, "url", asset.URL, "error", err)
			}
			s.forgetInFlight(asset.ID)
		}
	}
}

func (s *Service) forgetInFlight(assetID int) {
	s.mu.Lock()
	delete(s.inFlight, assetID)
	s.mu.Unlock()
}

func (s *Service) downloadAsset(ctx context.Context, rootAbs string, asset *core.MediaAsset) error {
	if asset == nil {
		return fmt.Errorf("asset is required")
	}
	parsed, err := url.Parse(strings.TrimSpace(asset.URL))
	if err != nil {
		return fmt.Errorf("parse asset url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported asset url scheme %q", parsed.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if err := s.policy.Apply(req, parsed); err != nil {
		return fmt.Errorf("configure request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("request asset: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mediaDownloadError{
			statusCode: resp.StatusCode,
			message:    fmt.Sprintf("unexpected status %d", resp.StatusCode),
		}
	}

	relPath := buildMediaRelativePath(asset, resp.Header.Get("Content-Type"))
	finalAbs := filepath.Join(rootAbs, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(finalAbs), 0o755); err != nil {
		return fmt.Errorf("create asset directory: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(finalAbs), "media-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tempFile, hasher), resp.Body); err != nil {
		tempFile.Close()
		return fmt.Errorf("write asset: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if _, err := os.Stat(finalAbs); err == nil {
		if removeErr := os.Remove(finalAbs); removeErr != nil {
			return fmt.Errorf("remove stale asset: %w", removeErr)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat target asset: %w", err)
	}

	if err := os.Rename(tempPath, finalAbs); err != nil {
		return fmt.Errorf("commit asset: %w", err)
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	if err := retrySQLiteBusy(ctx, func() error {
		return s.store.UpdateMediaAsset(ctx, asset.ID, filepath.ToSlash(relPath), hash)
	}); err != nil {
		return fmt.Errorf("update media asset row: %w", err)
	}
	return nil
}

func permanentMediaDownloadFailure(err error) bool {
	var downloadErr mediaDownloadError
	return errors.As(err, &downloadErr) && downloadErr.Permanent()
}

type mediaRequestPolicy interface {
	Apply(req *http.Request, parsed *url.URL) error
}

type mediaRequestPolicyMatcher interface {
	Matches(parsed *url.URL) bool
	Apply(req *http.Request) error
}

type mediaRequestPolicyRegistry struct {
	policies []mediaRequestPolicyMatcher
}

func (r mediaRequestPolicyRegistry) Apply(req *http.Request, parsed *url.URL) error {
	if req == nil || parsed == nil {
		return fmt.Errorf("request and parsed url are required")
	}
	for _, policy := range r.policies {
		if policy.Matches(parsed) {
			return policy.Apply(req)
		}
	}
	return nil
}

type hltbImageRequestPolicy struct{}

func (hltbImageRequestPolicy) Matches(parsed *url.URL) bool {
	return parsed != nil && normalizeMediaHost(parsed.Hostname()) == "howlongtobeat.com"
}

func (hltbImageRequestPolicy) Apply(req *http.Request) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Referer", "https://howlongtobeat.com/")
	req.Header.Set("User-Agent", defaultBrowserUserAgent)
	return nil
}

type retroAchievementsImageRequestPolicy struct{}

func (retroAchievementsImageRequestPolicy) Matches(parsed *url.URL) bool {
	return parsed != nil && normalizeMediaHost(parsed.Hostname()) == "retroachievements.org" && strings.HasPrefix(parsed.EscapedPath(), "/Images/")
}

func (retroAchievementsImageRequestPolicy) Apply(req *http.Request) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://retroachievements.org/")
	req.Header.Set("User-Agent", defaultBrowserUserAgent)
	return nil
}

func normalizeMediaHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimPrefix(host, "www.")
	return host
}

func retrySQLiteBusy(ctx context.Context, op func() error) error {
	if op == nil {
		return fmt.Errorf("retry op is required")
	}
	delay := defaultBusyRetryDelay
	var lastErr error
	for attempt := 0; attempt < defaultBusyRetryAttempts; attempt++ {
		err := op()
		if err == nil {
			return nil
		}
		if !isSQLiteBusyError(err) {
			return err
		}
		lastErr = err
		if attempt == defaultBusyRetryAttempts-1 {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if delay < time.Second {
			delay *= 2
		}
	}
	return lastErr
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "sqlite_busy")
}

func mediaRootAbs(config core.Configuration) (string, error) {
	root := config.Get("MEDIA_ROOT")
	if strings.TrimSpace(root) == "" {
		root = defaultMediaRoot
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root), nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(wd, root))
}

func clearMediaRootContents(rootAbs string) error {
	root := filepath.Clean(strings.TrimSpace(rootAbs))
	if root == "" || root == string(filepath.Separator) || filepath.VolumeName(root) == root || filepath.Dir(root) == root {
		return fmt.Errorf("unsafe media root %q", rootAbs)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create media root: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read media root: %w", err)
	}
	for _, entry := range entries {
		full := filepath.Join(root, entry.Name())
		if err := os.RemoveAll(full); err != nil {
			return fmt.Errorf("remove media cache entry %s: %w", full, err)
		}
	}
	return nil
}

func buildMediaRelativePath(asset *core.MediaAsset, contentType string) string {
	ext := mediaExtension(asset, contentType)
	return path.Join("assets", strconv.Itoa(asset.ID)+ext)
}

func mediaExtension(asset *core.MediaAsset, contentType string) string {
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && mediaType != "" {
		if exts, err := mime.ExtensionsByType(mediaType); err == nil && len(exts) > 0 {
			return exts[0]
		}
	}
	if asset != nil {
		if parsed, err := url.Parse(asset.URL); err == nil {
			if ext := path.Ext(parsed.Path); ext != "" && len(ext) <= 10 {
				return ext
			}
		}
	}
	return ".bin"
}
