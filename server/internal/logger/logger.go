package logger

import (
	"log/slog"
	"os"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type logService struct {
	logger *slog.Logger
}

func NewLogService() core.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewTextHandler(os.Stdout, opts)
	return &logService{
		logger: slog.New(handler),
	}
}

func (l *logService) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

func (l *logService) Error(msg string, err error, args ...any) {
	if err != nil {
		args = append(args, slog.Any("error", err))
	}
	l.logger.Error(msg, args...)
}

func (l *logService) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

func (l *logService) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}
