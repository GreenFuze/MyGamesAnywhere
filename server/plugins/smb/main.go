package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
	"github.com/hirochachacha/go-smb2"
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

type SMBConfig struct {
	Host         string                    `json:"host"`
	Share        string                    `json:"share"`
	Username     string                    `json:"username"`
	Password     string                    `json:"password"`
	Path         string                    `json:"path"`
	IncludePaths []sourcescope.IncludePath `json:"include_paths"`
}

func decodeSMBConfig(payload json.RawMessage) (SMBConfig, error) {
	var configMap map[string]any
	if err := json.Unmarshal(payload, &configMap); err != nil {
		return SMBConfig{}, err
	}
	if nestedConfig, ok := configMap["config"].(map[string]any); ok {
		configMap = nestedConfig
	}
	if err := sourcescope.ValidateConfig("game-source-smb", configMap); err != nil {
		return SMBConfig{}, err
	}
	normalized := sourcescope.NormalizeConfig("game-source-smb", configMap)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return SMBConfig{}, err
	}
	var config SMBConfig
	if err := json.Unmarshal(encoded, &config); err != nil {
		return SMBConfig{}, err
	}
	return config, nil
}

func main() {
	log.SetOutput(os.Stderr)
	log.Println("SMB source plugin started")

	var writeMu sync.Mutex
	for {
		var length uint32
		err := binary.Read(os.Stdin, binary.BigEndian, &length)
		if err != nil {
			if err == io.EOF {
				return
			}
			log.Fatalf("failed to read length: %v", err)
		}

		payload := make([]byte, length)
		_, err = io.ReadFull(os.Stdin, payload)
		if err != nil {
			log.Fatalf("failed to read payload: %v", err)
		}

		var req Request
		if err := json.Unmarshal(payload, &req); err != nil {
			log.Printf("failed to unmarshal request: %v", err)
			continue
		}

		var resp Response
		resp.ID = req.ID

		if req.Method == "source.file.materialize" {
			go func(req Request) {
				resp := Response{ID: req.ID}
				result, errObj := handleFileMaterialize(req.Params)
				if errObj != nil {
					resp.Error = errObj
				} else {
					resp.Result = result
				}
				if err := writeResponse(&writeMu, resp); err != nil {
					log.Printf("write materialize response: %v", err)
				}
			}(req)
			continue
		}

		switch req.Method {
		case "plugin.init":
			resp.Result = map[string]any{"status": "ok"}
		case "plugin.info":
			resp.Result = map[string]any{
				"plugin_id":      "game-source-smb",
				"plugin_version": "1.0.0",
				"capabilities":   []string{"source"},
				"provides":       []string{"source.filesystem.list", "source.filesystem.delete", "source.file.materialize", "plugin.check_config"},
				"config": map[string]any{
					"host":     map[string]any{"type": "string", "required": true},
					"share":    map[string]any{"type": "string", "required": true},
					"username": map[string]any{"type": "string", "required": true},
					"password": map[string]any{"type": "string", "required": true, "x-secret": true},
					"include_paths": map[string]any{
						"type":     "array",
						"required": true,
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"path":          map[string]any{"type": "string", "required": true},
								"recursive":     map[string]any{"type": "boolean"},
								"exclude_paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Folders inside this include path to skip recursively."},
							},
						},
					},
				},
			}
		case "source.filesystem.list":
			config, err := decodeSMBConfig(req.Params)
			if err != nil {
				resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
				break
			}
			files, err := listFiles(config)
			if err != nil {
				resp.Error = &Error{Code: "SCAN_FAILED", Message: err.Error()}
			} else {
				resp.Result = map[string]any{"files": files}
			}
		case "source.filesystem.delete":
			result, errObj := handleSourceDelete(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}
		case "plugin.check_config":
			var params map[string]json.RawMessage
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
				break
			}
			configPayload := req.Params
			if rawConfig, ok := params["config"]; ok {
				configPayload = rawConfig
			}
			config, err := decodeSMBConfig(configPayload)
			if err != nil {
				resp.Result = map[string]any{"status": "error", "message": err.Error()}
				break
			}
			if err := checkConfig(config); err != nil {
				resp.Result = map[string]any{"status": "error", "message": err.Error()}
			} else {
				resp.Result = map[string]any{
					"status":          "ok",
					"source_identity": sourceIdentity(config),
				}
			}
		default:
			resp.Error = &Error{Code: "NOT_SUPPORTED", Message: "Method not supported"}
		}

		if err := writeResponse(&writeMu, resp); err != nil {
			fmt.Fprintf(os.Stderr, "write response: %v\n", err)
			os.Exit(1)
		}
	}
}

