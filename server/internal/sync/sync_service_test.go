package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/crypto"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/keystore"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
)

type memoryKeyStore struct {
	key  string
	keys map[string]string
}

func (k *memoryKeyStore) Store(profileID, passphrase string) error {
	k.key = passphrase
	if k.keys == nil {
		k.keys = make(map[string]string)
	}
	k.keys[profileID] = passphrase
	return nil
}

func (k *memoryKeyStore) Load(profileID string) (string, error) {
	value := k.key
	if k.keys != nil {
		value = k.keys[profileID]
	}
	if value == "" {
		return "", keystore.ErrNoKey
	}
	return value, nil
}

func (k *memoryKeyStore) Clear(profileID string) error {
	k.key = ""
	delete(k.keys, profileID)
	return nil
}

func (k *memoryKeyStore) HasKey(profileID string) bool {
	if k.keys != nil {
		return k.keys[profileID] != ""
	}
	return k.key != ""
}

func syncTestContext() context.Context {
	return core.WithProfile(context.Background(), &core.Profile{ID: "profile-1", Role: core.ProfileRoleAdminPlayer})
}

func TestStoreKeyRequiresCurrentPassphraseWhenReplacingExistingKey(t *testing.T) {
	ks := &memoryKeyStore{}
	svc := &syncService{keyStore: ks}

	if err := svc.StoreKey(syncTestContext(), "first", ""); err != nil {
		t.Fatalf("StoreKey initial: %v", err)
	}
	if ks.key != "first" {
		t.Fatalf("stored key = %q, want first", ks.key)
	}

	if err := svc.StoreKey(syncTestContext(), "second", ""); !errors.Is(err, core.ErrSyncKeyCurrentRequired) {
		t.Fatalf("StoreKey without current error = %v, want ErrSyncKeyCurrentRequired", err)
	}
	if ks.key != "first" {
		t.Fatalf("stored key changed without current passphrase: %q", ks.key)
	}

	if err := svc.StoreKey(syncTestContext(), "second", "wrong"); !errors.Is(err, core.ErrSyncKeyCurrentIncorrect) {
		t.Fatalf("StoreKey wrong current error = %v, want ErrSyncKeyCurrentIncorrect", err)
	}
	if ks.key != "first" {
		t.Fatalf("stored key changed after wrong current passphrase: %q", ks.key)
	}

	if err := svc.StoreKey(syncTestContext(), "second", "first"); err != nil {
		t.Fatalf("StoreKey with current: %v", err)
	}
	if ks.key != "second" {
		t.Fatalf("stored key = %q, want second", ks.key)
	}
}

func TestStoreKeyNoopsWhenSamePassphraseIsStored(t *testing.T) {
	ks := &memoryKeyStore{key: "same"}
	svc := &syncService{keyStore: ks}

	if err := svc.StoreKey(syncTestContext(), "same", ""); err != nil {
		t.Fatalf("StoreKey same passphrase: %v", err)
	}
	if ks.key != "same" {
		t.Fatalf("stored key = %q, want same", ks.key)
	}
}

