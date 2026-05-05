package sync

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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
