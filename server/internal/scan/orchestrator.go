package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan/scanner"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
)

const (
	sourceFilesystemListMethod    = "source.filesystem.list"
	sourceGamesListMethod         = "source.games.list"
	maxScanPreparationConcurrency = 2
)

// PluginCaller is the subset of PluginHost that the orchestrator needs.
type PluginCaller interface {
	Call(ctx context.Context, pluginID string, method string, params any, result any) error
}

// PluginDiscovery lets the orchestrator find which plugins provide which methods.
type PluginDiscovery interface {
	GetPluginIDs() []string
	GetPlugin(pluginID string) (*core.Plugin, bool)
	GetPluginIDsProviding(method string) []string
}

// Orchestrator coordinates a game scan: it fetches files from each
// source plugin, runs the scanner pipeline, enriches with metadata,
// and persists results via GameStore.
type Orchestrator struct {
	pluginCaller       PluginCaller
	pluginDiscovery    PluginDiscovery
	integrationRepo    core.IntegrationRepository
	gameStore          core.GameStore
	mediaDownloadQueue core.MediaDownloadQueue
	scanner            *scanner.Scanner
	metadataResolver   *MetadataResolver
	refreshCoordinator *metadataRefreshCoordinator
	logger             core.Logger
	eventBus           *events.EventBus
}

type preparedScanIntegration struct {
	index           int
	integration     *core.Integration
	config          map[string]any
	games           []*core.Game
	result          core.ScanIntegrationResult
	skipped         bool
	filesystemScope *core.FilesystemScanScope
}

func NewOrchestrator(
	caller PluginCaller,
	discovery PluginDiscovery,
	integrationRepo core.IntegrationRepository,
	gameStore core.GameStore,
	mediaDownloadQueue core.MediaDownloadQueue,
	logger core.Logger,
) *Orchestrator {
	resolver := NewMetadataResolver(caller, logger)
	return &Orchestrator{
		pluginCaller:       caller,
		pluginDiscovery:    discovery,
		integrationRepo:    integrationRepo,
		gameStore:          gameStore,
		mediaDownloadQueue: mediaDownloadQueue,
		scanner:            scanner.New(logger),
		metadataResolver:   resolver,
		refreshCoordinator: newMetadataRefreshCoordinator(gameStore, mediaDownloadQueue, resolver, logger),
		logger:             logger,
	}
}

// SetEventBus attaches an optional event bus for scan progress SSE.
func (o *Orchestrator) SetEventBus(bus *events.EventBus) {
	o.eventBus = bus
	if bus == nil {
		o.metadataResolver.SetScanEventPublisher(nil)
		o.scanner.SetProgressReporter(nil)
		return
	}
	o.metadataResolver.SetScanEventPublisher(func(ctx context.Context, typ string, payload any) {
		o.publishEventWithContext(ctx, typ, payload)
	})
	o.scanner.SetProgressReporter(nil)
}

func (o *Orchestrator) publishEvent(eventType string, payload any) {
	o.publishEventWithContext(context.Background(), eventType, payload)
}

func (o *Orchestrator) publishEventWithContext(ctx context.Context, eventType string, payload any) {
	if o.eventBus == nil {
		return
	}
	if m, ok := payload.(map[string]any); ok {
		if jobID, ok := ScanJobIDFromContext(ctx); ok {
			if _, exists := m["job_id"]; !exists {
				m["job_id"] = jobID
			}
		}
		events.PublishJSON(o.eventBus, eventType, m)
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		o.logger.Warn("orchestrator: event marshal failed", "error", err)
		return
	}
	o.eventBus.Publish(events.Event{Type: eventType, Data: data})
}

func (o *Orchestrator) publishScanError(ctx context.Context, integrationID string, err error) {
	if err == nil {
		return
	}
	m := map[string]any{"error": err.Error()}
	if integrationID != "" {
		m["integration_id"] = integrationID
	}
	o.publishEventWithContext(ctx, "scan_error", m)
}

func buildScanIntegrationPayload(integrations []*core.Integration) []map[string]any {
	out := make([]map[string]any, 0, len(integrations))
	for _, integ := range integrations {
		out = append(out, map[string]any{
			"integration_id": integ.ID,
			"plugin_id":      integ.PluginID,
			"label":          integ.Label,
		})
	}
	return out
}

func scanSkipReasonForSourceError(err error) string {
	if err == nil {
		return "source_error"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "plugin error [NOT_CONFIGURED]"):
		return "invalid_config"
	case strings.Contains(msg, "plugin error [AUTH_REQUIRED]"):
		return "auth_required"
	default:
		return "source_error"
	}
}

