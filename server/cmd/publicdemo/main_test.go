package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
)

func TestRunCreatesPrivacySafeLibraryAndArtwork(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	opts := options{
		databasePath: filepath.Join(root, "data", "mga.db"),
		coversDir:    filepath.Join(root, "artwork"),
		coverBaseURL: "http://127.0.0.1:8766",
		serverConfig: filepath.Join(root, "server.json"),
		appDir:       appDir,
		port:         8911,
	}
	if err := run(context.Background(), opts); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	files, err := os.ReadDir(opts.coversDir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(files), len(publicDemoGames())*2; got != want {
		t.Fatalf("artwork file count = %d, want %d", got, want)
	}
	for _, game := range publicDemoGames() {
		for _, suffix := range []string{"-cover.webp", "-background.svg"} {
			data, err := os.ReadFile(filepath.Join(opts.coversDir, game.slug+suffix))
			if err != nil {
				t.Fatalf("read %s%s: %v", game.slug, suffix, err)
			}
			if len(data) < 500 {
				t.Fatalf("%s%s is unexpectedly small", game.slug, suffix)
			}
			if suffix == "-cover.webp" && string(data[:4]) != "RIFF" {
				t.Fatalf("%s%s is not WebP artwork", game.slug, suffix)
			}
		}
	}

	database := db.NewSQLiteDatabaseWithMigrationOptions(
		consoleLogger{},
		staticConfiguration{databasePath: opts.databasePath},
		core.MigrationOptions{BackupBeforeMigrate: false},
	)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var profiles, integrations, covers, backgrounds int
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM profiles WHERE id='public-demo-player' AND display_name='Demo Player'`).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM integrations WHERE profile_id=? AND config_json='{}'`, demoProfileID).Scan(&integrations); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM media_assets WHERE url LIKE '%-cover.webp'`).Scan(&covers); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM media_assets WHERE url LIKE '%-background.svg'`).Scan(&backgrounds); err != nil {
		t.Fatal(err)
	}
	if profiles != 1 || integrations != 3 {
		t.Fatalf("privacy-safe identities: profiles=%d integrations=%d", profiles, integrations)
	}
	if want := len(publicDemoGames()); covers != want || backgrounds != want {
		t.Fatalf("media rows: covers=%d backgrounds=%d, want %d each", covers, backgrounds, want)
	}
}
