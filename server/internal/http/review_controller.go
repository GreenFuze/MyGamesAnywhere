package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
)

const reviewMetadataLookupMethod = "metadata.game.lookup"

type ReviewController struct {
	integrationRepo core.IntegrationRepository
	pluginHost      plugins.PluginHost
	gameStore       core.GameStore
	manualReviewSvc core.ManualReviewService
	logger          core.Logger
}

func NewReviewController(
	integrationRepo core.IntegrationRepository,
	pluginHost plugins.PluginHost,
	gameStore core.GameStore,
	manualReviewSvc core.ManualReviewService,
	logger core.Logger,
) *ReviewController {
	return &ReviewController{
		integrationRepo: integrationRepo,
		pluginHost:      pluginHost,
		gameStore:       gameStore,
		manualReviewSvc: manualReviewSvc,
		logger:          logger,
	}
}

type ManualReviewCandidateSummaryDTO struct {
	ID                 string   `json:"id"`
	CanonicalGameID    string   `json:"canonical_game_id,omitempty"`
	CurrentTitle       string   `json:"current_title"`
	RawTitle           string   `json:"raw_title"`
	Platform           string   `json:"platform"`
	Kind               string   `json:"kind"`
	GroupKind          string   `json:"group_kind,omitempty"`
	IntegrationID      string   `json:"integration_id"`
	IntegrationLabel   string   `json:"integration_label,omitempty"`
	PluginID           string   `json:"plugin_id"`
	ExternalID         string   `json:"external_id"`
	RootPath           string   `json:"root_path,omitempty"`
	Status             string   `json:"status"`
	ReviewState        string   `json:"review_state"`
	FileCount          int      `json:"file_count"`
	ResolverMatchCount int      `json:"resolver_match_count"`
	ReviewReasons      []string `json:"review_reasons"`
	CreatedAt          string   `json:"created_at"`
	LastSeenAt         *string  `json:"last_seen_at,omitempty"`
}

type ManualReviewCandidateDetailDTO struct {
	ManualReviewCandidateSummaryDTO
	URL             string               `json:"url,omitempty"`
	Files           []GameFileDTO        `json:"files"`
	ResolverMatches []core.ResolverMatch `json:"resolver_matches"`
}

type manualReviewSearchRequest struct {
	Query string `json:"query"`
}

type manualReviewApplyRequest = core.ManualReviewSelection

type ManualReviewSearchProviderStatusDTO struct {
	IntegrationID    string `json:"integration_id"`
	IntegrationLabel string `json:"integration_label,omitempty"`
	PluginID         string `json:"plugin_id"`
	Status           string `json:"status"`
	Error            string `json:"error,omitempty"`
	ResultCount      int    `json:"result_count"`
}

type ManualReviewSearchResultDTO struct {
	ProviderIntegrationID string   `json:"provider_integration_id"`
	ProviderLabel         string   `json:"provider_label,omitempty"`
	ProviderPluginID      string   `json:"provider_plugin_id"`
	Title                 string   `json:"title"`
	Platform              string   `json:"platform,omitempty"`
	Kind                  string   `json:"kind,omitempty"`
	ParentGameID          string   `json:"parent_game_id,omitempty"`
	ExternalID            string   `json:"external_id"`
	URL                   string   `json:"url,omitempty"`
	Description           string   `json:"description,omitempty"`
	ReleaseDate           string   `json:"release_date,omitempty"`
	Genres                []string `json:"genres,omitempty"`
	Developer             string   `json:"developer,omitempty"`
	Publisher             string   `json:"publisher,omitempty"`
	Rating                float64  `json:"rating,omitempty"`
	MaxPlayers            int      `json:"max_players,omitempty"`
	ImageURL              string   `json:"image_url,omitempty"`
}

type ManualReviewSearchResponseDTO struct {
	CandidateID string                                `json:"candidate_id"`
	Query       string                                `json:"query"`
	Providers   []ManualReviewSearchProviderStatusDTO `json:"providers"`
	Results     []ManualReviewSearchResultDTO         `json:"results"`
}

type ManualReviewRedetectResponseDTO struct {
	Result    core.ManualReviewRedetectResult `json:"result"`
	Candidate ManualReviewCandidateDetailDTO  `json:"candidate"`
}