func pluginProvidesSource(plugin *core.Plugin) bool {
	if plugin == nil {
		return false
	}
	return pluginProvides(plugin, sourceFilesystemListMethod) || pluginProvides(plugin, sourceGamesListMethod)
}

func isSourceIntegrationCandidate(discovery PluginDiscovery, integration *core.Integration) bool {
	if integration == nil {
		return false
	}
	if integration.IntegrationType == "source" {
		return true
	}
	plugin, ok := discovery.GetPlugin(integration.PluginID)
	return ok && pluginProvidesSource(plugin)
}

func filterSourceIntegrations(discovery PluginDiscovery, integrations []*core.Integration, filter map[string]bool) []*core.Integration {
	out := make([]*core.Integration, 0, len(integrations))
	for _, integ := range integrations {
		if len(filter) > 0 && !filter[integ.ID] {
			continue
		}
		if !isSourceIntegrationCandidate(discovery, integ) {
			continue
		}
		out = append(out, integ)
	}
	return out
}

// RunScan scans all (or selected) integrations, enriches metadata,
// persists via GameStore, and returns the canonical game views.
func (o *Orchestrator) RunScan(ctx context.Context, integrationIDs []string) ([]*core.CanonicalGame, error) {
	o.logger.Info("orchestrator: starting scan", "requested", len(integrationIDs))

	integrations, err := o.integrationRepo.List(ctx)
	if err != nil {
		o.publishScanError(ctx, "", err)
		return nil, fmt.Errorf("list integrations: %w", err)
	}

	filter := make(map[string]bool, len(integrationIDs))
	for _, id := range integrationIDs {
		filter[id] = true
	}
	filteredIntegrations := filterSourceIntegrations(o.pluginDiscovery, integrations, filter)
	o.publishEventWithContext(ctx, "scan_started", map[string]any{
		"integration_count": len(filteredIntegrations),
		"metadata_only":     false,
		"integrations":      buildScanIntegrationPayload(filteredIntegrations),
	})

	scanStart := time.Now()

	// Snapshot pre-scan game counts for diff computation.
	preCounts, _ := o.gameStore.GetSourceGameCountsByIntegration(ctx)

	// Build integration label map for the report.
	integLabelMap := make(map[string]string, len(integrations))
	integPluginMap := make(map[string]string, len(integrations))
	for _, integ := range integrations {
		integLabelMap[integ.ID] = integ.Label
		integPluginMap[integ.ID] = integ.PluginID
	}

	metaSources, metadataProviders := o.findMetadataSources(integrations)

	// Track per-integration results for the report.
	var integResults []core.ScanIntegrationResult
	var scannedIDs []string

	prepared, err := o.prepareScanIntegrations(ctx, filteredIntegrations, metaSources, metadataProviders)
	if err != nil {
		return nil, err
	}

	for _, item := range prepared {
		if item == nil {
			continue
		}
		if item.skipped {
			integResults = append(integResults, item.result)
			continue
		}

		scannedIDs = append(scannedIDs, item.integration.ID)

		o.publishEventWithContext(ctx, "scan_persist_started", map[string]any{
			"integration_id":    item.integration.ID,
			"plugin_id":         item.integration.PluginID,
			"source_game_count": len(item.games),
		})
		batch := gamesToScanBatch(item.integration.ID, item.integration.PluginID, item.games)
		if item.filesystemScope != nil {
			batch.FilesystemScope = item.filesystemScope
		}
		if err := o.gameStore.PersistScanResults(ctx, batch); err != nil {
			o.publishScanError(ctx, item.integration.ID, err)
			return nil, fmt.Errorf("persist scan results for integration %q: %w", item.integration.ID, err)
		}
		if o.mediaDownloadQueue != nil {
			if err := o.mediaDownloadQueue.EnqueuePending(ctx); err != nil {
				o.logger.Warn("orchestrator: enqueue pending media downloads failed", "integration_id", item.integration.ID, "error", err)
			}
		}
		o.logger.Info("orchestrator: persisted", "integration_id", item.integration.ID, "source_games", len(batch.SourceGames))
		o.publishEventWithContext(ctx, "scan_integration_complete", map[string]any{
			"integration_id": item.integration.ID,
			"plugin_id":      item.integration.PluginID,
			"label":          item.integration.Label,
			"games_found":    len(batch.SourceGames),
		})

		item.result.GamesFound = len(batch.SourceGames)
		integResults = append(integResults, item.result)
	}

	// Return the canonical game views.
	result, err := o.gameStore.GetCanonicalGames(ctx)
	if err != nil {
		o.publishScanError(ctx, "", err)
		return nil, fmt.Errorf("get canonical games: %w", err)
	}

	// Compute diff and build scan report.
	postCounts, _ := o.gameStore.GetSourceGameCountsByIntegration(ctx)
	report := o.buildScanReport(scanStart, false, scannedIDs, preCounts, postCounts, integResults, len(result))
	if saveErr := o.gameStore.SaveScanReport(ctx, report); saveErr != nil {
		o.logger.Warn("orchestrator: failed to save scan report", "error", saveErr)
	}

	o.logger.Info("orchestrator: scan complete", "canonical_games", len(result))
	o.publishEventWithContext(ctx, "scan_complete", map[string]any{
		"canonical_games": len(result),
		"duration_ms":     report.DurationMs,
		"report_id":       report.ID,
		"games_added":     report.GamesAdded,
		"games_removed":   report.GamesRemoved,
	})
	return result, nil
}