func writeResponse(mu *sync.Mutex, resp Response) error {
	respPayload, err := json.Marshal(resp)
	if err != nil {
		resp = Response{ID: resp.ID, Error: &Error{Code: "INTERNAL", Message: "failed to encode response"}}
		respPayload, err = json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("marshal response: %w", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if err := binary.Write(os.Stdout, binary.BigEndian, uint32(len(respPayload))); err != nil {
		return err
	}
	_, err = os.Stdout.Write(respPayload)
	return err
}

func mountShare(config SMBConfig) (net.Conn, *smb2.Session, *smb2.Share, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:445", config.Host))
	if err != nil {
		return nil, nil, nil, err
	}

	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     config.Username,
			Password: config.Password,
		},
	}

	session, err := d.Dial(conn)
	if err != nil {
		conn.Close()
		return nil, nil, nil, err
	}

	share, err := session.Mount(config.Share)
	if err != nil {
		session.Logoff()
		conn.Close()
		return nil, nil, nil, fmt.Errorf("mount share %q: %w", config.Share, err)
	}

	return conn, session, share, nil
}

func checkConfig(config SMBConfig) error {
	conn, session, share, err := mountShare(config)
	if err != nil {
		return fmt.Errorf("failed to connect to host: %w", err)
	}
	defer conn.Close()
	defer session.Logoff()
	defer share.Umount()

	return nil
}

// listFiles walks the entire SMB share and returns every file and directory
// as a flat listing. No filtering — the scanner handles classification.
func listFiles(config SMBConfig) ([]map[string]any, error) {
	conn, session, remotefs, err := mountShare(config)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	defer session.Logoff()
	defer remotefs.Umount()

	seen := make(map[string]map[string]any)
	for _, include := range normalizedIncludePaths(config) {
		searchPath := include.Path
		if searchPath == "" {
			searchPath = "."
		}

		entries, err := remotefs.ReadDir(searchPath)
		if err != nil {
			return nil, fmt.Errorf("readdir %q: %w", searchPath, err)
		}
		log.Printf("SMB readdir %q: %d top-level entries", searchPath, len(entries))

		if include.Recursive {
			rootFS := remotefs.DirFS(searchPath)
			err = fs.WalkDir(rootFS, ".", func(walkPath string, d fs.DirEntry, err error) error {
				if err != nil {
					log.Printf("walk error at %q: %v", walkPath, err)
					return nil
				}
				if walkPath == "." {
					return nil
				}
				logicalPath := joinLogicalPath(include.Path, walkPath)
				if smbPathExcluded(logicalPath, include.ExcludePaths) {
					if d.IsDir() {
						return fs.SkipDir
					}
					return nil
				}
				recordSMBEntry(seen, logicalPath, d)
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}

		for _, entry := range entries {
			logicalPath := joinLogicalPath(include.Path, entry.Name())
			if smbPathExcluded(logicalPath, include.ExcludePaths) {
				continue
			}
			recordSMBDirEntry(seen, logicalPath, entry)
		}
	}

	files := make([]map[string]any, 0, len(seen))
	paths := make([]string, 0, len(seen))
	for logicalPath := range seen {
		paths = append(paths, logicalPath)
	}
	sort.Strings(paths)
	for _, logicalPath := range paths {
		files = append(files, seen[logicalPath])
	}
	return files, nil
}

type sourceDeleteFile struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

type sourceDeletePlanItem struct {
	Path   string `json:"path"`
	IsDir  bool   `json:"is_dir,omitempty"`
	Size   int64  `json:"size,omitempty"`
	Action string `json:"action"`
}

func handleSourceDelete(params json.RawMessage) (any, *Error) {
	var body struct {
		Config       SMBConfig          `json:"config"`
		RootPath     string             `json:"root_path"`
		SourceGameID string             `json:"source_game_id"`
		Files        []sourceDeleteFile `json:"files"`
		DryRun       bool               `json:"dry_run"`
	}
	if err := json.Unmarshal(params, &body); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	rootPath := sourcescope.NormalizeLogicalPath(body.RootPath)
	if rootPath == "" {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "root_path is required"}
	}
	if !sourcescope.ScopeContainsRootPath(rootPath, normalizedIncludePaths(body.Config)) {
		return nil, &Error{Code: "NOT_ALLOWED", Message: "root_path is outside the configured include_paths scope"}
	}
	if len(body.Files) == 0 {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "files are required"}
	}

	items, errObj := buildSourceDeletePlan(rootPath, body.Config, body.Files)
	if errObj != nil {
		return nil, errObj
	}
	if body.DryRun {
		return sourceDeleteResponse(body.SourceGameID, "game-source-smb", "delete", items, 0), nil
	}

	conn, session, share, err := mountShare(body.Config)
	if err != nil {
		return nil, &Error{Code: "DELETE_FAILED", Message: err.Error()}
	}
	defer conn.Close()
	defer session.Logoff()
	defer share.Umount()

	for _, item := range items {
		if err := share.Remove(item.Path); err != nil {
			return nil, &Error{Code: "DELETE_FAILED", Message: err.Error()}
		}
	}
	return sourceDeleteResponse(body.SourceGameID, "game-source-smb", "delete", items, len(items)), nil
}

