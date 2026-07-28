package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
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

		if req.Method == "source.file.materialize" || req.Method == "source.transfer.put" {
			go func(req Request) {
				resp := Response{ID: req.ID}
				var result any
				var errObj *Error
				if req.Method == "source.transfer.put" {
					result, errObj = handleTransferPut(req.Params)
				} else {
					result, errObj = handleFileMaterialize(req.Params)
				}
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
				"provides":       []string{"source.filesystem.list", "source.filesystem.delete", "source.file.materialize", "source.transfer.begin", "source.transfer.put", "source.transfer.commit", "source.transfer.abort", "plugin.check_config"},
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
		case "source.transfer.begin":
			result, errObj := handleTransferBegin(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}
		case "source.transfer.commit":
			result, errObj := handleTransferCommit(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}
		case "source.transfer.abort":
			result, errObj := handleTransferAbort(req.Params)
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
				if d.IsDir() && (strings.EqualFold(d.Name(), ".mga") || strings.HasPrefix(strings.ToLower(d.Name()), ".mga-transfer-")) {
					return fs.SkipDir
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
			if entry.IsDir() && (strings.EqualFold(entry.Name(), ".mga") || strings.HasPrefix(strings.ToLower(entry.Name()), ".mga-transfer-")) {
				continue
			}
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
		return sourceDeleteResponse(body.SourceGameID, "game-source-smb", "delete", items, 0, []string{}), nil
	}

	conn, session, share, err := mountShare(body.Config)
	if err != nil {
		return nil, &Error{Code: "DELETE_FAILED", Message: err.Error()}
	}
	defer conn.Close()
	defer session.Logoff()
	defer share.Umount()

	deletedCount, warnings, errObj := executeSourceDeletePlan(share, items)
	if errObj != nil {
		return nil, errObj
	}
	return sourceDeleteResponse(body.SourceGameID, "game-source-smb", "delete", items, deletedCount, warnings), nil
}

func buildSourceDeletePlan(rootPath string, config SMBConfig, files []sourceDeleteFile) ([]sourceDeletePlanItem, *Error) {
	items := make([]sourceDeletePlanItem, 0, len(files))
	for _, file := range files {
		filePath := sourcescope.NormalizeLogicalPath(file.Path)
		if filePath == "" {
			return nil, &Error{Code: "INVALID_PARAMS", Message: "file path is required"}
		}
		if !sourceDeletePathWithinRoot(rootPath, filePath) {
			return nil, &Error{Code: "NOT_ALLOWED", Message: fmt.Sprintf("file %q is outside root_path %q", filePath, rootPath)}
		}
		if !sourcescope.ScopeContainsRootPath(filePath, normalizedIncludePaths(config)) {
			return nil, &Error{Code: "NOT_ALLOWED", Message: fmt.Sprintf("file %q is outside the configured include_paths scope", filePath)}
		}
		items = append(items, sourceDeletePlanItem{
			Path:   filePath,
			IsDir:  file.IsDir,
			Size:   file.Size,
			Action: sourceDeleteAction(file.IsDir),
		})
	}
	return items, nil
}

func sourceDeleteAction(isDir bool) string {
	if isDir {
		return "prune_empty"
	}
	return "delete"
}

type smbDeleteShare interface {
	Lstat(name string) (os.FileInfo, error)
	ReadDir(dirname string) ([]os.FileInfo, error)
	Remove(name string) error
}

func executeSourceDeletePlan(share smbDeleteShare, items []sourceDeletePlanItem) (int, []string, *Error) {
	sortedItems := append([]sourceDeletePlanItem(nil), items...)
	sort.SliceStable(sortedItems, func(i, j int) bool {
		if sortedItems[i].IsDir != sortedItems[j].IsDir {
			return !sortedItems[i].IsDir
		}
		if sortedItems[i].IsDir {
			return len(sortedItems[i].Path) > len(sortedItems[j].Path)
		}
		return false
	})

	alreadyGone := make(map[string]bool)
	warnings := []string{}
	for _, item := range sortedItems {
		info, err := share.Lstat(item.Path)
		if err != nil {
			if sourceDeleteAlreadyGone(err) {
				alreadyGone[item.Path] = true
				warnings = append(warnings, fmt.Sprintf("%q was already deleted before this operation.", item.Path))
				continue
			}
			return 0, warnings, &Error{Code: "DELETE_FAILED", Message: fmt.Sprintf("inspect %q before deletion: %v", item.Path, err)}
		}
		if errObj := validateSourceDeleteFileInfo(item, info); errObj != nil {
			return 0, warnings, errObj
		}
	}

	deletedCount := 0
	for _, item := range sortedItems {
		if alreadyGone[item.Path] {
			deletedCount++
			continue
		}
		if item.IsDir {
			info, err := share.Lstat(item.Path)
			if err != nil {
				if sourceDeleteAlreadyGone(err) {
					warnings = append(warnings, fmt.Sprintf("%q was already deleted before empty-folder cleanup.", item.Path))
					deletedCount++
					continue
				}
				return deletedCount, warnings, &Error{Code: "DELETE_FAILED", Message: fmt.Sprintf("re-inspect directory %q before cleanup: %v", item.Path, err)}
			}
			if errObj := validateSourceDeleteFileInfo(item, info); errObj != nil {
				return deletedCount, warnings, errObj
			}
			entries, err := share.ReadDir(item.Path)
			if err != nil {
				if sourceDeleteAlreadyGone(err) {
					warnings = append(warnings, fmt.Sprintf("%q was already deleted before empty-folder cleanup.", item.Path))
					deletedCount++
					continue
				}
				return deletedCount, warnings, &Error{Code: "DELETE_FAILED", Message: fmt.Sprintf("check whether directory %q is empty: %v", item.Path, err)}
			}
			if len(entries) != 0 {
				warnings = append(warnings, fmt.Sprintf("Directory %q was kept because it contains files MGA did not authorize for deletion.", item.Path))
				continue
			}
		}
		if err := share.Remove(item.Path); err != nil {
			if sourceDeleteAlreadyGone(err) {
				warnings = append(warnings, fmt.Sprintf("%q was already deleted before this operation.", item.Path))
				deletedCount++
				continue
			}
			if item.IsDir && sourceDeleteDirectoryNotEmpty(err) {
				warnings = append(warnings, fmt.Sprintf("Directory %q was kept because it became non-empty during cleanup.", item.Path))
				continue
			}
			return deletedCount, warnings, &Error{Code: "DELETE_FAILED", Message: fmt.Sprintf("remove %q: %v", item.Path, err)}
		}
		deletedCount++
	}
	return deletedCount, warnings, nil
}

func validateSourceDeleteFileInfo(item sourceDeletePlanItem, info os.FileInfo) *Error {
	if info == nil {
		return &Error{Code: "DELETE_FAILED", Message: fmt.Sprintf("inspect %q before deletion: no file information returned", item.Path)}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return &Error{Code: "NOT_ALLOWED", Message: fmt.Sprintf("%q is a link or reparse point; MGA will not delete through it", item.Path)}
	}
	if item.IsDir != info.IsDir() {
		expected := "file"
		if item.IsDir {
			expected = "directory"
		}
		return &Error{Code: "NOT_ALLOWED", Message: fmt.Sprintf("%q is no longer the expected %s", item.Path, expected)}
	}
	if !item.IsDir && info.Mode()&os.ModeType != 0 {
		return &Error{Code: "NOT_ALLOWED", Message: fmt.Sprintf("%q is not an ordinary file", item.Path)}
	}
	return nil
}

func sourceDeleteDirectoryNotEmpty(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "directory not empty") ||
		strings.Contains(msg, "status_directory_not_empty") ||
		strings.Contains(msg, "folder is not empty")
}