// RunMetadataRefresh re-enriches existing source games without re-discovering.
// It loads found source games from the DB, groups them by integration, runs the
// metadata enrichment pipeline, and persists the updated resolver matches.
func (o *Orchestrator) RunMetadataRefresh(ctx context.Context, integrationIDs []string) ([]*core.CanonicalGame, error) {
	o.logger.Info("orchestrator: starting metadata refresh", "requested", len(integrationIDs))

	integrations, err := o.integrationRepo.List(ctx)
	if err != nil {
		o.publishScanError(ctx, "", err)
		return nil, fmt.Errorf("list integrations: %w", err)
	}

	metaSources, metadataProviders := o.findMetadataSources(integrations)
	if len(metaSources) == 0 {
		return nil, core.ErrMetadataProvidersUnavailable
	}

	// Load existing found source games from the DB.
	foundGames, err := o.gameStore.GetFoundSourceGameRecords(ctx, integrationIDs)
	if err != nil {
		o.publishScanError(ctx, "", err)
		return nil, fmt.Errorf("get found source games: %w", err)
	}

	// Group by integration ID.
	byIntegration := make(map[string][]*core.SourceGame)
	for _, sg := range foundGames {
		byIntegration[sg.IntegrationID] = append(byIntegration[sg.IntegrationID], sg)
	}

	scanStart := time.Now()
	activeIntegrations := make([]*core.Integration, 0, len(byIntegration))
	for _, integ := range integrations {
		if !isSourceIntegrationCandidate(o.pluginDiscovery, integ) {
			continue
		}
		if len(byIntegration[integ.ID]) == 0 {
			continue
		}
		activeIntegrations = append(activeIntegrations, integ)
	}
	o.publishEventWithContext(ctx, "scan_started", map[string]any{
		"integration_count": len(activeIntegrations),
		"metadata_only":     true,
		"integrations":      buildScanIntegrationPayload(activeIntegrations),
	})

	for _, integ := range activeIntegrations {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		integrationID := integ.ID
		sourceGames := byIntegration[integrationID]

		o.publishEventWithContext(ctx, "scan_integration_started", map[string]any{
			"integration_id": integrationID,
			"plugin_id":      integ.PluginID,
			"label":          integ.Label,
		})

		o.publishEventWithContext(ctx, "scan_metadata_started", map[string]any{
			"integration_id":     integrationID,
			"plugin_id":          integ.PluginID,
			"game_count":         len(sourceGames),
			"resolver_count":     len(metaSources),
			"metadata_providers": metadataProviders,
		})

		if _, err := o.refreshCoordinator.refreshExistingSourceGames(ctx, integrationID, sourceGames, metaSources); err != nil {
			o.publishScanError(ctx, integrationID, err)
			return nil, fmt.Errorf("persist metadata refresh for %q: %w", integrationID, err)
		}

		o.publishEventWithContext(ctx, "scan_integration_complete", map[string]any{
			"integration_id": integrationID,
			"plugin_id":      integ.PluginID,
			"label":          integ.Label,
			"games_found":    len(sourceGames),
		})
	}

	result, err := o.gameStore.GetCanonicalGames(ctx)
	if err != nil {
		o.publishScanError(ctx, "", err)
		return nil, fmt.Errorf("get canonical games: %w", err)
	}

	// Build a lightweight scan report for metadata-only refresh.
	var scannedIDs []string
	for _, integ := range activeIntegrations {
		scannedIDs = append(scannedIDs, integ.ID)
	}
	report := o.buildScanReport(scanStart, true, scannedIDs, nil, nil, nil, len(result))
	if saveErr := o.gameStore.SaveScanReport(ctx, report); saveErr != nil {
		o.logger.Warn("orchestrator: failed to save scan report", "error", saveErr)
	}

	o.logger.Info("orchestrator: metadata refresh complete", "canonical_games", len(result))
	o.publishEventWithContext(ctx, "scan_complete", map[string]any{
		"canonical_games": len(result),
		"duration_ms":     report.DurationMs,
		"metadata_only":   true,
		"report_id":       report.ID,
	})
	return result, nil
}