func buildSourceDeletePlan(rootPath string, config SMBConfig, files []sourceDeleteFile) ([]sourceDeletePlanItem, *Error) {
	items := make([]sourceDeletePlanItem, 0, len(files))
	for _, file := range files {
		filePath := sourcescope.NormalizeLogicalPath(file.Path)
		if filePath == "" {
			return nil, &Error{Code: "INVALID_PARAMS", Message: "file path is required"}
		}
		if file.IsDir {
			return nil, &Error{Code: "INVALID_PARAMS", Message: fmt.Sprintf("refusing to delete directory entry %q", filePath)}
		}
		if !sourceDeletePathWithinRoot(rootPath, filePath) {
			return nil, &Error{Code: "NOT_ALLOWED", Message: fmt.Sprintf("file %q is outside root_path %q", filePath, rootPath)}
		}
		if !sourcescope.ScopeContainsRootPath(filePath, normalizedIncludePaths(config)) {
			return nil, &Error{Code: "NOT_ALLOWED", Message: fmt.Sprintf("file %q is outside the configured include_paths scope", filePath)}
		}
		items = append(items, sourceDeletePlanItem{
			Path:   filePath,
			Size:   file.Size,
			Action: "delete",
		})
	}
	return items, nil
}

func sourceDeleteResponse(sourceGameID, pluginID, action string, items []sourceDeletePlanItem, deletedCount int) map[string]any {
	return map[string]any{
		"source_game_id": sourceGameID,
		"plugin_id":      pluginID,
		"action":         action,
		"summary":        fmt.Sprintf("%d file(s) will be permanently deleted.", len(items)),
		"items":          items,
		"warnings":       []string{},
		"deleted_count":  deletedCount,
	}
}

func sourceDeletePathWithinRoot(rootPath, filePath string) bool {
	rootPath = sourcescope.NormalizeLogicalPath(rootPath)
	filePath = sourcescope.NormalizeLogicalPath(filePath)
	if rootPath == "" {
		return filePath != ""
	}
	return filePath == rootPath || strings.HasPrefix(filePath, rootPath+"/")
}

func handleFileMaterialize(params json.RawMessage) (any, *Error) {
	var body struct {
		Config   SMBConfig `json:"config"`
		Path     string    `json:"path"`
		DestPath string    `json:"dest_path"`
	}
	if err := json.Unmarshal(params, &body); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	if strings.TrimSpace(body.Path) == "" {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "path is required"}
	}
	if strings.TrimSpace(body.DestPath) == "" {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "dest_path is required"}
	}

	sharePath, err := resolveSMBSharePath(body.Config.Path, body.Path)
	if err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	conn, session, share, err := mountShare(body.Config)
	if err != nil {
		return nil, &Error{Code: "MATERIALIZE_FAILED", Message: err.Error()}
	}
	defer conn.Close()
	defer session.Logoff()
	defer share.Umount()

	source, err := share.Open(sharePath)
	if err != nil {
		return nil, &Error{Code: "MATERIALIZE_FAILED", Message: err.Error()}
	}
	defer source.Close()

	if err := os.MkdirAll(filepath.Dir(body.DestPath), 0o755); err != nil {
		return nil, &Error{Code: "MATERIALIZE_FAILED", Message: err.Error()}
	}
	dest, err := os.Create(body.DestPath)
	if err != nil {
		return nil, &Error{Code: "MATERIALIZE_FAILED", Message: err.Error()}
	}
	size, copyErr := io.Copy(dest, source)
	closeErr := dest.Close()
	if copyErr != nil {
		_ = os.Remove(body.DestPath)
		return nil, &Error{Code: "MATERIALIZE_FAILED", Message: copyErr.Error()}
	}
	if closeErr != nil {
		_ = os.Remove(body.DestPath)
		return nil, &Error{Code: "MATERIALIZE_FAILED", Message: closeErr.Error()}
	}

	result := map[string]any{"size": size}
	if info, err := source.Stat(); err == nil {
		result["size"] = info.Size()
		if !info.ModTime().IsZero() {
			result["mod_time"] = info.ModTime().UTC().Format(time.RFC3339)
			result["revision"] = fmt.Sprintf("%s:%d", info.ModTime().UTC().Format(time.RFC3339), info.Size())
		}
	}
	return result, nil
}

