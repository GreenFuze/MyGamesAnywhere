package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// RouteBuilder holds controller references for building the HTTP router.
// If nil is passed to BuildRouter, routes are registered with no-op handlers for OpenAPI discovery.
type RouteBuilder struct {
	GameCtrl        *GameController
	MediaCtrl       *MediaController
	DiscoCtrl       *DiscoveryController
	ConfigCtrl      *ConfigController
	PluginCtrl      *PluginController
	AchievementCtrl *AchievementController
	SyncCtrl        *SyncController
	SSECtrl         *SSEController
	OAuthCtrl       *OAuthController
}

func noopHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {}
}

// BuildRouter builds the public API router. If b is nil, all routes use no-op handlers.
// spaStaticDir is optional: if non-empty and contains index.html, registers /* for the SPA.
func BuildRouter(b *RouteBuilder, middlewareTimeout time.Duration, spaStaticDir string) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	if b != nil {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("OK")) })
	} else {
		r.Get("/health", noopHandler())
	}

	r.Route("/api", func(api chi.Router) {
		if b != nil {
			// Routes with standard middleware timeout.
			api.Group(func(r chi.Router) {
				if middlewareTimeout > 0 {
					r.Use(middleware.Timeout(middlewareTimeout))
				}
				r.Get("/games", b.GameCtrl.ListGames)
				r.Delete("/games", b.GameCtrl.DeleteAll)
				r.Get("/games/{id}/detail", b.GameCtrl.GetDetail)
				r.Get("/games/{id}", b.GameCtrl.Get)
				r.Get("/games/{id}/achievements", b.AchievementCtrl.GetAchievements)
				r.Get("/media/{assetID}", b.MediaCtrl.ServeMedia)
				r.Get("/stats", b.GameCtrl.Stats)
				r.Get("/config/frontend", b.ConfigCtrl.GetFrontend)
				r.Post("/config/frontend", b.ConfigCtrl.SetFrontend)
				r.Get("/plugins", b.PluginCtrl.ListPlugins)
				r.Get("/plugins/{plugin_id}", b.PluginCtrl.GetPluginByID)
				r.Post("/config/{key}", b.ConfigCtrl.Set)
				r.Get("/integrations", b.PluginCtrl.List)
				r.Get("/integrations/status", b.PluginCtrl.Status)
				r.Post("/integrations", b.PluginCtrl.Create)
				r.Get("/integrations/{id}/status", b.PluginCtrl.StatusOne)
				r.Get("/integrations/{id}/games", b.PluginCtrl.IntegrationGames)
				r.Get("/integrations/{id}/enriched-games", b.PluginCtrl.IntegrationEnrichedGames)
				r.Put("/integrations/{id}", b.PluginCtrl.UpdateIntegration)
				r.Delete("/integrations/{id}", b.PluginCtrl.DeleteIntegration)
				r.Post("/plugins/{plugin_id}/browse", b.PluginCtrl.Browse)

				r.Get("/sync/status", b.SyncCtrl.Status)
				r.Post("/sync/push", b.SyncCtrl.Push)
				r.Post("/sync/pull", b.SyncCtrl.Pull)
				r.Post("/sync/key", b.SyncCtrl.StoreKey)
				r.Delete("/sync/key", b.SyncCtrl.ClearKey)

				r.Get("/auth/callback/{plugin_id}", b.OAuthCtrl.Callback)
			})

			// Scan can take many minutes; no middleware timeout.
			api.Get("/scan", b.DiscoCtrl.Scan)
			api.Post("/scan", b.DiscoCtrl.Scan)
			api.Get("/scan/reports", b.DiscoCtrl.GetScanReports)
			api.Get("/scan/reports/{id}", b.DiscoCtrl.GetScanReport)

			// Long-lived SSE stream; no middleware timeout.
			api.Get("/events", b.SSECtrl.Events)
		} else {
			api.Get("/games", noopHandler())
			api.Delete("/games", noopHandler())
			api.Get("/games/{id}/detail", noopHandler())
			api.Get("/games/{id}", noopHandler())
			api.Get("/games/{id}/achievements", noopHandler())
			api.Get("/media/{assetID}", noopHandler())
			api.Get("/stats", noopHandler())
			api.Get("/config/frontend", noopHandler())
			api.Post("/config/frontend", noopHandler())
			api.Get("/scan", noopHandler())
			api.Post("/scan", noopHandler())
			api.Get("/scan/reports", noopHandler())
			api.Get("/scan/reports/{id}", noopHandler())
			api.Get("/plugins", noopHandler())
			api.Get("/plugins/{plugin_id}", noopHandler())
			api.Post("/config/{key}", noopHandler())
			api.Get("/integrations", noopHandler())
			api.Get("/integrations/status", noopHandler())
			api.Post("/integrations", noopHandler())
			api.Get("/integrations/{id}/status", noopHandler())
			api.Get("/integrations/{id}/games", noopHandler())
			api.Get("/integrations/{id}/enriched-games", noopHandler())
			api.Put("/integrations/{id}", noopHandler())
			api.Delete("/integrations/{id}", noopHandler())
			api.Post("/plugins/{plugin_id}/browse", noopHandler())
			api.Get("/sync/status", noopHandler())
			api.Post("/sync/push", noopHandler())
			api.Post("/sync/pull", noopHandler())
			api.Post("/sync/key", noopHandler())
			api.Delete("/sync/key", noopHandler())
			api.Get("/auth/callback/{plugin_id}", noopHandler())
			api.Get("/events", noopHandler())
		}
	})

	MountSPA(r, spaStaticDir)
	return r
}
