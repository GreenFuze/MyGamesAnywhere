package v1

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	InstallKindSharedExisting    = "shared_existing"
	InstallationAuthorityManaged = "managed"
	InstallationAuthorityShared  = "shared_launch"
	maxExistingLaunchCandidates  = 64
)

type UseExistingInstallationRequest struct {
	LocalInstallationID string `json:"local_installation_id"`
	GameID              string `json:"game_id"`
	SourceGameID        string `json:"source_game_id"`
	Title               string `json:"title"`
}

func (r UseExistingInstallationRequest) Validate() error {
	if strings.TrimSpace(r.LocalInstallationID) == "" || strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" {
		return errors.New("local installation, game, and source IDs are required")
	}
	if strings.TrimSpace(r.Title) == "" || len(r.Title) > 256 {
		return errors.New("title is required and must not exceed 256 characters")
	}
	return nil
}

type UseExistingInstallationResult struct {
	LocalInstallationID string                     `json:"local_installation_id"`
	GameID              string                     `json:"game_id"`
	SourceGameID        string                     `json:"source_game_id"`
	InstallRoot         string                     `json:"install_root"`
	InstallPath         string                     `json:"install_path"`
	LaunchTarget        string                     `json:"launch_target"`
	LaunchCandidates    []string                   `json:"launch_candidates"`
	NativeProducts      []NativeProductObservation `json:"native_products,omitempty"`
	GrantedAt           time.Time                  `json:"granted_at"`
}

func (r UseExistingInstallationResult) Validate() error {
	if err := (UseExistingInstallationRequest{LocalInstallationID: r.LocalInstallationID, GameID: r.GameID, SourceGameID: r.SourceGameID, Title: "result"}).Validate(); err != nil {
		return err
	}
	if !filepath.IsAbs(strings.TrimSpace(r.InstallRoot)) || !filepath.IsAbs(strings.TrimSpace(r.InstallPath)) {
		return errors.New("absolute install root and path are required")
	}
	inside, err := filepath.Rel(filepath.Clean(r.InstallRoot), filepath.Clean(r.InstallPath))
	if err != nil || inside == "." || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return errors.New("install path must be a child of install root")
	}
	if err := ValidateLaunchTarget(r.LaunchTarget); err != nil {
		return fmt.Errorf("launch_target: %w", err)
	}
	if len(r.LaunchCandidates) == 0 || len(r.LaunchCandidates) > maxExistingLaunchCandidates {
		return fmt.Errorf("launch_candidates must contain 1 to %d items", maxExistingLaunchCandidates)
	}
	found := false
	seen := map[string]bool{}
	for index, candidate := range r.LaunchCandidates {
		if err := ValidateLaunchTarget(candidate); err != nil {
			return fmt.Errorf("launch_candidates[%d]: %w", index, err)
		}
		normalized := strings.ToLower(NormalizeLaunchTarget(candidate))
		if seen[normalized] {
			return fmt.Errorf("duplicate launch candidate %q", candidate)
		}
		seen[normalized] = true
		found = found || strings.EqualFold(normalized, NormalizeLaunchTarget(r.LaunchTarget))
	}
	if !found {
		return errors.New("launch target is not one of the launch candidates")
	}
	if r.GrantedAt.IsZero() {
		return errors.New("granted_at is required")
	}
	for index, product := range r.NativeProducts {
		if err := product.Validate(); err != nil {
			return fmt.Errorf("native_products[%d]: %w", index, err)
		}
	}
	return nil
}