type reviewMetadataLookupRequest struct {
	Games []reviewMetadataGameQuery `json:"games"`
}

type reviewMetadataGameQuery struct {
	Index     int    `json:"index"`
	Title     string `json:"title"`
	Platform  string `json:"platform"`
	RootPath  string `json:"root_path"`
	GroupKind string `json:"group_kind"`
}

type reviewMetadataLookupResponse struct {
	Results []reviewMetadataMatch `json:"results"`
}

type reviewMetadataMatch struct {
	Index        int                   `json:"index"`
	Title        string                `json:"title,omitempty"`
	Platform     string                `json:"platform,omitempty"`
	Kind         string                `json:"kind,omitempty"`
	ParentGameID string                `json:"parent_game_id,omitempty"`
	ExternalID   string                `json:"external_id"`
	URL          string                `json:"url,omitempty"`
	Description  string                `json:"description,omitempty"`
	ReleaseDate  string                `json:"release_date,omitempty"`
	Genres       []string              `json:"genres,omitempty"`
	Developer    string                `json:"developer,omitempty"`
	Publisher    string                `json:"publisher,omitempty"`
	Media        []reviewMetadataMedia `json:"media,omitempty"`
	Rating       float64               `json:"rating,omitempty"`
	MaxPlayers   int                   `json:"max_players,omitempty"`
}

