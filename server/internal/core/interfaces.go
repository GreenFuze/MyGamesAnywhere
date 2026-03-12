package core

import (
	"context"
	"database/sql"
)

// Logger defines the interface for structured logging.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, err error, args ...any)
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
}

// Configuration defines the interface for application settings.
type Configuration interface {
	Get(key string) string
	GetInt(key string) int
	GetBool(key string) bool
	Validate() error
}

// Database defines the interface for database operations.
type Database interface {
	Connect() error
	Close() error
	Migrate() error
	GetDB() *sql.DB
}

type SettingRepository interface {
	Upsert(ctx context.Context, setting *Setting) error
	Get(ctx context.Context, key string) (*Setting, error)
}

type IntegrationRepository interface {
	Create(ctx context.Context, integration *Integration) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*Integration, error)
	GetByID(ctx context.Context, id string) (*Integration, error)
}

// GameRepository persists games and their files.
type GameRepository interface {
	UpsertGames(ctx context.Context, games []*Game, files []*GameFile) error
	MarkGamesNotFoundExcept(ctx context.Context, keepGameIDs []string) error
	MarkScanGamesNotFoundExcept(ctx context.Context, keepScanGameIDs []string) error
	MarkPluginGamesNotFoundExcept(ctx context.Context, integrationID string, keepGameIDs []string) error
	DeleteAllGames(ctx context.Context) error
	GetGames(ctx context.Context) ([]*Game, error)
	GetGameByID(ctx context.Context, gameID string) (*Game, error)
	GetGameFiles(ctx context.Context, gameID string) ([]*GameFile, error)
}

// GameSourceProvider lists games from a source (e.g. SMB share, Drive folder).
type GameSourceProvider interface {
	ListGames(ctx context.Context, config map[string]any) ([]GameEntry, error)
}

// SettingsSyncProvider backs up and restores server state to/from remote storage.
type SettingsSyncProvider interface {
	Backup(ctx context.Context, config map[string]any) error
	Restore(ctx context.Context, config map[string]any) error
}

// Server defines the interface for the HTTP server.
type Server interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
