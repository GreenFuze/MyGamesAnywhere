package main

// Windows File Explorer icon: COFF resource (tray uses //go:embed in tray_windows.go separately).
// After editing mga.ico: go generate ./cmd/server  (amd64) or run server/build.ps1 (matches host GOARCH).
//
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico mga.ico -arch amd64 -o rsrc_windows_amd64.syso

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/app"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/config"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/gamesvc"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/http"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/keystore"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/logger"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/media"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	mgaruntime "github.com/GreenFuze/MyGamesAnywhere/server/internal/runtime"
	saveSync "github.com/GreenFuze/MyGamesAnywhere/server/internal/save_sync"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcecache"
	mgasync "github.com/GreenFuze/MyGamesAnywhere/server/internal/sync"
	mgaupdate "github.com/GreenFuze/MyGamesAnywhere/server/internal/update"
)

func main() {
	opts, err := parseOptions()
	if err != nil {
		log.Fatalf("parse options: %v", err)
	}
	run := func(ctx context.Context) error {
		err := runServer(ctx, opts)
		if err != nil {
			writeBootstrapError(opts, err)
		}
		return err
	}
	if opts.service {
		if err := runWindowsService("MyGamesAnywhere", run); err != nil {
			log.Fatalf("service failed: %v", err)
		}
		return
	}
	if err := run(context.Background()); err != nil {
		log.Fatalf("application failed: %v", err)
	}
}

func writeBootstrapError(opts serverOptions, err error) {
	if err == nil {
		return
	}
	dataDir := strings.TrimSpace(opts.dataDir)
	if dataDir == "" {
		return
	}
	if abs, absErr := filepath.Abs(dataDir); absErr == nil {
		dataDir = abs
	}
	if mkErr := os.MkdirAll(dataDir, 0o755); mkErr != nil {
		return
	}
	path := filepath.Join(dataDir, "mga_server_bootstrap.log")
	line := fmt.Sprintf("%s startup failed: %v\n", time.Now().Format(time.RFC3339Nano), err)
	_ = os.WriteFile(path, []byte(line), 0o644)
}

type serverOptions struct {
	configPath string
	dataDir    string
	appDir     string
	mode       mgaruntime.Mode
	service    bool
	noTray     bool
}

func parseOptions() (serverOptions, error) {
	var opts serverOptions
	var mode string
	flag.StringVar(&opts.configPath, "config", envString("MGA_CONFIG", ""), "Path to config.json.")
	flag.StringVar(&opts.dataDir, "data-dir", envString("MGA_DATA_DIR", ""), "Mutable data directory.")
	flag.StringVar(&opts.appDir, "app-dir", envString("MGA_APP_DIR", ""), "Immutable app directory.")
	flag.StringVar(&mode, "runtime-mode", envString("MGA_RUNTIME_MODE", ""), "Runtime mode: portable, user, or machine.")
	flag.BoolVar(&opts.service, "service", envBool("MGA_SERVICE", false), "Run as a Windows service.")
	flag.BoolVar(&opts.noTray, "no-tray", envBool("MGA_NO_TRAY", false), "Disable the tray companion from this server process.")
	flag.Parse()
	if strings.TrimSpace(mode) != "" {
		opts.mode = mgaruntime.Mode(strings.ToLower(strings.TrimSpace(mode)))
	}
	return opts, nil
}