type reviewMetadataMedia struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func (c *ReviewController) ListCandidates(w http.ResponseWriter, r *http.Request) {
	limit := 200
	scope := core.ManualReviewScopeActive
	if rawScope := strings.TrimSpace(r.URL.Query().Get("scope")); rawScope != "" {
		scope = core.ManualReviewScope(rawScope)
		if scope != core.ManualReviewScopeActive && scope != core.ManualReviewScopeArchive {
			http.Error(w, "invalid scope", http.StatusBadRequest)
			return
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		if parsed > 0 {
			limit = parsed
		}
	}

	candidates, err := c.gameStore.ListManualReviewCandidates(r.Context(), scope, limit)
	if err != nil {
		c.logger.Error("list manual review candidates", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	labels, err := c.integrationLabels(r.Context())
	if err != nil {
		c.logger.Error("list integrations for manual review", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out := make([]ManualReviewCandidateSummaryDTO, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		out = append(out, manualReviewCandidateSummaryDTO(candidate, labels))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (c *ReviewController) GetCandidate(w http.ResponseWriter, r *http.Request) {
	candidateID, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if candidateID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	candidate, err := c.gameStore.GetManualReviewCandidate(r.Context(), candidateID)
	if err != nil {
		c.logger.Error("get manual review candidate", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if candidate == nil {
		http.NotFound(w, r)
		return
	}

	labels, err := c.integrationLabels(r.Context())
	if err != nil {
		c.logger.Error("list integrations for manual review detail", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(manualReviewCandidateDetailDTO(candidate, labels))
}

func (c *ReviewController) SearchCandidate(w http.ResponseWriter, r *http.Request) {
	candidateID, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if candidateID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	var body manualReviewSearchRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
	}

	candidate, err := c.gameStore.GetManualReviewCandidate(r.Context(), candidateID)
	if err != nil {
		c.logger.Error("get manual review candidate for search", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if candidate == nil {
		http.NotFound(w, r)
		return
	}

	query := strings.TrimSpace(body.Query)
	if query == "" {
		query = strings.TrimSpace(candidate.CurrentTitle)
	}
	if query == "" {
		query = strings.TrimSpace(candidate.RawTitle)
	}
	if query == "" {
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}

	integrations, err := c.integrationRepo.List(r.Context())
	if err != nil {
		c.logger.Error("list metadata integrations for manual review search", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	providerPluginIDs := c.pluginHost.GetPluginIDsProviding(reviewMetadataLookupMethod)
	providerPluginSet := make(map[string]struct{}, len(providerPluginIDs))
	for _, pluginID := range providerPluginIDs {
		providerPluginSet[pluginID] = struct{}{}
	}

	resp := ManualReviewSearchResponseDTO{
		CandidateID: candidate.ID,
		Query:       query,
		Providers:   []ManualReviewSearchProviderStatusDTO{},
		Results:     []ManualReviewSearchResultDTO{},
	}

	lookupSources := make([]scan.MetadataSource, 0, len(integrations))
	for _, integration := range integrations {
		if integration == nil || integration.IntegrationType != "metadata" {
			continue
		}
		if _, ok := providerPluginSet[integration.PluginID]; !ok {
			continue
		}

		status := ManualReviewSearchProviderStatusDTO{
			IntegrationID:    integration.ID,
			IntegrationLabel: integration.Label,
			PluginID:         integration.PluginID,
		}

		source, err := scan.MetadataSourceFromIntegration(integration)
		if err != nil {
			status.Status = "error"
			status.Error = "invalid integration config"
			resp.Providers = append(resp.Providers, status)
			continue
		}
		lookupSources = append(lookupSources, source)
	}

	lookupResults := scan.LookupMetadataSources(r.Context(), c.pluginHost, lookupSources, []scan.MetadataLookupQuery{{
		Index:     0,
		Title:     query,
		Platform:  string(candidate.Platform),
		RootPath:  candidate.RootPath,
		GroupKind: string(candidate.GroupKind),
	}})
	for _, lookup := range lookupResults {
		status := ManualReviewSearchProviderStatusDTO{
			IntegrationID:    lookup.Source.IntegrationID,
			IntegrationLabel: lookup.Source.Label,
			PluginID:         lookup.Source.PluginID,
		}
		if lookup.Error != nil {
			status.Status = "error"
			status.Error = lookup.Error.Error()
			resp.Providers = append(resp.Providers, status)
			continue
		}

		status.ResultCount = len(lookup.Matches)
		if len(lookup.Matches) == 0 {
			status.Status = "no_results"
			resp.Providers = append(resp.Providers, status)
			continue
		}

		status.Status = "success"
		resp.Providers = append(resp.Providers, status)
		for _, match := range lookup.Matches {
			if strings.TrimSpace(match.ExternalID) == "" {
				continue
			}
			resp.Results = append(resp.Results, ManualReviewSearchResultDTO{
				ProviderIntegrationID: lookup.Source.IntegrationID,
				ProviderLabel:         lookup.Source.Label,
				ProviderPluginID:      lookup.Source.PluginID,
				Title:                 strings.TrimSpace(match.Title),
				Platform:              strings.TrimSpace(match.Platform),
				Kind:                  strings.TrimSpace(match.Kind),
				ParentGameID:          strings.TrimSpace(match.ParentGameID),
				ExternalID:            strings.TrimSpace(match.ExternalID),
				URL:                   strings.TrimSpace(match.URL),
				Description:           strings.TrimSpace(match.Description),
				ReleaseDate:           strings.TrimSpace(match.ReleaseDate),
				Genres:                slices.Clone(match.Genres),
				Developer:             strings.TrimSpace(match.Developer),
				Publisher:             strings.TrimSpace(match.Publisher),
				Rating:                match.Rating,
				MaxPlayers:            match.MaxPlayers,
				ImageURL:              reviewRepresentativeLookupImage(match.Media),
			})
		}
	}

	sort.Slice(resp.Results, func(i, j int) bool {
		left := resp.Results[i]
		right := resp.Results[j]
		if left.ProviderLabel == right.ProviderLabel {
			if left.Title == right.Title {
				return left.ExternalID < right.ExternalID
			}
			return left.Title < right.Title
		}
		return left.ProviderLabel < right.ProviderLabel
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (c *ReviewController) RedetectCandidate(w http.ResponseWriter, r *http.Request) {
	candidateID, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if candidateID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	result, err := c.manualReviewSvc.Redetect(r.Context(), candidateID)
	if err != nil {
		status := manualReviewMutationErrorStatus(err)
		c.logger.Error("redetect manual review candidate", err, "candidate_id", candidateID)
		http.Error(w, err.Error(), status)
		return
	}

	candidate, err := c.gameStore.GetManualReviewCandidate(r.Context(), candidateID)
	if err != nil {
		c.logger.Error("get manual review candidate after redetect", err, "candidate_id", candidateID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if candidate == nil {
		http.NotFound(w, r)
		return
	}
	labels, err := c.integrationLabels(r.Context())
	if err != nil {
		c.logger.Error("list integrations for manual review redetect detail", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ManualReviewRedetectResponseDTO{
		Result:    *result,
		Candidate: manualReviewCandidateDetailDTO(candidate, labels),
	})
}

func (c *ReviewController) RedetectActive(w http.ResponseWriter, r *http.Request) {
	result, err := c.manualReviewSvc.RedetectActive(r.Context())
	if result == nil {
		result = &core.ManualReviewRedetectBatchResult{Results: []core.ManualReviewRedetectResult{}}
	}
	if err != nil {
		if strings.TrimSpace(result.Error) == "" {
			result.Error = err.Error()
		}
		c.logger.Error("redetect active manual review candidates", err, "failed_candidate_id", result.FailedCandidateID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(manualReviewMutationErrorStatus(err))
		_ = json.NewEncoder(w).Encode(result)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (c *ReviewController) ApplyCandidate(w http.ResponseWriter, r *http.Request) {
	candidateID, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if candidateID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	var body manualReviewApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.ProviderPluginID) == "" || strings.TrimSpace(body.ExternalID) == "" {
		http.Error(w, "provider_plugin_id and external_id are required", http.StatusBadRequest)
		return
	}

	if err := c.manualReviewSvc.Apply(r.Context(), candidateID, core.ManualReviewSelection(body)); err != nil {
		status := manualReviewMutationErrorStatus(err)
		c.logger.Error("apply manual review candidate", err)
		http.Error(w, err.Error(), status)
		return
	}

	c.respondWithCandidateDetail(w, r, candidateID)
}

func (c *ReviewController) MarkCandidateNotAGame(w http.ResponseWriter, r *http.Request) {
	c.updateCandidateReviewState(w, r, core.ManualReviewStateNotAGame)
}

func (c *ReviewController) UnarchiveCandidate(w http.ResponseWriter, r *http.Request) {
	c.updateCandidateReviewState(w, r, core.ManualReviewStatePending)
}

func (c *ReviewController) updateCandidateReviewState(w http.ResponseWriter, r *http.Request, state core.ManualReviewState) {
	candidateID, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if candidateID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := c.gameStore.SetManualReviewState(r.Context(), candidateID, state); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, core.ErrManualReviewCandidateNotFound) {
			status = http.StatusNotFound
		}
		c.logger.Error("update manual review state", err, "candidate_id", candidateID, "state", state)
		http.Error(w, err.Error(), status)
		return
	}
	c.respondWithCandidateDetail(w, r, candidateID)
}

func (c *ReviewController) respondWithCandidateDetail(w http.ResponseWriter, r *http.Request, candidateID string) {
	candidate, err := c.gameStore.GetManualReviewCandidate(r.Context(), candidateID)
	if err != nil {
		c.logger.Error("get manual review candidate after mutation", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if candidate == nil {
		http.NotFound(w, r)
		return
	}
	labels, err := c.integrationLabels(r.Context())
	if err != nil {
		c.logger.Error("list integrations for manual review mutation detail", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(manualReviewCandidateDetailDTO(candidate, labels))
}

func manualReviewMutationErrorStatus(err error) int {
	switch {
	case errors.Is(err, core.ErrManualReviewCandidateNotFound):
		return http.StatusNotFound
	case errors.Is(err, core.ErrManualReviewSelectionInvalid), errors.Is(err, core.ErrManualReviewCandidateNotEligible):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func (c *ReviewController) integrationLabels(ctx context.Context) (map[string]string, error) {
	integrations, err := c.integrationRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	labels := make(map[string]string, len(integrations))
	for _, integration := range integrations {
		if integration == nil {
			continue
		}
		labels[integration.ID] = integration.Label
	}
	return labels, nil
}

func manualReviewCandidateSummaryDTO(candidate *core.ManualReviewCandidate, integrationLabels map[string]string) ManualReviewCandidateSummaryDTO {
	dto := ManualReviewCandidateSummaryDTO{
		ID:                 candidate.ID,
		CanonicalGameID:    candidate.CanonicalGameID,
		CurrentTitle:       candidate.CurrentTitle,
		RawTitle:           candidate.RawTitle,
		Platform:           string(candidate.Platform),
		Kind:               string(candidate.Kind),
		GroupKind:          string(candidate.GroupKind),
		IntegrationID:      candidate.IntegrationID,
		IntegrationLabel:   integrationLabels[candidate.IntegrationID],
		PluginID:           candidate.PluginID,
		ExternalID:         candidate.ExternalID,
		RootPath:           candidate.RootPath,
		Status:             candidate.Status,
		ReviewState:        string(candidate.ReviewState),
		FileCount:          candidate.FileCount,
		ResolverMatchCount: candidate.ResolverMatchCount,
		ReviewReasons:      append([]string(nil), candidate.ReviewReasons...),
		CreatedAt:          candidate.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if candidate.LastSeenAt != nil {
		lastSeen := candidate.LastSeenAt.UTC().Format(time.RFC3339Nano)
		dto.LastSeenAt = &lastSeen
	}
	return dto
}

func manualReviewCandidateDetailDTO(candidate *core.ManualReviewCandidate, integrationLabels map[string]string) ManualReviewCandidateDetailDTO {
	dto := ManualReviewCandidateDetailDTO{
		ManualReviewCandidateSummaryDTO: manualReviewCandidateSummaryDTO(candidate, integrationLabels),
		URL:                             candidate.URL,
		Files:                           make([]GameFileDTO, 0, len(candidate.Files)),
		ResolverMatches:                 append([]core.ResolverMatch(nil), candidate.ResolverMatches...),
	}
	for _, file := range candidate.Files {
		dto.Files = append(dto.Files, GameFileDTO{
			ID:       encodeGameFileID(candidate.ID, file.Path),
			Path:     file.Path,
			Role:     string(file.Role),
			FileKind: file.FileKind,
			Size:     file.Size,
		})
	}
	if dto.ResolverMatches == nil {
		dto.ResolverMatches = []core.ResolverMatch{}
	}
	return dto
}

func manualReviewSearchResultDTO(integration *core.Integration, match reviewMetadataMatch) (ManualReviewSearchResultDTO, bool) {
	if integration == nil || strings.TrimSpace(match.ExternalID) == "" {
		return ManualReviewSearchResultDTO{}, false
	}
	return ManualReviewSearchResultDTO{
		ProviderIntegrationID: integration.ID,
		ProviderLabel:         integration.Label,
		ProviderPluginID:      integration.PluginID,
		Title:                 strings.TrimSpace(match.Title),
		Platform:              strings.TrimSpace(match.Platform),
		Kind:                  strings.TrimSpace(match.Kind),
		ParentGameID:          strings.TrimSpace(match.ParentGameID),
		ExternalID:            strings.TrimSpace(match.ExternalID),
		URL:                   strings.TrimSpace(match.URL),
		Description:           strings.TrimSpace(match.Description),
		ReleaseDate:           strings.TrimSpace(match.ReleaseDate),
		Genres:                slices.Clone(match.Genres),
		Developer:             strings.TrimSpace(match.Developer),
		Publisher:             strings.TrimSpace(match.Publisher),
		Rating:                match.Rating,
		MaxPlayers:            match.MaxPlayers,
		ImageURL:              reviewRepresentativeImage(match.Media),
	}, true
}

func reviewRepresentativeImage(media []reviewMetadataMedia) string {
	if len(media) == 0 {
		return ""
	}
	order := []string{"cover", "artwork", "box_back", "box_side", "banner", "logo", "screenshot", "background", "icon"}
	for _, wanted := range order {
		for _, item := range media {
			if item.Type == wanted && strings.TrimSpace(item.URL) != "" {
				return item.URL
			}
		}
	}
	for _, item := range media {
		if strings.TrimSpace(item.URL) != "" {
			return item.URL
		}
	}
	return ""
}

func reviewRepresentativeLookupImage(media []scan.MetadataLookupMediaItem) string {
	if len(media) == 0 {
		return ""
	}
	order := []string{"cover", "artwork", "box_back", "box_side", "banner", "logo", "screenshot", "background", "icon"}
	for _, wanted := range order {
		for _, item := range media {
			if item.Type == wanted && strings.TrimSpace(item.URL) != "" {
				return item.URL
			}
		}
	}
	for _, item := range media {
		if strings.TrimSpace(item.URL) != "" {
			return item.URL
		}
	}
	return ""
}
