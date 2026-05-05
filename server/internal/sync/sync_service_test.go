package sync

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/crypto"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/keystore"
)

type memoryKeyStore struct {
	key string
}

func (k *memoryKeyStore) Store(passphrase string) error {
	k.key = passphrase
	return nil
}

func (k *memoryKeyStore) Load() (string, error) {
	if k.key == "" {
		return "", keystore.ErrNoKey
	}
	return k.key, nil
}

func (k *memoryKeyStore) Clear() error {
	k.key = ""
	return nil
}

func (k *memoryKeyStore) HasKey() bool {
	return k.key != ""
}

func TestStoreKeyRequiresCurrentPassphraseWhenReplacingExistingKey(t *testing.T) {
	ks := &memoryKeyStore{}
	svc := &syncService{keyStore: ks}

	if err := svc.StoreKey(context.Background(), "first", ""); err != nil {
		t.Fatalf("StoreKey initial: %v", err)
	}
	if ks.key != "first" {
		t.Fatalf("stored key = %q, want first", ks.key)
	}

	if err := svc.StoreKey(context.Background(), "second", ""); !errors.Is(err, core.ErrSyncKeyCurrentRequired) {
		t.Fatalf("StoreKey without current error = %v, want ErrSyncKeyCurrentRequired", err)
	}
	if ks.key != "first" {
		t.Fatalf("stored key changed without current passphrase: %q", ks.key)
	}

	if err := svc.StoreKey(context.Background(), "second", "wrong"); !errors.Is(err, core.ErrSyncKeyCurrentIncorrect) {
		t.Fatalf("StoreKey wrong current error = %v, want ErrSyncKeyCurrentIncorrect", err)
	}
	if ks.key != "first" {
		t.Fatalf("stored key changed after wrong current passphrase: %q", ks.key)
	}

	if err := svc.StoreKey(context.Background(), "second", "first"); err != nil {
		t.Fatalf("StoreKey with current: %v", err)
	}
	if ks.key != "second" {
		t.Fatalf("stored key = %q, want second", ks.key)
	}
}

func TestStoreKeyNoopsWhenSamePassphraseIsStored(t *testing.T) {
	ks := &memoryKeyStore{key: "same"}
	svc := &syncService{keyStore: ks}

	if err := svc.StoreKey(context.Background(), "same", ""); err != nil {
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
