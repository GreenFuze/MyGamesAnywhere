package http

import (
	"context"
	"net/url"
	"strings"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/emulation"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/savedomain"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcegames"
)

// GameDetailResponse is the body for GET /api/games/{id}/detail.
type GameDetailResponse struct {
	ID                 string                      `json:"id"`
	Title              string                      `json:"title"`
	Favorite           bool                        `json:"favorite"`
	Platform           string                      `json:"platform"`
	Kind               string                      `json:"kind"`
	GroupKind          string                      `json:"group_kind,omitempty"`
	RootPath           string                      `json:"root_path,omitempty"`
	Files              []GameFileDTO               `json:"files,omitempty"`
	ExternalIDs        []ExternalIDDTO             `json:"external_ids,omitempty"`
	Description        string                      `json:"description,omitempty"`
	ReleaseDate        string                      `json:"release_date,omitempty"`
	Genres             []string                    `json:"genres,omitempty"`
	Developer          string                      `json:"developer,omitempty"`
	Publisher          string                      `json:"publisher,omitempty"`
	Rating             float64                     `json:"rating,omitempty"`
	MaxPlayers         int                         `json:"max_players,omitempty"`
	CompletionTime     *core.CompletionTime        `json:"completion_time,omitempty"`
	Media              []GameMediaDetailDTO        `json:"media,omitempty"`
	CoverOverride      *GameMediaDetailDTO         `json:"cover_override,omitempty"`
	HoverOverride      *GameMediaDetailDTO         `json:"hover_override,omitempty"`
	BackgroundOverride *GameMediaDetailDTO         `json:"background_override,omitempty"`
	IsGamePass         bool                        `json:"is_game_pass,omitempty"`
	XcloudAvailable    bool                        `json:"xcloud_available,omitempty"`
	StoreProductID     string                      `json:"store_product_id,omitempty"`
	XcloudURL          string                      `json:"xcloud_url,omitempty"`
	Play               *GamePlayDTO                `json:"play,omitempty"`
	AchievementSummary *AchievementSummaryDTO      `json:"achievement_summary,omitempty"`
	Identity           *core.GameIdentity          `json:"identity,omitempty"`
	Devices            []GameDeviceAvailabilityDTO `json:"devices,omitempty"`
	SourceGames        []SourceGameDetailDTO       `json:"source_games"`
	// MetadataWarnings lists metadata providers that were skipped during a forced refresh
	// due to non-fatal errors (e.g. timeout). Only present in refresh responses; empty on
	// regular game-detail reads.
	MetadataWarnings []string `json:"metadata_warnings,omitempty"`
}

type DeviceEndpointLister interface {
	ListEndpoints(context.Context, string) ([]devices.Endpoint, error)
}

type EmulatorConfigurationProvider interface {
	Get(context.Context, string, string) (emulation.DeviceConfiguration, error)
	GetForEndpoint(context.Context, devices.Endpoint, string) (emulation.DeviceConfiguration, error)
}

type deviceAvailabilityFacts struct {
	Endpoint  devices.Endpoint
	Emulators *emulation.DeviceConfiguration
}

type GameEmulatorRouteDTO struct {
	EmulatorID   string                 `json:"emulator_id"`
	EmulatorName string                 `json:"emulator_name"`
	CoreID       string                 `json:"core_id,omitempty"`
	SourceGameID string                 `json:"source_game_id"`
	SourceTitle  string                 `json:"source_title"`
	State        string                 `json:"state"`
	Reason       string                 `json:"reason,omitempty"`
	Default      bool                   `json:"default"`
	Save         *savedomain.Capability `json:"save,omitempty"`
}

