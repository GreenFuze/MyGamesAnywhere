package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRotatingFileWriterResolvesRelativePathAndKeepsBackups(t *testing.T) {
	dir := t.TempDir()
	writer, err := NewRotatingFileWriter(Options{
		FilePath:   "logs/mga_server.log",
		BaseDir:    dir,
		MaxSizeMB:  1,
		MaxBackups: 2,
	})
	if err != nil {
		t.Fatalf("NewRotatingFileWriter() error = %v", err)
	}
	defer func() { _ = writer.Close() }()

	chunk := bytes.Repeat([]byte("a"), 700*1024)
	for i := 0; i < 5; i++ {
		if _, err := writer.Write(chunk); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	logPath := filepath.Join(dir, "logs", "mga_server.log")
	for _, path := range []string{logPath, logPath + ".1", logPath + ".2"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected log file %s: %v", path, err)
		}
	}
	if _, err := os.Stat(logPath + ".3"); !os.IsNotExist(err) {
		t.Fatalf("expected no third backup, stat error = %v", err)
	}
}

func TestLogServiceWritesConfiguredFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "mga_server.log")
	logSvc, err := NewLogServiceWithOptions(Options{FilePath: logPath, MaxSizeMB: 1, MaxBackups: 1})
	if err != nil {
		t.Fatalf("NewLogServiceWithOptions() error = %v", err)
	}
	logSvc.Info("hello from test", "component", "logger")
	if closer, ok := logSvc.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !bytes.Contains(data, []byte("hello from test")) {
		t.Fatalf("log file did not contain message: %s", data)
	}
}
