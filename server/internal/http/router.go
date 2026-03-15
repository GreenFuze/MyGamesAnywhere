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
	DiscoCtrl       *DiscoveryController
	ConfigCtrl      *ConfigController
	PluginCtrl      *PluginController
	AchievementCtrl *AchievementController
}

func noopHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {}
}

// BuildRouter builds the public API router. If b is nil, all routes use no-op handlers.
func BuildRouter(b *RouteBuilder, middlewareTimeout time.Duration) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	if middlewareTimeout > 0 {
		r.Use(middleware.Timeout(middlewareTimeout))
	}

	if b != nil {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("OK")) })
	} else {
		r.Get("/health", noopHandler())
	}

	r.Route("/api", func(r chi.Router) {
		if b != nil {
			r.Get("/games", b.GameCtrl.ListGames)
			r.Delete("/games", b.GameCtrl.DeleteAll)
			r.Get("/games/{id}", b.GameCtrl.Get)
			r.Get("/games/{id}/achievements", b.AchievementCtrl.GetAchievements)
			r.Get("/scan", b.DiscoCtrl.Scan)
			r.Post("/scan", b.DiscoCtrl.Scan)
			r.Get("/plugins", b.PluginCtrl.ListPlugins)
			r.Get("/plugins/{plugin_id}", b.PluginCtrl.GetPluginByID)
			r.Post("/config/{key}", b.ConfigCtrl.Set)
			r.Get("/integrations", b.PluginCtrl.List)
			r.Get("/integrations/status", b.PluginCtrl.Status)
			r.Post("/integrations", b.PluginCtrl.Create)
		} else {
			r.Get("/games", noopHandler())
			r.Delete("/games", noopHandler())
			r.Get("/games/{id}", noopHandler())
			r.Get("/games/{id}/achievements", noopHandler())
			r.Get("/scan", noopHandler())
			r.Post("/scan", noopHandler())
			r.Get("/plugins", noopHandler())
			r.Get("/plugins/{plugin_id}", noopHandler())
			r.Post("/config/{key}", noopHandler())
			r.Get("/integrations", noopHandler())
			r.Get("/integrations/status", noopHandler())
			r.Post("/integrations", noopHandler())
		}
	})
	return r
}
