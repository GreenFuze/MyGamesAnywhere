package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type httpServer struct {
	server          *http.Server
	logger          core.Logger
	config          core.Configuration
	gameCtrl        *GameController
	discoCtrl       *DiscoveryController
	configCtrl      *ConfigController
	pluginCtrl *PluginController
}

func NewHttpServer(
	logger core.Logger,
	config core.Configuration,
	gameCtrl *GameController,
	discoCtrl *DiscoveryController,
	configCtrl *ConfigController,
	pluginCtrl *PluginController,
) core.Server {
	return &httpServer{
		logger:          logger,
		config:          config,
		gameCtrl:        gameCtrl,
		discoCtrl:       discoCtrl,
		configCtrl:      configCtrl,
		pluginCtrl: pluginCtrl,
	}
}

func (h *httpServer) Start(ctx context.Context) error {
	port := h.config.Get("PORT")
	if port == "" {
		panic("No port was defined")
	}

	r := BuildRouter(&RouteBuilder{
		GameCtrl:        h.gameCtrl,
		DiscoCtrl:       h.discoCtrl,
		ConfigCtrl:      h.configCtrl,
		PluginCtrl: h.pluginCtrl,
	}, 60*time.Second)

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
