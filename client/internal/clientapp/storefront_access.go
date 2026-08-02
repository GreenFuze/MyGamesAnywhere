package clientapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/google/uuid"
)

const storefrontGrantCatalogSchemaVersion = 1

type StorefrontGrant struct {
	BindingID string    `json:"binding_id"`
	Provider  string    `json:"provider"`
	ProductID string    `json:"product_id"`
	GrantedAt time.Time `json:"granted_at"`
}

type storefrontGrantDocument struct {
	SchemaVersion int               `json:"schema_version"`
	Grants        []StorefrontGrant `json:"grants"`
}

type StorefrontGrantCatalog struct {
	mu   sync.RWMutex
	path string
	doc  storefrontGrantDocument
}

func OpenStorefrontGrantCatalog(path string) (*StorefrontGrantCatalog, error) {
	if !filepath.IsAbs(strings.TrimSpace(path)) {
		return nil, errors.New("storefront grant catalog path must be absolute")
	}
	catalog := &StorefrontGrantCatalog{path: path, doc: storefrontGrantDocument{SchemaVersion: storefrontGrantCatalogSchemaVersion, Grants: []StorefrontGrant{}}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := catalog.persistLocked(); err != nil {
			return nil, err
		}
		return catalog, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &catalog.doc); err != nil {
		return nil, fmt.Errorf("decode storefront grant catalog: %w", err)
	}
	if err := catalog.doc.validate(); err != nil {
		return nil, err
	}
	return catalog, nil
}

func (d storefrontGrantDocument) validate() error {
	if d.SchemaVersion != storefrontGrantCatalogSchemaVersion {
		return fmt.Errorf("unsupported storefront grant catalog schema %d", d.SchemaVersion)
	}
	seen := map[string]bool{}
	for index, grant := range d.Grants {
		if _, err := uuid.Parse(grant.BindingID); err != nil || grant.GrantedAt.IsZero() {
			return fmt.Errorf("storefront grant %d has invalid binding or timestamp", index)
		}
		if err := (devicev1.StorefrontLaunchRequest{GameID: "catalog", SourceGameID: "catalog", Provider: grant.Provider, ProductID: grant.ProductID}).Validate(); err != nil {
			return fmt.Errorf("storefront grant %d: %w", index, err)
		}
		key := storefrontGrantKey(grant.BindingID, grant.Provider, grant.ProductID)
		if seen[key] {
			return fmt.Errorf("duplicate storefront grant %q", key)
		}
		seen[key] = true
	}
	return nil
}

func (c *StorefrontGrantCatalog) Has(bindingID, provider, productID string) bool {
	if c == nil {
		return false
	}
	key := storefrontGrantKey(bindingID, provider, productID)
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, grant := range c.doc.Grants {
		if storefrontGrantKey(grant.BindingID, grant.Provider, grant.ProductID) == key {
			return true
		}
	}
	return false
}

func (c *StorefrontGrantCatalog) Grant(bindingID, provider, productID string, grantedAt time.Time) error {
	grant := StorefrontGrant{BindingID: strings.TrimSpace(bindingID), Provider: strings.TrimSpace(provider), ProductID: strings.TrimSpace(productID), GrantedAt: grantedAt.UTC()}
	document := storefrontGrantDocument{SchemaVersion: storefrontGrantCatalogSchemaVersion, Grants: []StorefrontGrant{grant}}
	if err := document.validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := storefrontGrantKey(bindingID, provider, productID)
	for _, existing := range c.doc.Grants {
		if storefrontGrantKey(existing.BindingID, existing.Provider, existing.ProductID) == key {
			return nil
		}
	}
	c.doc.Grants = append(c.doc.Grants, grant)
	sort.Slice(c.doc.Grants, func(i, j int) bool {
		return storefrontGrantKey(c.doc.Grants[i].BindingID, c.doc.Grants[i].Provider, c.doc.Grants[i].ProductID) < storefrontGrantKey(c.doc.Grants[j].BindingID, c.doc.Grants[j].Provider, c.doc.Grants[j].ProductID)
	})
	return c.persistLocked()
}

