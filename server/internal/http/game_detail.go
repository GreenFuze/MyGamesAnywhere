package http

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/gamesvc"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/savedomain"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcegames"
)

// GameDetailResponse is the body for GET /api/games/{id}/detail.
type GameDetailResponse struct {
	ID                 string                 `json:"id"`
	Title              string                 `json:"title"`
	Favorite           bool                   `json:"favorite"`
	Platform           string                 `json:"platform"`
	Kind               string                 `json:"kind"`
	GroupKind          string                 `json:"group_kind,omitempty"`
	RootPath           string                 `json:"root_path,omitempty"`
	Files              []GameFileDTO          `json:"files,omitempty"`
	ExternalIDs        []ExternalIDDTO        `json:"external_ids,omitempty"`
	Description        string                 `json:"description,omitempty"`
	ReleaseDate        string                 `json:"release_date,omitempty"`
	Genres             []string               `json:"genres,omitempty"`
	Developer          string                 `json:"developer,omitempty"`
	Publisher          string                 `json:"publisher,omitempty"`
	Rating             float64                `json:"rating,omitempty"`
	MaxPlayers         int                    `json:"max_players,omitempty"`
	CompletionTime     *core.CompletionTime   `json:"completion_time,omitempty"`
	Media              []GameMediaDetailDTO   `json:"media,omitempty"`
	CoverOverride      *GameMediaDetailDTO    `json:"cover_override,omitempty"`
	HoverOverride      *GameMediaDetailDTO    `json:"hover_override,omitempty"`
	BackgroundOverride *GameMediaDetailDTO    `json:"background_override,omitempty"`
	IsGamePass         bool                   `json:"is_game_pass,omitempty"`
	XcloudAvailable    bool                   `json:"xcloud_available,omitempty"`
	StoreProductID     string                 `json:"store_product_id,omitempty"`
	XcloudURL          string                 `json:"xcloud_url,omitempty"`
	Shared             bool                   `json:"shared,omitempty"`
	SharedOwner        string                 `json:"shared_owner,omitempty"`
	Play               *GamePlayDTO           `json:"play,omitempty"`
	AchievementSummary *AchievementSummaryDTO `json:"achievement_summary,omitempty"`
	Identity           *core.GameIdentity     `json:"identity,omitempty"`
	Content            *GameContentDTO        `json:"content,omitempty"`
	SourceGames        []SourceGameDetailDTO  `json:"source_games"`
	// MetadataWarnings lists metadata providers that were skipped during a forced refresh
	// due to non-fatal errors (e.g. timeout). Only present in refresh responses; empty on
	// regular game-detail reads.
	MetadataWarnings []string `json:"metadata_warnings,omitempty"`
}

type GameContentDTO struct {
	Parent            *RelatedContentGameDTO  `json:"parent,omitempty"`
	AddOns            []RelatedContentGameDTO `json:"add_ons,omitempty"`
	RelationshipState string                  `json:"relationship_state,omitempty"`
}

type RelatedContentGameDTO struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Platform string `json:"platform"`
	Kind     string `json:"kind"`
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
		Shared:          cg.Shared,
		SharedOwner:     cg.SharedOwner,
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
					IsDir:    f.IsDir,
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