func (o *Orchestrator) RefreshGameMetadata(ctx context.Context, canonicalID string) (*core.CanonicalGame, error) {
	if canonicalID == "" {
		return nil, nil
	}

	current, err := o.gameStore.GetCanonicalGameByID(ctx, canonicalID)
	if err != nil {
		return nil, fmt.Errorf("get canonical game: %w", err)
	}
	if current == nil {
		return nil, nil
	}

	sourceGames, err := o.gameStore.GetSourceGamesForCanonical(ctx, canonicalID)
	if err != nil {
		return nil, fmt.Errorf("get source games for canonical: %w", err)
	}
	if len(sourceGames) == 0 {
		return nil, core.ErrMetadataRefreshNoEligible
	}

	integrations, err := o.integrationRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}

	metaSources, _ := o.findMetadataSources(integrations)
	if len(metaSources) == 0 {
		return nil, core.ErrMetadataProvidersUnavailable
	}

	grouped := make(map[string][]*core.SourceGame)
	for _, sourceGame := range sourceGames {
		if sourceGame == nil {
			continue
		}
		grouped[sourceGame.IntegrationID] = append(grouped[sourceGame.IntegrationID], sourceGame)
	}
	if len(grouped) == 0 {
		return nil, core.ErrMetadataRefreshNoEligible
	}

	for integrationID, records := range grouped {
		if _, err := o.refreshCoordinator.refreshExistingSourceGames(ctx, integrationID, records, metaSources); err != nil {
			return nil, fmt.Errorf("refresh source records for %q: %w", integrationID, err)
		}
	}

	return o.gameStore.GetCanonicalGameByID(ctx, canonicalID)
}

// buildScanReport computes diff between pre- and post-scan game counts and
// produces a ScanReport. For metadata-only refreshes, pre/post counts may be nil.
func (o *Orchestrator) buildScanReport(
	scanStart time.Time,
	metadataOnly bool,
	integrationIDs []string,
	preCounts, postCounts map[string]int,
	integResults []core.ScanIntegrationResult,
	totalCanonical int,
) *core.ScanReport {
	now := time.Now()
	reportID := fmt.Sprintf("scan:%s", now.Format("20060102T150405"))

	// Compute per-integration diff if counts are available.
	totalAdded, totalRemoved := 0, 0
	if preCounts != nil && postCounts != nil {
		for i := range integResults {
			r := &integResults[i]
			pre := preCounts[r.IntegrationID]
			post := postCounts[r.IntegrationID]
			if post > pre {
				r.GamesAdded = post - pre
				totalAdded += r.GamesAdded
			} else if pre > post {
				r.GamesRemoved = pre - post
				totalRemoved += r.GamesRemoved
			}
		}
	}

	return &core.ScanReport{
		ID:             reportID,
		StartedAt:      scanStart,
		FinishedAt:     now,
		DurationMs:     now.Sub(scanStart).Milliseconds(),
		MetadataOnly:   metadataOnly,
		IntegrationIDs: integrationIDs,
		GamesAdded:     totalAdded,
		GamesRemoved:   totalRemoved,
		TotalGames:     totalCanonical,
		Results:        integResults,
	}
}

// fetchFiles calls source.filesystem.list on the plugin and parses the response.
func (o *Orchestrator) fetchFiles(ctx context.Context, pluginID string, config map[string]any) ([]core.FileEntry, error) {
	var result struct {
		Files []struct {
			Path     string `json:"path"`
			Name     string `json:"name"`
			IsDir    bool   `json:"is_dir"`
			Size     int64  `json:"size"`
			ModTime  string `json:"mod_time"`
			ObjectID string `json:"object_id"`
			Revision string `json:"revision"`
		} `json:"files"`
	}
	if err := o.pluginCaller.Call(ctx, pluginID, sourceFilesystemListMethod, config, &result); err != nil {
		return nil, err
	}

	entries := make([]core.FileEntry, 0, len(result.Files))
	for _, f := range result.Files {
		var modTime time.Time
		if f.ModTime != "" {
			modTime, _ = time.Parse(time.RFC3339, f.ModTime)
		}
		entries = append(entries, core.FileEntry{
			Path:     f.Path,
			Name:     f.Name,
			IsDir:    f.IsDir,
			Size:     f.Size,
			ModTime:  modTime,
			ObjectID: f.ObjectID,
			Revision: f.Revision,
		})
	}
	return entries, nil
}

