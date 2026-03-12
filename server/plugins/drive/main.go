package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
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

type DriveConfig struct {
	CredentialsJSON string `json:"credentials_json"`
	FolderID        string `json:"folder_id"`
}

type StorageParams struct {
	Config DriveConfig `json:"config"`
	DBPath string      `json:"db_path"`
}

func main() {
	log.SetOutput(os.Stderr)
	log.Println("Drive plugin started")

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
				"plugin_id":      "game-source-google-drive",
				"plugin_version": "1.0.0",
				"capabilities":   []string{"source", "storage"},
				"config": map[string]any{
					"credentials_json": map[string]any{"type": "string", "required": true, "x-secret": true},
					"folder_id":        map[string]any{"type": "string", "required": true},
				},
			}
		case "plugin.check_config":
			var config DriveConfig
			if err := json.Unmarshal(req.Params, &config); err != nil {
				resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
				break
			}
			if err := checkConfig(config); err != nil {
				resp.Result = map[string]any{"status": "error", "message": err.Error()}
			} else {
				resp.Result = map[string]any{"status": "ok"}
			}
		case "source.filesystem.list":
			var config DriveConfig
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
		case "storage.backup":
			var params StorageParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
				break
			}
			if err := backupDB(params); err != nil {
				resp.Error = &Error{Code: "BACKUP_FAILED", Message: err.Error()}
			} else {
				resp.Result = map[string]any{"status": "ok"}
			}
		case "storage.restore":
			var params StorageParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
				break
			}
			if err := restoreDB(params); err != nil {
				resp.Error = &Error{Code: "RESTORE_FAILED", Message: err.Error()}
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

// listFiles recursively lists all files and folders under the configured
// Drive folder, returning a flat listing for the scanner pipeline.
func listFiles(config DriveConfig) ([]map[string]any, error) {
	srv, err := newDriveService(config)
	if err != nil {
		return nil, fmt.Errorf("drive auth: %w", err)
	}

	type folderItem struct {
		id   string
		path string
	}

	var files []map[string]any
	queue := []folderItem{{id: config.FolderID, path: ""}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		query := fmt.Sprintf("'%s' in parents and trashed = false", current.id)
		pageToken := ""
		for {
			call := srv.Files.List().Q(query).
				Fields("nextPageToken, files(id, name, mimeType, size)").
				PageSize(1000)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}

			result, err := call.Do()
			if err != nil {
				return nil, fmt.Errorf("list folder %q: %w", current.path, err)
			}

			for _, f := range result.Files {
				isDir := f.MimeType == "application/vnd.google-apps.folder"
				entryPath := f.Name
				if current.path != "" {
					entryPath = current.path + "/" + f.Name
				}

				files = append(files, map[string]any{
					"path":   entryPath,
					"name":   f.Name,
					"is_dir": isDir,
					"size":   f.Size,
				})

				if isDir {
					queue = append(queue, folderItem{id: f.Id, path: entryPath})
				}
			}

			pageToken = result.NextPageToken
			if pageToken == "" {
				break
			}
		}
	}

	return files, nil
}

const backupFileName = "mga_backup.db"

func newDriveService(config DriveConfig) (*drive.Service, error) {
	ctx := context.Background()
	creds, err := google.CredentialsFromJSON(ctx, []byte(config.CredentialsJSON), drive.DriveScope)
	if err != nil {
		return nil, err
	}
	return drive.NewService(ctx, option.WithCredentials(creds))
}

func checkConfig(config DriveConfig) error {
	srv, err := newDriveService(config)
	if err != nil {
		return err
	}
	_, err = srv.Files.Get(config.FolderID).Fields("id").Do()
	return err
}

func backupDB(params StorageParams) error {
	srv, err := newDriveService(params.Config)
	if err != nil {
		return fmt.Errorf("drive auth: %w", err)
	}

	f, err := os.Open(params.DBPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer f.Close()

	query := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", backupFileName, params.Config.FolderID)
	existing, err := srv.Files.List().Q(query).Fields("files(id)").Do()
	if err != nil {
		return fmt.Errorf("find existing backup: %w", err)
	}

	if len(existing.Files) > 0 {
		_, err = srv.Files.Update(existing.Files[0].Id, nil).Media(f).Do()
	} else {
		meta := &drive.File{Name: backupFileName, Parents: []string{params.Config.FolderID}}
		_, err = srv.Files.Create(meta).Media(f).Do()
	}
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	return nil
}

func restoreDB(params StorageParams) error {
	srv, err := newDriveService(params.Config)
	if err != nil {
		return fmt.Errorf("drive auth: %w", err)
	}

	query := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", backupFileName, params.Config.FolderID)
	found, err := srv.Files.List().Q(query).Fields("files(id)").Do()
	if err != nil {
		return fmt.Errorf("find backup: %w", err)
	}
	if len(found.Files) == 0 {
		log.Println("No backup found on Drive, starting fresh")
		return nil
	}

	resp, err := srv.Files.Get(found.Files[0].Id).Download()
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	out, err := os.Create(params.DBPath)
	if err != nil {
		return fmt.Errorf("create db file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("write db: %w", err)
	}
	return nil
}
