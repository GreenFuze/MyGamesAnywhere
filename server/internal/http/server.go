package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type httpServer struct {
	server          *http.Server
	logger          core.Logger
	config          core.Configuration
	gameCtrl        *GameController
	mediaCtrl       *MediaController
	discoCtrl       *DiscoveryController
	aboutCtrl       *AboutController
	configCtrl      *ConfigController
	pluginCtrl      *PluginController
	achievementCtrl *AchievementController
	syncCtrl        *SyncController
	saveSyncCtrl    *SaveSyncController
	sseCtrl         *SSEController
	oauthCtrl       *OAuthController
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
	achievementCtrl *AchievementController,
	syncCtrl *SyncController,
	saveSyncCtrl *SaveSyncController,
	sseCtrl *SSEController,
	oauthCtrl *OAuthController,
) core.Server {
	return &httpServer{
		logger:          logger,
		config:          config,
		gameCtrl:        gameCtrl,
		mediaCtrl:       mediaCtrl,
		discoCtrl:       discoCtrl,
		aboutCtrl:       aboutCtrl,
		configCtrl:      configCtrl,
		pluginCtrl:      pluginCtrl,
		achievementCtrl: achievementCtrl,
		syncCtrl:        syncCtrl,
		saveSyncCtrl:    saveSyncCtrl,
		sseCtrl:         sseCtrl,
		oauthCtrl:       oauthCtrl,
	}
}

func (h *httpServer) Start(ctx context.Context) error {
	port := h.config.Get("PORT")
	if port == "" {
		panic("No port was defined")
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
		GameCtrl:        h.gameCtrl,
		MediaCtrl:       h.mediaCtrl,
		DiscoCtrl:       h.discoCtrl,
		AboutCtrl:       h.aboutCtrl,
		ConfigCtrl:      h.configCtrl,
		PluginCtrl:      h.pluginCtrl,
		AchievementCtrl: h.achievementCtrl,
		SyncCtrl:        h.syncCtrl,
		SaveSyncCtrl:    h.saveSyncCtrl,
		SSECtrl:         h.sseCtrl,
		OAuthCtrl:       h.oauthCtrl,
	}, 60*time.Second, spaDir)

	h.server = &http.Server{
		Addr:    "127.0.0.1:" + port,
		Handler: r,
	}

	h.logger.Info("Starting HTTP server", "port", port)

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