func (o *Orchestrator) prepareScanIntegrations(
	ctx context.Context,
	integrations []*core.Integration,
	metaSources []MetadataSource,
	metadataProviders []map[string]any,
) ([]*preparedScanIntegration, error) {
	if len(integrations) == 0 {
		return nil, nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	prepared := make([]*preparedScanIntegration, len(integrations))
	sem := make(chan struct{}, scanPreparationConcurrency(len(integrations)))
	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	for index, integration := range integrations {
		idx := index
		integ := integration
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-runCtx.Done():
				return
			}
			defer func() { <-sem }()

			item, err := o.prepareScanIntegration(runCtx, idx, integ, metaSources, metadataProviders)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				cancel()
				return
			}
			prepared[idx] = item
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}
	return prepared, nil
}

func (o *Orchestrator) prepareScanIntegration(
	ctx context.Context,
	index int,
	integ *core.Integration,
	metaSources []MetadataSource,
	metadataProviders []map[string]any,
) (*preparedScanIntegration, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	item := &preparedScanIntegration{
		index:       index,
		integration: integ,
		result: core.ScanIntegrationResult{
			IntegrationID: integ.ID,
			Label:         integ.Label,
			PluginID:      integ.PluginID,
		},
	}

	plugin, ok := o.pluginDiscovery.GetPlugin(integ.PluginID)
	if !ok {
		o.logger.Warn("orchestrator: plugin not found", "integration_id", integ.ID, "plugin_id", integ.PluginID)
		o.publishEventWithContext(ctx, "scan_integration_skipped", map[string]any{
			"integration_id": integ.ID,
			"plugin_id":      integ.PluginID,
			"label":          integ.Label,
			"reason":         "plugin_not_found",
		})
		item.skipped = true
		return item, nil
	}

	config, err := parseConfig(integ.ConfigJSON)
	if err != nil {
		o.logger.Warn("orchestrator: bad config", "integration_id", integ.ID, "error", err)
		o.publishEventWithContext(ctx, "scan_integration_skipped", map[string]any{
			"integration_id": integ.ID,
			"plugin_id":      integ.PluginID,
			"label":          integ.Label,
			"reason":         "invalid_config",
			"error":          err.Error(),
		})
		item.skipped = true
		item.result.Error = err.Error()
		return item, nil
	}
	item.config = config

	o.publishEventWithContext(ctx, "scan_integration_started", map[string]any{
		"integration_id": integ.ID,
		"plugin_id":      integ.PluginID,
		"label":          integ.Label,
	})

	switch {
	case pluginProvides(plugin, sourceFilesystemListMethod):
		item.config = sourcescope.NormalizeConfig(integ.PluginID, item.config)
		item.filesystemScope = filesystemScanScope(integ.PluginID, item.config)
		o.publishEventWithContext(ctx, "scan_source_list_started", map[string]any{
			"integration_id": integ.ID,
			"plugin_id":      integ.PluginID,
		})
		files, err := o.fetchFiles(ctx, integ.PluginID, item.config)
		if err != nil {
			o.logger.Warn("orchestrator: source listing failed", "integration_id", integ.ID, "plugin_id", integ.PluginID, "error", err)
			o.publishEventWithContext(ctx, "scan_integration_skipped", map[string]any{
				"integration_id": integ.ID,
				"plugin_id":      integ.PluginID,
				"label":          integ.Label,
				"reason":         scanSkipReasonForSourceError(err),
				"error":          err.Error(),
			})
			item.skipped = true
			item.result.Error = err.Error()
			return item, nil
		}
		o.logger.Info("orchestrator: fetched files", "integration_id", integ.ID, "count", len(files))
		o.publishEventWithContext(ctx, "scan_source_list_complete", map[string]any{
			"integration_id": integ.ID,
			"plugin_id":      integ.PluginID,
			"file_count":     len(files),
		})

		o.publishEventWithContext(ctx, "scan_scanner_started", map[string]any{
			"integration_id": integ.ID,
			"plugin_id":      integ.PluginID,
			"file_count":     len(files),
		})
		localScanner := scanner.New(o.logger)
		localScanner.SetProgressReporter(func(scanCtx context.Context, update scanner.ProgressUpdate) {
			o.publishEventWithContext(scanCtx, "scan_scanner_progress", map[string]any{
				"integration_id":  integ.ID,
				"plugin_id":       integ.PluginID,
				"processed_count": update.ProcessedCount,
				"file_count":      update.FileCount,
			})
		})
		groups, err := localScanner.ScanFiles(ctx, files)
		if err != nil {
			o.logger.Warn("orchestrator: file scanner failed", "integration_id", integ.ID, "plugin_id", integ.PluginID, "error", err)
			o.publishEventWithContext(ctx, "scan_integration_skipped", map[string]any{
				"integration_id": integ.ID,
				"plugin_id":      integ.PluginID,
				"label":          integ.Label,
				"reason":         "source_error",
				"error":          err.Error(),
			})
			item.skipped = true
			item.result.Error = err.Error()
			return item, nil
		}
		o.publishEventWithContext(ctx, "scan_scanner_complete", map[string]any{
			"integration_id": integ.ID,
			"plugin_id":      integ.PluginID,
			"group_count":    len(groups),
		})
		item.games = buildGames(integ.ID, integ.PluginID, groups)
		o.logger.Info("orchestrator: built games", "integration_id", integ.ID, "games", len(item.games))

	case pluginProvides(plugin, sourceGamesListMethod):
		o.publishEventWithContext(ctx, "scan_source_list_started", map[string]any{
			"integration_id": integ.ID,
			"plugin_id":      integ.PluginID,
		})
		games, err := o.fetchGames(ctx, integ.ID, integ.PluginID, item.config)
		if err != nil {
			o.logger.Warn("orchestrator: storefront source listing failed", "integration_id", integ.ID, "plugin_id", integ.PluginID, "error", err)
			o.publishEventWithContext(ctx, "scan_integration_skipped", map[string]any{
				"integration_id": integ.ID,
				"plugin_id":      integ.PluginID,
				"label":          integ.Label,
				"reason":         scanSkipReasonForSourceError(err),
				"error":          err.Error(),
			})
			item.skipped = true
			item.result.Error = err.Error()
			return item, nil
		}
		o.logger.Info("orchestrator: fetched storefront games", "integration_id", integ.ID, "count", len(games))
		o.publishEventWithContext(ctx, "scan_source_list_complete", map[string]any{
			"integration_id": integ.ID,
			"plugin_id":      integ.PluginID,
			"game_count":     len(games),
		})
		item.games = games

	default:
		o.publishEventWithContext(ctx, "scan_integration_skipped", map[string]any{
			"integration_id": integ.ID,
			"plugin_id":      integ.PluginID,
			"label":          integ.Label,
			"reason":         "no_source_capability",
		})
		item.skipped = true
		return item, nil
	}

	if len(item.games) == 0 {
		o.publishEventWithContext(ctx, "scan_integration_skipped", map[string]any{
			"integration_id": integ.ID,
			"plugin_id":      integ.PluginID,
			"label":          integ.Label,
			"reason":         "no_games",
		})
		item.skipped = true
		item.result.GamesFound = 0
		return item, nil
	}

	if len(metaSources) > 0 {
		o.publishEventWithContext(ctx, "scan_metadata_started", map[string]any{
			"integration_id":     integ.ID,
			"plugin_id":          integ.PluginID,
			"game_count":         len(item.games),
			"resolver_count":     len(metaSources),
			"metadata_providers": metadataProviders,
		})
		summary, err := o.refreshCoordinator.enrichDiscoveredGames(ctx, integ.ID, item.games, metaSources)
		if err != nil {
			return nil, err
		}
		if summary != nil && summary.Degraded() {
			item.result.Error = summary.Error()
		}
	}

	return item, nil
}