// canonicalToLibraryGameWithIntegrationLabels deliberately excludes
// detail-only collections (all files, resolver matches, external IDs,
// identity evidence, descriptions, and the complete media gallery). It keeps
// only the fields needed to paint a card, group/filter it, and expose accurate
// play routes. Context-menu operations fetch GET /api/games/{id} on demand.
func (c *GameController) canonicalToLibraryGameWithIntegrationLabels(ctx context.Context, cg *core.CanonicalGame, integrationLabels map[string]string, deliveryProfiles map[string][]core.SourceDeliveryProfile) GameDetailResponse {
	if cg == nil {
		return GameDetailResponse{SourceGames: []SourceGameDetailDTO{}}
	}
	out := GameDetailResponse{
		ID:              cg.ID,
		Title:           cg.Title,
		Favorite:        cg.Favorite,
		Platform:        string(cg.Platform),
		Kind:            string(cg.Kind),
		ReleaseDate:     cg.ReleaseDate,
		Developer:       cg.Developer,
		Publisher:       cg.Publisher,
		Rating:          cg.Rating,
		IsGamePass:      cg.IsGamePass,
		XcloudAvailable: cg.XcloudAvailable,
		StoreProductID:  cg.StoreProductID,
		XcloudURL:       cg.XcloudURL,
		Shared:          cg.Shared,
		SharedOwner:     cg.SharedOwner,
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
	selectedMedia := make(map[int]bool)
	appendSelectedMedia := func(ref *core.MediaRef) *GameMediaDetailDTO {
		if ref == nil {
			return nil
		}
		dto := mediaRefToDTO(*ref)
		if ref.AssetID > 0 && !selectedMedia[ref.AssetID] {
			selectedMedia[ref.AssetID] = true
			out.Media = append(out.Media, dto)
		}
		return &dto
	}
	out.CoverOverride = appendSelectedMedia(cg.CoverOverride)
	out.HoverOverride = appendSelectedMedia(cg.HoverOverride)
	out.BackgroundOverride = appendSelectedMedia(cg.BackgroundOverride)

	for _, sg := range cg.SourceGames {
		if sg == nil {
			continue
		}
		if out.GroupKind == "" && sg.Status == "found" {
			out.GroupKind = string(sg.GroupKind)
		}
		sourceDTO, launchSource, launchCandidate := c.sourceGameToDetailDTOWithDelivery(ctx, sg, cg.Platform, out.Play.PlatformSupported, integrationLabels, deliveryProfiles[sg.ID])
		for _, option := range launchOptionsForSource(sourceDTO, launchSource, launchCandidate) {
			if option.Launchable {
				out.Play.Available = true
			}
			out.Play.Options = append(out.Play.Options, option)
		}
		// Retain card identity and lightweight play capability, but omit the
		// potentially large detail collections from every library row.
		sourceDTO.Files = []GameFileDTO{}
		sourceDTO.ResolverMatches = []core.ResolverMatch{}
		sourceDTO.Delivery = nil
		sourceDTO.Save = nil
		sourceDTO.HardDelete = nil
		sourceDTO.CanonicalPin = nil
		out.SourceGames = append(out.SourceGames, sourceDTO)
	}
	return out
}

// attachContentRelationships projects existing provider metadata into a
// player-facing relationship view. NO_MIGRATION_NEEDED: this reads the
// resolver match parent_game_id already stored in metadata_json and adds only
// response fields; persisted rows and configuration remain unchanged.
func (c *GameController) attachContentRelationships(ctx context.Context, response *GameDetailResponse, target *core.CanonicalGame) error {
	if response == nil || target == nil {
		return nil
	}
	type relationshipProjectionStore interface {
		GetCanonicalContentRelationshipProjectionGames(context.Context) ([]*core.CanonicalGame, error)
	}
	var games []*core.CanonicalGame
	var err error
	if projectionStore, ok := c.gameStore.(relationshipProjectionStore); ok {
		games, err = projectionStore.GetCanonicalContentRelationshipProjectionGames(ctx)
	} else {
		games, err = c.gameStore.GetCanonicalGames(ctx)
	}
	if err != nil {
		return err
	}
	foundTarget := false
	for _, game := range games {
		if game != nil && game.ID == target.ID {
			foundTarget = true
			break
		}
	}
	if !foundTarget {
		games = append(games, target)
	}

	projection := gamesvc.NewContentRelationshipProjector().Project(target.ID, games)
	if projection.Parent == nil && len(projection.AddOns) == 0 && projection.State == gamesvc.ContentRelationshipStateNone {
		return nil
	}
	content := &GameContentDTO{
		RelationshipState: string(projection.State),
		AddOns:            make([]RelatedContentGameDTO, 0, len(projection.AddOns)),
	}
	if projection.Parent != nil {
		parent := relatedContentGameDTO(projection.Parent)
		content.Parent = &parent
	}
	for _, addOn := range projection.AddOns {
		if addOn != nil {
			content.AddOns = append(content.AddOns, relatedContentGameDTO(addOn))
		}
	}
	response.Content = content
	return nil
}

func relatedContentGameDTO(game *core.CanonicalGame) RelatedContentGameDTO {
	return RelatedContentGameDTO{
		ID:       game.ID,
		Title:    game.Title,
		Platform: string(game.Platform),
		Kind:     string(game.Kind),
	}
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
	return c.sourceGameToDetailDTOWithDelivery(ctx, sg, canonicalPlatform, platformSupported, integrationLabels, nil)
}

func (c *GameController) sourceGameToDetailDTOWithDelivery(
	ctx context.Context,
	sg *core.SourceGame,
	canonicalPlatform core.Platform,
	platformSupported bool,
	integrationLabels map[string]string,
	preloadedDelivery []core.SourceDeliveryProfile,
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
			IsDir:    f.IsDir,
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

	deliveryProfiles := preloadedDelivery
	if deliveryProfiles == nil {
		deliveryProfiles = c.describeSourceGameDelivery(ctx, canonicalPlatform, sg)
	}
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