func normalizedIncludePaths(config SMBConfig) []sourcescope.IncludePath {
	if len(config.IncludePaths) > 0 {
		includes := make([]sourcescope.IncludePath, 0, len(config.IncludePaths))
		for _, include := range config.IncludePaths {
			includes = append(includes, sourcescope.IncludePath{
				Path:         sourcescope.NormalizeLogicalPath(include.Path),
				Recursive:    include.Recursive,
				ExcludePaths: normalizeStringPaths(include.ExcludePaths),
			})
		}
		return includes
	}
	return []sourcescope.IncludePath{{
		Path:      sourcescope.NormalizeLogicalPath(config.Path),
		Recursive: true,
	}}
}

func smbPathExcluded(logicalPath string, excludes []string) bool {
	logicalPath = sourcescope.NormalizeLogicalPath(logicalPath)
	for _, exclude := range excludes {
		exclude = sourcescope.NormalizeLogicalPath(exclude)
		if exclude == "" {
			continue
		}
		if logicalPath == exclude || strings.HasPrefix(logicalPath, exclude+"/") {
			return true
		}
	}
	return false
}

func normalizeStringPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		item := sourcescope.NormalizeLogicalPath(path)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		normalized = append(normalized, item)
	}
	return normalized
}

func sourceIdentity(config SMBConfig) string {
	host := strings.ToLower(strings.TrimSpace(config.Host))
	share := strings.ToLower(strings.TrimSpace(config.Share))
	return "smb://" + host + "/" + share
}

func joinLogicalPath(basePath, child string) string {
	base := sourcescope.NormalizeLogicalPath(basePath)
	part := sourcescope.NormalizeLogicalPath(child)
	if base == "" {
		return part
	}
	if part == "" {
		return base
	}
	return filepath.ToSlash(base + "/" + part)
}

func resolveSMBSharePath(basePath, relativePath string) (string, error) {
	path := strings.TrimSpace(relativePath)
	if path == "" {
		return "", fmt.Errorf("empty file path")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute file path not allowed")
	}
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("path traversal")
	}

	full := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	base := strings.TrimSpace(basePath)
	if base != "" && base != "." {
		full = filepath.ToSlash(filepath.Clean(filepath.Join(filepath.FromSlash(base), filepath.FromSlash(path))))
	}
	if strings.HasPrefix(full, "../") || full == ".." {
		return "", fmt.Errorf("outside smb root")
	}
	return full, nil
}

func recordSMBEntry(seen map[string]map[string]any, logicalPath string, entry fs.DirEntry) {
	if logicalPath == "" {
		return
	}
	record := map[string]any{
		"path":   logicalPath,
		"name":   entry.Name(),
		"is_dir": entry.IsDir(),
	}
	if !entry.IsDir() {
		if info, err := entry.Info(); err == nil {
			record["size"] = info.Size()
		}
	}
	seen[logicalPath] = record
}

func recordSMBDirEntry(seen map[string]map[string]any, logicalPath string, entry os.FileInfo) {
	if logicalPath == "" {
		return
	}
	record := map[string]any{
		"path":   logicalPath,
		"name":   entry.Name(),
		"is_dir": entry.IsDir(),
	}
	if !entry.IsDir() {
		record["size"] = entry.Size()
	}
	seen[logicalPath] = record
}