func scanPreparationConcurrency(total int) int {
	if total <= 1 {
		return 1
	}
	if total < maxScanPreparationConcurrency {
		return total
	}
	return maxScanPreparationConcurrency
}

// fetchGames calls source.games.list on a storefront plugin and converts
// the result directly into core.Game entities. Storefront games arrive
// pre-identified with title, platform, and external IDs, so they skip
// the file scanner entirely.
func (o *Orchestrator) fetchGames(ctx context.Context, integrationID, pluginID string, config map[string]any) ([]*core.Game, error) {
	type ipcMedia struct {
		Type     string `json:"type"`
		URL      string `json:"url"`
		Width    int    `json:"width,omitempty"`
		Height   int    `json:"height,omitempty"`
		MimeType string `json:"mime_type,omitempty"`
	}
	var result struct {
		Games []struct {
			ExternalID      string     `json:"external_id"`
			Title           string     `json:"title"`
			Platform        string     `json:"platform,omitempty"`
			URL             string     `json:"url,omitempty"`
			Description     string     `json:"description,omitempty"`
			ReleaseDate     string     `json:"release_date,omitempty"`
			Genres          []string   `json:"genres,omitempty"`
			Developer       string     `json:"developer,omitempty"`
			Publisher       string     `json:"publisher,omitempty"`
			Media           []ipcMedia `json:"media,omitempty"`
			PlaytimeMinutes int        `json:"playtime_minutes,omitempty"`
			IsGamePass      bool       `json:"is_game_pass,omitempty"`
			XcloudAvailable bool       `json:"xcloud_available,omitempty"`
			StoreProductID  string     `json:"store_product_id,omitempty"`
			XcloudURL       string     `json:"xcloud_url,omitempty"`
		} `json:"games"`
	}
	if err := o.pluginCaller.Call(ctx, pluginID, sourceGamesListMethod, config, &result); err != nil {
		return nil, err
	}

	now := time.Now()
	games := make([]*core.Game, 0, len(result.Games))
	for _, sg := range result.Games {
		if sg.Title == "" || sg.ExternalID == "" {
			continue
		}

		gameID := deterministicID(integrationID, pluginID, sg.ExternalID)
		platform := core.Platform("windows_pc")
		if sg.Platform != "" {
			platform = core.Platform(sg.Platform)
		}

		media := make([]core.MediaItem, 0, len(sg.Media))
		for _, mi := range sg.Media {
			media = append(media, core.MediaItem{
				Type:     core.MediaType(mi.Type),
				URL:      mi.URL,
				Width:    mi.Width,
				Height:   mi.Height,
				MimeType: mi.MimeType,
				Source:   pluginID,
			})
		}

		g := &core.Game{
			ID:            gameID,
			Title:         sg.Title,
			RawTitle:      sg.Title,
			Platform:      platform,
			Kind:          core.GameKindBaseGame,
			GroupKind:     core.GroupKindSelfContained,
			IntegrationID: integrationID,
			Status:        "identified",
			LastSeenAt:    &now,
			ExternalIDs: []core.ExternalID{{
				Source:     pluginID,
				ExternalID: sg.ExternalID,
				URL:        sg.URL,
			}},
			ResolverMatches: []core.ResolverMatch{{
				PluginID:        pluginID,
				Title:           sg.Title,
				Platform:        string(platform),
				ExternalID:      sg.ExternalID,
				URL:             sg.URL,
				Description:     sg.Description,
				ReleaseDate:     sg.ReleaseDate,
				Genres:          sg.Genres,
				Developer:       sg.Developer,
				Publisher:       sg.Publisher,
				Media:           media,
				IsGamePass:      sg.IsGamePass,
				XcloudAvailable: sg.XcloudAvailable,
				StoreProductID:  sg.StoreProductID,
				XcloudURL:       sg.XcloudURL,
			}},
			Description: sg.Description,
			ReleaseDate: sg.ReleaseDate,
			Genres:      sg.Genres,
			Developer:   sg.Developer,
			Publisher:   sg.Publisher,
			Media:       media,
		}
		games = append(games, g)
	}
	return games, nil
}

