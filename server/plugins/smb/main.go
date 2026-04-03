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

func main() {
	log.SetOutput(os.Stderr)
	log.Println("SMB source plugin started")

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

		switch req.Method {
		case "plugin.init":
			resp.Result = map[string]any{"status": "ok"}
		case "plugin.info":
			resp.Result = map[string]any{
				"plugin_id":      "game-source-smb",
				"plugin_version": "1.0.0",
				"capabilities":   []string{"source"},
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
								"path":      map[string]any{"type": "string", "required": true},
								"recursive": map[string]any{"type": "boolean"},
							},
						},
					},
				},
			}
		case "source.filesystem.list":
			var config SMBConfig
			if err := json.Unmarshal(req.Params, &config); err != nil {
				resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
				break
			}
			files, err := listFiles(config)
			if err != nil {
				resp.Error = &Error{Code: "SCAN_FAILED", Message: err.Error()}
			} else {
				resp.Result = map[string]any{"files": files}
			}
		case "plugin.check_config":
			var params struct {
				Config SMBConfig `json:"config"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
				break
			}
			if err := checkConfig(params.Config); err != nil {
				resp.Result = map[string]any{"status": "error", "message": err.Error()}
			} else {
				resp.Result = map[string]any{
					"status":          "ok",
					"source_identity": sourceIdentity(params.Config),
				}
			}
		default:
			resp.Error = &Error{Code: "NOT_SUPPORTED", Message: "Method not supported"}
		}

		respPayload, err := json.Marshal(resp)
		if err != nil {
			resp = Response{Error: &Error{Code: "INTERNAL", Message: "failed to encode response"}}
			respPayload, err = json.Marshal(resp)
			if err != nil {
				fmt.Fprintf(os.Stderr, "marshal response: %v\n", err)
				os.Exit(1)
			}
		}
		binary.Write(os.Stdout, binary.BigEndian, uint32(len(respPayload)))
		os.Stdout.Write(respPayload)
	}
}

func checkConfig(config SMBConfig) error {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:445", config.Host))
	if err != nil {
		return fmt.Errorf("failed to connect to host: %w", err)
	}
	defer conn.Close()

	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     config.Username,
			Password: config.Password,
		},
	}

	s, err := d.Dial(conn)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}
	defer s.Logoff()

	fs, err := s.Mount(config.Share)
	if err != nil {
		return fmt.Errorf("failed to mount share: %w", err)
	}
	defer fs.Umount()

	return nil
}

// listFiles walks the entire SMB share and returns every file and directory
// as a flat listing. No filtering — the scanner handles classification.
func listFiles(config SMBConfig) ([]map[string]any, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:445", config.Host))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     config.Username,
			Password: config.Password,
		},
	}

	s, err := d.Dial(conn)
	if err != nil {
		return nil, err
	}
	defer s.Logoff()

	remotefs, err := s.Mount(config.Share)
	if err != nil {
		return nil, fmt.Errorf("mount share %q: %w", config.Share, err)
	}
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

func normalizedIncludePaths(config SMBConfig) []sourcescope.IncludePath {
	if len(config.IncludePaths) > 0 {
		includes := make([]sourcescope.IncludePath, 0, len(config.IncludePaths))
		for _, include := range config.IncludePaths {
			includes = append(includes, sourcescope.IncludePath{
				Path:      sourcescope.NormalizeLogicalPath(include.Path),
				Recursive: include.Recursive,
			})
		}
		return includes
	}
	return []sourcescope.IncludePath{{
		Path:      sourcescope.NormalizeLogicalPath(config.Path),
		Recursive: true,
	}}
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
