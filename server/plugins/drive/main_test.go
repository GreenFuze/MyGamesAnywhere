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