// buildGames converts scanner GameGroups into core.Game entities.
// Each group becomes one Game; files carry over with their roles.
func buildGames(integrationID, pluginID string, groups []scanner.GameGroup) []*core.Game {
	now := time.Now()
	games := make([]*core.Game, 0, len(groups))

	for _, g := range groups {
		gameID := deterministicID(integrationID, g.RootDir, g.Name)

		files := make([]core.GameFile, 0, len(g.Files))
		for _, af := range g.Files {
			var modifiedAt *time.Time
			if !af.ModTime.IsZero() {
				t := af.ModTime.UTC()
				modifiedAt = &t
			}
			files = append(files, core.GameFile{
				GameID:     gameID,
				Path:       af.Path,
				FileName:   af.Name,
				Role:       af.Role,
				FileKind:   string(af.Kind),
				Size:       af.Size,
				IsDir:      af.IsDir,
				ObjectID:   af.ObjectID,
				Revision:   af.Revision,
				ModifiedAt: modifiedAt,
			})
		}

		games = append(games, &core.Game{
			ID:            gameID,
			Title:         g.Name,
			RawTitle:      g.Name,
			Platform:      g.Platform,
			Kind:          core.GameKindBaseGame,
			GroupKind:     g.GroupKind,
			RootPath:      g.RootDir,
			IntegrationID: integrationID,
			Status:        "found",
			LastSeenAt:    &now,
			Files:         files,
		})
	}
	return games
}

