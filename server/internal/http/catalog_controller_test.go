package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/catalog"
	"github.com/go-chi/chi/v5"
)

type catalogControllerServiceStub struct {
	filter       catalog.OfferFilter
	getOfferID   string
	historyID    string
	historyLimit int
	offers       []catalog.Offer
	offer        *catalog.Offer
	events       []catalog.HistoryEvent
	err          error
}

func (s *catalogControllerServiceStub) GetOffer(_ context.Context, offerID string) (*catalog.Offer, error) {
	s.getOfferID = offerID
	return s.offer, s.err
}

func (s *catalogControllerServiceStub) ListOffers(_ context.Context, filter catalog.OfferFilter) ([]catalog.Offer, error) {
	s.filter = filter
	return s.offers, s.err
}

func (s *catalogControllerServiceStub) ListHistory(_ context.Context, offerID string, limit int) ([]catalog.HistoryEvent, error) {
	s.historyID = offerID
	s.historyLimit = limit
	return s.events, s.err
}

func TestCatalogControllerListOffersParsesStrictFilters(t *testing.T) {
	service := &catalogControllerServiceStub{offers: []catalog.Offer{{ID: "offer-a"}}}
	controller, err := NewCatalogController(service, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/catalog/offers?canonical_game_id=game-a&provider=Steam&availability=leaving_soon&stale=true", nil)
	controller.ListOffers(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.filter.CanonicalGameID != "game-a" || service.filter.Provider != "Steam" || service.filter.Availability != catalog.AvailabilityLeavingSoon || !service.filter.StaleOnly {
		t.Fatalf("filter=%+v", service.filter)
	}
	var response struct {
		Offers []catalog.Offer `json:"offers"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || len(response.Offers) != 1 || response.Offers[0].ID != "offer-a" {
		t.Fatalf("response=%+v err=%v", response, err)
	}

	bad := httptest.NewRecorder()
	controller.ListOffers(bad, httptest.NewRequest(http.MethodGet, "/api/catalog/offers?availability=missing", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid availability status=%d", bad.Code)
	}
}

func TestCatalogControllerGetAndHistoryUseOfferPathScope(t *testing.T) {
	service := &catalogControllerServiceStub{
		offer:  &catalog.Offer{ID: "offer-a"},
		events: []catalog.HistoryEvent{{ID: "event-a", Type: catalog.EventAdded}},
	}
	controller, err := NewCatalogController(service, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/api/catalog/offers/{offer_id}", controller.GetOffer)
	router.Get("/api/catalog/offers/{offer_id}/history", controller.ListHistory)

	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, httptest.NewRequest(http.MethodGet, "/api/catalog/offers/offer-a", nil))
	if getRecorder.Code != http.StatusOK || service.getOfferID != "offer-a" {
		t.Fatalf("get status=%d id=%q body=%s", getRecorder.Code, service.getOfferID, getRecorder.Body.String())
	}
	historyRecorder := httptest.NewRecorder()
	router.ServeHTTP(historyRecorder, httptest.NewRequest(http.MethodGet, "/api/catalog/offers/offer-a/history?limit=25", nil))
	if historyRecorder.Code != http.StatusOK || service.historyID != "offer-a" || service.historyLimit != 25 {
		t.Fatalf("history status=%d id=%q limit=%d body=%s", historyRecorder.Code, service.historyID, service.historyLimit, historyRecorder.Body.String())
	}

	badLimit := httptest.NewRecorder()
	router.ServeHTTP(badLimit, httptest.NewRequest(http.MethodGet, "/api/catalog/offers/offer-a/history?limit=1001", nil))
	if badLimit.Code != http.StatusBadRequest {
		t.Fatalf("bad limit status=%d", badLimit.Code)
	}
}

func TestCatalogControllerFailsClosedForForeignOffer(t *testing.T) {
	service := &catalogControllerServiceStub{err: catalog.ErrOfferNotFound}
	controller, err := NewCatalogController(service, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/api/catalog/offers/{offer_id}", controller.GetOffer)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/catalog/offers/foreign", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	service.err = errors.New("database unavailable")
	serverError := httptest.NewRecorder()
	router.ServeHTTP(serverError, httptest.NewRequest(http.MethodGet, "/api/catalog/offers/offer-a", nil))
	if serverError.Code != http.StatusInternalServerError {
		t.Fatalf("server error status=%d", serverError.Code)
	}
}