func sourceDeleteAlreadyGone(err error) bool {
	if err == nil {
		return false
	}
	if os.IsNotExist(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "file does not exist") ||
		strings.Contains(msg, "cannot find the file") ||
		strings.Contains(msg, "object name not found") ||
		strings.Contains(msg, "status_object_name_not_found") ||
		strings.Contains(msg, "status_no_such_file")
}

func sourceDeleteResponse(sourceGameID, pluginID, action string, items []sourceDeletePlanItem, deletedCount int, warnings []string) map[string]any {
	return map[string]any{
		"source_game_id": sourceGameID,
		"plugin_id":      pluginID,
		"action":         action,
		"summary":        fmt.Sprintf("%d item(s) will be permanently deleted.", len(items)),
		"items":          items,
		"warnings":       warnings,
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

type transferFile struct {
	RelativePath string `json:"relative_path"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
}

type transferRequest struct {
	Config          SMBConfig      `json:"config"`
	TransferID      string         `json:"transfer_id"`
	DestinationPath string         `json:"destination_path"`
	DryRun          bool           `json:"dry_run"`
	Files           []transferFile `json:"files"`
	RelativePath    string         `json:"relative_path"`
	SourcePath      string         `json:"source_path"`
	Size            int64          `json:"size"`
	SHA256          string         `json:"sha256"`
}

type transferMarker struct {
	TransferID   string `json:"transfer_id"`
	ManifestHash string `json:"manifest_hash"`
}

func handleTransferBegin(params json.RawMessage) (any, *Error) {
	body, errObj := decodeTransferRequest(params)
	if errObj != nil {
		return nil, errObj
	}
	if len(body.Files) == 0 {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "files are required"}
	}
	if !sourcescope.ScopeContainsRootPath(body.DestinationPath, normalizedIncludePaths(body.Config)) {
		return nil, &Error{Code: "NOT_ALLOWED", Message: "destination_path is outside the configured include_paths scope"}
	}
	for _, file := range body.Files {
		if _, err := safeTransferRelativePath(file.RelativePath); err != nil {
			return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
		}
	}
	if body.DryRun {
		conn, session, share, err := mountShare(body.Config)
		if err != nil {
			return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
		}
		defer conn.Close()
		defer session.Logoff()
		defer share.Umount()
		if _, err := share.Stat(body.DestinationPath); err == nil {
			return nil, &Error{Code: "DESTINATION_EXISTS", Message: fmt.Sprintf("%q already exists", body.DestinationPath)}
		} else if !os.IsNotExist(err) && !sourceDeleteAlreadyGone(err) {
			return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
		}
		return transferResult("ready", body.DestinationPath), nil
	}

	conn, session, share, err := mountShare(body.Config)
	if err != nil {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	defer conn.Close()
	defer session.Logoff()
	defer share.Umount()

	marker := transferMarker{TransferID: body.TransferID, ManifestHash: transferManifestHash(body.Files)}
	if existing, ok := readTransferMarker(share, body.DestinationPath); ok {
		if existing == marker {
			return transferResult("committed", body.DestinationPath), nil
		}
		return nil, &Error{Code: "DESTINATION_EXISTS", Message: fmt.Sprintf("%q belongs to another transfer", body.DestinationPath)}
	}
	if _, err := share.Stat(body.DestinationPath); err == nil {
		return nil, &Error{Code: "DESTINATION_EXISTS", Message: fmt.Sprintf("%q already exists", body.DestinationPath)}
	} else if !os.IsNotExist(err) && !sourceDeleteAlreadyGone(err) {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}

	stage := transferStagePath(body.DestinationPath, body.TransferID)
	if existing, ok := readTransferMarker(share, stage); ok {
		if existing == marker {
			return transferResult("staging", body.DestinationPath), nil
		}
		return nil, &Error{Code: "TRANSFER_COLLISION", Message: "temporary destination belongs to another transfer"}
	}
	if _, err := share.Stat(stage); err == nil {
		return nil, &Error{Code: "TRANSFER_COLLISION", Message: "temporary destination already exists without an MGA marker"}
	} else if !os.IsNotExist(err) && !sourceDeleteAlreadyGone(err) {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	if err := share.MkdirAll(joinLogicalPath(stage, ".mga"), 0o700); err != nil {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	if err := writeTransferMarker(share, stage, marker); err != nil {
		_ = share.RemoveAll(stage)
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	return transferResult("staging", body.DestinationPath), nil
}

func handleTransferPut(params json.RawMessage) (any, *Error) {
	body, errObj := decodeTransferRequest(params)
	if errObj != nil {
		return nil, errObj
	}
	relative, err := safeTransferRelativePath(body.RelativePath)
	if err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	if strings.TrimSpace(body.SourcePath) == "" {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "source_path is required"}
	}
	localHash, localSize, err := hashLocalFile(body.SourcePath)
	if err != nil {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	if body.Size >= 0 && localSize != body.Size {
		return nil, &Error{Code: "SOURCE_CHANGED", Message: "source file size changed"}
	}
	if !strings.EqualFold(localHash, strings.TrimSpace(body.SHA256)) {
		return nil, &Error{Code: "SOURCE_CHANGED", Message: "source file checksum changed"}
	}

	conn, session, share, err := mountShare(body.Config)
	if err != nil {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	defer conn.Close()
	defer session.Logoff()
	defer share.Umount()
	stage := transferStagePath(body.DestinationPath, body.TransferID)
	if marker, ok := readTransferMarker(share, body.DestinationPath); ok && marker.TransferID == body.TransferID {
		return transferResult("committed", body.DestinationPath), nil
	}
	marker, ok := readTransferMarker(share, stage)
	if !ok || marker.TransferID != body.TransferID {
		return nil, &Error{Code: "TRANSFER_NOT_STARTED", Message: "temporary destination is missing or does not belong to this transfer"}
	}
	target := joinLogicalPath(stage, relative)
	if hash, size, err := hashSMBFile(share, target); err == nil && size == body.Size && strings.EqualFold(hash, body.SHA256) {
		return transferResult("staged", body.DestinationPath), nil
	}
	if err := share.MkdirAll(filepath.ToSlash(filepath.Dir(filepath.FromSlash(target))), 0o700); err != nil {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	temp := target + ".mga-part"
	_ = share.Remove(temp)
	source, err := os.Open(body.SourcePath)
	if err != nil {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	defer source.Close()
	dest, err := share.Create(temp)
	if err != nil {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	_, copyErr := io.Copy(dest, source)
	closeErr := dest.Close()
	if copyErr != nil || closeErr != nil {
		_ = share.Remove(temp)
		if copyErr != nil {
			return nil, &Error{Code: "TRANSFER_FAILED", Message: copyErr.Error()}
		}
		return nil, &Error{Code: "TRANSFER_FAILED", Message: closeErr.Error()}
	}
	hash, size, err := hashSMBFile(share, temp)
	if err != nil || size != body.Size || !strings.EqualFold(hash, body.SHA256) {
		_ = share.Remove(temp)
		if err != nil {
			return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
		}
		return nil, &Error{Code: "DESTINATION_VERIFY_FAILED", Message: "copied file checksum does not match"}
	}
	_ = share.Remove(target)
	if err := share.Rename(temp, target); err != nil {
		_ = share.Remove(temp)
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	return transferResult("staged", body.DestinationPath), nil
}

func handleTransferCommit(params json.RawMessage) (any, *Error) {
	body, errObj := decodeTransferRequest(params)
	if errObj != nil {
		return nil, errObj
	}
	if len(body.Files) == 0 {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "files are required"}
	}
	conn, session, share, err := mountShare(body.Config)
	if err != nil {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	defer conn.Close()
	defer session.Logoff()
	defer share.Umount()
	expected := transferMarker{TransferID: body.TransferID, ManifestHash: transferManifestHash(body.Files)}
	if marker, ok := readTransferMarker(share, body.DestinationPath); ok {
		if marker == expected {
			return transferResult("committed", body.DestinationPath), nil
		}
		return nil, &Error{Code: "DESTINATION_EXISTS", Message: "destination belongs to another transfer"}
	}
	if _, err := share.Stat(body.DestinationPath); err == nil {
		return nil, &Error{Code: "DESTINATION_EXISTS", Message: fmt.Sprintf("%q already exists", body.DestinationPath)}
	} else if !os.IsNotExist(err) && !sourceDeleteAlreadyGone(err) {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	stage := transferStagePath(body.DestinationPath, body.TransferID)
	marker, ok := readTransferMarker(share, stage)
	if !ok || marker != expected {
		return nil, &Error{Code: "TRANSFER_NOT_STARTED", Message: "temporary destination marker does not match this transfer"}
	}
	for _, file := range body.Files {
		relative, err := safeTransferRelativePath(file.RelativePath)
		if err != nil {
			return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
		}
		hash, size, err := hashSMBFile(share, joinLogicalPath(stage, relative))
		if err != nil {
			return nil, &Error{Code: "DESTINATION_VERIFY_FAILED", Message: err.Error()}
		}
		if size != file.Size || !strings.EqualFold(hash, file.SHA256) {
			return nil, &Error{Code: "DESTINATION_VERIFY_FAILED", Message: fmt.Sprintf("%q does not match the expected file", relative)}
		}
	}
	if err := share.Rename(stage, body.DestinationPath); err != nil {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	return transferResult("committed", body.DestinationPath), nil
}

func handleTransferAbort(params json.RawMessage) (any, *Error) {
	body, errObj := decodeTransferRequest(params)
	if errObj != nil {
		return nil, errObj
	}
	conn, session, share, err := mountShare(body.Config)
	if err != nil {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	defer conn.Close()
	defer session.Logoff()
	defer share.Umount()
	if marker, ok := readTransferMarker(share, body.DestinationPath); ok && marker.TransferID == body.TransferID {
		return transferResult("committed", body.DestinationPath), nil
	}
	stage := transferStagePath(body.DestinationPath, body.TransferID)
	marker, ok := readTransferMarker(share, stage)
	if !ok {
		if _, err := share.Stat(stage); err == nil {
			return nil, &Error{Code: "NOT_ALLOWED", Message: "temporary destination has no matching MGA ownership marker"}
		}
		return transferResult("aborted", body.DestinationPath), nil
	}
	if marker.TransferID != body.TransferID {
		return nil, &Error{Code: "NOT_ALLOWED", Message: "temporary destination belongs to another transfer"}
	}
	if err := share.RemoveAll(stage); err != nil && !sourceDeleteAlreadyGone(err) {
		return nil, &Error{Code: "TRANSFER_FAILED", Message: err.Error()}
	}
	return transferResult("aborted", body.DestinationPath), nil
}

func decodeTransferRequest(params json.RawMessage) (transferRequest, *Error) {
	var body transferRequest
	if err := json.Unmarshal(params, &body); err != nil {
		return body, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	configPayload, err := json.Marshal(body.Config)
	if err != nil {
		return body, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	validatedConfig, err := decodeSMBConfig(configPayload)
	if err != nil {
		return body, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	body.Config = validatedConfig
	body.TransferID = strings.TrimSpace(body.TransferID)
	rawDestinationPath := strings.TrimSpace(body.DestinationPath)
	if transferPathHasTraversal(rawDestinationPath) || filepath.IsAbs(rawDestinationPath) {
		return body, &Error{Code: "INVALID_PARAMS", Message: "valid destination_path is required"}
	}
	body.DestinationPath = sourcescope.NormalizeLogicalPath(rawDestinationPath)
	if body.TransferID == "" || len(body.TransferID) > 128 || strings.ContainsAny(body.TransferID, `/\`) {
		return body, &Error{Code: "INVALID_PARAMS", Message: "valid transfer_id is required"}
	}
	if body.DestinationPath == "" || strings.HasPrefix(body.DestinationPath, "../") {
		return body, &Error{Code: "INVALID_PARAMS", Message: "valid destination_path is required"}
	}
	if !sourcescope.ScopeContainsRootPath(body.DestinationPath, normalizedIncludePaths(body.Config)) {
		return body, &Error{Code: "NOT_ALLOWED", Message: "destination_path is outside the configured include_paths scope"}
	}
	return body, nil
}

func transferStagePath(destinationPath, transferID string) string {
	parent := sourcescope.NormalizeLogicalPath(filepath.ToSlash(filepath.Dir(filepath.FromSlash(destinationPath))))
	if parent == "." {
		parent = ""
	}
	return joinLogicalPath(parent, ".mga-transfer-"+transferID)
}

func safeTransferRelativePath(value string) (string, error) {
	if transferPathHasTraversal(value) || filepath.IsAbs(value) {
		return "", fmt.Errorf("unsafe relative_path %q", value)
	}
	normalized := sourcescope.NormalizeLogicalPath(value)
	if normalized == "" || normalized == "." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("unsafe relative_path %q", value)
	}
	if normalized == ".mga" || strings.HasPrefix(normalized, ".mga/") {
		return "", fmt.Errorf("relative_path %q uses MGA's reserved control folder", value)
	}
	return normalized, nil
}

func transferPathHasTraversal(value string) bool {
	for _, component := range strings.Split(strings.ReplaceAll(value, `\`, "/"), "/") {
		if strings.TrimSpace(component) == ".." {
			return true
		}
	}
	return false
}

func transferManifestHash(files []transferFile) string {
	copyFiles := append([]transferFile(nil), files...)
	sort.SliceStable(copyFiles, func(i, j int) bool { return copyFiles[i].RelativePath < copyFiles[j].RelativePath })
	hash := sha256.New()
	for _, file := range copyFiles {
		fmt.Fprintf(hash, "%s\x00%d\x00%s\n", sourcescope.NormalizeLogicalPath(file.RelativePath), file.Size, strings.ToLower(strings.TrimSpace(file.SHA256)))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func transferMarkerPath(root string) string {
	return joinLogicalPath(root, ".mga/transfer.json")
}

func readTransferMarker(share *smb2.Share, root string) (transferMarker, bool) {
	data, err := share.ReadFile(transferMarkerPath(root))
	if err != nil {
		return transferMarker{}, false
	}
	var marker transferMarker
	if json.Unmarshal(data, &marker) != nil || strings.TrimSpace(marker.TransferID) == "" || strings.TrimSpace(marker.ManifestHash) == "" {
		return transferMarker{}, false
	}
	return marker, true
}

func writeTransferMarker(share *smb2.Share, root string, marker transferMarker) error {
	data, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return share.WriteFile(transferMarkerPath(root), data, 0o600)
}

func hashLocalFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	return hashReader(file)
}

func hashSMBFile(share *smb2.Share, path string) (string, int64, error) {
	file, err := share.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	return hashReader(file)
}

func hashReader(reader io.Reader) (string, int64, error) {
	hash := sha256.New()
	size, err := io.Copy(hash, reader)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func transferResult(status, destinationPath string) map[string]any {
	return map[string]any{"status": status, "destination_path": destinationPath}
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
