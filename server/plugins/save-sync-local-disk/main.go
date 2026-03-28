package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type Response struct {
	ID     string `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  *Error `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type saveSyncListParams struct {
	Prefix string `json:"prefix"`
}

type saveSyncPathParams struct {
	Path string `json:"path"`
}

type saveSyncPutParams struct {
	Path       string `json:"path"`
	DataBase64 string `json:"data_base64"`
}

func serverRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Dir(filepath.Dir(wd)), nil
}

func saveRoot() (string, error) {
	root, err := serverRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "save_syncs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func normalizePath(p string) (string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(strings.ReplaceAll(p, "/", string(filepath.Separator))))
	if cleaned == "." || cleaned == string(filepath.Separator) || cleaned == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", fmt.Errorf("invalid path")
	}
	return cleaned, nil
}

func objectPath(objectPath string) (string, error) {
	root, err := saveRoot()
	if err != nil {
		return "", err
	}
	cleaned, err := normalizePath(objectPath)
	if err != nil {
		return "", err
	}
	abs := filepath.Join(root, cleaned)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("invalid path")
	}
	return abs, nil
}

func handleInit() (any, *Error) {
	if _, err := saveRoot(); err != nil {
		return nil, &Error{Code: "INIT_FAILED", Message: err.Error()}
	}
	return map[string]any{"status": "ok"}, nil
}

func handleCheckConfig() (any, *Error) {
	return map[string]any{"status": "ok"}, nil
}

func handleSaveSyncList(params json.RawMessage) (any, *Error) {
	var p saveSyncListParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	root, err := saveRoot()
	if err != nil {
		return nil, &Error{Code: "ROOT_FAILED", Message: err.Error()}
	}
	prefix := ""
	if strings.TrimSpace(p.Prefix) != "" {
		prefix, err = normalizePath(p.Prefix)
		if err != nil {
			return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
		}
	}
	start := filepath.Join(root, prefix)
	if _, err := os.Stat(start); err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"status": "ok", "files": []map[string]any{}}, nil
		}
		return nil, &Error{Code: "STAT_FAILED", Message: err.Error()}
	}

	files := []map[string]any{}
	err = filepath.WalkDir(start, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		files = append(files, map[string]any{
			"path":     filepath.ToSlash(rel),
			"size":     info.Size(),
			"mod_time": info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
		return nil
	})
	if err != nil {
		return nil, &Error{Code: "LIST_FAILED", Message: err.Error()}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i]["path"].(string) < files[j]["path"].(string)
	})
	return map[string]any{"status": "ok", "files": files}, nil
}

func handleSaveSyncGet(params json.RawMessage) (any, *Error) {
	var p saveSyncPathParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	target, err := objectPath(p.Path)
	if err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"status": "not_found"}, nil
		}
		return nil, &Error{Code: "READ_FAILED", Message: err.Error()}
	}
	return map[string]any{
		"status":      "ok",
		"data_base64": base64.StdEncoding.EncodeToString(data),
	}, nil
}

func handleSaveSyncPut(params json.RawMessage) (any, *Error) {
	var p saveSyncPutParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	if p.DataBase64 == "" {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "data_base64 is required"}
	}
	target, err := objectPath(p.Path)
	if err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	data, err := base64.StdEncoding.DecodeString(p.DataBase64)
	if err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, &Error{Code: "MKDIR_FAILED", Message: err.Error()}
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return nil, &Error{Code: "WRITE_FAILED", Message: err.Error()}
	}
	return map[string]any{"status": "ok"}, nil
}

func handleSaveSyncDelete(params json.RawMessage) (any, *Error) {
	var p saveSyncPathParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	target, err := objectPath(p.Path)
	if err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"status": "not_found"}, nil
		}
		return nil, &Error{Code: "DELETE_FAILED", Message: err.Error()}
	}
	return map[string]any{"status": "ok"}, nil
}

func main() {
	log.SetOutput(os.Stderr)
	log.Println("Local disk save sync plugin started")

	for {
		var length uint32
		if err := binary.Read(os.Stdin, binary.BigEndian, &length); err != nil {
			if err == io.EOF {
				return
			}
			log.Fatalf("read length: %v", err)
		}

		payload := make([]byte, length)
		if _, err := io.ReadFull(os.Stdin, payload); err != nil {
			log.Fatalf("read payload: %v", err)
		}

		var req Request
		if err := json.Unmarshal(payload, &req); err != nil {
			log.Printf("unmarshal request: %v", err)
			continue
		}

		resp := Response{ID: req.ID}
		switch req.Method {
		case "plugin.init":
			resp.Result, resp.Error = handleInit()
		case "plugin.info":
			resp.Result = map[string]any{
				"plugin_id":      "save-sync-local-disk",
				"plugin_version": "1.0.0",
				"capabilities":   []string{"save_sync"},
			}
		case "plugin.check_config":
			resp.Result, resp.Error = handleCheckConfig()
		case "save_sync.list":
			resp.Result, resp.Error = handleSaveSyncList(req.Params)
		case "save_sync.get":
			resp.Result, resp.Error = handleSaveSyncGet(req.Params)
		case "save_sync.put":
			resp.Result, resp.Error = handleSaveSyncPut(req.Params)
		case "save_sync.delete":
			resp.Result, resp.Error = handleSaveSyncDelete(req.Params)
		default:
			resp.Error = &Error{Code: "UNKNOWN_METHOD", Message: "unknown method: " + req.Method}
		}

		out, _ := json.Marshal(resp)
		_ = binary.Write(os.Stdout, binary.BigEndian, uint32(len(out)))
		_, _ = os.Stdout.Write(out)
	}
}
