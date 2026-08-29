package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/catalog"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

type CatalogService interface {
	GetOffer(ctx context.Context, offerID string) (*catalog.Offer, error)
	ListOffers(ctx context.Context, filter catalog.OfferFilter) ([]catalog.Offer, error)
	ListHistory(ctx context.Context, offerID string, limit int) ([]catalog.HistoryEvent, error)
}

type CatalogController struct {
	service CatalogService
	logger  core.Logger
}

func NewCatalogController(service CatalogService, logger core.Logger) (*CatalogController, error) {
	if service == nil || logger == nil {
		return nil, errors.New("catalog service and logger are required")
	}
	return &CatalogController{service: service, logger: logger}, nil
}

func (c *CatalogController) ListOffers(w http.ResponseWriter, r *http.Request) {
	filter := catalog.OfferFilter{
		CanonicalGameID: strings.TrimSpace(r.URL.Query().Get("canonical_game_id")),
		Provider:        strings.TrimSpace(r.URL.Query().Get("provider")),
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("availability")); raw != "" {
		filter.Availability = catalog.Availability(raw)
		if !filter.Availability.Valid() {
			http.Error(w, "unsupported availability", http.StatusBadRequest)
			return
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("stale")); raw != "" {
		stale, err := strconv.ParseBool(raw)
		if err != nil {
			http.Error(w, "stale must be true or false", http.StatusBadRequest)
			return
		}
		filter.StaleOnly = stale
	}
	offers, err := c.service.ListOffers(r.Context(), filter)
	if err != nil {
		c.writeError(w, err, "list catalog offers")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"offers": offers})
}

func (c *CatalogController) GetOffer(w http.ResponseWriter, r *http.Request) {
	offerID := strings.TrimSpace(chi.URLParam(r, "offer_id"))
	if offerID == "" {
		http.Error(w, "offer_id is required", http.StatusBadRequest)
		return
	}
	offer, err := c.service.GetOffer(r.Context(), offerID)
	if err != nil {
		c.writeError(w, err, "get catalog offer", "offer_id", offerID)
		return
	}
	writeJSON(w, http.StatusOK, offer)
}

func (c *CatalogController) ListHistory(w http.ResponseWriter, r *http.Request) {
	offerID := strings.TrimSpace(chi.URLParam(r, "offer_id"))
	if offerID == "" {
		http.Error(w, "offer_id is required", http.StatusBadRequest)
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 1000 {
			http.Error(w, "limit must be between 1 and 1000", http.StatusBadRequest)
			return
		}
		limit = parsed
	}
	events, err := c.service.ListHistory(r.Context(), offerID, limit)
	if err != nil {
		c.writeError(w, err, "list catalog offer history", "offer_id", offerID)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (c *CatalogController) writeError(w http.ResponseWriter, err error, message string, attrs ...any) {
	switch {
	case errors.Is(err, catalog.ErrOfferNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, catalog.ErrProfileRequired), errors.Is(err, catalog.ErrCatalogIdentityNotVisible):
		http.Error(w, err.Error(), http.StatusForbidden)
	default:
		c.logger.Error(message, err, attrs...)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
