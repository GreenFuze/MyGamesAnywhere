package http

import (
	"net/http"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func testProfileMiddleware(profileID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			profile := &core.Profile{ID: profileID, Role: core.ProfileRoleAdminPlayer}
			next.ServeHTTP(w, r.WithContext(core.WithProfile(r.Context(), profile)))
		})
	}
}
