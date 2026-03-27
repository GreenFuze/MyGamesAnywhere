package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestDecodedPathParamUnescapesLegacyGameIDs(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/api/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := decodedPathParam(r, "id")
		if err != nil {
			t.Fatalf("decodedPathParam returned error: %v", err)
		}
		if id != "scan:225d313af056fa3e" {
			t.Fatalf("id = %q, want %q", id, "scan:225d313af056fa3e")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/games/scan%3A225d313af056fa3e", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