func (c *StorefrontGrantCatalog) persistLocked() error {
	if err := c.doc.validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c.doc, "", "  ")
	if err != nil {
		return err
	}
	temporary := c.path + ".tmp"
	if err := os.WriteFile(temporary, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, c.path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func storefrontGrantKey(bindingID, provider, productID string) string {
	return strings.ToLower(strings.TrimSpace(bindingID) + ":" + strings.TrimSpace(provider) + ":" + strings.TrimSpace(productID))
}

type StorefrontProductObserver interface {
	Observe(context.Context, []devicev1.StorefrontProductCandidate) ([]devicev1.StorefrontProductObservation, error)
}

type StorefrontRouteLauncher interface {
	Launch(context.Context, string, string) error
}

type LocalStorefrontAccess struct {
	bindingID string
	serverURL string
	catalog   *StorefrontGrantCatalog
	observer  StorefrontProductObserver
	launcher  StorefrontRouteLauncher
	confirmer UseExistingConfirmer
	now       func() time.Time
}

func NewLocalStorefrontAccess(bindingID, serverURL string, catalog *StorefrontGrantCatalog) (*LocalStorefrontAccess, error) {
	if _, err := uuid.Parse(strings.TrimSpace(bindingID)); err != nil || strings.TrimSpace(serverURL) == "" || catalog == nil {
		return nil, errors.New("binding, server URL, and storefront grant catalog are required")
	}
	return &LocalStorefrontAccess{
		bindingID: bindingID, serverURL: serverURL, catalog: catalog,
		observer: newStorefrontProductObserver(), launcher: newStorefrontRouteLauncher(),
		confirmer: desktopUseExistingConfirmer{}, now: time.Now,
	}, nil
}

func (a *LocalStorefrontAccess) Observe(ctx context.Context, candidates []devicev1.StorefrontProductCandidate) ([]devicev1.StorefrontProductObservation, error) {
	request := devicev1.InventoryRefreshRequest{StorefrontCandidates: candidates}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return a.observer.Observe(ctx, candidates)
}

func (a *LocalStorefrontAccess) Use(ctx context.Context, request devicev1.UseStorefrontProductRequest) (devicev1.UseStorefrontProductResult, error) {
	var result devicev1.UseStorefrontProductResult
	if a == nil || a.catalog == nil || a.observer == nil || a.confirmer == nil || a.now == nil {
		return result, errors.New("storefront access is unavailable")
	}
	if err := request.Validate(); err != nil {
		return result, err
	}
	observed, err := a.observer.Observe(ctx, []devicev1.StorefrontProductCandidate{request.StorefrontProductCandidate})
	if err != nil {
		return result, err
	}
	if len(observed) != 1 {
		return result, errors.New("the storefront no longer reports this game as installed")
	}
	if !a.catalog.Has(a.bindingID, request.Provider, request.ProductID) {
		approved, err := a.confirmer.Confirm(ctx, request.Title, observed[0].InstallPath, a.serverURL)
		if err != nil {
			return result, err
		}
		if !approved {
			return result, ErrUseExistingDeclined
		}
		if err := a.catalog.Grant(a.bindingID, request.Provider, request.ProductID, a.now().UTC()); err != nil {
			return result, err
		}
	}
	result = devicev1.UseStorefrontProductResult{StorefrontProductObservation: observed[0], GrantedAt: a.now().UTC()}
	return result, result.Validate()
}

func (a *LocalStorefrontAccess) Launch(ctx context.Context, request devicev1.StorefrontLaunchRequest) (devicev1.StorefrontLaunchResult, error) {
	var result devicev1.StorefrontLaunchResult
	if a == nil || a.catalog == nil || a.observer == nil || a.launcher == nil || a.now == nil {
		return result, errors.New("storefront launch is unavailable")
	}
	if err := request.Validate(); err != nil {
		return result, err
	}
	if !a.catalog.Has(a.bindingID, request.Provider, request.ProductID) {
		return result, errors.New("this MGA Server has no local launch grant for the storefront game")
	}
	candidate := devicev1.StorefrontProductCandidate{GameID: request.GameID, SourceGameID: request.SourceGameID, Provider: request.Provider, ProductID: request.ProductID, Title: "Storefront game"}
	observed, err := a.observer.Observe(ctx, []devicev1.StorefrontProductCandidate{candidate})
	if err != nil {
		return result, err
	}
	if len(observed) != 1 {
		return result, errors.New("the storefront no longer reports this game as installed")
	}
	if err := a.launcher.Launch(ctx, request.Provider, request.ProductID); err != nil {
		return result, err
	}
	result = devicev1.StorefrontLaunchResult{GameID: request.GameID, SourceGameID: request.SourceGameID, Provider: request.Provider, ProductID: request.ProductID, StartedAt: a.now().UTC()}
	return result, result.Validate()
}