type GameDeviceAvailabilityDTO struct {
	DeviceID                string                 `json:"device_id"`
	DisplayName             string                 `json:"display_name"`
	OSUser                  string                 `json:"os_user"`
	Status                  string                 `json:"status"`
	Connected               bool                   `json:"connected"`
	CanManage               bool                   `json:"can_manage"`
	CanPlay                 bool                   `json:"can_play"`
	PlatformSupported       bool                   `json:"platform_supported"`
	EmulatorRoutes          []GameEmulatorRouteDTO `json:"emulator_routes,omitempty"`
	FreeBytes               uint64                 `json:"free_bytes,omitempty"`
	TotalBytes              uint64                 `json:"total_bytes,omitempty"`
	InventoryCapturedAt     string                 `json:"inventory_captured_at,omitempty"`
	Installed               bool                   `json:"installed"`
	InstalledSourceID       string                 `json:"installed_source_id,omitempty"`
	InstalledSave           *savedomain.Capability `json:"installed_save,omitempty"`
	InstallPath             string                 `json:"install_path,omitempty"`
	ArchiveInstallSupported bool                   `json:"archive_install_supported"`
	GogInnoInstallSupported bool                   `json:"gog_inno_install_supported"`
	FailedCleanupSupported  bool                   `json:"failed_cleanup_supported"`
	UninstallSupported      bool                   `json:"uninstall_supported"`
	LaunchSupported         bool                   `json:"launch_supported"`
	InstallKind             string                 `json:"install_kind,omitempty"`
	InstallState            string                 `json:"install_state,omitempty"`
	StateReason             string                 `json:"state_reason,omitempty"`
	CleanupMarkerID         string                 `json:"cleanup_marker_id,omitempty"`
	CleanupIgnoredAt        string                 `json:"cleanup_ignored_at,omitempty"`
	LaunchTarget            string                 `json:"launch_target,omitempty"`
	LaunchCandidates        []string               `json:"launch_candidates,omitempty"`
}

func (c *GameController) attachDeviceAvailability(ctx context.Context, response *GameDetailResponse, game *core.CanonicalGame) {
	if c == nil || c.deviceLister == nil || response == nil || game == nil {
		return
	}
	profileID := core.ProfileIDFromContext(ctx)
	if profileID == "" {
		return
	}
	facts, err := c.loadDeviceAvailabilityFacts(ctx, profileID)
	if err != nil {
		c.logger.Warn("list devices for game availability failed", "error", err, "game_id", game.ID)
		return
	}
	c.attachDeviceAvailabilityWithFacts(response, game, facts)
}

