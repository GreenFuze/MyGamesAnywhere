package v1

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	StorefrontProviderSteam = "steam"
	maxStorefrontCandidates = 4096
)

var steamProductIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)

// InventoryRefreshRequest optionally asks the client to observe an exact,
// profile-owned candidate set while collecting its ordinary bounded inventory.
type InventoryRefreshRequest struct {
	StorefrontCandidates []StorefrontProductCandidate `json:"storefront_candidates,omitempty"`
}

func (r InventoryRefreshRequest) Validate() error {
	if len(r.StorefrontCandidates) > maxStorefrontCandidates {
		return fmt.Errorf("storefront_candidates exceeds %d items", maxStorefrontCandidates)
	}
	seen := map[string]bool{}
	for index, candidate := range r.StorefrontCandidates {
		if err := candidate.Validate(); err != nil {
			return fmt.Errorf("storefront_candidates[%d]: %w", index, err)
		}
		key := strings.ToLower(candidate.SourceGameID + ":" + candidate.Provider + ":" + candidate.ProductID)
		if seen[key] {
			return fmt.Errorf("duplicate storefront candidate %q", candidate.SourceGameID)
		}
		seen[key] = true
	}
	return nil
}

type StorefrontProductCandidate struct {
	GameID       string `json:"game_id"`
	SourceGameID string `json:"source_game_id"`
	Provider     string `json:"provider"`
	ProductID    string `json:"product_id"`
	Title        string `json:"title"`
}

func (c StorefrontProductCandidate) Validate() error {
	if strings.TrimSpace(c.GameID) == "" || strings.TrimSpace(c.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	if strings.TrimSpace(c.Title) == "" || len(c.Title) > 256 {
		return errors.New("title is required and must not exceed 256 characters")
	}
	return validateStorefrontIdentity(c.Provider, c.ProductID)
}

type StorefrontProductObservation struct {
	StorefrontProductCandidate
	InstallPath string    `json:"install_path"`
	ObservedAt  time.Time `json:"observed_at"`
}

func (o StorefrontProductObservation) Validate() error {
	if err := o.StorefrontProductCandidate.Validate(); err != nil {
		return err
	}
	if !filepath.IsAbs(strings.TrimSpace(o.InstallPath)) {
		return errors.New("storefront install_path must be absolute")
	}
	if o.ObservedAt.IsZero() {
		return errors.New("storefront observed_at is required")
	}
	return nil
}

type UseStorefrontProductRequest struct {
	StorefrontProductCandidate
}

func (r UseStorefrontProductRequest) Validate() error { return r.StorefrontProductCandidate.Validate() }

type UseStorefrontProductResult struct {
	StorefrontProductObservation
	GrantedAt time.Time `json:"granted_at"`
}

func (r UseStorefrontProductResult) Validate() error {
	if err := r.StorefrontProductObservation.Validate(); err != nil {
		return err
	}
	if r.GrantedAt.IsZero() {
		return errors.New("storefront granted_at is required")
	}
	return nil
}

type StorefrontLaunchRequest struct {
	GameID       string `json:"game_id"`
	SourceGameID string `json:"source_game_id"`
	Provider     string `json:"provider"`
	ProductID    string `json:"product_id"`
}

func (r StorefrontLaunchRequest) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("game_id and source_game_id are required")
	}
	return validateStorefrontIdentity(r.Provider, r.ProductID)
}

type StorefrontLaunchResult struct {
	GameID       string    `json:"game_id"`
	SourceGameID string    `json:"source_game_id"`
	Provider     string    `json:"provider"`
	ProductID    string    `json:"product_id"`
	StartedAt    time.Time `json:"started_at"`
}

func (r StorefrontLaunchResult) Validate() error {
	if err := (StorefrontLaunchRequest{GameID: r.GameID, SourceGameID: r.SourceGameID, Provider: r.Provider, ProductID: r.ProductID}).Validate(); err != nil {
		return err
	}
	if r.StartedAt.IsZero() {
		return errors.New("storefront started_at is required")
	}
	return nil
}

func validateStorefrontIdentity(provider, productID string) error {
	provider = strings.TrimSpace(provider)
	productID = strings.TrimSpace(productID)
	if provider != StorefrontProviderSteam {
		return fmt.Errorf("unsupported storefront provider %q", provider)
	}
	if !steamProductIDPattern.MatchString(productID) {
		return errors.New("Steam product_id must be a bounded numeric App ID")
	}
	return nil
}