func TestReencryptSyncPayloadDataRewritesEncryptedConfigs(t *testing.T) {
	encrypted, err := crypto.Encrypt([]byte(`{"token":"secret"}`), "old-key")
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"version":2,"integrations":[{"plugin_id":"sync-settings-google-drive","label":"Google Drive Sync","integration_type":"sync","config_encrypted":"` + encrypted + `"}],"settings":[]}`

	updated, err := reencryptSyncPayloadData(payload, "old-key", "new-key")
	if err != nil {
		t.Fatalf("reencryptSyncPayloadData: %v", err)
	}
	var parsed core.SyncPayload
	if err := json.Unmarshal([]byte(updated), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Integrations) != 1 {
		t.Fatalf("integrations = %d, want 1", len(parsed.Integrations))
	}
	if _, err := crypto.Decrypt(parsed.Integrations[0].ConfigEncrypted, "old-key"); err == nil {
		t.Fatal("updated payload still decrypts with old key")
	}
	plain, err := crypto.Decrypt(parsed.Integrations[0].ConfigEncrypted, "new-key")
	if err != nil {
		t.Fatalf("decrypt with new key: %v", err)
	}
	if string(plain) != `{"token":"secret"}` {
		t.Fatalf("plain = %q", plain)
	}
}

func TestNormalizeSyncIntegrationTypeUpgradesLegacyStorageSettingsSync(t *testing.T) {
	if got := normalizeSyncIntegrationType("sync-settings-google-drive", "storage"); got != "sync" {
		t.Fatalf("normalizeSyncIntegrationType legacy sync = %q, want sync", got)
	}
	if got := normalizeSyncIntegrationType("save-sync-google-drive", "storage"); got != "storage" {
		t.Fatalf("normalizeSyncIntegrationType save sync = %q, want storage", got)
	}
}

func TestPreviewRestorePointIncludesStableIntegrationKeys(t *testing.T) {
	exportedAt := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	updatedAt := exportedAt.Add(-time.Hour)
	payload := core.SyncPayload{
		Version:    2,
		ExportedAt: exportedAt,
		MGAVersion: "0.0.8-beta",
		Profiles:   []core.Profile{{ID: "profile-1"}},
		Integrations: []core.SyncIntegration{{
			ProfileID:       "profile-1",
			PluginID:        "sync-settings-google-drive",
			Label:           "Google Drive Sync",
			IntegrationType: "storage",
			UpdatedAt:       updatedAt,
		}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	point := previewRestorePoint(remotePayloadRef{ID: "file-1", Name: "latest.json", Data: string(data)})
	if !point.IsLatest {
		t.Fatal("expected latest payload")
	}
	if point.ProfileCount != 1 || point.IntegrationCount != 1 {
		t.Fatalf("counts = profiles %d integrations %d", point.ProfileCount, point.IntegrationCount)
	}
	if got := point.Integrations[0].IntegrationType; got != "sync" {
		t.Fatalf("integration type = %q, want sync", got)
	}
	wantKey := restoreIntegrationKey("sync-settings-google-drive", "Google Drive Sync", "sync")
	if point.Integrations[0].Key != wantKey {
		t.Fatalf("key = %q, want %q", point.Integrations[0].Key, wantKey)
	}
}

func TestFilterPayloadIntegrationsKeepsSelectedKeysOnly(t *testing.T) {
	payload := &core.SyncPayload{Integrations: []core.SyncIntegration{
		{PluginID: "game-source-steam", Label: "Steam", IntegrationType: "source"},
		{PluginID: "game-source-xbox", Label: "Xbox", IntegrationType: "source"},
	}}
	selected := []string{restoreIntegrationKey("game-source-xbox", "Xbox", "source")}

	filterPayloadIntegrations(payload, selected)

	if len(payload.Integrations) != 1 {
		t.Fatalf("integrations = %d, want 1", len(payload.Integrations))
	}
	if payload.Integrations[0].PluginID != "game-source-xbox" {
		t.Fatalf("kept plugin = %q, want xbox", payload.Integrations[0].PluginID)
	}
}

func TestBuildPayloadExportsOnlySelectedOwner(t *testing.T) {
	now := time.Now().UTC()
	profiles := &syncProfileRepo{items: map[string]*core.Profile{
		"profile-a": {ID: "profile-a", DisplayName: "Player A", Role: core.ProfileRolePlayer, UpdatedAt: now},
		"profile-b": {ID: "profile-b", DisplayName: "Player B", Role: core.ProfileRoleAdminPlayer, UpdatedAt: now},
	}}
	service := &syncService{
		profileRepo: profiles,
		integrationRepo: &syncIntegrationRepo{items: []*core.Integration{
			{ID: "a", ProfileID: "profile-a", PluginID: "plugin-a", Label: "A", ConfigJSON: `{"token":"a"}`, UpdatedAt: now},
			{ID: "b", ProfileID: "profile-b", PluginID: "plugin-b", Label: "B", ConfigJSON: `{"token":"b"}`, UpdatedAt: now},
		}},
		settingRepo: &syncSettingRepo{items: []*core.Setting{
			{ProfileID: "profile-a", Key: "frontend", Value: `{"a":true}`},
			{ProfileID: "profile-b", Key: "frontend", Value: `{"b":true}`},
		}},
	}
	ctx := core.WithProfile(context.Background(), profiles.items["profile-a"])
	payload, err := service.buildPayload(ctx, "key-a")
	if err != nil {
		t.Fatal(err)
	}
	if payload.Version != 3 || payload.OwnerProfileID != "profile-a" || len(payload.Profiles) != 1 || payload.Profiles[0].ID != "profile-a" {
		t.Fatalf("payload owner = %+v", payload)
	}
	if len(payload.Integrations) != 1 || payload.Integrations[0].ProfileID != "profile-a" {
		t.Fatalf("payload integrations = %+v", payload.Integrations)
	}
	if len(payload.Settings) != 1 || payload.Settings[0].ProfileID != "profile-a" {
		t.Fatalf("payload settings = %+v", payload.Settings)
	}
	plain, err := crypto.Decrypt(payload.Integrations[0].ConfigEncrypted, "key-a")
	if err != nil || string(plain) != `{"token":"a"}` {
		t.Fatalf("decrypt owner config = %s, %v", plain, err)
	}
	if _, err := crypto.Decrypt(payload.Integrations[0].ConfigEncrypted, "key-b"); err == nil {
		t.Fatal("profile B key decrypted profile A payload")
	}
}

func TestLegacyV2PayloadRequiresUnambiguousOwner(t *testing.T) {
	local := &core.Profile{ID: "local", DisplayName: "Local", UpdatedAt: time.Now()}
	service := &syncService{profileRepo: &syncProfileRepo{items: map[string]*core.Profile{"local": local}}}
	ctx := core.WithProfile(context.Background(), local)
	payload := &core.SyncPayload{Version: 2, Profiles: []core.Profile{{ID: "remote-a"}, {ID: "remote-b"}}, Integrations: []core.SyncIntegration{{ProfileID: "remote-a"}, {ProfileID: "remote-b"}}}
	if _, err := service.preparePayloadProfiles(ctx, payload); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("ambiguous v2 import error = %v", err)
	}
}

func TestFirstRunRestoreCreatesOnlyVersion3Owner(t *testing.T) {
	now := time.Now().UTC()
	repo := &syncProfileRepo{items: map[string]*core.Profile{}}
	service := &syncService{profileRepo: repo}
	payload := &core.SyncPayload{
		Version:        3,
		OwnerProfileID: "remote-owner",
		Profiles:       []core.Profile{{ID: "remote-owner", DisplayName: "Owner", Role: core.ProfileRoleAdminPlayer, UpdatedAt: now}},
		Integrations:   []core.SyncIntegration{{ProfileID: "remote-owner", PluginID: "xbox", Label: "Owner Xbox"}},
		Settings:       []core.Setting{{ProfileID: "remote-owner", Key: "frontend", Value: `{}`}},
	}
	profileMap, err := service.preparePayloadProfiles(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.items) != 1 || repo.items["remote-owner"] == nil || profileMap[""] != "remote-owner" {
		t.Fatalf("first-run owner result = profiles=%+v map=%+v", repo.items, profileMap)
	}
	if len(payload.Integrations) != 1 || payload.Integrations[0].ProfileID != "remote-owner" || len(payload.Settings) != 1 || payload.Settings[0].ProfileID != "remote-owner" {
		t.Fatalf("first-run owner rows = integrations=%+v settings=%+v", payload.Integrations, payload.Settings)
	}
}

func TestFirstRunRestoreRejectsAmbiguousVersion2WithoutMutation(t *testing.T) {
	repo := &syncProfileRepo{items: map[string]*core.Profile{}}
	service := &syncService{profileRepo: repo}
	payload := &core.SyncPayload{Version: 2, Profiles: []core.Profile{{ID: "remote-a"}, {ID: "remote-b"}}}
	if _, err := service.preparePayloadProfiles(context.Background(), payload); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("ambiguous first-run v2 error = %v", err)
	}
	if len(repo.items) != 0 {
		t.Fatalf("ambiguous restore created profiles: %+v", repo.items)
	}
}

func TestVersion3PayloadRejectsForeignRowsAndNeverElevatesRole(t *testing.T) {
	now := time.Now().UTC()
	local := &core.Profile{ID: "local", DisplayName: "Local", Role: core.ProfileRolePlayer, UpdatedAt: now.Add(-time.Hour)}
	repo := &syncProfileRepo{items: map[string]*core.Profile{"local": local}}
	service := &syncService{profileRepo: repo}
	ctx := core.WithProfile(context.Background(), local)
	payload := &core.SyncPayload{Version: 3, OwnerProfileID: "remote", Profiles: []core.Profile{{ID: "remote", DisplayName: "Remote", Role: core.ProfileRoleAdminPlayer, UpdatedAt: now}}, Integrations: []core.SyncIntegration{{ProfileID: "foreign"}}}
	if _, err := service.preparePayloadProfiles(ctx, payload); err == nil {
		t.Fatal("v3 payload containing foreign integration was accepted")
	}
	payload.Integrations = nil
	if _, err := service.preparePayloadProfiles(ctx, payload); err != nil {
		t.Fatal(err)
	}
	if repo.items["local"].Role != core.ProfileRolePlayer {
		t.Fatalf("sync elevated selected profile to %q", repo.items["local"].Role)
	}
}

func TestSyncProviderSelectionAndProtectedKeysAreProfileScoped(t *testing.T) {
	repo := &syncIntegrationRepo{items: []*core.Integration{
		{ID: "drive-a", ProfileID: "profile-a", PluginID: "sync-settings-google-drive", ConfigJSON: `{"account":"a@example.test","refresh_token":"secret-a"}`},
		{ID: "drive-b", ProfileID: "profile-b", PluginID: "sync-settings-google-drive", ConfigJSON: `{"account":"b@example.test","refresh_token":"secret-b"}`},
	}}
	keys := &memoryKeyStore{keys: map[string]string{"profile-a": "key-a", "profile-b": "key-b"}}
	service := &syncService{integrationRepo: repo, pluginHost: syncTestPluginHost{}, keyStore: keys, logger: syncTestLogger{}}
	for _, tc := range []struct{ profileID, account, key string }{{"profile-a", "a@example.test", "key-a"}, {"profile-b", "b@example.test", "key-b"}} {
		ctx := core.WithProfile(context.Background(), &core.Profile{ID: tc.profileID})
		_, config, err := service.findSyncIntegration(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if config["account"] != tc.account {
			t.Fatalf("%s selected config %#v", tc.profileID, config)
		}
		resolved, err := service.resolveKey(ctx, "")
		if err != nil || resolved != tc.key {
			t.Fatalf("%s resolved key %q, %v", tc.profileID, resolved, err)
		}
	}
}

func TestTwoProfilesCannotOverwriteOrPullAnotherProviderAccount(t *testing.T) {
	now := time.Now().UTC()
	profiles := &syncProfileRepo{items: map[string]*core.Profile{
		"profile-a": {ID: "profile-a", DisplayName: "Player A", Role: core.ProfileRoleAdminPlayer, UpdatedAt: now},
		"profile-b": {ID: "profile-b", DisplayName: "Player B", Role: core.ProfileRoleAdminPlayer, UpdatedAt: now},
	}}
	integrations := &syncIntegrationRepo{items: []*core.Integration{
		{ID: "drive-a", ProfileID: "profile-a", PluginID: "sync-settings-google-drive", Label: "A Drive", ConfigJSON: `{"account":"a@example.test","refresh_token":"secret-a"}`, UpdatedAt: now},
		{ID: "xbox-a", ProfileID: "profile-a", PluginID: "xbox", Label: "A Xbox", ConfigJSON: `{"refresh_token":"xbox-a"}`, UpdatedAt: now},
		{ID: "drive-b", ProfileID: "profile-b", PluginID: "sync-settings-google-drive", Label: "B Drive", ConfigJSON: `{"account":"b@example.test","refresh_token":"secret-b"}`, UpdatedAt: now},
		{ID: "xbox-b", ProfileID: "profile-b", PluginID: "xbox", Label: "B Xbox", ConfigJSON: `{"refresh_token":"xbox-b"}`, UpdatedAt: now},
	}}
	host := &isolatedSyncPluginHost{payloads: make(map[string]string)}
	service := &syncService{
		profileRepo:     profiles,
		integrationRepo: integrations,
		settingRepo:     &syncSettingRepo{},
		pluginHost:      host,
		keyStore:        &memoryKeyStore{keys: map[string]string{"profile-a": "key-a", "profile-b": "key-b"}},
		logger:          syncTestLogger{},
	}
	ctxA := core.WithProfile(context.Background(), profiles.items["profile-a"])
	ctxB := core.WithProfile(context.Background(), profiles.items["profile-b"])
	if _, err := service.Push(ctxA, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Push(ctxB, ""); err != nil {
		t.Fatal(err)
	}
	payloadA := host.payload("a@example.test")
	payloadB := host.payload("b@example.test")
	if payloadA == "" || payloadB == "" || payloadA == payloadB {
		t.Fatalf("provider payloads were not separated: A=%d B=%d", len(payloadA), len(payloadB))
	}
	for _, tc := range []struct {
		data, owner, key, foreignKey string
	}{{payloadA, "profile-a", "key-a", "key-b"}, {payloadB, "profile-b", "key-b", "key-a"}} {
		var payload core.SyncPayload
		if err := json.Unmarshal([]byte(tc.data), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.OwnerProfileID != tc.owner || len(payload.Profiles) != 1 || payload.Profiles[0].ID != tc.owner {
			t.Fatalf("remote owner = %+v, want %s", payload, tc.owner)
		}
		for _, integration := range payload.Integrations {
			if integration.ProfileID != tc.owner {
				t.Fatalf("foreign integration in %s payload: %+v", tc.owner, integration)
			}
			if _, err := crypto.Decrypt(integration.ConfigEncrypted, tc.key); err != nil {
				t.Fatalf("owner key could not decrypt %s payload: %v", tc.owner, err)
			}
			if _, err := crypto.Decrypt(integration.ConfigEncrypted, tc.foreignKey); err == nil {
				t.Fatalf("foreign key decrypted %s payload", tc.owner)
			}
		}
	}
	beforeB := payloadB
	profiles.items["profile-a"].DisplayName = "Player A updated"
	profiles.items["profile-a"].UpdatedAt = now.Add(time.Minute)
	if _, err := service.Push(ctxA, ""); err != nil {
		t.Fatal(err)
	}
	if got := host.payload("b@example.test"); got != beforeB {
		t.Fatal("profile A push overwrote profile B provider payload")
	}
	if _, err := service.Pull(ctxA, ""); err != nil {
		t.Fatal(err)
	}
	if got := host.lastPulledAccount(); got != "a@example.test" {
		t.Fatalf("profile A pulled provider account %q", got)
	}

	for _, tc := range []struct {
		account string
		owner   string
	}{{"a@example.test", "profile-a"}, {"b@example.test", "profile-b"}} {
		listProfiles := &syncProfileRepo{items: map[string]*core.Profile{}}
		listService := &syncService{profileRepo: listProfiles, pluginHost: host}
		points, err := listService.ListBootstrapPayloads(context.Background(), core.RestoreSyncRequest{
			PluginID: "sync-settings-google-drive",
			Config:   map[string]any{"account": tc.account},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(points.Payloads) != 1 || points.Payloads[0].ProfileCount != 1 {
			t.Fatalf("%s restore points = %+v", tc.account, points.Payloads)
		}
		for _, integration := range points.Payloads[0].Integrations {
			if integration.ProfileID != tc.owner {
				t.Fatalf("%s listed foreign restore integration %+v", tc.account, integration)
			}
		}
	}

	restoreProfiles := &syncProfileRepo{items: map[string]*core.Profile{}}
	restoreIntegrations := &syncIntegrationRepo{}
	restoreService := &syncService{
		profileRepo:     restoreProfiles,
		integrationRepo: restoreIntegrations,
		settingRepo:     &syncSettingRepo{},
		pluginHost:      host,
		keyStore:        &memoryKeyStore{},
		logger:          syncTestLogger{},
	}
	restored, err := restoreService.RestoreBootstrap(context.Background(), core.RestoreSyncRequest{
		PluginID:        "sync-settings-google-drive",
		Label:           "A Drive",
		IntegrationType: "sync",
		Config:          map[string]any{"account": "a@example.test"},
		Passphrase:      "key-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if restored.ProfileID != "profile-a" || len(restoreProfiles.items) != 1 || restoreProfiles.items["profile-a"] == nil {
		t.Fatalf("provider A restore profiles = result=%+v profiles=%+v", restored, restoreProfiles.items)
	}
	for _, integration := range restoreIntegrations.items {
		if integration.ProfileID != "profile-a" || strings.Contains(integration.Label, "B") {
			t.Fatalf("provider A restore imported foreign integration %+v", integration)
		}
	}
}

type isolatedSyncPluginHost struct {
	mu       sync.Mutex
	payloads map[string]string
	lastPull string
}

func (h *isolatedSyncPluginHost) Discover(context.Context) error { return nil }
func (h *isolatedSyncPluginHost) Close() error                   { return nil }
func (h *isolatedSyncPluginHost) GetPluginIDs() []string {
	return []string{"sync-settings-google-drive"}
}
func (h *isolatedSyncPluginHost) GetPlugin(string) (*core.Plugin, bool) {
	return nil, false
}
func (h *isolatedSyncPluginHost) ListPlugins() []plugins.PluginInfo { return nil }
func (h *isolatedSyncPluginHost) GetPluginIDsProviding(method string) []string {
	if method == "sync.push" || method == "sync.pull" || method == "sync.list_payloads" {
		return []string{"sync-settings-google-drive"}
	}
	return nil
}
func (h *isolatedSyncPluginHost) Call(_ context.Context, _ string, method string, params any, result any) error {
	raw, _ := json.Marshal(params)
	var request struct {
		Data   string         `json:"data"`
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return err
	}
	account, _ := request.Config["account"].(string)
	if account == "" {
		return errors.New("provider account is required")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	switch method {
	case "plugin.check_config":
		return assignSyncTestResult(result, map[string]any{"status": "ok"})
	case "sync.push":
		h.payloads[account] = request.Data
		return assignSyncTestResult(result, map[string]any{"status": "ok", "version_count": 1})
	case "sync.pull":
		h.lastPull = account
		data := h.payloads[account]
		if data == "" {
			return assignSyncTestResult(result, map[string]any{"status": "empty"})
		}
		return assignSyncTestResult(result, map[string]any{"status": "ok", "data": data})
	case "sync.list_payloads":
		data := h.payloads[account]
		payloads := []map[string]any{}
		if data != "" {
			payloads = append(payloads, map[string]any{"id": "latest-" + account, "name": "latest.json", "data": data})
		}
		return assignSyncTestResult(result, map[string]any{"status": "ok", "payloads": payloads})
	default:
		return fmt.Errorf("unexpected sync method %s", method)
	}
}
func (h *isolatedSyncPluginHost) payload(account string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.payloads[account]
}
func (h *isolatedSyncPluginHost) lastPulledAccount() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastPull
}

func assignSyncTestResult(target any, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

type syncTestPluginHost struct{}

func (syncTestPluginHost) Discover(context.Context) error                       { return nil }
func (syncTestPluginHost) Call(context.Context, string, string, any, any) error { return nil }
func (syncTestPluginHost) Close() error                                         { return nil }
func (syncTestPluginHost) GetPluginIDs() []string                               { return []string{"sync-settings-google-drive"} }
func (syncTestPluginHost) GetPlugin(string) (*core.Plugin, bool)                { return nil, false }
func (syncTestPluginHost) ListPlugins() []plugins.PluginInfo                    { return nil }
func (syncTestPluginHost) GetPluginIDsProviding(method string) []string {
	if method == "sync.push" {
		return []string{"sync-settings-google-drive"}
	}
	return nil
}

type syncTestLogger struct{}

func (syncTestLogger) Debug(string, ...any)        {}
func (syncTestLogger) Info(string, ...any)         {}
func (syncTestLogger) Warn(string, ...any)         {}
func (syncTestLogger) Error(string, error, ...any) {}

type syncProfileRepo struct{ items map[string]*core.Profile }

func (r *syncProfileRepo) Create(_ context.Context, p *core.Profile) error {
	copy := *p
	r.items[p.ID] = &copy
	return nil
}
func (r *syncProfileRepo) Update(_ context.Context, p *core.Profile) error {
	copy := *p
	r.items[p.ID] = &copy
	return nil
}
func (r *syncProfileRepo) Delete(_ context.Context, id string) error { delete(r.items, id); return nil }
func (r *syncProfileRepo) List(context.Context) ([]*core.Profile, error) {
	out := make([]*core.Profile, 0, len(r.items))
	for _, p := range r.items {
		copy := *p
		out = append(out, &copy)
	}
	return out, nil
}
func (r *syncProfileRepo) GetByID(_ context.Context, id string) (*core.Profile, error) {
	p := r.items[id]
	if p == nil {
		return nil, nil
	}
	copy := *p
	return &copy, nil
}
func (r *syncProfileRepo) Count(context.Context) (int, error) { return len(r.items), nil }
func (r *syncProfileRepo) CountAdmins(context.Context) (int, error) {
	count := 0
	for _, p := range r.items {
		if p.Role == core.ProfileRoleAdminPlayer {
			count++
		}
	}
	return count, nil
}
func (r *syncProfileRepo) EnsureDefaultForExistingData(context.Context) (*core.Profile, error) {
	return nil, nil
}

type syncIntegrationRepo struct{ items []*core.Integration }

func (r *syncIntegrationRepo) visible(ctx context.Context) []*core.Integration {
	owner := core.ProfileIDFromContext(ctx)
	out := []*core.Integration{}
	for _, item := range r.items {
		if item.ProfileID == owner {
			copy := *item
			out = append(out, &copy)
		}
	}
	return out
}
func (r *syncIntegrationRepo) Create(ctx context.Context, integration *core.Integration) error {
	owner := core.ProfileIDFromContext(ctx)
	if owner == "" {
		return core.ErrProfileRequired
	}
	if integration.ProfileID != "" && integration.ProfileID != owner {
		return core.ErrProfileForbidden
	}
	copy := *integration
	copy.ProfileID = owner
	r.items = append(r.items, &copy)
	return nil
}
func (r *syncIntegrationRepo) Update(ctx context.Context, integration *core.Integration) error {
	owner := core.ProfileIDFromContext(ctx)
	if owner == "" {
		return core.ErrProfileRequired
	}
	if integration.ProfileID != owner {
		return core.ErrProfileForbidden
	}
	for index, item := range r.items {
		if item.ID == integration.ID && item.ProfileID == owner {
			copy := *integration
			r.items[index] = &copy
			return nil
		}
	}
	return nil
}
func (r *syncIntegrationRepo) Delete(context.Context, string) error { return nil }
func (r *syncIntegrationRepo) List(ctx context.Context) ([]*core.Integration, error) {
	return r.visible(ctx), nil
}
func (r *syncIntegrationRepo) GetByID(ctx context.Context, id string) (*core.Integration, error) {
	for _, item := range r.visible(ctx) {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, nil
}
func (r *syncIntegrationRepo) ListByPluginID(ctx context.Context, pluginID string) ([]*core.Integration, error) {
	out := []*core.Integration{}
	for _, item := range r.visible(ctx) {
		if item.PluginID == pluginID {
			out = append(out, item)
		}
	}
	return out, nil
}

type syncSettingRepo struct{ items []*core.Setting }

func (r *syncSettingRepo) visible(ctx context.Context) []*core.Setting {
	owner := core.ProfileIDFromContext(ctx)
	out := []*core.Setting{}
	for _, item := range r.items {
		if item.ProfileID == owner {
			copy := *item
			out = append(out, &copy)
		}
	}
	return out
}
func (r *syncSettingRepo) Upsert(context.Context, *core.Setting) error { return nil }
func (r *syncSettingRepo) Get(ctx context.Context, key string) (*core.Setting, error) {
	for _, item := range r.visible(ctx) {
		if item.Key == key {
			return item, nil
		}
	}
	return nil, nil
}
func (r *syncSettingRepo) List(ctx context.Context) ([]*core.Setting, error) {
	return r.visible(ctx), nil
}
