package logger

import (
	"bytes"
	"errors"
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

// A server running as a Windows service has no console, and writes to its
// stdout handle fail. With io.MultiWriter that failure came first and the log
// file never saw the line, so the server ran for months writing nothing while
// its log viewer showed an empty page. The file is the record; the console is
// not allowed to take it down.

type failingWriter struct{ writes int }

func (w *failingWriter) Write(p []byte) (int, error) {
	w.writes++
	return 0, errors.New("The handle is invalid.")
}

func TestLogFileIsWrittenWhenTheConsoleCannotBe(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "mga_server.log")
	console := &failingWriter{}

	log, err := NewLogServiceWithOptions(Options{FilePath: logPath, Console: console})
	if err != nil {
		t.Fatalf("NewLogServiceWithOptions() error = %v", err)
	}
	log.Info("periodic artwork reclaim", "removed_unreferenced", 2)
	if closer, ok := log.(interface{ Close() error }); ok {
		_ = closer.Close()
	}

	written, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !bytes.Contains(written, []byte("periodic artwork reclaim")) {
		t.Fatalf("a console that cannot be written to silenced the log file; file held %q", written)
	}
	if console.writes == 0 {
		t.Fatal("the console was never attempted, so this test would pass without proving anything")
	}
}

func TestConsoleStillReceivesEveryLine(t *testing.T) {
	// The fix must not quietly stop writing to the console to protect the file.
	dir := t.TempDir()
	console := &bytes.Buffer{}

	log, err := NewLogServiceWithOptions(Options{
		FilePath: filepath.Join(dir, "mga_server.log"),
		Console:  console,
	})
	if err != nil {
		t.Fatalf("NewLogServiceWithOptions() error = %v", err)
	}
	log.Info("scan finished")
	if closer, ok := log.(interface{ Close() error }); ok {
		_ = closer.Close()
	}

	if !bytes.Contains(console.Bytes(), []byte("scan finished")) {
		t.Fatalf("the console received %q", console.Bytes())
	}
}
