package plugins

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestValidPluginID(t *testing.T) {
	valid := []string{
		"game-source-smb",
		"game-source-google-drive",
		"sync-settings-google-drive",
		"game-source-mock",
		"a",
		"a1",
		"ab-cd",
	}
	for _, id := range valid {
		if !validPluginID(id) {
			t.Errorf("expected valid: %q", id)
		}
	}

	invalid := []string{
		"",
		"com.mga.drive",
		"com.example.plugin",
		"Abc",
		"-leading",
		"UPPERCASE",
	}
	for _, id := range invalid {
		if validPluginID(id) {
			t.Errorf("expected invalid: %q", id)
		}
	}
}

func TestPluginHostStartsDifferentPluginsConcurrently(t *testing.T) {
	manager := &recordingProcessManager{delay: 80 * time.Millisecond}
	host := newPluginHostForTest(manager)
	host.plugins["game-source-one"] = testPlugin("game-source-one")
	host.plugins["game-source-two"] = testPlugin("game-source-two")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	startedAt := time.Now()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, pluginID := range []string{"game-source-one", "game-source-two"} {
		wg.Add(1)
		go func(pluginID string) {
			defer wg.Done()
			_, err := host.getClient(ctx, pluginID)
			errs <- err
		}(pluginID)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("getClient failed: %v", err)
		}
	}
	if manager.maxActive < 2 {
		t.Fatalf("expected different plugins to spawn concurrently, max active spawns = %d", manager.maxActive)
	}
	if elapsed := time.Since(startedAt); elapsed >= 140*time.Millisecond {
		t.Fatalf("expected concurrent startup, elapsed = %s", elapsed)
	}
}

func TestPluginHostStartsSamePluginOnce(t *testing.T) {
	manager := &recordingProcessManager{delay: 50 * time.Millisecond}
	host := newPluginHostForTest(manager)
	host.plugins["game-source-one"] = testPlugin("game-source-one")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := host.getClient(ctx, "game-source-one")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("getClient failed: %v", err)
		}
	}
	if manager.spawnCount != 1 {
		t.Fatalf("expected one spawn for concurrent same-plugin startup, got %d", manager.spawnCount)
	}
}

func newPluginHostForTest(manager ProcessManager) *pluginHost {
	return &pluginHost{
		logger:         testLogger{},
		processManager: manager,
		plugins:        make(map[string]*core.Plugin),
		clients:        make(map[string]IpcClient),
		starting:       make(map[string]*pluginClientStart),
		clientFactory: func(Process, core.Logger, string, DisconnectFunc) IpcClient {
			return fakeIpcClient{}
		},
	}
}

func testPlugin(id string) *core.Plugin {
	return &core.Plugin{
		Manifest: core.PluginManifest{
			ID:             id,
			Exec:           "test-plugin.exe",
			DefaultTimeout: 1000,
		},
		Path:    ".",
		Enabled: true,
	}
}

type recordingProcessManager struct {
	delay      time.Duration
	mu         sync.Mutex
	active     int
	maxActive  int
	spawnCount int
}

func (m *recordingProcessManager) Spawn(context.Context, string, []string, string) (Process, error) {
	m.mu.Lock()
	m.active++
	m.spawnCount++
	if m.active > m.maxActive {
		m.maxActive = m.active
	}
	m.mu.Unlock()

	time.Sleep(m.delay)

	m.mu.Lock()
	m.active--
	m.mu.Unlock()

	return fakeProcess{}, nil
}

type fakeIpcClient struct{}

func (fakeIpcClient) Call(context.Context, string, any, any) error {
	return nil
}

func (fakeIpcClient) Close() error {
	return nil
}

type fakeProcess struct{}

func (fakeProcess) Stdin() io.WriteCloser {
	return discardWriteCloser{}
}

func (fakeProcess) Stdout() io.ReadCloser {
	return io.NopCloser(&emptyReader{})
}

func (fakeProcess) Stderr() io.ReadCloser {
	return io.NopCloser(&emptyReader{})
}

func (fakeProcess) Wait() error {
	return nil
}

func (fakeProcess) Kill() error {
	return nil
}

type discardWriteCloser struct{}

func (discardWriteCloser) Write(p []byte) (int, error) {
	return len(p), nil
}

func (discardWriteCloser) Close() error {
	return nil
}

type emptyReader struct{}

func (*emptyReader) Read([]byte) (int, error) {
	return 0, io.EOF
}

type testLogger struct{}

func (testLogger) Info(string, ...any)         {}
func (testLogger) Error(string, error, ...any) {}
func (testLogger) Debug(string, ...any)        {}
func (testLogger) Warn(string, ...any)         {}
