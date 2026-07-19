package main

// Windows File Explorer icon: COFF resource (tray uses //go:embed in tray_windows.go separately).
// After editing mga.ico: go generate ./cmd/server  (amd64) or run server/build.ps1 (matches host GOARCH).
//
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico mga.ico -arch amd64 -o rsrc_windows_amd64.syso

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/app"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/auth"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/config"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/emulation"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/gamesvc"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/http"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/installprefs"
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
	configPath                 string
	dataDir                    string
	appDir                     string
	mode                       mgaruntime.Mode
	service                    bool
	noTray                     bool
	migrateOnly                bool
	resetCredentialProfile     string
	migrationBackupDir         string
	skipStartupMigrationBackup bool
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
	flag.BoolVar(&opts.migrateOnly, "migrate-only", envBool("MGA_MIGRATE_ONLY", false), "Run database migrations and exit without starting MGA.")
	flag.StringVar(&opts.resetCredentialProfile, "reset-profile-credential", envString("MGA_RESET_PROFILE_CREDENTIAL", ""), "Reset one profile credential to changeme and require an immediate change, then exit.")
	flag.StringVar(&opts.migrationBackupDir, "migration-backup-dir", envString("MGA_MIGRATION_BACKUP_DIR", ""), "Directory for database migration backups.")
	flag.BoolVar(&opts.skipStartupMigrationBackup, "skip-startup-migration-backup", envBool("MGA_SKIP_STARTUP_MIGRATION_BACKUP", false), "Skip the pre-migration database backup.")
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
	migrationOptions := core.MigrationOptions{
		BackupBeforeMigrate: !opts.skipStartupMigrationBackup,
		BackupDir:           opts.migrationBackupDir,
	}
	dbSvc := db.NewSQLiteDatabaseWithMigrationOptions(logSvc, configSvc, migrationOptions)
	if strings.TrimSpace(opts.resetCredentialProfile) != "" {
		dbPath := strings.TrimSpace(configSvc.Get("DB_PATH"))
		if dbPath == "" {
			return errors.New("credential recovery requires DB_PATH")
		}
		if !filepath.IsAbs(dbPath) {
			dbPath = filepath.Join(layout.AppDir, dbPath)
		}
		if _, err := os.Stat(dbPath); err != nil {
			return fmt.Errorf("credential recovery database %s: %w", dbPath, err)
		}
		if err := dbSvc.Connect(); err != nil {
			return fmt.Errorf("credential recovery database connection failed: %w", err)
		}
		defer dbSvc.Close()
		profileRepo := db.NewProfileRepository(dbSvc)
		authSvc, err := auth.NewService(db.NewAuthStore(dbSvc), profileRepo)
		if err != nil {
			return fmt.Errorf("configure credential recovery: %w", err)
		}
		profileID := strings.TrimSpace(opts.resetCredentialProfile)
		if err := authSvc.ResetCredentialToBootstrap(ctx, profileID); err != nil {
			return fmt.Errorf("reset profile credential: %w", err)
		}
		logSvc.Info("profile credential reset; changeme must be replaced at next sign-in", "profile_id", profileID)
		return nil
	}
	if opts.migrateOnly {
		if err := dbSvc.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer dbSvc.Close()
		if err := dbSvc.Migrate(migrationOptions); err != nil {
			return fmt.Errorf("database migration failed: %w", err)
		}
		logSvc.Info("database migration completed")
		return nil
	}

	settingRepo := db.NewSettingRepository(dbSvc)
	profileRepo := db.NewProfileRepository(dbSvc)
	authStore := db.NewAuthStore(dbSvc)
	authSvc, err := auth.NewService(authStore, profileRepo)
	if err != nil {
		return fmt.Errorf("configure profile authentication: %w", err)
	}
	deviceStore := db.NewDeviceStore(dbSvc)
	commandRecovery, err := devices.NewCommandRecovery(deviceStore, logSvc)
	if err != nil {
		return fmt.Errorf("configure device command recovery: %w", err)
	}
	deviceHub := devices.NewHub()
	deviceSvc, err := devices.NewService(deviceStore, deviceHub)
	if err != nil {
		return fmt.Errorf("configure device service: %w", err)
	}
	installPreferenceRepo := db.NewInstallPreferenceRepository(dbSvc)
	installPreferenceSvc, err := installprefs.NewService(installPreferenceRepo, deviceSvc)
	if err != nil {
		return fmt.Errorf("configure install preferences: %w", err)
	}
	emulatorPreferenceRepo := db.NewEmulatorPreferenceRepository(dbSvc)
	emulationSvc, err := emulation.NewService(emulatorPreferenceRepo, deviceSvc, emulation.NewDefaultCatalog())
	if err != nil {
		return fmt.Errorf("configure emulator settings: %w", err)
	}
	integrationRepo := db.NewIntegrationRepository(dbSvc)
	gameStore := db.NewGameStore(dbSvc, logSvc)
	cacheStore := db.NewSourceCacheStore(dbSvc)

	processManager := plugins.NewProcessManager()
	eventBus := events.New()
	deviceSvc.SetEventBus(eventBus)
	pluginHost := plugins.NewPluginHost(logSvc, configSvc, processManager, eventBus)

	ks := keystore.New()
	syncSvc := mgasync.NewSyncService(integrationRepo, settingRepo, profileRepo, pluginHost, ks, logSvc)
	updateSvc := mgaupdate.NewService(configSvc, logSvc, eventBus)
	saveSyncSvc := saveSync.NewService(integrationRepo, gameStore, pluginHost, logSvc, eventBus)
	cacheSvc := sourcecache.NewService(cacheStore, integrationRepo, pluginHost, configSvc, logSvc)
	mediaSvc := media.NewService(gameStore, configSvc, logSvc)
	orchestrator := scan.NewOrchestrator(pluginHost, pluginHost, integrationRepo, gameStore, mediaSvc, logSvc)
	orchestrator.SetEventBus(eventBus)
	manualReviewSvc := scan.NewManualReviewService(pluginHost, pluginHost, integrationRepo, gameStore, mediaSvc, logSvc)
	integrationRefreshSvc := scan.NewIntegrationRefreshService(integrationRepo, gameStore, pluginHost, mediaSvc, configSvc, logSvc)
	achievementRefreshSvc := scan.NewAchievementRefreshService(integrationRepo, gameStore, pluginHost, logSvc)
	deletionSvc := gamesvc.NewDeletionService(gameStore, integrationRepo, pluginHost, logSvc)
	groupingSvc := gamesvc.NewCanonicalGroupingService(gameStore)

	achievementRefreshCtrl := http.NewAchievementRefreshController(achievementRefreshSvc, eventBus, logSvc)
	gameCtrl := http.NewGameController(gameStore, orchestrator, deletionSvc, integrationRepo, cacheSvc, logSvc)
	gameCtrl.SetCanonicalGroupingService(groupingSvc)
	gameCtrl.SetDeviceEndpointLister(deviceSvc)
	gameCtrl.SetEmulationService(emulationSvc)
	gameCtrl.SetEmulatorContentRoot(envString("MGA_GOOGLE_DRIVE_DESKTOP_ROOT", ""))
	mediaCtrl := http.NewMediaController(gameStore, configSvc, logSvc, mediaSvc)
	discoCtrl := http.NewDiscoveryController(orchestrator, gameStore, logSvc, eventBus, achievementRefreshCtrl)
	backgroundScanSvc, err := http.NewBackgroundScanService(discoCtrl, profileRepo, settingRepo, logSvc, eventBus)
	if err != nil {
		return fmt.Errorf("configure background library scans: %w", err)
	}
	discoCtrl.SetBackgroundScanService(backgroundScanSvc)
	installationValidationSvc, err := http.NewInstallationValidationService(deviceSvc, profileRepo, settingRepo, logSvc, eventBus)
	if err != nil {
		return fmt.Errorf("configure installation validation: %w", err)
	}
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
	oauthCtrl.SetGameStore(gameStore)
	profileCtrl := http.NewProfileController(profileRepo, syncSvc, discoCtrl, configSvc, logSvc)
	profileCtrl.SetAuthService(authSvc)
	authCtrl, err := http.NewAuthController(authSvc, profileRepo, logSvc)
	if err != nil {
		return fmt.Errorf("configure auth controller: %w", err)
	}
	clientInstallerPath := envString("MGA_CLIENT_INSTALLER_PATH", "")
	if clientInstallerPath == "" {
		candidate := filepath.Join(layout.AppDir, "downloads", "mga-client-windows-amd64-installer.exe")
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			clientInstallerPath = candidate
		}
	}
	if clientInstallerPath != "" {
		clientInstallerPath, err = filepath.Abs(clientInstallerPath)
		if err != nil {
			return fmt.Errorf("resolve MGA Client installer path: %w", err)
		}
	}
	deviceCtrl, err := http.NewDeviceController(deviceSvc, deviceHub, logSvc, clientInstallerPath)
	if err != nil {
		return fmt.Errorf("configure device controller: %w", err)
	}
	deviceCtrl.SetArchiveInstallDependencies(gameStore, integrationRepo, envString("MGA_GOOGLE_DRIVE_DESKTOP_ROOT", ""))
	deviceCtrl.SetInstallationValidationService(installationValidationSvc)
	deviceCtrl.SetInstallPreferenceService(installPreferenceSvc)
	deviceCtrl.SetEmulationService(emulationSvc)
	deviceCtrl.SetSaveDomainDependencies(saveSyncSvc)

	httpSvc := http.NewHttpServer(logSvc, configSvc, gameCtrl, mediaCtrl, discoCtrl, aboutCtrl, configCtrl, pluginCtrl, integrationRefreshCtrl, reviewCtrl, achievementCtrl, achievementRefreshCtrl, syncCtrl, updateCtrl, saveSyncCtrl, cacheCtrl, sseCtrl, oauthCtrl, profileCtrl, profileRepo, authCtrl, authSvc, deviceCtrl)

	a := app.NewApp(logSvc, configSvc, dbSvc, httpSvc, authSvc, pluginHost, eventBus, mediaSvc, backgroundScanSvc, installationValidationSvc)
	a.AddStartupTask(commandRecovery)

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
