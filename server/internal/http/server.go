package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	appconfig "github.com/GreenFuze/MyGamesAnywhere/server/internal/config"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type httpServer struct {
	server                 *http.Server
	logger                 core.Logger
	config                 core.Configuration
	gameCtrl               *GameController
	mediaCtrl              *MediaController
	discoCtrl              *DiscoveryController
	aboutCtrl              *AboutController
	configCtrl             *ConfigController
	pluginCtrl             *PluginController
	integrationRefreshCtrl *IntegrationRefreshController
	reviewCtrl             *ReviewController
	achievementCtrl        *AchievementController
	syncCtrl               *SyncController
	saveSyncCtrl           *SaveSyncController
	cacheCtrl              *CacheController
	sseCtrl                *SSEController
	oauthCtrl              *OAuthController
	profileCtrl            *ProfileController
	profileRepo            core.ProfileRepository
}

func NewHttpServer(
	logger core.Logger,
	config core.Configuration,
	gameCtrl *GameController,
	mediaCtrl *MediaController,
	discoCtrl *DiscoveryController,
	aboutCtrl *AboutController,
	configCtrl *ConfigController,
	pluginCtrl *PluginController,
	integrationRefreshCtrl *IntegrationRefreshController,
	reviewCtrl *ReviewController,
	achievementCtrl *AchievementController,
	syncCtrl *SyncController,
	saveSyncCtrl *SaveSyncController,
	cacheCtrl *CacheController,
	sseCtrl *SSEController,
	oauthCtrl *OAuthController,
	profileCtrl *ProfileController,
	profileRepo core.ProfileRepository,
) core.Server {
	return &httpServer{
		logger:                 logger,
		config:                 config,
		gameCtrl:               gameCtrl,
		mediaCtrl:              mediaCtrl,
		discoCtrl:              discoCtrl,
		aboutCtrl:              aboutCtrl,
		configCtrl:             configCtrl,
		pluginCtrl:             pluginCtrl,
		integrationRefreshCtrl: integrationRefreshCtrl,
		reviewCtrl:             reviewCtrl,
		achievementCtrl:        achievementCtrl,
		syncCtrl:               syncCtrl,
		saveSyncCtrl:           saveSyncCtrl,
		cacheCtrl:              cacheCtrl,
		sseCtrl:                sseCtrl,
		oauthCtrl:              oauthCtrl,
		profileCtrl:            profileCtrl,
		profileRepo:            profileRepo,
	}
}

func (h *httpServer) Start(ctx context.Context) error {
	addr, err := appconfig.ListenAddr(h.config)
	if err != nil {
		return fmt.Errorf("http server listen address: %w", err)
	}

	spaDir := h.config.Get("FRONTEND_DIST")
	if spaDir == "" {
		spaDir = "frontend/dist"
	}
	if !filepath.IsAbs(spaDir) {
		if wd, err := os.Getwd(); err == nil {
			spaDir = filepath.Join(wd, spaDir)
		}
	}

	r := BuildRouter(&RouteBuilder{
		GameCtrl:               h.gameCtrl,
		MediaCtrl:              h.mediaCtrl,
		DiscoCtrl:              h.discoCtrl,
		AboutCtrl:              h.aboutCtrl,
		ConfigCtrl:             h.configCtrl,
		PluginCtrl:             h.pluginCtrl,
		IntegrationRefreshCtrl: h.integrationRefreshCtrl,
		ReviewCtrl:             h.reviewCtrl,
		AchievementCtrl:        h.achievementCtrl,
		SyncCtrl:               h.syncCtrl,
		SaveSyncCtrl:           h.saveSyncCtrl,
		CacheCtrl:              h.cacheCtrl,
		SSECtrl:                h.sseCtrl,
		OAuthCtrl:              h.oauthCtrl,
		ProfileCtrl:            h.profileCtrl,
		ProfileRepo:            h.profileRepo,
	}, 60*time.Second, spaDir)

	h.server = &http.Server{
		Addr:    addr,
		Handler: r,
	}

	h.logger.Info("Starting HTTP server", "addr", addr)

	if err := h.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server start failed: %w", err)
	}

	return nil
}

func (h *httpServer) Stop(ctx context.Context) error {
	if h.server != nil {
		h.logger.Info("Stopping HTTP server")
		return h.server.Shutdown(ctx)
	}
	return nil
}
