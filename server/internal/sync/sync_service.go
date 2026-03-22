package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/crypto"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/keystore"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/google/uuid"
)

const payloadVersion = 1

// PluginHost is the subset of plugins.PluginHost that SyncService needs.
type PluginHost = plugins.PluginHost

type syncService struct {
	integrationRepo core.IntegrationRepository
	settingRepo     core.SettingRepository
	pluginHost      PluginHost
	keyStore        core.KeyStore
	logger          core.Logger
}

func NewSyncService(
	integrationRepo core.IntegrationRepository,
	settingRepo core.SettingRepository,
	pluginHost PluginHost,
	ks core.KeyStore,
	logger core.Logger,
) core.SyncService {
	return &syncService{
		integrationRepo: integrationRepo,
		settingRepo:     settingRepo,
		pluginHost:      pluginHost,
		keyStore:        ks,
		logger:          logger,
	}
}

func (s *syncService) resolveKey(passphrase string) (string, error) {
	if passphrase != "" {
		return passphrase, nil
	}
	key, err := s.keyStore.Load()
	if err != nil {
		if errors.Is(err, keystore.ErrNoKey) {
			return "", fmt.Errorf("no encryption key available — provide a passphrase or configure a server-stored key via POST /api/sync/key")
		}
		return "", fmt.Errorf("load stored key: %w", err)
	}
	return key, nil
}

// findSyncIntegration returns the plugin ID and parsed config for the first
// integration whose plugin provides "sync.push".
func (s *syncService) findSyncIntegration(ctx context.Context) (string, map[string]any, error) {
	syncPlugins := s.pluginHost.GetPluginIDsProviding("sync.push")
	if len(syncPlugins) == 0 {
		return "", nil, fmt.Errorf("no sync plugin installed (need a plugin providing sync.push)")
	}

	integrations, err := s.integrationRepo.List(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("list integrations: %w", err)
	}

	syncSet := make(map[string]bool, len(syncPlugins))
	for _, id := range syncPlugins {
		syncSet[id] = true
	}

	for _, ig := range integrations {
		if !syncSet[ig.PluginID] {
			continue
		}
		var cfg map[string]any
		if ig.ConfigJSON != "" {
			if err := json.Unmarshal([]byte(ig.ConfigJSON), &cfg); err != nil {
				s.logger.Warn("bad config in sync integration, skipping", "id", ig.ID, "error", err)
				continue
			}
		}
		if cfg == nil {
			cfg = map[string]any{}
		}
		return ig.PluginID, cfg, nil
	}
	return "", nil, fmt.Errorf("no sync integration configured — create one via POST /api/integrations")
}

func (s *syncService) Push(ctx context.Context, passphrase string) (*core.PushResult, error) {
	key, err := s.resolveKey(passphrase)
	if err != nil {
		return nil, err
	}

	pluginID, cfg, err := s.findSyncIntegration(ctx)
	if err != nil {
		return nil, err
	}

	payload, err := s.buildPayload(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("build sync payload: %w", err)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	params := map[string]any{
		"data":   string(data),
		"config": cfg,
	}
	var result struct {
		Status       string `json:"status"`
		VersionCount int    `json:"version_count"`
		Latest       string `json:"latest"`
		Message      string `json:"message"`
	}
	if err := s.pluginHost.Call(ctx, pluginID, "sync.push", params, &result); err != nil {
		return nil, fmt.Errorf("sync.push plugin call: %w", err)
	}
	if result.Status != "ok" {
		return nil, fmt.Errorf("sync.push failed: %s", result.Message)
	}

	now := time.Now().UTC()
	_ = s.settingRepo.Upsert(ctx, &core.Setting{Key: "last_sync_push", Value: now.Format(time.RFC3339), UpdatedAt: now})

	return &core.PushResult{
		ExportedAt:     payload.ExportedAt,
		Integrations:   len(payload.Integrations),
		Settings:       len(payload.Settings),
		RemoteVersions: result.VersionCount,
	}, nil
}

func (s *syncService) Pull(ctx context.Context, passphrase string) (*core.PullResult, error) {
	key, err := s.resolveKey(passphrase)
	if err != nil {
		return nil, err
	}

	pluginID, cfg, err := s.findSyncIntegration(ctx)
	if err != nil {
		return nil, err
	}

	params := map[string]any{"config": cfg}
	var result struct {
		Status  string `json:"status"`
		Data    string `json:"data"`
		Message string `json:"message"`
	}
	if err := s.pluginHost.Call(ctx, pluginID, "sync.pull", params, &result); err != nil {
		return nil, fmt.Errorf("sync.pull plugin call: %w", err)
	}
	if result.Status == "empty" {
		return &core.PullResult{}, nil
	}
	if result.Status != "ok" {
		return nil, fmt.Errorf("sync.pull failed: %s", result.Message)
	}

	var payload core.SyncPayload
	if err := json.Unmarshal([]byte(result.Data), &payload); err != nil {
		return nil, fmt.Errorf("parse remote payload: %w", err)
	}

	pr, err := s.mergePayload(ctx, &payload, key)
	if err != nil {
		return nil, fmt.Errorf("merge payload: %w", err)
	}

	now := time.Now().UTC()
	_ = s.settingRepo.Upsert(ctx, &core.Setting{Key: "last_sync_pull", Value: now.Format(time.RFC3339), UpdatedAt: now})

	return pr, nil
}

func (s *syncService) Status(ctx context.Context) (*core.SyncStatus, error) {
	_, _, findErr := s.findSyncIntegration(ctx)
	configured := findErr == nil

	st := &core.SyncStatus{
		Configured:   configured,
		HasStoredKey: s.keyStore.HasKey(),
	}

	if push, err := s.settingRepo.Get(ctx, "last_sync_push"); err == nil && push != nil {
		st.LastPush = push.Value
	}
	if pull, err := s.settingRepo.Get(ctx, "last_sync_pull"); err == nil && pull != nil {
		st.LastPull = pull.Value
	}
	return st, nil
}

func (s *syncService) StoreKey(passphrase string) error {
	if passphrase == "" {
		return fmt.Errorf("passphrase cannot be empty")
	}
	return s.keyStore.Store(passphrase)
}

func (s *syncService) ClearKey() error {
	return s.keyStore.Clear()
}

func (s *syncService) buildPayload(ctx context.Context, key string) (*core.SyncPayload, error) {
	integrations, err := s.integrationRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}

	syncInts := make([]core.SyncIntegration, 0, len(integrations))
	for _, ig := range integrations {
		encrypted, err := crypto.Encrypt([]byte(ig.ConfigJSON), key)
		if err != nil {
			return nil, fmt.Errorf("encrypt config for %s: %w", ig.PluginID, err)
		}
		syncInts = append(syncInts, core.SyncIntegration{
			PluginID:        ig.PluginID,
			Label:           ig.Label,
			IntegrationType: ig.IntegrationType,
			ConfigEncrypted: encrypted,
			UpdatedAt:       ig.UpdatedAt,
		})
	}

	// Collect all settings except internal sync timestamps.
	allSettings, err := s.listAllSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}

	return &core.SyncPayload{
		Version:      payloadVersion,
		ExportedAt:   time.Now().UTC(),
		MGAVersion:   "1.0.0",
		Integrations: syncInts,
		Settings:     allSettings,
	}, nil
}

