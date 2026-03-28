package http

import (
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// GameDetailResponse is the body for GET /api/games/{id}/detail.
type GameDetailResponse struct {
	ID                 string                 `json:"id"`
	Title              string                 `json:"title"`
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
	ID              string               `json:"id"`
	IntegrationID   string               `json:"integration_id"`
	PluginID        string               `json:"plugin_id"`
	ExternalID      string               `json:"external_id"`
	RawTitle        string               `json:"raw_title"`
	Platform        string               `json:"platform"`
	Kind            string               `json:"kind"`
	GroupKind       string               `json:"group_kind,omitempty"`
	RootPath        string               `json:"root_path,omitempty"`
	URL             string               `json:"url,omitempty"`
	Status          string               `json:"status"`
	LastSeenAt      *string              `json:"last_seen_at,omitempty"`
	CreatedAt       string               `json:"created_at"`
	Files           []GameFileDTO        `json:"files"`
	Play            *SourceGamePlayDTO   `json:"play,omitempty"`
	ResolverMatches []core.ResolverMatch `json:"resolver_matches"`
}

func canonicalToGameDetail(cg *core.CanonicalGame) GameDetailResponse {
	if cg == nil {
		return GameDetailResponse{SourceGames: []SourceGameDetailDTO{}}
	}
	out := GameDetailResponse{
		ID:              cg.ID,
		Title:           cg.Title,
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
		SourceGames:     make([]SourceGameDetailDTO, 0, len(cg.SourceGames)),
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
		sourceDTO, launchSource, launchCandidate := sourceGameToDetailDTO(sg, cg.Platform, out.Play.PlatformSupported)
		if launchSource != nil {
			out.Play.LaunchSources = append(out.Play.LaunchSources, *launchSource)
			if launchSource.Launchable {
				out.Play.Available = true
			}
		}
		if launchSource != nil && launchSource.Launchable && launchCandidate != nil {
			out.Play.LaunchCandidates = append(out.Play.LaunchCandidates, *launchCandidate)
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

func sourceGameToDetailDTO(
	sg *core.SourceGame,
	canonicalPlatform core.Platform,
	platformSupported bool,
) (SourceGameDetailDTO, *GameLaunchSourceDTO, *GameLaunchCandidateDTO) {
	dto := SourceGameDetailDTO{
		ID:              sg.ID,
		IntegrationID:   sg.IntegrationID,
		PluginID:        sg.PluginID,
		ExternalID:      sg.ExternalID,
		RawTitle:        sg.RawTitle,
		Platform:        string(sg.Platform),
		Kind:            string(sg.Kind),
		GroupKind:       string(sg.GroupKind),
		RootPath:        sg.RootPath,
		URL:             sg.URL,
		Status:          sg.Status,
		CreatedAt:       sg.CreatedAt.UTC().Format(time.RFC3339Nano),
		Files:           make([]GameFileDTO, 0, len(sg.Files)),
		ResolverMatches: sg.ResolverMatches,
	}
	if sg.LastSeenAt != nil {
		s := sg.LastSeenAt.UTC().Format(time.RFC3339Nano)
		dto.LastSeenAt = &s
	}

	playSource := &GameLaunchSourceDTO{SourceGameID: sg.ID}
	dto.Play = &SourceGamePlayDTO{}
	rootPlatform := sg.Platform
	if rootPlatform == core.PlatformUnknown {
		rootPlatform = canonicalPlatform
	}
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

	if sg.Status == "found" && platformSupported && sg.GroupKind == core.GroupKindSelfContained && len(sg.Files) > 0 {
		launchable := rootFileID != ""
		if !launchable && rootPlatform == core.PlatformScummVM {
			launchable = supportsScummVMLaunchSource(sg.Files)
		}
		dto.Play.Launchable = launchable
		dto.Play.RootFileID = rootFileID
		playSource.Launchable = launchable
		playSource.RootFileID = rootFileID
	} else {
		dto.Play.Launchable = false
		playSource.Launchable = false
	}

	if dto.ResolverMatches == nil {
		dto.ResolverMatches = []core.ResolverMatch{}
	}
	return dto, playSource, rootCandidate
}
