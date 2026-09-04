package plugins

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

// scriptedProcess replays frames a plugin would write and records what the
// host sends, so the framing contract can be exercised without a real plugin.
type scriptedProcess struct {
	stdin  *pipeWriter
	stdout *io.PipeReader
	stderr *io.PipeReader
}

type pipeWriter struct {
	mu      sync.Mutex
	written []byte
}

func (w *pipeWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.written = append(w.written, data...)
	return len(data), nil
}

func (w *pipeWriter) Close() error { return nil }

func (p *scriptedProcess) Stdin() io.WriteCloser { return p.stdin }
func (p *scriptedProcess) Stdout() io.ReadCloser { return p.stdout }
func (p *scriptedProcess) Stderr() io.ReadCloser { return p.stderr }
func (p *scriptedProcess) Wait() error           { return nil }
func (p *scriptedProcess) Kill() error           { return nil }

func writeTestFrame(t *testing.T, writer io.Writer, payload any) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode frame: %v", err)
	}
	if err := binary.Write(writer, binary.BigEndian, uint32(len(encoded))); err != nil {
		t.Fatalf("write length: %v", err)
	}
	if _, err := writer.Write(encoded); err != nil {
		t.Fatalf("write body: %v", err)
	}
}

// readRequestID pulls the id the host generated out of what it wrote to stdin.
func readRequestID(t *testing.T, stdin *pipeWriter) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stdin.mu.Lock()
		data := append([]byte(nil), stdin.written...)
		stdin.mu.Unlock()
		if len(data) > 4 {
			var request struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(data[4:], &request); err == nil && request.ID != "" {
				return request.ID
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("plugin never received a request")
	return ""
}

func TestProgressFramesDoNotCompleteTheCall(t *testing.T) {
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, _ := io.Pipe()
	stdin := &pipeWriter{}
	process := &scriptedProcess{stdin: stdin, stdout: stdoutReader, stderr: stderrReader}

	client := NewIpcClient(process, testLogger{}, "test-plugin", nil)
	defer client.Close()

	var mu sync.Mutex
	var seen []Progress
	ctx := WithProgress(context.Background(), func(update Progress) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, update)
	})

	done := make(chan error, 1)
	var result map[string]any
	go func() { done <- client.Call(ctx, "source.filesystem.list", map[string]any{}, &result) }()

	id := readRequestID(t, stdin)

	// Two interim reports, then the real answer.
	writeTestFrame(t, stdoutWriter, map[string]any{
		"id":       id,
		"progress": map[string]any{"current": 250, "unit": "items", "item": "Reading Games…"},
	})
	writeTestFrame(t, stdoutWriter, map[string]any{
		"id":       id,
		"progress": map[string]any{"current": 500, "total": 1000, "unit": "items"},
	})

	// If a progress frame had completed the call, this result would never be
	// delivered and the test would hang.
	select {
	case err := <-done:
		t.Fatalf("call completed on a progress frame: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	writeTestFrame(t, stdoutWriter, map[string]any{
		"id":     id,
		"result": map[string]any{"files": []any{}},
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call never completed after its result arrived")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("expected 2 progress reports, got %d: %+v", len(seen), seen)
	}
	if seen[0].Current != 250 || seen[0].Item != "Reading Games…" || seen[0].Total != 0 {
		t.Fatalf("unexpected first report: %+v", seen[0])
	}
	if seen[1].Current != 500 || seen[1].Total != 1000 {
		t.Fatalf("unexpected second report: %+v", seen[1])
	}
}

func TestProgressFramesAreIgnoredWhenNobodyIsListening(t *testing.T) {
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, _ := io.Pipe()
	stdin := &pipeWriter{}
	process := &scriptedProcess{stdin: stdin, stdout: stdoutReader, stderr: stderrReader}

	client := NewIpcClient(process, testLogger{}, "test-plugin", nil)
	defer client.Close()

	done := make(chan error, 1)
	var result map[string]any
	go func() { done <- client.Call(context.Background(), "source.games.list", map[string]any{}, &result) }()

	id := readRequestID(t, stdin)

	// A plugin that reports progress to a caller that did not ask for it must
	// not break the call. This is what keeps a new plugin working on an old host.
	writeTestFrame(t, stdoutWriter, map[string]any{
		"id":       id,
		"progress": map[string]any{"current": 1, "item": "Signing in…"},
	})
	writeTestFrame(t, stdoutWriter, map[string]any{
		"id":     id,
		"result": map[string]any{"games": []any{}},
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("an unheard progress frame broke the call")
	}
}

func TestProgressForAnUnknownRequestIsDropped(t *testing.T) {
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, _ := io.Pipe()
	stdin := &pipeWriter{}
	process := &scriptedProcess{stdin: stdin, stdout: stdoutReader, stderr: stderrReader}

	client := NewIpcClient(process, testLogger{}, "test-plugin", nil)
	defer client.Close()

	// No call is in flight, so this correlates to nothing. It must be ignored
	// rather than panicking the read loop.
	writeTestFrame(t, stdoutWriter, map[string]any{
		"id":       "never-issued",
		"progress": map[string]any{"current": 5},
	})

	var result map[string]any
	done := make(chan error, 1)
	go func() { done <- client.Call(context.Background(), "plugin.init", map[string]any{}, &result) }()

	id := readRequestID(t, stdin)
	writeTestFrame(t, stdoutWriter, map[string]any{"id": id, "result": map[string]any{"status": "ok"}})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("call failed after a stray progress frame: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the read loop did not survive a stray progress frame")
	}
}