func (c *GameController) loadDeviceAvailabilityFacts(ctx context.Context, profileID string) ([]deviceAvailabilityFacts, error) {
	endpoints, err := c.deviceLister.ListEndpoints(ctx, profileID)
	if err != nil {
		return nil, err
	}
	facts := make([]deviceAvailabilityFacts, 0, len(endpoints))
	for _, endpoint := range endpoints {
		fact := deviceAvailabilityFacts{Endpoint: endpoint}
		if c.emulation != nil && endpoint.Platform == "windows" {
			configuration, configurationErr := c.emulation.GetForEndpoint(ctx, endpoint, profileID)
			if configurationErr != nil {
				c.logger.Warn("resolve emulator routes failed", "device_id", endpoint.ID, "error", configurationErr)
			} else {
				fact.Emulators = &configuration
			}
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

func (c *GameController) attachDeviceAvailabilityWithFacts(response *GameDetailResponse, game *core.CanonicalGame, facts []deviceAvailabilityFacts) {
	for _, fact := range facts {
		endpoint := fact.Endpoint
		allowed, _ := endpoint.AccessLevel.Allows(devicev1.AccessManage)
		canPlay, _ := endpoint.AccessLevel.Allows(devicev1.AccessPlay)
		item := GameDeviceAvailabilityDTO{
			DeviceID:          endpoint.ID,
			DisplayName:       endpoint.DisplayName,
			OSUser:            endpoint.OSUser,
			Connected:         endpoint.Status == devicev1.EndpointReady || endpoint.Status == devicev1.EndpointBusy,
			CanManage:         allowed,
			CanPlay:           canPlay,
			PlatformSupported: game.Platform == core.PlatformWindowsPC && endpoint.Platform == "windows",
		}
		if fact.Emulators != nil {
			c.attachEmulatorRoutes(&item, *fact.Emulators, game)
		}
		for _, capability := range endpoint.Capabilities {
			switch capability {
			case devicev1.CapabilityGameInstallArchive:
				item.ArchiveInstallSupported = true
			case devicev1.CapabilityGameInstallGogInno:
				item.GogInnoInstallSupported = true
			case devicev1.CapabilityGameCleanupGogInnoFailed:
				item.FailedCleanupSupported = true
			case devicev1.CapabilityGameUninstall, devicev1.CapabilityGameUninstallGogInno:
				item.UninstallSupported = true
			case devicev1.CapabilityGameLaunch:
				item.LaunchSupported = true
			}
		}
		for _, installation := range endpoint.Installations {
			if installation.GameID == game.ID {
				item.Installed = true
				item.InstalledSourceID = installation.SourceGameID
				if source := saveDomainSource(game, installation.SourceGameID); source.SourceGameID != "" {
					save := c.saveDomainResolver().Installed(source, endpoint.ID)
					item.InstalledSave = &save
				}
				item.InstallPath = installation.InstallPath
				item.InstallKind = installation.InstallKind
				item.InstallState = installation.InstallState
				item.StateReason = installation.StateReason
				item.CleanupMarkerID = installation.CleanupMarkerID
				if installation.CleanupIgnoredAt != nil {
					item.CleanupIgnoredAt = installation.CleanupIgnoredAt.UTC().Format(time.RFC3339Nano)
				}
				item.LaunchTarget = installation.LaunchTarget
				item.LaunchCandidates = installation.LaunchCandidates
				break
			}
		}
		switch {
		case item.Installed && item.InstallState != devicev1.InstallStateInstalled:
			item.Status = item.InstallState
		case item.Installed:
			item.Status = "installed"
		case endpoint.Status == devicev1.EndpointUpdateRequired:
			item.Status = "update_required"
		case !item.Connected:
			item.Status = "offline"
		case !item.PlatformSupported:
			item.Status = "unsupported"
		case endpoint.Inventory == nil:
			item.Status = "not_scanned"
		default:
			item.InventoryCapturedAt = endpoint.Inventory.CapturedAt.UTC().Format(time.RFC3339Nano)
			for _, storage := range endpoint.Inventory.Storage {
				item.FreeBytes += storage.FreeBytes
				item.TotalBytes += storage.TotalBytes
			}
			if game.Platform == core.PlatformWindowsPC {
				item.Status = "ready_for_setup"
			} else if hasReadyEmulatorRoute(item.EmulatorRoutes) {
				item.Status = "ready_to_play"
			} else {
				item.Status = "needs_setup"
			}
		}
		response.Devices = append(response.Devices, item)
	}
}

func (c *GameController) attachEmulatorRoutes(item *GameDeviceAvailabilityDTO, configuration emulation.DeviceConfiguration, game *core.CanonicalGame) {
	if item == nil || game == nil {
		return
	}
	for _, platform := range configuration.Platforms {
		if platform.Platform != game.Platform {
			continue
		}
		item.PlatformSupported = true
		defaultAssigned := false
		for _, option := range platform.Emulators {
			for _, source := range game.SourceGames {
				if source == nil || source.Status != "found" || source.GroupKind != core.GroupKindSelfContained || len(source.Files) == 0 {
					continue
				}
				route := GameEmulatorRouteDTO{
					EmulatorID: option.ID, EmulatorName: option.Name, SourceGameID: source.ID, SourceTitle: source.RawTitle,
					CoreID: option.ResolvedCore, State: option.State, Reason: option.Reason,
				}
				save := c.saveDomainResolver().Emulator(savedomain.Source{
					SourceGameID: source.ID, PluginID: source.PluginID,
				}, item.DeviceID, option.ID, option.ResolvedCore)
				route.Save = &save
				if route.State == "ready" && !supportsEmulatorContentSource(source, c.emulatorContentRoot) {
					route.State = "needs_setup"
					route.Reason = "Download this copy to the MGA Server before playing on a device"
				}
				if !defaultAssigned && option.ID == platform.ResolvedDefault && route.State == "ready" {
					route.Default = true
					defaultAssigned = true
				}
				item.EmulatorRoutes = append(item.EmulatorRoutes, route)
			}
		}
		return
	}
}

func hasReadyEmulatorRoute(routes []GameEmulatorRouteDTO) bool {
	for _, route := range routes {
		if route.State == "ready" {
			return true
		}
	}
	return false
}

func (c *GameController) saveDomainResolver() *savedomain.Resolver {
	if c != nil && c.saveDomains != nil {
		return c.saveDomains
	}
	return savedomain.NewResolver()
}

func saveDomainSource(game *core.CanonicalGame, sourceGameID string) savedomain.Source {
	if game == nil || strings.TrimSpace(sourceGameID) == "" {
		return savedomain.Source{}
	}
	for _, source := range game.SourceGames {
		if source != nil && source.ID == sourceGameID {
			return savedomain.Source{SourceGameID: source.ID, PluginID: source.PluginID}
		}
	}
	return savedomain.Source{}
}

type AchievementSummaryDTO struct {
	SourceCount   int `json:"source_count"`
	TotalCount    int `json:"total_count"`
	UnlockedCount int `json:"unlocked_count"`
	TotalPoints   int `json:"total_points,omitempty"`
	EarnedPoints  int `json:"earned_points,omitempty"`
}

// GameMediaDetailDTO is one media asset linked to the canonical game.
type GameMediaDetailDTO struct {
	AssetID   int    `json:"asset_id"`
	Type      string `json:"type"`
	URL       string `json:"url"`
	Source    string `json:"source,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	LocalPath string `json:"local_path,omitempty"`
	Hash      string `json:"hash,omitempty"`
	MimeType  string `json:"mime_type,omitempty"`
}

// SourceGameDetailDTO is one source row with resolver matches for the detail view.
type SourceGameDetailDTO struct {
	ID               string                   `json:"id"`
	IntegrationID    string                   `json:"integration_id"`
	IntegrationLabel string                   `json:"integration_label,omitempty"`
	PluginID         string                   `json:"plugin_id"`
	ExternalID       string                   `json:"external_id"`
	RawTitle         string                   `json:"raw_title"`
	Platform         string                   `json:"platform"`
	Kind             string                   `json:"kind"`
	GroupKind        string                   `json:"group_kind,omitempty"`
	RootPath         string                   `json:"root_path,omitempty"`
	URL              string                   `json:"url,omitempty"`
	Status           string                   `json:"status"`
	LastSeenAt       *string                  `json:"last_seen_at,omitempty"`
	CreatedAt        string                   `json:"created_at"`
	Files            []GameFileDTO            `json:"files"`
	Delivery         *SourceDeliveryDTO       `json:"delivery,omitempty"`
	Play             *SourceGamePlayDTO       `json:"play,omitempty"`
	Save             *savedomain.Capability   `json:"save,omitempty"`
	HardDelete       *SourceGameHardDeleteDTO `json:"hard_delete,omitempty"`
	CanonicalPin     *CanonicalSourcePin      `json:"canonical_pin,omitempty"`
	ResolverMatches  []core.ResolverMatch     `json:"resolver_matches"`
}

type SourceGameHardDeleteDTO struct {
	Eligible bool   `json:"eligible"`
	Reason   string `json:"reason,omitempty"`
}

type SourceDeliveryDTO struct {
	Profiles []SourceDeliveryProfileDTO `json:"profiles,omitempty"`
}

type SourceDeliveryProfileDTO struct {
	Profile         string `json:"profile"`
	Mode            string `json:"mode"`
	PrepareRequired bool   `json:"prepare_required,omitempty"`
	Ready           bool   `json:"ready,omitempty"`
	RootFileID      string `json:"root_file_id,omitempty"`
}

func canonicalToGameDetail(cg *core.CanonicalGame) GameDetailResponse {
	return (&GameController{}).canonicalToGameDetailWithIntegrationLabels(context.Background(), cg, nil)
}

func (c *GameController) canonicalToGameDetail(ctx context.Context, cg *core.CanonicalGame) GameDetailResponse {
	return c.canonicalToGameDetailWithIntegrationLabels(ctx, cg, nil)
}

func (c *GameController) canonicalToGameDetailWithIntegrationLabels(ctx context.Context, cg *core.CanonicalGame, integrationLabels map[string]string) GameDetailResponse {
	if cg == nil {
		return GameDetailResponse{SourceGames: []SourceGameDetailDTO{}}
	}
	out := GameDetailResponse{
		ID:              cg.ID,
		Title:           cg.Title,
		Favorite:        cg.Favorite,
		Platform:        string(cg.Platform),
		Kind:            string(cg.Kind),
		Description:     cg.Description,
		ReleaseDate:     cg.ReleaseDate,
		Genres:          cg.Genres,
		Developer:       cg.Developer,
		Publisher:       cg.Publisher,
		Rating:          cg.Rating,
		MaxPlayers:      cg.MaxPlayers,
		CompletionTime:  cg.CompletionTime,
		IsGamePass:      cg.IsGamePass,
		XcloudAvailable: cg.XcloudAvailable,
		StoreProductID:  cg.StoreProductID,
		XcloudURL:       cg.XcloudURL,
		Identity:        cg.Identity,
		Play: &GamePlayDTO{
			PlatformSupported: supportsBrowserPlayPlatform(cg.Platform),
		},
		SourceGames: make([]SourceGameDetailDTO, 0, len(cg.SourceGames)),
	}
	if cg.AchievementSummary != nil {
		out.AchievementSummary = &AchievementSummaryDTO{
			SourceCount:   cg.AchievementSummary.SourceCount,
			TotalCount:    cg.AchievementSummary.TotalCount,
			UnlockedCount: cg.AchievementSummary.UnlockedCount,
			TotalPoints:   cg.AchievementSummary.TotalPoints,
			EarnedPoints:  cg.AchievementSummary.EarnedPoints,
		}
	}
	if cg.CoverOverride != nil {
		cover := mediaRefToDTO(*cg.CoverOverride)
		out.CoverOverride = &cover
	}
	if cg.HoverOverride != nil {
		hover := mediaRefToDTO(*cg.HoverOverride)
		out.HoverOverride = &hover
	}
	if cg.BackgroundOverride != nil {
		background := mediaRefToDTO(*cg.BackgroundOverride)
		out.BackgroundOverride = &background
	}

	for _, sg := range cg.SourceGames {
		if sg == nil {
			continue
		}
		if out.GroupKind == "" && sg.Status == "found" {
			out.GroupKind = string(sg.GroupKind)
		}
		if out.RootPath == "" && sg.Status == "found" {
			out.RootPath = sg.RootPath
		}
		if sg.Status == "found" {
			for _, f := range sg.Files {
				out.Files = append(out.Files, GameFileDTO{
					ID:       encodeGameFileID(sg.ID, f.Path),
					Path:     f.Path,
					Role:     string(f.Role),
					FileKind: f.FileKind,
					Size:     f.Size,
				})
			}
		}
		sourceDTO, launchSource, launchCandidate := c.sourceGameToDetailDTO(ctx, sg, cg.Platform, out.Play.PlatformSupported, integrationLabels)
		if launchSource != nil {
			out.Play.LaunchSources = append(out.Play.LaunchSources, *launchSource)
			if launchSource.Launchable {
				out.Play.Available = true
			}
		}
		if launchSource != nil && launchSource.Launchable && launchCandidate != nil {
			out.Play.LaunchCandidates = append(out.Play.LaunchCandidates, *launchCandidate)
		}
		for _, option := range launchOptionsForSource(sourceDTO, launchSource, launchCandidate) {
			if option.Launchable {
				out.Play.Available = true
			}
			out.Play.Options = append(out.Play.Options, option)
		}
		out.SourceGames = append(out.SourceGames, sourceDTO)
	}

	for _, ref := range cg.Media {
		out.Media = append(out.Media, GameMediaDetailDTO{
			AssetID:   ref.AssetID,
			Type:      string(ref.Type),
			URL:       ref.URL,
			Source:    ref.Source,
			Width:     ref.Width,
			Height:    ref.Height,
			LocalPath: ref.LocalPath,
			Hash:      ref.Hash,
			MimeType:  ref.MimeType,
		})
	}

	for _, eid := range cg.ExternalIDs {
		out.ExternalIDs = append(out.ExternalIDs, ExternalIDDTO{
			Source:     eid.Source,
			ExternalID: eid.ExternalID,
			URL:        externalIDURL(cg, eid),
		})
	}
	return out
}

func externalIDURL(cg *core.CanonicalGame, eid core.ExternalID) string {
	if eid.Source == "metadata-launchbox" {
		// The numeric DatabaseID in LaunchBox's Metadata.xml does NOT correspond to
		// the ID used in gamesdb.launchbox-app.com/games/details/{id} URLs — the two
		// numbering systems are independent.  Always fall back to a title-based search
		// URL so we never link to the wrong game.
		if title := launchBoxTitleForExternalID(cg, eid.ExternalID); title != "" {
			return launchBoxSearchURL(title)
		}
		// No title available: return the stored URL as-is.
	}
	return eid.URL
}

func launchBoxTitleForExternalID(cg *core.CanonicalGame, externalID string) string {
	if cg == nil {
		return ""
	}
	for _, sg := range cg.SourceGames {
		if sg == nil {
			continue
		}
		for _, match := range sg.ResolverMatches {
			if match.PluginID == "metadata-launchbox" && match.ExternalID == externalID && strings.TrimSpace(match.Title) != "" {
				return strings.TrimSpace(match.Title)
			}
		}
	}
	return strings.TrimSpace(cg.Title)
}

func launchBoxSearchURL(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "https://gamesdb.launchbox-app.com/games/search"
	}
	return "https://gamesdb.launchbox-app.com/games/results?id=" + url.QueryEscape(title)
}

func launchOptionsForSource(source SourceGameDetailDTO, launchSource *GameLaunchSourceDTO, launchCandidate *GameLaunchCandidateDTO) []GameLaunchOptionDTO {
	resolver := savedomain.NewResolver()
	saveSource := savedomain.Source{SourceGameID: source.ID, PluginID: source.PluginID, IntegrationLabel: source.IntegrationLabel}
	options := make([]GameLaunchOptionDTO, 0, 2)
	if launchSource != nil {
		option := GameLaunchOptionDTO{
			Kind:             "browser",
			SourceGameID:     source.ID,
			SourceTitle:      source.RawTitle,
			Platform:         source.Platform,
			PluginID:         source.PluginID,
			IntegrationID:    source.IntegrationID,
			IntegrationLabel: source.IntegrationLabel,
			Launchable:       launchSource.Launchable,
			RootFileID:       launchSource.RootFileID,
			Profile:          firstReadyDeliveryProfile(source.Delivery),
		}
		if launchSource.Launchable {
			browserSave := resolver.Browser(saveSource, source.Platform)
			option.Save = &browserSave
		}
		if launchCandidate != nil {
			option.FileID = launchCandidate.FileID
			option.Path = launchCandidate.Path
			option.FileKind = launchCandidate.FileKind
			option.Size = launchCandidate.Size
		}
		options = append(options, option)
	}

	seenXcloud := map[string]bool{}
	for _, match := range source.ResolverMatches {
		if match.Outvoted || (!match.XcloudAvailable && strings.TrimSpace(match.XcloudURL) == "") {
			continue
		}
		key := match.PluginID + "|" + match.XcloudURL
		if seenXcloud[key] {
			continue
		}
		seenXcloud[key] = true
		xcloudSave := resolver.XCloud(saveSource)
		options = append(options, GameLaunchOptionDTO{
			Kind:             "xcloud",
			SourceGameID:     source.ID,
			SourceTitle:      source.RawTitle,
			Platform:         source.Platform,
			PluginID:         match.PluginID,
			IntegrationID:    source.IntegrationID,
			IntegrationLabel: source.IntegrationLabel,
			Launchable:       strings.TrimSpace(match.XcloudURL) != "",
			URL:              match.XcloudURL,
			Save:             &xcloudSave,
		})
	}
	return options
}

func firstReadyDeliveryProfile(delivery *SourceDeliveryDTO) string {
	if delivery == nil {
		return ""
	}
	for _, profile := range delivery.Profiles {
		if profile.Ready && profile.Profile != "" {
			return profile.Profile
		}
	}
	for _, profile := range delivery.Profiles {
		if profile.Profile != "" {
			return profile.Profile
		}
	}
	return ""
}

func mediaRefToDTO(ref core.MediaRef) GameMediaDetailDTO {
	return GameMediaDetailDTO{
		AssetID:   ref.AssetID,
		Type:      string(ref.Type),
		URL:       ref.URL,
		Source:    ref.Source,
		Width:     ref.Width,
		Height:    ref.Height,
		LocalPath: ref.LocalPath,
		Hash:      ref.Hash,
		MimeType:  ref.MimeType,
	}
}

func (c *GameController) sourceGameToDetailDTO(
	ctx context.Context,
	sg *core.SourceGame,
	canonicalPlatform core.Platform,
	platformSupported bool,
	integrationLabels map[string]string,
) (SourceGameDetailDTO, *GameLaunchSourceDTO, *GameLaunchCandidateDTO) {
	dto := SourceGameDetailDTO{
		ID:               sg.ID,
		IntegrationID:    sg.IntegrationID,
		IntegrationLabel: integrationLabels[sg.IntegrationID],
		PluginID:         sg.PluginID,
		ExternalID:       sg.ExternalID,
		RawTitle:         sg.RawTitle,
		Platform:         string(sg.Platform),
		Kind:             string(sg.Kind),
		GroupKind:        string(sg.GroupKind),
		RootPath:         sg.RootPath,
		URL:              sg.URL,
		Status:           sg.Status,
		CreatedAt:        sg.CreatedAt.UTC().Format(time.RFC3339Nano),
		Files:            make([]GameFileDTO, 0, len(sg.Files)),
		CanonicalPin:     canonicalSourcePinDTO(sg.CanonicalPin),
		ResolverMatches:  resolverMatchesForDetail(sg.ResolverMatches),
	}
	save := c.saveDomainResolver().Source(savedomain.Source{
		SourceGameID: sg.ID, PluginID: sg.PluginID, IntegrationLabel: integrationLabels[sg.IntegrationID],
	})
	dto.Save = &save
	eligible, reason := sourcegames.HardDeleteEligibility(sg)
	dto.HardDelete = &SourceGameHardDeleteDTO{Eligible: eligible, Reason: reason}
	if sg.LastSeenAt != nil {
		s := sg.LastSeenAt.UTC().Format(time.RFC3339Nano)
		dto.LastSeenAt = &s
	}

	playSource := &GameLaunchSourceDTO{SourceGameID: sg.ID}
	dto.Play = &SourceGamePlayDTO{}
	dto.Delivery = &SourceDeliveryDTO{}
	rootPlatform := core.EffectiveBrowserPlayPlatform(sg.Platform, canonicalPlatform)
	var rootFileID string
	var rootCandidate *GameLaunchCandidateDTO
	for _, f := range sg.Files {
		fileID := encodeGameFileID(sg.ID, f.Path)
		dto.Files = append(dto.Files, GameFileDTO{
			ID:       fileID,
			Path:     f.Path,
			Role:     string(f.Role),
			FileKind: f.FileKind,
			Size:     f.Size,
		})
		if f.Role == core.GameFileRoleRoot && rootFileID == "" {
			rootFileID = fileID
			rootCandidate = &GameLaunchCandidateDTO{
				SourceGameID: sg.ID,
				FileID:       fileID,
				Path:         f.Path,
				FileKind:     f.FileKind,
				Size:         f.Size,
			}
		}
	}

	deliveryProfiles := c.describeSourceGameDelivery(ctx, canonicalPlatform, sg)
	for _, profile := range deliveryProfiles {
		profileDTO := SourceDeliveryProfileDTO{
			Profile:         profile.Profile,
			Mode:            string(profile.Mode),
			PrepareRequired: profile.PrepareRequired,
			Ready:           profile.Ready,
		}
		if rootFileID != "" {
			profileDTO.RootFileID = rootFileID
		}
		dto.Delivery.Profiles = append(dto.Delivery.Profiles, profileDTO)
	}

	launchable := false
	if sg.Status == "found" && platformSupported && sg.GroupKind == core.GroupKindSelfContained && len(sg.Files) > 0 {
		for _, profile := range deliveryProfiles {
			if profile.Mode == core.SourceDeliveryModeUnavailable {
				continue
			}
			launchable = rootFileID != ""
			if !launchable && rootPlatform == core.PlatformScummVM {
				launchable = supportsScummVMLaunchSource(sg.Files)
			}
			if launchable {
				break
			}
		}
	}
	dto.Play.Launchable = launchable
	dto.Play.RootFileID = rootFileID
	playSource.Launchable = launchable
	playSource.RootFileID = rootFileID

	if dto.ResolverMatches == nil {
		dto.ResolverMatches = []core.ResolverMatch{}
	}
	return dto, playSource, rootCandidate
}

func resolverMatchesForDetail(matches []core.ResolverMatch) []core.ResolverMatch {
	out := make([]core.ResolverMatch, 0, len(matches))
	for _, match := range matches {
		if match.PluginID == "metadata-launchbox" {
			match.URL = launchBoxSearchURL(match.Title)
		}
		out = append(out, match)
	}
	return out
}

func (c *GameController) describeSourceGameDelivery(ctx context.Context, canonicalPlatform core.Platform, sg *core.SourceGame) []core.SourceDeliveryProfile {
	if c != nil && c.cacheSvc != nil {
		return c.cacheSvc.DescribeSourceGame(ctx, canonicalPlatform, sg)
	}
	if sg == nil {
		return nil
	}
	profile, ok := core.BrowserPlayProfileForSourceGame(sg.Platform, canonicalPlatform)
	if !ok {
		return nil
	}
	mode := core.SourceDeliveryModeUnavailable
	ready := false
	if supportsDirectSourceGame(sg) {
		mode = core.SourceDeliveryModeDirect
		ready = true
	}
	delivery := core.SourceDeliveryProfile{
		Profile: profile,
		Mode:    mode,
		Ready:   ready,
	}
	if rootFile := selectRootGameFile(sg.Files); rootFile != nil {
		delivery.RootFilePath = rootFile.Path
	}
	return []core.SourceDeliveryProfile{delivery}
}

func selectRootGameFile(files []core.GameFile) *core.GameFile {
	for i := range files {
		if files[i].Role == core.GameFileRoleRoot {
			return &files[i]
		}
	}
	return nil
}
