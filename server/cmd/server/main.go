package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/app"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/config"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/http"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/logger"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/scan"
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
	pluginHost := plugins.NewPluginHost(logSvc, configSvc, processManager)
	syncSvc := plugins.NewPluginSyncProvider(pluginHost, integrationRepo, dbSvc, configSvc, logSvc)

	orchestrator := scan.NewOrchestrator(pluginHost, pluginHost, integrationRepo, gameStore, logSvc)

	gameCtrl := http.NewGameController(gameStore, logSvc)
	discoCtrl := http.NewDiscoveryController(orchestrator, logSvc)
	configCtrl := http.NewConfigController(settingRepo, logSvc)
	pluginCtrl := http.NewPluginController(integrationRepo, pluginHost, logSvc)
	achievementCtrl := http.NewAchievementController(gameStore, pluginHost, logSvc)

	httpSvc := http.NewHttpServer(logSvc, configSvc, gameCtrl, discoCtrl, configCtrl, pluginCtrl, achievementCtrl)

	a := app.NewApp(logSvc, configSvc, dbSvc, httpSvc, nil, pluginHost, syncSvc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runTray(cancel)

	if err := a.Run(ctx); err != nil {
		log.Fatalf("application failed: %v", err)
	}
}
