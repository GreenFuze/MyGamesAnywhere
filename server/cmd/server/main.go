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
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/http"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/keystore"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/logger"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
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

	processManager := plugins.NewProcessManager()
	eventBus := events.New()
	pluginHost := plugins.NewPluginHost(logSvc, configSvc, processManager, eventBus)

	ks := keystore.New()
	syncSvc := mgasync.NewSyncService(integrationRepo, settingRepo, pluginHost, ks, logSvc)
	orchestrator := scan.NewOrchestrator(pluginHost, pluginHost, integrationRepo, gameStore, logSvc)
	orchestrator.SetEventBus(eventBus)

	gameCtrl := http.NewGameController(gameStore, logSvc)
	discoCtrl := http.NewDiscoveryController(orchestrator, logSvc)
	configCtrl := http.NewConfigController(settingRepo, logSvc)
	pluginCtrl := http.NewPluginController(integrationRepo, pluginHost, logSvc, eventBus)
	achievementCtrl := http.NewAchievementController(gameStore, pluginHost, logSvc, eventBus)
	syncCtrl := http.NewSyncController(syncSvc, logSvc, eventBus)
	sseCtrl := http.NewSSEController(eventBus, logSvc)

	httpSvc := http.NewHttpServer(logSvc, configSvc, gameCtrl, discoCtrl, configCtrl, pluginCtrl, achievementCtrl, syncCtrl, sseCtrl)

	a := app.NewApp(logSvc, configSvc, dbSvc, httpSvc, nil, pluginHost, eventBus)

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