func (s *syncService) listAllSettings(ctx context.Context) ([]core.Setting, error) {
	all, err := s.settingRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]core.Setting, 0, len(all))
	for _, st := range all {
		if st.Key == "last_sync_push" || st.Key == "last_sync_pull" {
			continue
		}
		out = append(out, *st)
	}
	return out, nil
}

func (s *syncService) mergePayload(ctx context.Context, payload *core.SyncPayload, key string) (*core.PullResult, error) {
	pr := &core.PullResult{RemoteExportedAt: payload.ExportedAt}

	existing, err := s.integrationRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list local integrations: %w", err)
	}

	type igKey struct{ pluginID, label string }
	localMap := make(map[igKey]*core.Integration, len(existing))
	for _, ig := range existing {
		localMap[igKey{ig.PluginID, ig.Label}] = ig
	}

	for _, remote := range payload.Integrations {
		configJSON, err := crypto.Decrypt(remote.ConfigEncrypted, key)
		if err != nil {
			return nil, fmt.Errorf("decrypt config for %s/%s: %w", remote.PluginID, remote.Label, err)
		}

		k := igKey{remote.PluginID, remote.Label}
		local, exists := localMap[k]

		if !exists {
			ig := &core.Integration{
				ID:              uuid.New().String(),
				PluginID:        remote.PluginID,
				Label:           remote.Label,
				IntegrationType: remote.IntegrationType,
				ConfigJSON:      string(configJSON),
				CreatedAt:       time.Now(),
				UpdatedAt:       remote.UpdatedAt,
			}
			if err := s.integrationRepo.Create(ctx, ig); err != nil {
				return nil, fmt.Errorf("create integration %s/%s: %w", remote.PluginID, remote.Label, err)
			}
			pr.IntegrationsAdded++
		} else if remote.UpdatedAt.After(local.UpdatedAt) {
			local.ConfigJSON = string(configJSON)
			local.IntegrationType = remote.IntegrationType
			local.UpdatedAt = remote.UpdatedAt
			if err := s.integrationRepo.Update(ctx, local); err != nil {
				return nil, fmt.Errorf("update integration %s/%s: %w", remote.PluginID, remote.Label, err)
			}
			pr.IntegrationsUpdated++
		} else {
			pr.IntegrationsSkipped++
		}
	}

	for _, rs := range payload.Settings {
		local, err := s.settingRepo.Get(ctx, rs.Key)
		if err != nil {
			return nil, fmt.Errorf("get setting %q: %w", rs.Key, err)
		}

		if local == nil {
			if err := s.settingRepo.Upsert(ctx, &core.Setting{Key: rs.Key, Value: rs.Value, UpdatedAt: rs.UpdatedAt}); err != nil {
				return nil, fmt.Errorf("insert setting %q: %w", rs.Key, err)
			}
			pr.SettingsAdded++
		} else if rs.UpdatedAt.After(local.UpdatedAt) {
			if err := s.settingRepo.Upsert(ctx, &core.Setting{Key: rs.Key, Value: rs.Value, UpdatedAt: rs.UpdatedAt}); err != nil {
				return nil, fmt.Errorf("update setting %q: %w", rs.Key, err)
			}
			pr.SettingsUpdated++
		} else {
			pr.SettingsSkipped++
		}
	}

	return pr, nil
}
