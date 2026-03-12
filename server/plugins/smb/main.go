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
	"strings"

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
	Host     string `json:"host"`
	Share    string `json:"share"`
	Username string `json:"username"`
	Password string `json:"password"`
	Path     string `json:"path"`
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
					"path":     map[string]any{"type": "string", "default": ""},
				},
			}
		case "source.library.list":
			var config SMBConfig
			if err := json.Unmarshal(req.Params, &config); err != nil {
				resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
				break
			}
			games, err := listGames(config)
			if err != nil {
				resp.Error = &Error{Code: "SCAN_FAILED", Message: err.Error()}
			} else {
				resp.Result = map[string]any{"games": games}
			}
		case "plugin.check_config":
			var config SMBConfig
			if err := json.Unmarshal(req.Params, &config); err != nil {
				resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
				break
			}
			if err := checkConfig(config); err != nil {
				resp.Result = map[string]any{"status": "error", "message": err.Error()}
			} else {
				resp.Result = map[string]any{"status": "ok"}
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

func listGames(config SMBConfig) ([]any, error) {
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
		return nil, err
	}
	defer remotefs.Umount()

	games := []any{}
	searchPath := config.Path
	if searchPath == "" {
		searchPath = "."
	}

	// Walk the remote share (remotefs.DirFS returns an fs.FS rooted at searchPath).
	rootFS := remotefs.DirFS(searchPath)
	err = fs.WalkDir(rootFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors (e.g. permission)
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !isGameOrInstallerExt(ext) {
			return nil
		}
		games = append(games, map[string]any{
			"source_game_key": path,
			"display_name":    strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
			"provider_ids":    map[string]any{},
			"source_payload":  map[string]any{"path": path},
		})
		return nil
	})

	return games, nil
}

// isGameOrInstallerExt returns true for common game/installer file extensions.
func isGameOrInstallerExt(ext string) bool {
	switch ext {
	case ".exe", ".com", ".bat":
		return true
	case ".iso", ".cue", ".bin", ".img", ".ccd", ".chd", ".mdf", ".mds":
		return true
	case ".zip", ".7z", ".rar":
		return true
	default:
		return false
	}
}
