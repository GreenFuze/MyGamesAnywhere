package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

const retiredFeatureDocumentationURL = "https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/20119553"

type retiredFeatureResponse struct {
	Error retiredFeatureError `json:"error"`
}

type retiredFeatureError struct {
	Code             string `json:"code"`
	Message          string `json:"message"`
	Replacement      string `json:"replacement"`
	DocumentationURL string `json:"documentation_url"`
}

func RetiredFeatureHandler(code string) http.HandlerFunc {
	if code == "" {
		code = "feature_retired"
	}
	response := retiredFeatureResponse{Error: retiredFeatureError{
		Code:             code,
		Message:          "MGA no longer manages local devices, installation, or game launch.",
		Replacement:      "Use a supported frontend integration and MGA's catalog/content APIs.",
		DocumentationURL: retiredFeatureDocumentationURL,
	}}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusGone)
			return
		}
		writeJSON(w, http.StatusGone, response)
	}
}

type retiredRoute struct {
	method string
	path   string
}

func registerProtectedRetiredRoutes(router chi.Router, handler http.HandlerFunc) {
	routes := []retiredRoute{
		{http.MethodGet, "/devices"},
		{http.MethodGet, "/install-preferences/profile"},
		{http.MethodPut, "/install-preferences/profile"},
		{http.MethodGet, "/devices/validation-schedule"},
		{http.MethodPut, "/devices/validation-schedule"},
		{http.MethodPost, "/devices/client-launches"},
		{http.MethodGet, "/devices/client-launches/{id}"},
		{http.MethodPost, "/devices/pairing-challenges"},
		{http.MethodPut, "/devices/{id}"},
		{http.MethodGet, "/devices/{id}/install-preference"},
		{http.MethodPut, "/devices/{id}/install-preference"},
		{http.MethodGet, "/devices/{id}/emulators"},
		{http.MethodPut, "/devices/{id}/emulators/{platform}/default"},
		{http.MethodPut, "/devices/{id}/emulators/{platform}/{emulator_id}/core"},
		{http.MethodPost, "/devices/{id}/emulators/{emulator_id}/setup"},
		{http.MethodDelete, "/devices/{id}"},
		{http.MethodGet, "/devices/{id}/grants"},
		{http.MethodPut, "/devices/{id}/grants/{profile_id}"},
		{http.MethodDelete, "/devices/{id}/grants/{profile_id}"},
		{http.MethodPost, "/devices/{id}/commands"},
		{http.MethodGet, "/devices/{id}/commands"},
		{http.MethodPost, "/devices/{id}/validate-installations"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/installation-preflight"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/download-files"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/install-archive"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/install-gog-inno"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/use-existing"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/use-storefront"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/launch-storefront"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/uninstall"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/repair"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/reinstall"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/cleanup"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/forget"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/cleanup-failed"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/ignore-failed"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/reopen-failed-cleanup"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/launch"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/launch-emulator"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/save-domain/claim"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/save-domain/release"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/save-domain/snapshot"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/save-domain/restore"},
		{http.MethodPost, "/devices/{id}/games/{game_id}/sources/{source_game_id}/save-domain/reconcile"},
		{http.MethodPut, "/devices/{id}/games/{game_id}/sources/{source_game_id}/launch-target"},
		{http.MethodGet, "/play/devices/{id}/installed-games"},
	}
	for _, route := range routes {
		router.MethodFunc(route.method, route.path, handler)
	}
}