func runServer(ctx context.Context, opts serverOptions) error {
	layout, err := mgaruntime.Resolve(mgaruntime.Options{
		AppDir:     opts.appDir,
		DataDir:    opts.dataDir,
		ConfigPath: opts.configPath,
		Mode:       opts.mode,
		Service:    opts.service,
	})
	if err != nil {
		return fmt.Errorf("resolve runtime layout: %w", err)
	}
	if err := layout.EnsureConfig(); err != nil {
		return fmt.Errorf("prepare runtime config: %w", err)
	}
	if err := os.Chdir(layout.AppDir); err != nil {
		return fmt.Errorf("chdir to app directory %s: %w", layout.AppDir, err)
	}
	configSvc, err := config.NewConfigService(layout.ConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := configSvc.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	logFile := strings.TrimSpace(configSvc.Get("LOG_FILE"))
	if logFile == "" {
		logFile = layout.LogFile
	}
	logSvc, err := logger.NewLogServiceWithOptions(logger.Options{
		FilePath:   logFile,
		BaseDir:    layout.DataDir,
		MaxSizeMB:  configSvc.GetInt("LOG_MAX_SIZE_MB"),
		MaxBackups: configSvc.GetInt("LOG_MAX_BACKUPS"),
	})
	if err != nil {
		return fmt.Errorf("configure logging: %w", err)
	}
	if closer, ok := logSvc.(interface{ Close() error }); ok {
		defer func() { _ = closer.Close() }()
	}
	dbSvc := db.NewSQLiteDatabase(logSvc, configSvc)

	settingRepo := db.NewSettingRepository(dbSvc)
	profileRepo := db.NewProfileRepository(dbSvc)
	integrationRepo := db.NewIntegrationRepository(dbSvc)
	gameStore := db.NewGameStore(dbSvc, logSvc)
	cacheStore := db.NewSourceCacheStore(dbSvc)

	processManager := plugins.NewProcessManager()
	eventBus := events.New()
	pluginHost := plugins.NewPluginHost(logSvc, configSvc, processManager, eventBus)

	ks := keystore.New()
	syncSvc := mgasync.NewSyncService(integrationRepo, settingRepo, profileRepo, pluginHost, ks, logSvc)
	updateSvc := mgaupdate.NewService(configSvc, logSvc)
	saveSyncSvc := saveSync.NewService(integrationRepo, gameStore, pluginHost, logSvc, eventBus)
	cacheSvc := sourcecache.NewService(cacheStore, integrationRepo, pluginHost, configSvc, logSvc)
	mediaSvc := media.NewService(gameStore, configSvc, logSvc)
	orchestrator := scan.NewOrchestrator(pluginHost, pluginHost, integrationRepo, gameStore, mediaSvc, logSvc)
	orchestrator.SetEventBus(eventBus)
	manualReviewSvc := scan.NewManualReviewService(pluginHost, pluginHost, integrationRepo, gameStore, mediaSvc, logSvc)
	integrationRefreshSvc := scan.NewIntegrationRefreshService(integrationRepo, gameStore, pluginHost, mediaSvc, configSvc, logSvc)
	deletionSvc := gamesvc.NewDeletionService(gameStore, integrationRepo, pluginHost, logSvc)

	gameCtrl := http.NewGameController(gameStore, orchestrator, deletionSvc, integrationRepo, cacheSvc, logSvc)
	mediaCtrl := http.NewMediaController(gameStore, configSvc, logSvc, mediaSvc)
	discoCtrl := http.NewDiscoveryController(orchestrator, gameStore, logSvc, eventBus)
	aboutCtrl := http.NewAboutController(logSvc)
	configCtrl := http.NewConfigController(settingRepo, logSvc)
	pluginCtrl := http.NewPluginController(integrationRepo, pluginHost, gameStore, configSvc, logSvc, eventBus, syncSvc)
	integrationRefreshCtrl := http.NewIntegrationRefreshController(integrationRepo, pluginHost, integrationRefreshSvc, eventBus, logSvc)
	reviewCtrl := http.NewReviewController(integrationRepo, pluginHost, gameStore, manualReviewSvc, deletionSvc, logSvc)
	achievementCtrl := http.NewAchievementController(gameStore, pluginHost, integrationRepo, logSvc, eventBus)
	syncCtrl := http.NewSyncController(syncSvc, logSvc, eventBus)
	updateCtrl := http.NewUpdateController(updateSvc, logSvc)
	saveSyncCtrl := http.NewSaveSyncController(saveSyncSvc, logSvc)
	cacheCtrl := http.NewCacheController(gameStore, integrationRepo, cacheSvc, logSvc)
	sseCtrl := http.NewSSEController(eventBus, logSvc)
	oauthCtrl := http.NewOAuthController(pluginHost, configSvc, logSvc, eventBus, integrationRepo)
	profileCtrl := http.NewProfileController(profileRepo, syncSvc, discoCtrl, configSvc, logSvc)

	httpSvc := http.NewHttpServer(logSvc, configSvc, gameCtrl, mediaCtrl, discoCtrl, aboutCtrl, configCtrl, pluginCtrl, integrationRefreshCtrl, reviewCtrl, achievementCtrl, syncCtrl, updateCtrl, saveSyncCtrl, cacheCtrl, sseCtrl, oauthCtrl, profileCtrl, profileRepo)

	a := app.NewApp(logSvc, configSvc, dbSvc, httpSvc, nil, pluginHost, eventBus, mediaSvc)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	baseURL, err := config.LocalBaseURL(configSvc)
	if err != nil {
		return fmt.Errorf("resolve local URL: %w", err)
	}
	if !opts.noTray && !opts.service {
		go runTray(cancel, baseURL)
	}

	if err := a.Run(ctx); err != nil {
		return fmt.Errorf("application failed: %w", err)
	}
	return nil
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		switch strings.ToLower(value) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}
