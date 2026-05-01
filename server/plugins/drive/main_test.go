package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

func ipcCall(stdin io.Writer, stdout io.Reader, method string, params any) (*Response, error) {
	req := Request{
		ID:     fmt.Sprintf("test-%d", time.Now().UnixNano()),
		Method: method,
	}
	if params != nil {
		b, _ := json.Marshal(params)
		req.Params = b
	}
	payload, _ := json.Marshal(req)

	if err := binary.Write(stdin, binary.BigEndian, uint32(len(payload))); err != nil {
		return nil, fmt.Errorf("write length: %w", err)
	}
	if _, err := stdin.Write(payload); err != nil {
		return nil, fmt.Errorf("write payload: %w", err)
	}

	var respLen uint32
	if err := binary.Read(stdout, binary.BigEndian, &respLen); err != nil {
		return nil, fmt.Errorf("read resp length: %w", err)
	}
	respData := make([]byte, respLen)
	if _, err := io.ReadFull(stdout, respData); err != nil {
		return nil, fmt.Errorf("read resp payload: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &resp, nil
}

func TestDriveSourcePlugin(t *testing.T) {
	if os.Getenv("DRIVE_SOURCE_INTEGRATION") != "1" {
		t.Skip("set DRIVE_SOURCE_INTEGRATION=1 to run")
	}

	exePath := "./drive.exe"
	if _, err := os.Stat(exePath); err != nil {
		t.Fatalf("build the plugin first: go build -o drive.exe .")
	}

	cmd := exec.Command(exePath)
	cmd.Dir, _ = os.Getwd()
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stdin.Close()
		cmd.Wait()
	}()

	// 1. plugin.init
	t.Log("calling plugin.init (may open browser for Google auth)...")
	resp, err := ipcCall(stdin, stdout, "plugin.init", nil)
	if err != nil {
		t.Fatalf("plugin.init: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("plugin.init error: %s: %s", resp.Error.Code, resp.Error.Message)
	}
	t.Logf("init result: %v", resp.Result)

	// 2. source.filesystem.list
	t.Log("calling source.filesystem.list...")
	resp, err = ipcCall(stdin, stdout, "source.filesystem.list", nil)
	if err != nil {
		t.Fatalf("source.filesystem.list: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("source.filesystem.list error: %s: %s", resp.Error.Code, resp.Error.Message)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result struct {
		Files []struct {
			Path  string `json:"path"`
			Name  string `json:"name"`
			IsDir bool   `json:"is_dir"`
			Size  int64  `json:"size"`
		} `json:"files"`
	}
	json.Unmarshal(resultBytes, &result)

	t.Logf("=== Google Drive Source Results ===")
	t.Logf("Total files/folders: %d", len(result.Files))

	dirCount := 0
	fileCount := 0
	var totalSize int64
	for _, f := range result.Files {
		if f.IsDir {
			dirCount++
		} else {
			fileCount++
			totalSize += f.Size
		}
	}

	t.Logf("  Directories: %d", dirCount)
	t.Logf("  Files:       %d", fileCount)
	t.Logf("  Total size:  %.2f GB", float64(totalSize)/(1024*1024*1024))

	t.Logf("\nSample entries (first 20):")
	count := 20
	if len(result.Files) < count {
		count = len(result.Files)
	}
	for i := 0; i < count; i++ {
		f := result.Files[i]
		kind := "FILE"
		if f.IsDir {
			kind = "DIR "
		}
		t.Logf("  [%s] %s (%d bytes)", kind, f.Path, f.Size)
	}

	if len(result.Files) == 0 {
		t.Error("expected at least some files from Google Drive")
	}
}

func TestSourceDeletePathWithinRoot(t *testing.T) {
	tests := []struct {
		name     string
		rootPath string
		filePath string
		want     bool
	}{
		{name: "child file", rootPath: `Games\Platforms\SNES`, filePath: "Games/Platforms/SNES/Game.sfc", want: true},
		{name: "same file root", rootPath: "Games/Platforms/SNES/Game.sfc", filePath: "Games/Platforms/SNES/Game.sfc", want: true},
		{name: "sibling prefix rejected", rootPath: "Games/Platforms/SNES", filePath: "Games/Platforms/SNES Extras/Game.sfc", want: false},
		{name: "outside root rejected", rootPath: "Games/Platforms/SNES", filePath: "Games/Platforms/N64/Game.z64", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceDeletePathWithinRoot(tt.rootPath, tt.filePath); got != tt.want {
				t.Fatalf("sourceDeletePathWithinRoot(%q, %q) = %t, want %t", tt.rootPath, tt.filePath, got, tt.want)
			}
		})
	}
}

func TestHandleSourceDeleteDryRunReturnsTrashPlan(t *testing.T) {
	result, errObj := handleSourceDelete(mustJSON(t, map[string]any{
		"dry_run":        true,
		"source_game_id": "scan:drive-game",
		"root_path":      "Games/Platforms/SNES",
		"config": map[string]any{
			"include_paths": []map[string]any{{"path": "Games", "recursive": true}},
		},
		"files": []map[string]any{{
			"path":      "Games/Platforms/SNES/Game.sfc",
			"object_id": "drive-file-1",
			"size":      1024,
		}},
	}))
	if errObj != nil {
		t.Fatalf("handleSourceDelete dry run error = %s: %s", errObj.Code, errObj.Message)
	}
	encoded, _ := json.Marshal(result)
	var resp struct {
		SourceGameID string `json:"source_game_id"`
		PluginID     string `json:"plugin_id"`
		Action       string `json:"action"`
		Items        []struct {
			Path     string `json:"path"`
			ObjectID string `json:"object_id"`
			Action   string `json:"action"`
		} `json:"items"`
		DeletedCount int `json:"deleted_count"`
	}
	if err := json.Unmarshal(encoded, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.SourceGameID != "scan:drive-game" || resp.PluginID != "game-source-google-drive" || resp.Action != "trash" {
		t.Fatalf("response = %+v, want drive trash plan metadata", resp)
	}
	if len(resp.Items) != 1 || resp.Items[0].Path != "Games/Platforms/SNES/Game.sfc" || resp.Items[0].ObjectID != "drive-file-1" || resp.Items[0].Action != "trash" {
		t.Fatalf("items = %+v, want exact trash item", resp.Items)
	}
	if resp.DeletedCount != 0 {
		t.Fatalf("deleted_count = %d, want 0 for dry run", resp.DeletedCount)
	}
}

func TestHandleSourceDeleteRejectsDirectoryEntry(t *testing.T) {
	_, errObj := handleSourceDelete(mustJSON(t, map[string]any{
		"dry_run":        true,
		"source_game_id": "scan:drive-game",
		"root_path":      "Games/Platforms/SNES",
		"config": map[string]any{
			"include_paths": []map[string]any{{"path": "Games", "recursive": true}},
		},
		"files": []map[string]any{{
			"path":      "Games/Platforms/SNES",
			"object_id": "drive-folder-1",
			"is_dir":    true,
		}},
	}))
	if errObj == nil {
		t.Fatal("expected directory delete to be rejected")
	}
	if errObj.Code != "INVALID_PARAMS" {
		t.Fatalf("error code = %q, want INVALID_PARAMS", errObj.Code)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
