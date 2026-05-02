package http

import (
	"net/http"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// RouteBuilder holds controller references for building the HTTP router.
// If nil is passed to BuildRouter, routes are registered with no-op handlers for OpenAPI discovery.
type RouteBuilder struct {
	GameCtrl               *GameController
	MediaCtrl              *MediaController
	DiscoCtrl              *DiscoveryController
	AboutCtrl              *AboutController
	ConfigCtrl             *ConfigController
	PluginCtrl             *PluginController
	IntegrationRefreshCtrl *IntegrationRefreshController
	ReviewCtrl             *ReviewController
	AchievementCtrl        *AchievementController
	SyncCtrl               *SyncController
	SaveSyncCtrl           *SaveSyncController
	CacheCtrl              *CacheController
	SSECtrl                *SSEController
	OAuthCtrl              *OAuthController
	ProfileCtrl            *ProfileController
	ProfileRepo            core.ProfileRepository
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
			api.Get("/setup/status", b.ProfileCtrl.SetupStatus)
			api.Post("/setup/start-fresh", b.ProfileCtrl.StartFresh)
			api.Post("/setup/restore-sync", b.ProfileCtrl.RestoreSync)
			api.Get("/profiles", b.ProfileCtrl.ListProfiles)
			api.Get("/media/{assetID}", b.MediaCtrl.ServeMedia)
			api.Put("/media/{assetID}/metadata", ProfileContextMiddleware(b.ProfileRepo)(RequireAdminProfile(http.HandlerFunc(b.MediaCtrl.UpdateMediaMetadata))).ServeHTTP)
			api.Get("/games/{id}/play", b.GameCtrl.ServePlayFile)
			api.Head("/games/{id}/play", b.GameCtrl.ServePlayFile)
			api.Get("/about", b.AboutCtrl.GetAbout)
			api.Get("/about/license", b.AboutCtrl.GetLicense)

			adminOnly := func(h http.HandlerFunc) http.HandlerFunc {
				return RequireAdminProfile(http.HandlerFunc(h)).ServeHTTP
			}

			// Routes with standard middleware timeout.
			api.Group(func(r chi.Router) {
				if middlewareTimeout > 0 {
					r.Use(middleware.Timeout(middlewareTimeout))
				}
				r.Use(ProfileContextMiddleware(b.ProfileRepo))
				r.Get("/games", b.GameCtrl.ListGames)
				r.Delete("/games", adminOnly(b.GameCtrl.DeleteAll))
				r.Get("/games/{id}/detail", b.GameCtrl.GetDetail)
				r.Post("/games/{id}/refresh-metadata", adminOnly(b.GameCtrl.RefreshMetadata))
				r.Put("/games/{id}/cover-override", b.GameCtrl.SetCoverOverride)
				r.Delete("/games/{id}/cover-override", b.GameCtrl.ClearCoverOverride)
				r.Put("/games/{id}/hover-override", b.GameCtrl.SetHoverOverride)
				r.Put("/games/{id}/background-override", b.GameCtrl.SetBackgroundOverride)
				r.Put("/games/{id}/favorite", b.GameCtrl.SetFavorite)
				r.Delete("/games/{id}/favorite", b.GameCtrl.ClearFavorite)
				r.Post("/games/{id}/sources/{source_game_id}/delete-preview", b.GameCtrl.PreviewDeleteSourceGame)
				r.Delete("/games/{id}/sources/{source_game_id}", b.GameCtrl.DeleteSourceGame)
				r.Post("/games/{id}/cache/prepare", b.CacheCtrl.PrepareGameCache)
				r.Post("/games/{id}/save-sync/prefetch", b.SaveSyncCtrl.StartPrefetch)
				r.Get("/games/{id}/save-sync/slots", b.SaveSyncCtrl.ListSlots)
				r.Get("/games/{id}/save-sync/slots/{slot_id}", b.SaveSyncCtrl.GetSlot)
				r.Put("/games/{id}/save-sync/slots/{slot_id}", b.SaveSyncCtrl.PutSlot)
				r.Get("/games/{id}", b.GameCtrl.Get)
				r.Get("/games/{id}/achievements", b.AchievementCtrl.GetAchievements)
				r.Get("/achievements", b.GameCtrl.AchievementsDashboard)
				r.Get("/achievements/explorer", b.GameCtrl.AchievementsExplorer)
				r.Get("/stats", b.GameCtrl.Stats)
				r.Get("/config/frontend", adminOnly(b.ConfigCtrl.GetFrontend))
				r.Post("/config/frontend", adminOnly(b.ConfigCtrl.SetFrontend))
				r.Get("/plugins", adminOnly(b.PluginCtrl.ListPlugins))
				r.Get("/plugins/{plugin_id}", adminOnly(b.PluginCtrl.GetPluginByID))
				r.Post("/config/{key}", adminOnly(b.ConfigCtrl.Set))
				r.Post("/profiles", b.ProfileCtrl.CreateProfile)
				r.Put("/profiles/{id}", b.ProfileCtrl.UpdateProfile)
				r.Delete("/profiles/{id}", b.ProfileCtrl.DeleteProfile)
				r.Get("/integrations", adminOnly(b.PluginCtrl.List))
				r.Get("/integrations/status", adminOnly(b.PluginCtrl.Status))
				r.Post("/integrations", adminOnly(b.PluginCtrl.Create))
				r.Get("/integrations/{id}/status", adminOnly(b.PluginCtrl.StatusOne))
				r.Post("/integrations/{id}/authorize", adminOnly(b.PluginCtrl.StartIntegrationAuth))
				if b.IntegrationRefreshCtrl != nil {
					r.Post("/integrations/{id}/refresh", adminOnly(b.IntegrationRefreshCtrl.Start))
					r.Get("/integration-refresh/jobs/{job_id}", adminOnly(b.IntegrationRefreshCtrl.GetJob))
				} else {
					r.Post("/integrations/{id}/refresh", noopHandler())
					r.Get("/integration-refresh/jobs/{job_id}", noopHandler())
				}
				r.Get("/integrations/{id}/games", adminOnly(b.PluginCtrl.IntegrationGames))
				r.Get("/integrations/{id}/enriched-games", adminOnly(b.PluginCtrl.IntegrationEnrichedGames))
				r.Get("/review-candidates", adminOnly(b.ReviewCtrl.ListCandidates))
				r.Post("/review-candidates/redetect", adminOnly(b.ReviewCtrl.RedetectActive))
				r.Get("/review-candidates/{id}", adminOnly(b.ReviewCtrl.GetCandidate))
				r.Post("/review-candidates/{id}/search", adminOnly(b.ReviewCtrl.SearchCandidate))
				r.Post("/review-candidates/{id}/redetect", adminOnly(b.ReviewCtrl.RedetectCandidate))
				r.Post("/review-candidates/{id}/apply", adminOnly(b.ReviewCtrl.ApplyCandidate))
				r.Post("/review-candidates/{id}/not-a-game", adminOnly(b.ReviewCtrl.MarkCandidateNotAGame))
				r.Post("/review-candidates/{id}/unarchive", adminOnly(b.ReviewCtrl.UnarchiveCandidate))
				r.Post("/review-candidates/{id}/files/delete-preview", adminOnly(b.ReviewCtrl.PreviewDeleteCandidateFiles))
				r.Delete("/review-candidates/{id}/files", adminOnly(b.ReviewCtrl.DeleteCandidateFiles))
				r.Put("/integrations/{id}", adminOnly(b.PluginCtrl.UpdateIntegration))
				r.Delete("/integrations/{id}", adminOnly(b.PluginCtrl.DeleteIntegration))
				r.Post("/plugins/{plugin_id}/browse", adminOnly(b.PluginCtrl.Browse))

				r.Get("/sync/status", adminOnly(b.SyncCtrl.Status))
				r.Post("/sync/push", adminOnly(b.SyncCtrl.Push))
				r.Post("/sync/pull", adminOnly(b.SyncCtrl.Pull))
				r.Post("/sync/key", adminOnly(b.SyncCtrl.StoreKey))
				r.Delete("/sync/key", adminOnly(b.SyncCtrl.ClearKey))
				r.Post("/save-sync/migrations", adminOnly(b.SaveSyncCtrl.StartMigration))
				r.Get("/save-sync/migrations/{job_id}", adminOnly(b.SaveSyncCtrl.GetMigrationStatus))
				r.Get("/save-sync/prefetch/{job_id}", b.SaveSyncCtrl.GetPrefetchStatus)
				r.Get("/cache/jobs", adminOnly(b.CacheCtrl.ListJobs))
				r.Get("/cache/jobs/{job_id}", adminOnly(b.CacheCtrl.GetJob))
				r.Get("/cache/entries", adminOnly(b.CacheCtrl.ListEntries))
				r.Delete("/cache/entries/{entry_id}", adminOnly(b.CacheCtrl.DeleteEntry))
				r.Post("/cache/clear", adminOnly(b.CacheCtrl.Clear))

				r.Get("/auth/callback/{plugin_id}", b.OAuthCtrl.Callback)
			})

			// Scan can take many minutes; no middleware timeout.
			api.Group(func(r chi.Router) {
				r.Use(ProfileContextMiddleware(b.ProfileRepo))
				r.Use(RequireAdminProfile)
				r.Get("/scan", b.DiscoCtrl.Scan)
				r.Post("/scan", b.DiscoCtrl.Scan)
				r.Get("/scan/jobs/{job_id}", b.DiscoCtrl.GetScanJob)
				r.Post("/scan/jobs/{job_id}/cancel", b.DiscoCtrl.CancelScanJob)
				r.Get("/scan/reports", b.DiscoCtrl.GetScanReports)
				r.Get("/scan/reports/{id}", b.DiscoCtrl.GetScanReport)
			})

			// Long-lived SSE stream; no middleware timeout.
			api.Get("/events", b.SSECtrl.Events)
		} else {
			api.Get("/games", noopHandler())
			api.Delete("/games", noopHandler())
			api.Get("/games/{id}/detail", noopHandler())
			api.Post("/games/{id}/refresh-metadata", noopHandler())
			api.Put("/games/{id}/cover-override", noopHandler())
			api.Delete("/games/{id}/cover-override", noopHandler())
			api.Put("/games/{id}/hover-override", noopHandler())
			api.Put("/games/{id}/background-override", noopHandler())
			api.Put("/games/{id}/favorite", noopHandler())
			api.Delete("/games/{id}/favorite", noopHandler())
			api.Post("/games/{id}/sources/{source_game_id}/delete-preview", noopHandler())
			api.Delete("/games/{id}/sources/{source_game_id}", noopHandler())
			api.Get("/games/{id}/play", noopHandler())
			api.Head("/games/{id}/play", noopHandler())
			api.Post("/games/{id}/cache/prepare", noopHandler())
			api.Post("/games/{id}/save-sync/prefetch", noopHandler())
			api.Get("/games/{id}/save-sync/slots", noopHandler())
			api.Get("/games/{id}/save-sync/slots/{slot_id}", noopHandler())
			api.Put("/games/{id}/save-sync/slots/{slot_id}", noopHandler())
			api.Get("/games/{id}", noopHandler())
			api.Get("/games/{id}/achievements", noopHandler())
			api.Get("/achievements", noopHandler())
			api.Get("/achievements/explorer", noopHandler())
			api.Get("/media/{assetID}", noopHandler())
			api.Put("/media/{assetID}/metadata", noopHandler())
			api.Get("/stats", noopHandler())
			api.Get("/about", noopHandler())
			api.Get("/about/license", noopHandler())
			api.Get("/config/frontend", noopHandler())
			api.Post("/config/frontend", noopHandler())
			api.Get("/scan", noopHandler())
			api.Post("/scan", noopHandler())
			api.Get("/scan/jobs/{job_id}", noopHandler())
			api.Post("/scan/jobs/{job_id}/cancel", noopHandler())
			api.Get("/scan/reports", noopHandler())
			api.Get("/scan/reports/{id}", noopHandler())
			api.Get("/plugins", noopHandler())
			api.Get("/plugins/{plugin_id}", noopHandler())
			api.Post("/config/{key}", noopHandler())
			api.Get("/integrations", noopHandler())
			api.Get("/integrations/status", noopHandler())
			api.Post("/integrations", noopHandler())
			api.Get("/integrations/{id}/status", noopHandler())
			api.Post("/integrations/{id}/authorize", noopHandler())
			api.Post("/integrations/{id}/refresh", noopHandler())
			api.Get("/integrations/{id}/games", noopHandler())
			api.Get("/integrations/{id}/enriched-games", noopHandler())
			api.Get("/integration-refresh/jobs/{job_id}", noopHandler())
			api.Get("/review-candidates", noopHandler())
			api.Post("/review-candidates/redetect", noopHandler())
			api.Get("/review-candidates/{id}", noopHandler())
			api.Post("/review-candidates/{id}/search", noopHandler())
			api.Post("/review-candidates/{id}/redetect", noopHandler())
			api.Post("/review-candidates/{id}/apply", noopHandler())
			api.Post("/review-candidates/{id}/not-a-game", noopHandler())
			api.Post("/review-candidates/{id}/unarchive", noopHandler())
			api.Post("/review-candidates/{id}/files/delete-preview", noopHandler())
			api.Delete("/review-candidates/{id}/files", noopHandler())
			api.Put("/integrations/{id}", noopHandler())
			api.Delete("/integrations/{id}", noopHandler())
			api.Post("/plugins/{plugin_id}/browse", noopHandler())
			api.Get("/sync/status", noopHandler())
			api.Post("/sync/push", noopHandler())
			api.Post("/sync/pull", noopHandler())
			api.Post("/sync/key", noopHandler())
			api.Delete("/sync/key", noopHandler())
			api.Post("/save-sync/migrations", noopHandler())
			api.Get("/save-sync/migrations/{job_id}", noopHandler())
			api.Get("/save-sync/prefetch/{job_id}", noopHandler())
			api.Get("/cache/jobs", noopHandler())
			api.Get("/cache/jobs/{job_id}", noopHandler())
			api.Get("/cache/entries", noopHandler())
			api.Delete("/cache/entries/{entry_id}", noopHandler())
			api.Post("/cache/clear", noopHandler())
			api.Get("/auth/callback/{plugin_id}", noopHandler())
			api.Get("/events", noopHandler())
		}
	})

	MountSPA(r, spaStaticDir)
	return r
}
