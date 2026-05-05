package sync

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/buildinfo"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/crypto"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/keystore"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/google/uuid"
)

const payloadVersion = 2

// PluginHost is the subset of plugins.PluginHost that SyncService needs.
type PluginHost = plugins.PluginHost

type syncService struct {
	integrationRepo core.IntegrationRepository
	settingRepo     core.SettingRepository
	profileRepo     core.ProfileRepository
	pluginHost      PluginHost
	keyStore        core.KeyStore
	logger          core.Logger
}

type integrationCheckResult struct {
	Status       string `json:"status"`
	Message      string `json:"message"`
	AuthorizeURL string `json:"authorize_url,omitempty"`
	State        string `json:"state,omitempty"`
}

type remotePayloadRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Data string `json:"data"`
}

type remotePayloadListResult struct {
	Status   string             `json:"status"`
	Message  string             `json:"message"`
	Payloads []remotePayloadRef `json:"payloads"`
}

type remotePayloadUpdateResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func NewSyncService(
	integrationRepo core.IntegrationRepository,
	settingRepo core.SettingRepository,
	profileRepo core.ProfileRepository,
	pluginHost PluginHost,
	ks core.KeyStore,
	logger core.Logger,
) core.SyncService {
	return &syncService{
		integrationRepo: integrationRepo,
		settingRepo:     settingRepo,
		profileRepo:     profileRepo,
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

func (s *syncService) RestoreBootstrap(ctx context.Context, req core.RestoreSyncRequest) (*core.RestoreSyncResult, error) {
	if count, err := s.profileRepo.Count(ctx); err != nil {
		return nil, fmt.Errorf("count profiles: %w", err)
	} else if count > 0 {
		return nil, fmt.Errorf("first-run restore requires an empty profile set")
	}

	key, err := s.resolveKey(req.Passphrase)
	if err != nil {
		return nil, err
	}
	req.PluginID = strings.TrimSpace(req.PluginID)
	if req.PluginID == "" {
		return nil, fmt.Errorf("plugin_id is required")
	}
	if !s.pluginProvides(req.PluginID, "sync.pull") {
		return nil, fmt.Errorf("plugin %s does not provide sync.pull", req.PluginID)
	}
	if req.Config == nil {
		req.Config = map[string]any{}
	}

	var check integrationCheckResult
	if err := s.pluginHost.Call(ctx, req.PluginID, "plugin.check_config", map[string]any{
		"config":       req.Config,
		"redirect_uri": req.RedirectURI,
	}, &check); err != nil {
		return nil, fmt.Errorf("plugin validation failed: %w", err)
	}
	if check.Status == "oauth_required" {
		return &core.RestoreSyncResult{
			Status:       "oauth_required",
			PluginID:     req.PluginID,
			AuthorizeURL: check.AuthorizeURL,
			State:        check.State,
		}, nil
	}
	if check.Status != "" && check.Status != "ok" {
		msg := check.Message
		if msg == "" {
			msg = check.Status
		}
		return nil, fmt.Errorf("plugin validation failed: %s", msg)
	}

	var pull struct {
		Status  string `json:"status"`
		Data    string `json:"data"`
		Message string `json:"message"`
	}
	if err := s.pluginHost.Call(ctx, req.PluginID, "sync.pull", map[string]any{"config": req.Config}, &pull); err != nil {
		return nil, fmt.Errorf("sync.pull plugin call: %w", err)
	}
	if pull.Status == "empty" {
		return nil, fmt.Errorf("remote sync payload is empty")
	}
	if pull.Status != "ok" {
		msg := pull.Message
		if msg == "" {
			msg = pull.Status
		}
		return nil, fmt.Errorf("sync.pull failed: %s", msg)
	}

	var payload core.SyncPayload
	if err := json.Unmarshal([]byte(pull.Data), &payload); err != nil {
		return nil, fmt.Errorf("parse remote payload: %w", err)
	}
	pr, err := s.mergePayload(ctx, &payload, key)
	if err != nil {
		return nil, fmt.Errorf("merge payload: %w", err)
	}

	profileID, err := s.firstAdminProfileID(ctx)
	if err != nil {
		return nil, err
	}
	integrationID, err := s.ensureBootstrapIntegration(ctx, profileID, req)
	if err != nil {
		return nil, err
	}
	if req.StoreKey && strings.TrimSpace(req.Passphrase) != "" {
		if err := s.StoreKey(ctx, req.Passphrase, ""); err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	_ = s.settingRepo.Upsert(core.WithProfile(ctx, &core.Profile{ID: profileID, Role: core.ProfileRoleAdminPlayer}), &core.Setting{Key: "last_sync_pull", Value: now.Format(time.RFC3339), UpdatedAt: now})

	return &core.RestoreSyncResult{
		Status:        "ok",
		PluginID:      req.PluginID,
		ProfileID:     profileID,
		IntegrationID: integrationID,
		Result:        *pr,
	}, nil
}

func (s *syncService) CheckBootstrap(ctx context.Context, req core.RestoreSyncRequest) (*core.RestoreSyncResult, error) {
	if count, err := s.profileRepo.Count(ctx); err != nil {
		return nil, fmt.Errorf("count profiles: %w", err)
	} else if count > 0 {
		return nil, fmt.Errorf("first-run restore requires an empty profile set")
	}
	req.PluginID = strings.TrimSpace(req.PluginID)
	if req.PluginID == "" {
		return nil, fmt.Errorf("plugin_id is required")
	}
	if !s.pluginProvides(req.PluginID, "sync.pull") {
		return nil, fmt.Errorf("plugin %s does not provide sync.pull", req.PluginID)
	}
	if req.Config == nil {
		req.Config = map[string]any{}
	}
	var check integrationCheckResult
	if err := s.pluginHost.Call(ctx, req.PluginID, "plugin.check_config", map[string]any{
		"config":       req.Config,
		"redirect_uri": req.RedirectURI,
	}, &check); err != nil {
		return nil, fmt.Errorf("plugin validation failed: %w", err)
	}
	if check.Status == "oauth_required" {
		return &core.RestoreSyncResult{
			Status:       "oauth_required",
			PluginID:     req.PluginID,
			AuthorizeURL: check.AuthorizeURL,
			State:        check.State,
		}, nil
	}
	if check.Status != "" && check.Status != "ok" {
		msg := check.Message
		if msg == "" {
			msg = check.Status
		}
		return nil, fmt.Errorf("plugin validation failed: %s", msg)
	}
	return &core.RestoreSyncResult{Status: "ok", PluginID: req.PluginID}, nil
}

func (s *syncService) BrowseBootstrap(ctx context.Context, req core.RestoreSyncBrowseRequest) (any, error) {
	if count, err := s.profileRepo.Count(ctx); err != nil {
		return nil, fmt.Errorf("count profiles: %w", err)
	} else if count > 0 {
		return nil, fmt.Errorf("first-run restore requires an empty profile set")
	}
	pluginID := strings.TrimSpace(req.PluginID)
	if pluginID == "" {
		return nil, fmt.Errorf("plugin_id is required")
	}
	if !s.pluginProvides(pluginID, "sync.pull") {
		return nil, fmt.Errorf("plugin %s does not provide sync.pull", pluginID)
	}
	var result any
	if err := s.pluginHost.Call(ctx, pluginID, "source.browse", map[string]any{"path": req.Path}, &result); err != nil {
		return nil, fmt.Errorf("browse failed: %w", err)
	}
	return result, nil
}

func (s *syncService) pluginProvides(pluginID string, method string) bool {
	for _, id := range s.pluginHost.GetPluginIDsProviding(method) {
		if id == pluginID {
			return true
		}
	}
	return false
}

func (s *syncService) firstAdminProfileID(ctx context.Context) (string, error) {
	profiles, err := s.profileRepo.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list profiles: %w", err)
	}
	for _, profile := range profiles {
		if profile.Role == core.ProfileRoleAdminPlayer {
			return profile.ID, nil
		}
	}
	if len(profiles) > 0 {
		return profiles[0].ID, nil
	}
	return "", fmt.Errorf("restore did not create any profiles")
}

func (s *syncService) ensureBootstrapIntegration(ctx context.Context, profileID string, req core.RestoreSyncRequest) (string, error) {
	label := strings.TrimSpace(req.Label)
	if label == "" {
		label = "Settings Sync"
	}
	integrationType := strings.TrimSpace(req.IntegrationType)
	if integrationType == "" {
		integrationType = "sync"
	}

	existing, err := s.integrationRepo.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list integrations: %w", err)
	}
	for _, ig := range existing {
		if ig.ProfileID == profileID && ig.PluginID == req.PluginID && ig.Label == label {
			return ig.ID, nil
		}
	}

	configBytes, err := json.Marshal(req.Config)
	if err != nil {
		return "", fmt.Errorf("marshal sync integration config: %w", err)
	}
	now := time.Now()
	ig := &core.Integration{
		ID:              uuid.NewString(),
		ProfileID:       profileID,
		PluginID:        req.PluginID,
		Label:           label,
		IntegrationType: integrationType,
		ConfigJSON:      string(configBytes),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	createCtx := core.WithProfile(ctx, &core.Profile{ID: profileID, Role: core.ProfileRoleAdminPlayer})
	if err := s.integrationRepo.Create(createCtx, ig); err != nil {
		return "", fmt.Errorf("create bootstrap sync integration: %w", err)
	}
	return ig.ID, nil
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

func (s *syncService) StoreKey(ctx context.Context, passphrase string, currentPassphrase string) error {
	if passphrase == "" {
		return fmt.Errorf("passphrase cannot be empty")
	}
	current, err := s.keyStore.Load()
	if err != nil {
		if errors.Is(err, keystore.ErrNoKey) {
			return s.keyStore.Store(passphrase)
		}
		return fmt.Errorf("load stored key: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(current), []byte(passphrase)) == 1 {
		return nil
	}
	if currentPassphrase == "" {
		return core.ErrSyncKeyCurrentRequired
	}
	if subtle.ConstantTimeCompare([]byte(current), []byte(currentPassphrase)) != 1 {
		return core.ErrSyncKeyCurrentIncorrect
	}
	if err := s.reencryptRemotePayloads(ctx, current, passphrase); err != nil {
		return err
	}
	return s.keyStore.Store(passphrase)
}

func (s *syncService) reencryptRemotePayloads(ctx context.Context, oldKey string, newKey string) error {
	if s.integrationRepo == nil || s.pluginHost == nil {
		return nil
	}
	pluginID, cfg, err := s.findSyncIntegration(ctx)
	if err != nil {
		s.logger.Warn("no remote sync integration found while replacing sync key; stored key will be changed only locally", "error", err)
		return nil
	}
	if !s.pluginProvides(pluginID, "sync.list_payloads") || !s.pluginProvides(pluginID, "sync.update_payload") {
		return fmt.Errorf("sync plugin %s cannot re-encrypt existing remote payloads", pluginID)
	}
	var list remotePayloadListResult
	if err := s.pluginHost.Call(ctx, pluginID, "sync.list_payloads", map[string]any{"config": cfg}, &list); err != nil {
		return fmt.Errorf("list remote sync payloads: %w", err)
	}
	if list.Status != "" && list.Status != "ok" {
		msg := list.Message
		if msg == "" {
			msg = list.Status
		}
		return fmt.Errorf("list remote sync payloads failed: %s", msg)
	}
	for _, payloadRef := range list.Payloads {
		if strings.TrimSpace(payloadRef.ID) == "" || strings.TrimSpace(payloadRef.Data) == "" {
			continue
		}
		updated, err := reencryptSyncPayloadData(payloadRef.Data, oldKey, newKey)
		if err != nil {
			name := payloadRef.Name
			if name == "" {
				name = payloadRef.ID
			}
			return fmt.Errorf("re-encrypt remote sync payload %s: %w", name, err)
		}
		var result remotePayloadUpdateResult
		if err := s.pluginHost.Call(ctx, pluginID, "sync.update_payload", map[string]any{
			"config": cfg,
			"id":     payloadRef.ID,
			"data":   updated,
		}, &result); err != nil {
			return fmt.Errorf("update remote sync payload %s: %w", payloadRef.ID, err)
		}
		if result.Status != "" && result.Status != "ok" {
			msg := result.Message
			if msg == "" {
				msg = result.Status
			}
			return fmt.Errorf("update remote sync payload %s failed: %s", payloadRef.ID, msg)
		}
	}
	return nil
}

func reencryptSyncPayloadData(data string, oldKey string, newKey string) (string, error) {
	var payload core.SyncPayload
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "", fmt.Errorf("parse payload: %w", err)
	}
	for i := range payload.Integrations {
		if strings.TrimSpace(payload.Integrations[i].ConfigEncrypted) == "" {
			continue
		}
		plain, err := crypto.Decrypt(payload.Integrations[i].ConfigEncrypted, oldKey)
		if err != nil {
			return "", fmt.Errorf("decrypt config for %s/%s: %w", payload.Integrations[i].PluginID, payload.Integrations[i].Label, err)
		}
		encrypted, err := crypto.Encrypt(plain, newKey)
		if err != nil {
			return "", fmt.Errorf("encrypt config for %s/%s: %w", payload.Integrations[i].PluginID, payload.Integrations[i].Label, err)
		}
		payload.Integrations[i].ConfigEncrypted = encrypted
	}
	updated, err := json.Marshal(&payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	return string(updated), nil
}

func (s *syncService) ClearKey() error {
	return s.keyStore.Clear()
}

func (s *syncService) buildPayload(ctx context.Context, key string) (*core.SyncPayload, error) {
	integrations, err := s.integrationRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}

	profiles, err := s.profileRepo.List(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}

	syncInts := make([]core.SyncIntegration, 0, len(integrations))
	for _, ig := range integrations {
		encrypted, err := crypto.Encrypt([]byte(ig.ConfigJSON), key)
		if err != nil {
			return nil, fmt.Errorf("encrypt config for %s: %w", ig.PluginID, err)
		}
		syncInts = append(syncInts, core.SyncIntegration{
			ProfileID:       ig.ProfileID,
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
		MGAVersion:   buildinfo.Version,
		Profiles:     derefProfiles(profiles),
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

	profileMap, err := s.ensurePayloadProfiles(ctx, payload)
	if err != nil {
		return nil, err
	}

	existing, err := s.integrationRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list local integrations: %w", err)
	}

	type igKey struct{ profileID, pluginID, label string }
	localMap := make(map[igKey]*core.Integration, len(existing))
	for _, ig := range existing {
		localMap[igKey{ig.ProfileID, ig.PluginID, ig.Label}] = ig
	}

	for _, remote := range payload.Integrations {
		remote.IntegrationType = normalizeSyncIntegrationType(remote.PluginID, remote.IntegrationType)
		configJSON, err := crypto.Decrypt(remote.ConfigEncrypted, key)
		if err != nil {
			return nil, fmt.Errorf("decrypt config for %s/%s: %w", remote.PluginID, remote.Label, err)
		}

		profileID := remote.ProfileID
		if profileID == "" {
			profileID = profileMap[""]
		}
		k := igKey{profileID, remote.PluginID, remote.Label}
		local, exists := localMap[k]

		if !exists {
			ig := &core.Integration{
				ID:              uuid.New().String(),
				ProfileID:       profileID,
				PluginID:        remote.PluginID,
				Label:           remote.Label,
				IntegrationType: remote.IntegrationType,
				ConfigJSON:      string(configJSON),
				CreatedAt:       time.Now(),
				UpdatedAt:       remote.UpdatedAt,
			}
			createCtx := core.WithProfile(ctx, &core.Profile{ID: profileID, Role: core.ProfileRoleAdminPlayer})
			if err := s.integrationRepo.Create(createCtx, ig); err != nil {
				return nil, fmt.Errorf("create integration %s/%s: %w", remote.PluginID, remote.Label, err)
			}
			pr.IntegrationsAdded++
		} else if remote.UpdatedAt.After(local.UpdatedAt) || local.IntegrationType != remote.IntegrationType {
			local.ConfigJSON = string(configJSON)
			local.IntegrationType = remote.IntegrationType
			local.UpdatedAt = remote.UpdatedAt
			updateCtx := core.WithProfile(ctx, &core.Profile{ID: profileID, Role: core.ProfileRoleAdminPlayer})
			if err := s.integrationRepo.Update(updateCtx, local); err != nil {
				return nil, fmt.Errorf("update integration %s/%s: %w", remote.PluginID, remote.Label, err)
			}
			pr.IntegrationsUpdated++
		} else {
			pr.IntegrationsSkipped++
		}
	}

	for _, rs := range payload.Settings {
		settingCtx := ctx
		if rs.ProfileID != "" {
			profileID := profileMap[rs.ProfileID]
			if profileID == "" {
				profileID = rs.ProfileID
			}
			rs.ProfileID = profileID
			settingCtx = core.WithProfile(ctx, &core.Profile{ID: profileID, Role: core.ProfileRoleAdminPlayer})
		} else if rs.Key == "frontend" && profileMap[""] != "" {
			rs.ProfileID = profileMap[""]
			settingCtx = core.WithProfile(ctx, &core.Profile{ID: rs.ProfileID, Role: core.ProfileRoleAdminPlayer})
		}
		local, err := s.settingRepo.Get(settingCtx, rs.Key)
		if err != nil {
			return nil, fmt.Errorf("get setting %q: %w", rs.Key, err)
		}

		if local == nil {
			if err := s.settingRepo.Upsert(settingCtx, &core.Setting{ProfileID: rs.ProfileID, Key: rs.Key, Value: rs.Value, UpdatedAt: rs.UpdatedAt}); err != nil {
				return nil, fmt.Errorf("insert setting %q: %w", rs.Key, err)
			}
			pr.SettingsAdded++
		} else if rs.UpdatedAt.After(local.UpdatedAt) {
			if err := s.settingRepo.Upsert(settingCtx, &core.Setting{ProfileID: rs.ProfileID, Key: rs.Key, Value: rs.Value, UpdatedAt: rs.UpdatedAt}); err != nil {
				return nil, fmt.Errorf("update setting %q: %w", rs.Key, err)
			}
			pr.SettingsUpdated++
		} else {
			pr.SettingsSkipped++
		}
	}

	return pr, nil
}

func normalizeSyncIntegrationType(pluginID string, integrationType string) string {
	integrationType = strings.TrimSpace(integrationType)
	if pluginID == "sync-settings-google-drive" && integrationType == "storage" {
		return "sync"
	}
	return integrationType
}

func derefProfiles(profiles []*core.Profile) []core.Profile {
	out := make([]core.Profile, 0, len(profiles))
	for _, profile := range profiles {
		if profile == nil {
			continue
		}
		out = append(out, *profile)
	}
	return out
}

func (s *syncService) ensurePayloadProfiles(ctx context.Context, payload *core.SyncPayload) (map[string]string, error) {
	out := map[string]string{}
	existing, err := s.profileRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list local profiles: %w", err)
	}
	existingByID := make(map[string]*core.Profile, len(existing))
	for _, profile := range existing {
		existingByID[profile.ID] = profile
	}

	if len(payload.Profiles) == 0 {
		if len(existing) > 0 {
			out[""] = existing[0].ID
			return out, nil
		}
		now := time.Now()
		profile := &core.Profile{
			ID:          uuid.NewString(),
			DisplayName: "Admin Player",
			AvatarKey:   "player-1",
			Role:        core.ProfileRoleAdminPlayer,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.profileRepo.Create(ctx, profile); err != nil {
			return nil, fmt.Errorf("create fallback profile: %w", err)
		}
		out[""] = profile.ID
		return out, nil
	}

	for _, remote := range payload.Profiles {
		if remote.ID == "" {
			continue
		}
		if local := existingByID[remote.ID]; local != nil {
			out[remote.ID] = local.ID
			continue
		}
		profile := remote
		if profile.Role == "" {
			profile.Role = core.ProfileRolePlayer
		}
		if err := s.profileRepo.Create(ctx, &profile); err != nil {
			return nil, fmt.Errorf("create profile %s: %w", profile.DisplayName, err)
		}
		out[remote.ID] = profile.ID
	}
	if out[""] == "" {
		for _, id := range out {
			out[""] = id
			break
		}
	}
	return out, nil
}
