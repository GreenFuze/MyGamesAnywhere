package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/auth"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
)

type App struct {
	logger     core.Logger
	config     core.Configuration
	db         core.Database
	server     core.Server
	authSvc    auth.AuthService
	pluginHost plugins.PluginHost
	syncSvc    core.SettingsSyncProvider
}

func NewApp(
	logger core.Logger,
	config core.Configuration,
	db core.Database,
	server core.Server,
	authSvc auth.AuthService,
	pluginHost plugins.PluginHost,
	syncSvc core.SettingsSyncProvider,
) *App {
	return &App{
		logger:     logger,
		config:     config,
		db:         db,
		server:     server,
		authSvc:    authSvc,
		pluginHost: pluginHost,
		syncSvc:    syncSvc,
	}
}

// Run starts the application and blocks until the context is cancelled (e.g. by signal or tray Exit).
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("Starting MyGamesAnywhere Server...")

	if err := a.config.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	if err := a.pluginHost.Discover(context.Background()); err != nil {
		a.logger.Error("Plugin discovery failed", err)
	}

	if err := a.db.Connect(); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer a.db.Close()

	// Cloud restore: the sync provider reads integration config from the DB,
	// closes the DB, lets the plugin replace the file, then we reconnect.
	if a.syncSvc != nil {
		if err := a.syncSvc.Restore(context.Background(), nil); err != nil {
			a.logger.Error("Cloud restore failed", err)
		}
		if err := a.db.Connect(); err != nil {
			return fmt.Errorf("database connection failed (post-restore): %w", err)
		}
	}

	if err := a.db.EnsureSchema(); err != nil {
		return fmt.Errorf("database schema creation failed: %w", err)
	}

	if a.authSvc != nil {
		if err := a.authSvc.CreateInitialAdmin(context.Background(), "admin", "admin"); err != nil {
			a.logger.Error("Failed to create initial admin", err)
		} else {
			a.logger.Warn("Initial admin user created or already exists (admin:admin). Change password immediately!")
		}
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errChan := make(chan error, 1)
	go func() {
		if err := a.server.Start(ctx); err != nil {
			errChan <- err
		}
	}()

	select {
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		a.logger.Info("Shutting down...")

		// Close plugin host
		if err := a.pluginHost.Close(); err != nil {
			a.logger.Error("Plugin host shutdown failed", err)
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.server.Stop(shutdownCtx); err != nil {
			a.logger.Error("Server shutdown failed", err)
		}
	}

	return nil
}
