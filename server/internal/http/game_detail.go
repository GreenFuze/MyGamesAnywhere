package http

import (
	"context"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
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
	Play               *GamePlayDTO           `json:"play,omitempty"`
	AchievementSummary *AchievementSummaryDTO `json:"achievement_summary,omitempty"`
	SourceGames        []SourceGameDetailDTO  `json:"source_games"`
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
	HardDelete       *SourceGameHardDeleteDTO `json:"hard_delete,omitempty"`
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
			URL:        eid.URL,
		})
	}
	return out
}

func launchOptionsForSource(source SourceGameDetailDTO, launchSource *GameLaunchSourceDTO, launchCandidate *GameLaunchCandidateDTO) []GameLaunchOptionDTO {
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
		ResolverMatches:  sg.ResolverMatches,
	}
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
