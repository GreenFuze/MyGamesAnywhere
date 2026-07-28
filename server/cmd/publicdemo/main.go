// Command publicdemo creates an isolated, fictional MGA library for public
// documentation screenshots. It must never be pointed at a real MGA database.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
)

const demoProfileID = "public-demo-player"

// demoArtwork keeps screenshot fixtures reproducible and independent of
// commercial artwork or third-party services.
//
//go:embed assets/*-cover.webp assets/*-background.webp
var demoArtwork embed.FS

type options struct {
	databasePath string
	coversDir    string
	coverBaseURL string
	serverConfig string
	appDir       string
	port         int
}

type staticConfiguration struct {
	databasePath string
}

func (c staticConfiguration) Get(key string) string {
	if strings.EqualFold(strings.TrimSpace(key), "DB_PATH") {
		return c.databasePath
	}
	return ""
}

func (staticConfiguration) GetInt(string) int   { return 0 }
func (staticConfiguration) GetBool(string) bool { return false }
func (c staticConfiguration) Validate() error {
	if strings.TrimSpace(c.databasePath) == "" {
		return errors.New("database path is required")
	}
	return nil
}

type consoleLogger struct{}

func (consoleLogger) Info(string, ...any)                   {}
func (consoleLogger) Debug(string, ...any)                  {}
func (consoleLogger) Warn(msg string, args ...any)          { fmt.Printf("warning: "+msg+"\n", args...) }
func (consoleLogger) Error(msg string, err error, _ ...any) { fmt.Printf("error: %s: %v\n", msg, err) }

type demoGame struct {
	slug        string
	title       string
	platform    core.Platform
	description string
	genres      []string
	developer   string
	releaseDate string
	rating      float64
	steam       bool
	xbox        bool
	retro       bool
	gamePass    bool
	xcloud      bool
	favorite    bool
}

type seeder struct {
	database core.Database
	store    core.GameStore
	profiles core.ProfileRepository
	sources  core.IntegrationRepository
	options  options
}

