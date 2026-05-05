package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const bytesPerMB = 1024 * 1024

type logService struct {
	logger *slog.Logger
	closer io.Closer
}

type Options struct {
	FilePath   string
	BaseDir    string
	MaxSizeMB  int
	MaxBackups int
}

func NewLogService() core.Logger {
	log, _ := NewLogServiceWithOptions(Options{})
	return log
}

func NewLogServiceWithOptions(options Options) (core.Logger, error) {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	output := io.Writer(os.Stdout)
	var closer io.Closer
	if options.FilePath != "" {
		writer, err := NewRotatingFileWriter(options)
		if err != nil {
			return nil, err
		}
		output = io.MultiWriter(os.Stdout, writer)
		closer = writer
	}
	handler := slog.NewTextHandler(output, opts)
	return &logService{
		logger: slog.New(handler),
		closer: closer,
	}, nil
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

func (l *logService) Close() error {
	if l == nil || l.closer == nil {
		return nil
	}
	return l.closer.Close()
}

type RotatingFileWriter struct {
	path       string
	maxBytes   int64
	maxBackups int
	file       *os.File
}

func NewRotatingFileWriter(options Options) (*RotatingFileWriter, error) {
	path := options.FilePath
	if !filepath.IsAbs(path) {
		path = filepath.Join(options.BaseDir, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	maxSizeMB := options.MaxSizeMB
	if maxSizeMB <= 0 {
		maxSizeMB = 50
	}
	maxBackups := options.MaxBackups
	if maxBackups <= 0 {
		maxBackups = 5
	}
	writer := &RotatingFileWriter{
		path:       path,
		maxBytes:   int64(maxSizeMB) * bytesPerMB,
		maxBackups: maxBackups,
	}
	if err := writer.open(); err != nil {
		return nil, err
	}
	return writer, nil
}

func (w *RotatingFileWriter) Write(p []byte) (int, error) {
	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if w.maxBytes > 0 {
		if info, err := w.file.Stat(); err == nil && info.Size()+int64(len(p)) > w.maxBytes {
			if err := w.rotate(); err != nil {
				return 0, err
			}
		}
	}
	return w.file.Write(p)
}

func (w *RotatingFileWriter) Close() error {
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *RotatingFileWriter) open() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", w.path, err)
	}
	w.file = file
	return nil
}

func (w *RotatingFileWriter) rotate() error {
	if err := w.Close(); err != nil {
		return err
	}
	for i := w.maxBackups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", w.path, i)
		dst := fmt.Sprintf("%s.%d", w.path, i+1)
		if _, err := os.Stat(src); err == nil {
			if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove old log backup %s: %w", dst, err)
			}
			if err := os.Rename(src, dst); err != nil {
				return fmt.Errorf("rotate log backup %s: %w", src, err)
			}
		}
	}
	if w.maxBackups > 0 {
		backup := fmt.Sprintf("%s.1", w.path)
		if _, err := os.Stat(w.path); err == nil {
			if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove old log backup %s: %w", backup, err)
			}
			if err := os.Rename(w.path, backup); err != nil {
				return fmt.Errorf("rotate log file %s: %w", w.path, err)
			}
		}
	} else {
		if err := os.Remove(w.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove log file %s: %w", w.path, err)
		}
	}
	return w.open()
}