// gamesToScanBatch converts the in-memory enriched Game list into a ScanBatch
// ready for persistence. Each Game becomes a SourceGame; resolver matches
// and media are split out into their own maps.
func gamesToScanBatch(integrationID, pluginID string, games []*core.Game) *core.ScanBatch {
	batch := &core.ScanBatch{
		IntegrationID:   integrationID,
		SourceGames:     make([]*core.SourceGame, 0, len(games)),
		ResolverMatches: make(map[string][]core.ResolverMatch),
		MediaItems:      make(map[string][]core.MediaRef),
	}

	for _, g := range games {
		extID := g.ID
		if len(g.ExternalIDs) > 0 {
			extID = g.ExternalIDs[0].ExternalID
		}

		sg := &core.SourceGame{
			ID:            g.ID,
			IntegrationID: integrationID,
			PluginID:      pluginID,
			ExternalID:    extID,
			RawTitle:      g.RawTitle,
			Platform:      g.Platform,
			Kind:          g.Kind,
			GroupKind:     g.GroupKind,
			RootPath:      g.RootPath,
			Status:        g.Status,
			LastSeenAt:    g.LastSeenAt,
			Files:         g.Files,
		}
		batch.SourceGames = append(batch.SourceGames, sg)

		batch.ResolverMatches[g.ID] = append([]core.ResolverMatch(nil), g.ResolverMatches...)

		// Collect media from accepted resolver matches + game-level media.
		var refs []core.MediaRef
		seen := map[string]bool{}
		addMedia := func(mi core.MediaItem) {
			if mi.URL == "" || seen[mi.URL] {
				return
			}
			seen[mi.URL] = true
			refs = append(refs, core.MediaRef{
				Type:   mi.Type,
				URL:    mi.URL,
				Source: mi.Source,
				Width:  mi.Width,
				Height: mi.Height,
			})
		}
		for _, mi := range g.Media {
			addMedia(mi)
		}
		for _, m := range g.ResolverMatches {
			if m.Outvoted {
				continue
			}
			for _, mi := range m.Media {
				addMedia(mi)
			}
		}
		batch.MediaItems[g.ID] = refs
	}
	return batch
}

func deterministicID(integrationID, rootDir, name string) string {
	h := sha256.Sum256([]byte(integrationID + "|" + rootDir + "|" + name))
	return "scan:" + hex.EncodeToString(h[:])[:16]
}

func filesystemScanScope(pluginID string, config map[string]any) *core.FilesystemScanScope {
	if !sourcescope.IsFilesystemBackedPlugin(pluginID) {
		return nil
	}
	includes := sourcescope.ReadIncludePaths(pluginID, config)
	scope := &core.FilesystemScanScope{
		PluginID:     pluginID,
		IncludePaths: make([]core.FilesystemIncludePath, 0, len(includes)),
	}
	for _, include := range includes {
		scope.IncludePaths = append(scope.IncludePaths, core.FilesystemIncludePath{
			Path:      include.Path,
			Recursive: include.Recursive,
		})
	}
	return scope
}

// findMetadataSources returns ordered metadata sources plus per-provider scan snapshots.
func (o *Orchestrator) findMetadataSources(integrations []*core.Integration) ([]MetadataSource, []map[string]any) {
	metaPluginIDs := o.pluginDiscovery.GetPluginIDsProviding(metadataGameLookupMethod)
	if len(metaPluginIDs) == 0 {
		return nil, nil
	}
	metaSet := make(map[string]bool, len(metaPluginIDs))
	for _, id := range metaPluginIDs {
		metaSet[id] = true
	}

	var sources []MetadataSource
	var providerStates []map[string]any
	for _, integ := range integrations {
		if !metaSet[integ.PluginID] {
			continue
		}
		state := map[string]any{
			"integration_id": integ.ID,
			"label":          integ.Label,
			"plugin_id":      integ.PluginID,
		}
		source, err := MetadataSourceFromIntegration(integ)
		if err != nil {
			o.logger.Warn("orchestrator: bad metadata config", "integration_id", integ.ID, "error", err)
			state["status"] = "error"
			state["reason"] = "invalid_config"
			state["error"] = err.Error()
			providerStates = append(providerStates, state)
			continue
		}
		sources = append(sources, source)
		state["status"] = "pending"
		providerStates = append(providerStates, state)
	}
	return sources, providerStates
}

func pluginProvides(plugin *core.Plugin, method string) bool {
	for _, p := range plugin.Manifest.Provides {
		if p == method {
			return true
		}
	}
	return false
}

func parseConfig(configJSON string) (map[string]any, error) {
	if configJSON == "" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(configJSON), &m); err != nil {
		return nil, err
	}
	return m, nil
}
