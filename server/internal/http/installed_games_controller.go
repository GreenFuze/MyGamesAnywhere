package http

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
)

type InstalledGamesDeviceDTO struct {
	ID          string               `json:"id"`
	DisplayName string               `json:"display_name"`
	Status      string               `json:"status"`
	Connected   bool                 `json:"connected"`
	AccessLevel devicev1.AccessLevel `json:"access_level"`
}

type InstalledGameDTO struct {
	Game            GameDetailResponse `json:"game"`
	SourceGameID    string             `json:"source_game_id"`
	InstallKind     string             `json:"install_kind"`
	InstallState    string             `json:"install_state"`
	LaunchTarget    string             `json:"launch_target,omitempty"`
	LaunchSupported bool               `json:"launch_supported"`
	CanPlay         bool               `json:"can_play"`
	InstalledAt     time.Time          `json:"installed_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type InstalledGamesResponse struct {
	Device         InstalledGamesDeviceDTO `json:"device"`
	Games          []InstalledGameDTO      `json:"games"`
	AttentionCount int                     `json:"attention_count"`
}

// ListInstalledGames returns the installed, canonicalized game shelf for one
// explicitly selected endpoint owned by the active profile.
func (c *GameController) ListInstalledGames(w http.ResponseWriter, r *http.Request) {
	endpointID, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if endpointID == "" {
		http.Error(w, "device id is required", http.StatusBadRequest)
		return
	}
	if c == nil || c.deviceLister == nil || c.gameStore == nil {
		http.Error(w, "installed games are unavailable", http.StatusServiceUnavailable)
		return
	}
	profileID := core.ProfileIDFromContext(r.Context())
	if profileID == "" {
		http.Error(w, "profile is required", http.StatusUnauthorized)
		return
	}

	endpoints, err := c.deviceLister.ListEndpoints(r.Context(), profileID)
	if err != nil {
		c.logger.Error("list devices for installed games", err, "profile_id", profileID)
		http.Error(w, "could not list devices", http.StatusInternalServerError)
		return
	}
	var endpoint *devices.Endpoint
	for index := range endpoints {
		if endpoints[index].ID == endpointID {
			endpoint = &endpoints[index]
			break
		}
	}
	if endpoint == nil {
		http.NotFound(w, r)
		return
	}

	response, err := c.buildInstalledGamesResponse(r.Context(), *endpoint)
	if err != nil {
		c.logger.Error("build installed games shelf", err, "endpoint_id", endpointID, "profile_id", profileID)
		http.Error(w, "could not load installed games", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (c *GameController) buildInstalledGamesResponse(ctx context.Context, endpoint devices.Endpoint) (InstalledGamesResponse, error) {
	selected := make(map[string]devices.GameInstallation)
	attentionGames := make(map[string]struct{})
	for _, installation := range endpoint.Installations {
		if installation.InstallState != devicev1.InstallStateInstalled {
			attentionGames[installation.GameID] = struct{}{}
			continue
		}
		current, exists := selected[installation.GameID]
		if !exists || preferInstalledShelfCopy(installation, current) {
			selected[installation.GameID] = installation
		}
	}

	canonicalIDs := make([]string, 0, len(selected))
	for gameID := range selected {
		canonicalIDs = append(canonicalIDs, gameID)
	}
	sort.Strings(canonicalIDs)
	games, err := c.gameStore.GetCanonicalGamesByIDs(ctx, canonicalIDs)
	if err != nil {
		return InstalledGamesResponse{}, err
	}
	canonicalByID := make(map[string]*core.CanonicalGame, len(games))
	for _, game := range games {
		if game != nil {
			canonicalByID[game.ID] = game
		}
	}

	connected := endpointIsConnected(endpoint.Status)
	launchSupported := endpointHasCapability(endpoint, devicev1.CapabilityGameLaunch)
	playAccess, _ := endpoint.AccessLevel.Allows(devicev1.AccessPlay)
	compatibleConnection := endpoint.Status == devicev1.EndpointReady || endpoint.Status == devicev1.EndpointBusy
	labels := c.loadIntegrationLabels(ctx)
	items := make([]InstalledGameDTO, 0, len(selected))
	for gameID, installation := range selected {
		game := canonicalByID[gameID]
		if game == nil {
			continue
		}
		items = append(items, InstalledGameDTO{
			Game:            c.canonicalToGameDetailWithIntegrationLabels(ctx, game, labels),
			SourceGameID:    installation.SourceGameID,
			InstallKind:     installation.InstallKind,
			InstallState:    installation.InstallState,
			LaunchTarget:    installation.LaunchTarget,
			LaunchSupported: launchSupported,
			CanPlay:         playAccess && compatibleConnection && launchSupported && strings.TrimSpace(installation.LaunchTarget) != "",
			InstalledAt:     installation.InstalledAt.UTC(),
			UpdatedAt:       installation.UpdatedAt.UTC(),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := normalizedPlayerTitle(items[i].Game.Title)
		right := normalizedPlayerTitle(items[j].Game.Title)
		if left != right {
			return left < right
		}
		return items[i].Game.ID < items[j].Game.ID
	})

	return InstalledGamesResponse{
		Device: InstalledGamesDeviceDTO{
			ID: endpoint.ID, DisplayName: endpoint.DisplayName, Status: string(endpoint.Status),
			Connected: connected, AccessLevel: endpoint.AccessLevel,
		},
		Games:          items,
		AttentionCount: len(attentionGames),
	}, nil
}

func preferInstalledShelfCopy(candidate, current devices.GameInstallation) bool {
	candidateHasTarget := strings.TrimSpace(candidate.LaunchTarget) != ""
	currentHasTarget := strings.TrimSpace(current.LaunchTarget) != ""
	if candidateHasTarget != currentHasTarget {
		return candidateHasTarget
	}
	if !candidate.UpdatedAt.Equal(current.UpdatedAt) {
		return candidate.UpdatedAt.After(current.UpdatedAt)
	}
	return candidate.SourceGameID < current.SourceGameID
}

func endpointIsConnected(status devicev1.EndpointState) bool {
	switch status {
	case devicev1.EndpointReady, devicev1.EndpointBusy, devicev1.EndpointUpdateRequired, devicev1.EndpointError:
		return true
	default:
		return false
	}
}

func endpointHasCapability(endpoint devices.Endpoint, capability string) bool {
	for _, current := range endpoint.Capabilities {
		if current == capability {
			return true
		}
	}
	return false
}

func normalizedPlayerTitle(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
