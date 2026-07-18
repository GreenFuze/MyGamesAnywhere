package launchlog

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const FileName = "mga-client-launch.log"

// Logger appends diagnostic lines beside the running executable. This is where
// mga:// protocol launches write when the windowless agent has no console.
type Logger struct {
	file *os.File
}

// OpenBesideExecutable creates or appends the launch log next to the current
// executable. Failure is non-fatal: callers should continue without it.
func OpenBesideExecutable() (*Logger, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	path := filepath.Join(filepath.Dir(executable), FileName)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open launch log %s: %w", path, err)
	}
	return &Logger{file: file}, nil
}

// Path returns the log file path, or empty when the logger is unavailable.
func (l *Logger) Path() string {
	if l == nil || l.file == nil {
		return ""
	}
	return l.file.Name()
}

// Writer exposes the underlying file for teeing other loggers.
func (l *Logger) Writer() io.Writer {
	if l == nil || l.file == nil {
		return io.Discard
	}
	return l.file
}

// Printf writes one timestamped line. Safe on a nil Logger.
func (l *Logger) Printf(format string, values ...any) {
	if l == nil || l.file == nil {
		return
	}
	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, _ = fmt.Fprintf(l.file, "%s "+format+"\n", append([]any{timestamp}, values...)...)
	_ = l.file.Sync()
}

// Close flushes and closes the log file. Safe on a nil Logger.
func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// FormatArgs returns process arguments with short-lived secrets redacted.
func FormatArgs(args []string) string {
	redacted := make([]string, len(args))
	for i, arg := range args {
		redacted[i] = redactArg(arg)
	}
	return strings.Join(redacted, " ")
}

func redactArg(arg string) string {
	if !strings.Contains(arg, "://") {
		return arg
	}
	parsed, err := url.Parse(arg)
	if err != nil || parsed.Scheme == "" {
		return arg
	}
	query := parsed.Query()
	changed := false
	for _, key := range []string{"code", "token"} {
		if query.Has(key) {
			query.Set(key, "REDACTED")
			changed = true
		}
	}
	if !changed {
		return arg
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
