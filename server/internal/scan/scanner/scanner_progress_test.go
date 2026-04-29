package scanner

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type progressTestLogger struct{}

func (progressTestLogger) Info(string, ...any)         {}
func (progressTestLogger) Error(string, error, ...any) {}
func (progressTestLogger) Debug(string, ...any)        {}
func (progressTestLogger) Warn(string, ...any)         {}

func TestScanFilesProgressIsMonotonic(t *testing.T) {
	files := make([]core.FileEntry, 0, 60)
	for i := 0; i < 60; i++ {
		files = append(files, core.FileEntry{
			Path: fmt.Sprintf("Game/Disc%d.iso", i),
			Name: fmt.Sprintf("Disc%d.iso", i),
		})
	}

	s := New(progressTestLogger{})
	var updates []ProgressUpdate
	s.SetProgressReporter(func(_ context.Context, update ProgressUpdate) {
		updates = append(updates, update)
	})

	if _, err := s.ScanFiles(context.Background(), files); err != nil {
		t.Fatalf("ScanFiles returned error: %v", err)
	}
	if len(updates) == 0 {
		t.Fatal("expected progress updates")
	}

	last := 0
	for _, update := range updates {
		if update.FileCount != len(files) {
			t.Fatalf("file count = %d, want %d", update.FileCount, len(files))
		}
		if update.ProcessedCount <= last {
			t.Fatalf("progress not monotonic: previous=%d current=%d", last, update.ProcessedCount)
		}
		last = update.ProcessedCount
	}
	if last != len(files) {
		t.Fatalf("last processed = %d, want %d", last, len(files))
	}
}

func TestScanFilesHonorsContextCancellation(t *testing.T) {
	files := make([]core.FileEntry, 0, 100)
	for i := 0; i < 100; i++ {
		files = append(files, core.FileEntry{
			Path: fmt.Sprintf("Game/File%d.zip", i),
			Name: fmt.Sprintf("File%d.zip", i),
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := New(progressTestLogger{})
	s.SetProgressReporter(func(_ context.Context, update ProgressUpdate) {
		if update.ProcessedCount == 1 {
			cancel()
		}
	})

	_, err := s.ScanFiles(ctx, files)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ScanFiles error = %v, want context.Canceled", err)
	}
}

func TestScanFilesKeepsGenesisMdRom(t *testing.T) {
	files := []core.FileEntry{{
		Path: "Platforms/Sega Genesis/Altered Beast (USA, Europe).md",
		Name: "Altered Beast (USA, Europe).md",
		Size: 512 * 1024,
	}}

	groups, err := New(progressTestLogger{}).ScanFiles(context.Background(), files)
	if err != nil {
		t.Fatalf("ScanFiles returned error: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1: %+v", len(groups), groups)
	}
	group := groups[0]
	if group.Platform != core.PlatformGenesis {
		t.Fatalf("platform = %q, want %q", group.Platform, core.PlatformGenesis)
	}
	if group.GroupKind != core.GroupKindSelfContained {
		t.Fatalf("group kind = %q, want %q", group.GroupKind, core.GroupKindSelfContained)
	}
	if len(group.Files) != 1 || group.Files[0].Role != core.GameFileRoleRoot {
		t.Fatalf("files = %+v, want one root file", group.Files)
	}
}