func main() {
	var opts options
	flag.StringVar(&opts.databasePath, "db", "", "Path to the isolated SQLite database.")
	flag.StringVar(&opts.coversDir, "covers-dir", "", "Directory in which to create fictional showcase artwork.")
	flag.StringVar(&opts.coverBaseURL, "cover-base-url", "http://127.0.0.1:8766", "HTTP root serving covers-dir.")
	flag.StringVar(&opts.serverConfig, "server-config", "", "Path at which to create an isolated MGA server config.")
	flag.StringVar(&opts.appDir, "app-dir", "", "Current MGA application directory containing frontend and plugins.")
	flag.IntVar(&opts.port, "port", 8911, "Loopback port for the isolated MGA server.")
	flag.Parse()

	if err := run(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, "public demo seed failed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, opts options) error {
	if strings.TrimSpace(opts.databasePath) == "" {
		return errors.New("--db is required")
	}
	if strings.TrimSpace(opts.coversDir) == "" {
		return errors.New("--covers-dir is required")
	}
	if strings.TrimSpace(opts.serverConfig) == "" {
		return errors.New("--server-config is required")
	}
	if strings.TrimSpace(opts.appDir) == "" {
		return errors.New("--app-dir is required")
	}
	if opts.port < 1 || opts.port > 65535 {
		return fmt.Errorf("--port must be between 1 and 65535")
	}
	absoluteDB, err := filepath.Abs(opts.databasePath)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	absoluteCovers, err := filepath.Abs(opts.coversDir)
	if err != nil {
		return fmt.Errorf("resolve covers directory: %w", err)
	}
	opts.databasePath = absoluteDB
	opts.coversDir = absoluteCovers
	absoluteConfig, err := filepath.Abs(opts.serverConfig)
	if err != nil {
		return fmt.Errorf("resolve server config path: %w", err)
	}
	absoluteAppDir, err := filepath.Abs(opts.appDir)
	if err != nil {
		return fmt.Errorf("resolve app directory: %w", err)
	}
	opts.serverConfig = absoluteConfig
	opts.appDir = absoluteAppDir
	opts.coverBaseURL = strings.TrimRight(strings.TrimSpace(opts.coverBaseURL), "/")

	if _, err := os.Stat(opts.databasePath); err == nil {
		return fmt.Errorf("refusing to overwrite existing database %s", opts.databasePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect database path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(opts.databasePath), 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	if err := ensureEmptyOrAbsentDirectory(opts.coversDir); err != nil {
		return fmt.Errorf("prepare covers directory: %w", err)
	}
	if _, err := os.Stat(opts.serverConfig); err == nil {
		return fmt.Errorf("refusing to overwrite existing server config %s", opts.serverConfig)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect server config path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(opts.serverConfig), 0o755); err != nil {
		return fmt.Errorf("create server config directory: %w", err)
	}

	logger := consoleLogger{}
	database := db.NewSQLiteDatabaseWithMigrationOptions(
		logger,
		staticConfiguration{databasePath: opts.databasePath},
		core.MigrationOptions{BackupBeforeMigrate: false},
	)
	if err := database.Connect(); err != nil {
		return err
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		return fmt.Errorf("migrate isolated database: %w", err)
	}

	s := &seeder{
		database: database,
		store:    db.NewGameStore(database, logger),
		profiles: db.NewProfileRepository(database),
		sources:  db.NewIntegrationRepository(database),
		options:  opts,
	}
	if err := s.seed(ctx); err != nil {
		return err
	}
	if err := writeServerConfig(opts); err != nil {
		return err
	}
	fmt.Printf("Created privacy-safe MGA public demo at %s\n", opts.databasePath)
	return nil
}

func ensureEmptyOrAbsentDirectory(path string) error {
	entries, err := os.ReadDir(path)
	switch {
	case err == nil:
		if len(entries) != 0 {
			return fmt.Errorf("refusing to write into non-empty directory %s", path)
		}
		return nil
	case errors.Is(err, os.ErrNotExist):
		return os.MkdirAll(path, 0o755)
	default:
		return err
	}
}

func writeServerConfig(opts options) error {
	runtimeDir := filepath.Dir(opts.serverConfig)
	values := map[string]string{
		"PORT":                fmt.Sprintf("%d", opts.port),
		"LISTEN_IP":           "127.0.0.1",
		"DB_PATH":             opts.databasePath,
		"PLUGINS_DIR":         filepath.Join(opts.appDir, "plugins"),
		"FRONTEND_DIST":       filepath.Join(opts.appDir, "frontend", "dist"),
		"MEDIA_ROOT":          filepath.Join(runtimeDir, "media"),
		"SOURCE_CACHE_ROOT":   filepath.Join(runtimeDir, "source_cache"),
		"UPDATES_DIR":         filepath.Join(runtimeDir, "updates"),
		"LOG_FILE":            filepath.Join(runtimeDir, "logs", "mga_server.log"),
		"LOG_MAX_SIZE_MB":     "10",
		"LOG_MAX_BACKUPS":     "2",
		"APP_INSTALL_TYPE":    "user",
		"UPDATE_MANIFEST_URL": "",
	}
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal isolated server config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(opts.serverConfig, data, 0o644); err != nil {
		return fmt.Errorf("write isolated server config: %w", err)
	}
	return nil
}

func (s *seeder) seed(ctx context.Context) error {
	now := time.Now().UTC()
	profile := &core.Profile{
		ID:          demoProfileID,
		DisplayName: "Demo Player",
		AvatarKey:   "player-1",
		Role:        core.ProfileRoleAdminPlayer,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.profiles.Create(ctx, profile); err != nil {
		return fmt.Errorf("create demo profile: %w", err)
	}
	profileCtx := core.WithProfile(ctx, profile)

	integrations := []*core.Integration{
		{ID: "demo-steam", PluginID: "game-source-steam", Label: "Steam", ConfigJSON: "{}", IntegrationType: "game_source", CreatedAt: now, UpdatedAt: now},
		{ID: "demo-xbox", PluginID: "game-source-xbox", Label: "Xbox & Game Pass", ConfigJSON: "{}", IntegrationType: "game_source", CreatedAt: now, UpdatedAt: now},
		{ID: "demo-retro", PluginID: "game-source-local", Label: "Retro Collection", ConfigJSON: "{}", IntegrationType: "game_source", CreatedAt: now, UpdatedAt: now},
	}
	for _, integration := range integrations {
		if err := s.sources.Create(profileCtx, integration); err != nil {
			return fmt.Errorf("create integration %s: %w", integration.Label, err)
		}
	}

	games := publicDemoGames()
	for _, game := range games {
		if err := s.writeArtwork(game); err != nil {
			return err
		}
	}
	for _, source := range []struct {
		integrationID string
		pluginID      string
		include       func(demoGame) bool
	}{
		{"demo-steam", "game-source-steam", func(game demoGame) bool { return game.steam }},
		{"demo-xbox", "game-source-xbox", func(game demoGame) bool { return game.xbox }},
		{"demo-retro", "game-source-local", func(game demoGame) bool { return game.retro }},
	} {
		batch := &core.ScanBatch{
			IntegrationID:   source.integrationID,
			ResolverMatches: map[string][]core.ResolverMatch{},
			MediaItems:      map[string][]core.MediaRef{},
		}
		for _, game := range games {
			if !source.include(game) {
				continue
			}
			sourceID := source.integrationID + ":" + game.slug
			sourceGame := &core.SourceGame{
				ID:            sourceID,
				IntegrationID: source.integrationID,
				PluginID:      source.pluginID,
				ExternalID:    game.slug,
				RawTitle:      game.title,
				Platform:      game.platform,
				Kind:          core.GameKindBaseGame,
				GroupKind:     core.GroupKindSelfContained,
				RootPath:      `C:\Games\` + game.title,
				Status:        "found",
				LastSeenAt:    &now,
				CreatedAt:     now,
			}
			if source.pluginID == "game-source-steam" {
				sourceGame.URL = "steam://rungameid/" + game.slug
			}
			match := core.ResolverMatch{
				PluginID:        "metadata-public-demo",
				ExternalID:      game.slug,
				Title:           game.title,
				Platform:        string(game.platform),
				Description:     game.description,
				ReleaseDate:     game.releaseDate,
				Genres:          game.genres,
				Developer:       game.developer,
				Publisher:       game.developer,
				Rating:          game.rating,
				MaxPlayers:      2,
				IsGamePass:      game.gamePass,
				XcloudAvailable: game.xcloud,
			}
			if game.xcloud && source.pluginID == "game-source-xbox" {
				match.XcloudURL = "https://www.xbox.com/play/games/" + game.slug
				match.StoreProductID = "DEMO-" + strings.ToUpper(game.slug)
			}
			coverURL := s.options.coverBaseURL + "/" + game.slug + "-cover.webp"
			backgroundURL := s.options.coverBaseURL + "/" + game.slug + "-background.webp"
			batch.SourceGames = append(batch.SourceGames, sourceGame)
			batch.ResolverMatches[sourceID] = []core.ResolverMatch{match}
			batch.MediaItems[sourceID] = []core.MediaRef{
				{
					Type:     core.MediaTypeCover,
					URL:      coverURL,
					Source:   "metadata-public-demo",
					Width:    600,
					Height:   900,
					MimeType: "image/webp",
				},
				{
					Type:     core.MediaTypeBackground,
					URL:      backgroundURL,
					Source:   "metadata-public-demo",
					Width:    1600,
					Height:   900,
					MimeType: "image/webp",
				},
			}
		}
		if err := s.store.PersistScanResults(profileCtx, batch); err != nil {
			return fmt.Errorf("persist %s demo batch: %w", source.integrationID, err)
		}
	}

	canonical, err := s.store.GetCanonicalGames(profileCtx)
	if err != nil {
		return fmt.Errorf("load seeded games: %w", err)
	}
	favoriteByTitle := map[string]bool{}
	for _, game := range games {
		favoriteByTitle[game.title] = game.favorite
	}
	for _, game := range canonical {
		if favoriteByTitle[game.Title] {
			if err := s.store.SetCanonicalFavorite(profileCtx, game.ID); err != nil {
				return fmt.Errorf("favorite %s: %w", game.Title, err)
			}
		}
	}
	return nil
}

func (s *seeder) writeArtwork(game demoGame) error {
	cover, err := demoArtwork.ReadFile("assets/" + game.slug + "-cover.webp")
	if err != nil {
		return fmt.Errorf("read embedded cover %s: %w", game.title, err)
	}
	coverPath := filepath.Join(s.options.coversDir, game.slug+"-cover.webp")
	if err := os.WriteFile(coverPath, cover, 0o644); err != nil {
		return fmt.Errorf("write cover %s: %w", game.title, err)
	}
	background, err := demoArtwork.ReadFile("assets/" + game.slug + "-background.webp")
	if err != nil {
		return fmt.Errorf("read embedded background %s: %w", game.title, err)
	}
	backgroundPath := filepath.Join(s.options.coversDir, game.slug+"-background.webp")
	if err := os.WriteFile(backgroundPath, background, 0o644); err != nil {
		return fmt.Errorf("write background %s: %w", game.title, err)
	}
	return nil
}

func publicDemoGames() []demoGame {
	return []demoGame{
		{"celestial-drift", "Celestial Drift", core.PlatformWindowsPC, "Race through luminous cities across several connected worlds.", []string{"Racing", "Arcade"}, "Northstar Studio", "2025-03-14", 8.8, true, true, false, true, true, true},
		{"moon-harbor", "Moon Harbor", core.PlatformWindowsPC, "Build a quiet settlement on the edge of a silver sea.", []string{"Simulation", "Strategy"}, "Quiet Giant", "2024-10-08", 8.4, true, true, false, true, false, false},
		{"pixel-quest", "Pixel Quest", core.PlatformGBA, "A compact adventure across forgotten handheld kingdoms.", []string{"Adventure", "RPG"}, "Paper Lantern", "2003-06-21", 8.1, false, false, true, false, false, true},
		{"iron-tactics", "Iron Tactics", core.PlatformWindowsPC, "Lead a small squad through short, replayable tactical missions.", []string{"Strategy", "Tactical"}, "Foundry Games", "2023-11-02", 8.6, true, false, false, false, false, false},
		{"deep-signal", "Deep Signal", core.PlatformWindowsPC, "Decode a signal from beneath an endless alien ocean.", []string{"Adventure", "Mystery"}, "Beacon Works", "2025-01-19", 9.0, true, true, false, true, true, false},
		{"arcadia-falls", "Arcadia Falls", core.PlatformSNES, "Restore a floating city in a colorful 16-bit action adventure.", []string{"Action", "Adventure"}, "Mosaic Studio", "1994-09-17", 8.7, false, false, true, false, false, false},
		{"cloudbound", "Cloudbound", core.PlatformWindowsPC, "Explore hand-painted islands alone or with a friend.", []string{"Adventure", "Co-op"}, "Soft Horizon", "2024-05-30", 8.9, true, true, false, true, true, true},
		{"neon-garden", "Neon Garden", core.PlatformGenesis, "Grow impossible plants in a rhythm-powered greenhouse.", []string{"Puzzle", "Music"}, "Afterglow", "1992-04-11", 7.9, false, false, true, false, false, false},
	}
}
