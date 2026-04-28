package main

// Windows File Explorer icon: COFF resource (tray uses //go:embed in tray_windows.go separately).
// After editing mga.ico: go generate ./cmd/server  (amd64) or run server/build.ps1 (matches host GOARCH).
//
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico mga.ico -arch amd64 -o rsrc_windows_amd64.syso

import (
	"context"
	"log"
	"os"
	"path/filepath"

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
	saveSync "github.com/GreenFuze/MyGamesAnywhere/server/internal/save_sync"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcecache"
	mgasync "github.com/GreenFuze/MyGamesAnywhere/server/internal/sync"
)

func main() {
	execPath, err := os.Executable()
	if err != nil {
		log.Fatalf("get executable path: %v", err)
	}
	exeDir := filepath.Dir(execPath)
	if err := os.Chdir(exeDir); err != nil {
		log.Fatalf("chdir to executable directory %s: %v", exeDir, err)
	}

	logSvc := logger.NewLogService()
	configSvc, err := config.NewConfigService("config.json")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	dbSvc := db.NewSQLiteDatabase(logSvc, configSvc)

	settingRepo := db.NewSettingRepository(dbSvc)
	integrationRepo := db.NewIntegrationRepository(dbSvc)
	gameStore := db.NewGameStore(dbSvc, logSvc)
	cacheStore := db.NewSourceCacheStore(dbSvc)

	processManager := plugins.NewProcessManager()
	eventBus := events.New()
	pluginHost := plugins.NewPluginHost(logSvc, configSvc, processManager, eventBus)

	ks := keystore.New()
	syncSvc := mgasync.NewSyncService(integrationRepo, settingRepo, pluginHost, ks, logSvc)
	saveSyncSvc := saveSync.NewService(integrationRepo, gameStore, pluginHost, logSvc, eventBus)
	cacheSvc := sourcecache.NewService(cacheStore, integrationRepo, pluginHost, configSvc, logSvc)
	mediaSvc := media.NewService(gameStore, configSvc, logSvc)
	orchestrator := scan.NewOrchestrator(pluginHost, pluginHost, integrationRepo, gameStore, mediaSvc, logSvc)
	orchestrator.SetEventBus(eventBus)
	manualReviewSvc := scan.NewManualReviewService(pluginHost, pluginHost, integrationRepo, gameStore, mediaSvc, logSvc)
	integrationRefreshSvc := scan.NewIntegrationRefreshService(integrationRepo, gameStore, pluginHost, mediaSvc, configSvc, logSvc)
	deletionSvc := gamesvc.NewDeletionService(gameStore, integrationRepo, pluginHost, logSvc)

	gameCtrl := http.NewGameController(gameStore, orchestrator, deletionSvc, integrationRepo, cacheSvc, logSvc)
	mediaCtrl := http.NewMediaController(gameStore, configSvc, logSvc)
	discoCtrl := http.NewDiscoveryController(orchestrator, gameStore, logSvc, eventBus)
	aboutCtrl := http.NewAboutController(logSvc)
	configCtrl := http.NewConfigController(settingRepo, logSvc)
	pluginCtrl := http.NewPluginController(integrationRepo, pluginHost, gameStore, configSvc, logSvc, eventBus)
	integrationRefreshCtrl := http.NewIntegrationRefreshController(integrationRepo, pluginHost, integrationRefreshSvc, eventBus, logSvc)
	reviewCtrl := http.NewReviewController(integrationRepo, pluginHost, gameStore, manualReviewSvc, logSvc)
	achievementCtrl := http.NewAchievementController(gameStore, pluginHost, integrationRepo, logSvc, eventBus)
	syncCtrl := http.NewSyncController(syncSvc, logSvc, eventBus)
	saveSyncCtrl := http.NewSaveSyncController(saveSyncSvc, logSvc)
	cacheCtrl := http.NewCacheController(gameStore, integrationRepo, cacheSvc, logSvc)
	sseCtrl := http.NewSSEController(eventBus, logSvc)
	oauthCtrl := http.NewOAuthController(pluginHost, configSvc, logSvc, eventBus)

	httpSvc := http.NewHttpServer(logSvc, configSvc, gameCtrl, mediaCtrl, discoCtrl, aboutCtrl, configCtrl, pluginCtrl, integrationRefreshCtrl, reviewCtrl, achievementCtrl, syncCtrl, saveSyncCtrl, cacheCtrl, sseCtrl, oauthCtrl)

	a := app.NewApp(logSvc, configSvc, dbSvc, httpSvc, nil, pluginHost, eventBus, mediaSvc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	port := configSvc.Get("PORT")
	if port == "" {
		port = "8900"
	}
	go runTray(cancel, "http://127.0.0.1:"+port)

	if err := a.Run(ctx); err != nil {
		log.Fatalf("application failed: %v", err)
	}
}
