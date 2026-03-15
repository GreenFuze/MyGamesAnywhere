package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// --------------- IPC protocol ---------------

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

// --------------- Google OAuth constants ---------------

const (
	googleClientID     = "628863254475-ocs0abrdiba0mq6hev7fm1kke3nd2a99.apps.googleusercontent.com"
	googleClientSecret = "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"
	localPort          = "9092"
	redirectURI        = "http://localhost:9092/auth/google/callback"
	tokenFile          = "tokens.json"
	configFile         = "config.json"
	backupFileName     = "mga_backup.db"
)

var oauthConfig = &oauth2.Config{
	ClientID:     googleClientID,
	ClientSecret: googleClientSecret,
	Endpoint:     google.Endpoint,
	RedirectURL:  redirectURI,
	Scopes:       []string{drive.DriveScope},
}

// --------------- Config ---------------

type driveConfig struct {
	RootPath       string `json:"root_path"`
	BackupFolderID string `json:"backup_folder_id"`
}

var cfg driveConfig

func loadConfig() {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return
	}
	json.Unmarshal(data, &cfg)
}

// --------------- Token persistence ---------------

type savedTokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry"`
}

var tokenMu sync.Mutex
var cachedToken *oauth2.Token

func loadTokens() *oauth2.Token {
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil
	}
	var st savedTokens
	if err := json.Unmarshal(data, &st); err != nil {
		return nil
	}
	return &oauth2.Token{
		AccessToken:  st.AccessToken,
		RefreshToken: st.RefreshToken,
		TokenType:    st.TokenType,
		Expiry:       st.Expiry,
	}
}

func saveToken(tok *oauth2.Token) {
	st := savedTokens{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		Expiry:       tok.Expiry,
	}
	data, _ := json.MarshalIndent(st, "", "  ")
	os.WriteFile(tokenFile, data, 0600)
}

// --------------- OAuth browser flow ---------------

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}

func runOAuthFlow(ctx context.Context) (*oauth2.Token, error) {
	state := randomState()
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/google/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("OAuth state mismatch")
			return
		}
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			http.Error(w, "Auth error: "+errMsg, http.StatusBadRequest)
			errCh <- fmt.Errorf("OAuth error: %s", errMsg)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "no code", http.StatusBadRequest)
			errCh <- fmt.Errorf("no code in callback")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><h2>Google Drive authentication successful!</h2><p>You can close this tab and return to MyGamesAnywhere.</p></body></html>`)
		codeCh <- code
	})

	listener, err := net.Listen("tcp", "localhost:"+localPort)
	if err != nil {
		return nil, fmt.Errorf("listen on localhost:%s: %w", localPort, err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)
	defer srv.Shutdown(context.Background())

	authURL := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	log.Printf("=== GOOGLE DRIVE AUTHENTICATION REQUIRED ===")
	log.Printf("Open this URL in your browser:")
	log.Printf("%s", authURL)
	log.Printf("=============================================")
	openBrowser(authURL)

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(300 * time.Second):
		return nil, fmt.Errorf("OAuth flow timed out (300s)")
	}

	tok, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	return tok, nil
}

// --------------- Ensure authenticated ---------------

func ensureAuthenticated(ctx context.Context) error {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	if cachedToken == nil {
		cachedToken = loadTokens()
	}

	if cachedToken != nil && cachedToken.Valid() {
		log.Printf("using cached Google token (expires %s)", cachedToken.Expiry.Format(time.RFC3339))
		return nil
	}

	// Try refresh via the oauth2 library (it handles refresh automatically with TokenSource).
	if cachedToken != nil && cachedToken.RefreshToken != "" {
		log.Println("refreshing Google token...")
		src := oauthConfig.TokenSource(ctx, cachedToken)
		newTok, err := src.Token()
		if err == nil {
			cachedToken = newTok
			saveToken(newTok)
			log.Printf("refreshed Google token (expires %s)", newTok.Expiry.Format(time.RFC3339))
			return nil
		}
		log.Printf("refresh failed: %v, falling back to browser flow", err)
	}

	// Full browser flow.
	tokenMu.Unlock()
	tok, err := runOAuthFlow(ctx)
	tokenMu.Lock()
	if err != nil {
		return fmt.Errorf("OAuth flow: %w", err)
	}

	cachedToken = tok
	saveToken(tok)
	log.Println("Google Drive authentication complete")
	return nil
}

func getDriveService(ctx context.Context) (*drive.Service, error) {
	tokenMu.Lock()
	tok := cachedToken
	tokenMu.Unlock()

	if tok == nil {
		return nil, fmt.Errorf("not authenticated")
	}

	src := oauthConfig.TokenSource(ctx, tok)
	return drive.NewService(ctx, option.WithTokenSource(src))
}

// --------------- Resolve path to folder ID ---------------

func resolvePathToFolderID(srv *drive.Service, rootPath string) (string, error) {
	if rootPath == "" {
		return "root", nil
	}

	parts := strings.Split(rootPath, "/")
	currentID := "root"

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		query := fmt.Sprintf("'%s' in parents and name = '%s' and mimeType = 'application/vnd.google-apps.folder' and trashed = false",
			currentID, strings.ReplaceAll(part, "'", "\\'"))
		result, err := srv.Files.List().Q(query).Fields("files(id, name)").PageSize(1).Do()
		if err != nil {
			return "", fmt.Errorf("find folder %q: %w", part, err)
		}
		if len(result.Files) == 0 {
			return "", fmt.Errorf("folder %q not found under %s", part, currentID)
		}
		currentID = result.Files[0].Id
	}

	return currentID, nil
}

// --------------- File listing (source.filesystem.list) ---------------

func listFiles(ctx context.Context, rootPath string) ([]map[string]any, error) {
	srv, err := getDriveService(ctx)
	if err != nil {
		return nil, err
	}

	folderID, err := resolvePathToFolderID(srv, rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve root path: %w", err)
	}
	log.Printf("scanning Drive folder ID %s (path=%q)", folderID, rootPath)

	type folderItem struct {
		id   string
		path string
	}

	var files []map[string]any
	queue := []folderItem{{id: folderID, path: ""}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		query := fmt.Sprintf("'%s' in parents and trashed = false", current.id)
		pageToken := ""
		for {
			call := srv.Files.List().Q(query).
				Fields("nextPageToken, files(id, name, mimeType, size, modifiedTime)").
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

				entry := map[string]any{
					"path":   entryPath,
					"name":   f.Name,
					"is_dir": isDir,
					"size":   f.Size,
				}
				if f.ModifiedTime != "" {
					entry["mod_time"] = f.ModifiedTime
				}

				files = append(files, entry)

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

// --------------- Settings sync (storage.backup / storage.restore) ---------------

type StorageParams struct {
	DBPath string `json:"db_path"`
}

func getBackupFolderID(srv *drive.Service) (string, error) {
	if cfg.BackupFolderID != "" {
		return cfg.BackupFolderID, nil
	}
	return "root", nil
}

func backupDB(ctx context.Context, params StorageParams) error {
	srv, err := getDriveService(ctx)
	if err != nil {
		return err
	}

	folderID, err := getBackupFolderID(srv)
	if err != nil {
		return err
	}

	f, err := os.Open(params.DBPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer f.Close()

	query := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", backupFileName, folderID)
	existing, err := srv.Files.List().Q(query).Fields("files(id)").Do()
	if err != nil {
		return fmt.Errorf("find existing backup: %w", err)
	}

	if len(existing.Files) > 0 {
		_, err = srv.Files.Update(existing.Files[0].Id, nil).Media(f).Do()
	} else {
		meta := &drive.File{Name: backupFileName, Parents: []string{folderID}}
		_, err = srv.Files.Create(meta).Media(f).Do()
	}
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	log.Printf("backup uploaded to Drive folder %s", folderID)
	return nil
}

func restoreDB(ctx context.Context, params StorageParams) error {
	srv, err := getDriveService(ctx)
	if err != nil {
		return err
	}

	folderID, err := getBackupFolderID(srv)
	if err != nil {
		return err
	}

	query := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", backupFileName, folderID)
	found, err := srv.Files.List().Q(query).Fields("files(id)").Do()
	if err != nil {
		return fmt.Errorf("find backup: %w", err)
	}
	if len(found.Files) == 0 {
		log.Println("no backup found on Drive, starting fresh")
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
	log.Printf("restored backup from Drive to %s", params.DBPath)
	return nil
}

// --------------- IPC handlers ---------------

func handleInit() (any, *Error) {
	loadConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	if err := ensureAuthenticated(ctx); err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	return map[string]any{
		"status":    "ok",
		"root_path": cfg.RootPath,
	}, nil
}

func handleFileList(params json.RawMessage) (any, *Error) {
	ctx, cancel := context.WithTimeout(context.Background(), 540*time.Second)
	defer cancel()

	if err := ensureAuthenticated(ctx); err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	files, err := listFiles(ctx, cfg.RootPath)
	if err != nil {
		return nil, &Error{Code: "SCAN_FAILED", Message: err.Error()}
	}
	log.Printf("listed %d files from Drive (root_path=%q)", len(files), cfg.RootPath)
	return map[string]any{"files": files}, nil
}

func handleBackup(params json.RawMessage) (any, *Error) {
	var sp StorageParams
	if err := json.Unmarshal(params, &sp); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := ensureAuthenticated(ctx); err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	if err := backupDB(ctx, sp); err != nil {
		return nil, &Error{Code: "BACKUP_FAILED", Message: err.Error()}
	}
	return map[string]any{"status": "ok"}, nil
}

func handleRestore(params json.RawMessage) (any, *Error) {
	var sp StorageParams
	if err := json.Unmarshal(params, &sp); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := ensureAuthenticated(ctx); err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	if err := restoreDB(ctx, sp); err != nil {
		return nil, &Error{Code: "RESTORE_FAILED", Message: err.Error()}
	}
	return map[string]any{"status": "ok"}, nil
}

func handleCheckConfig(params json.RawMessage) (any, *Error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := ensureAuthenticated(ctx); err != nil {
		return map[string]any{"status": "error", "message": err.Error()}, nil
	}

	srv, err := getDriveService(ctx)
	if err != nil {
		return map[string]any{"status": "error", "message": err.Error()}, nil
	}

	if cfg.RootPath != "" {
		_, err := resolvePathToFolderID(srv, cfg.RootPath)
		if err != nil {
			return map[string]any{"status": "error", "message": fmt.Sprintf("root_path %q: %v", cfg.RootPath, err)}, nil
		}
	}

	return map[string]any{"status": "ok"}, nil
}

// --------------- Main ---------------

func main() {
	log.SetOutput(os.Stderr)
	log.Println("Google Drive plugin started")

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

		var resp Response
		resp.ID = req.ID

		switch req.Method {
		case "plugin.init":
			result, errObj := handleInit()
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "plugin.info":
			resp.Result = map[string]any{
				"plugin_id":      "game-source-google-drive",
				"plugin_version": "2.0.0",
				"capabilities":   []string{"source", "storage"},
			}

		case "plugin.check_config":
			result, errObj := handleCheckConfig(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "source.filesystem.list":
			result, errObj := handleFileList(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "storage.backup":
			result, errObj := handleBackup(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "storage.restore":
			result, errObj := handleRestore(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		default:
			resp.Error = &Error{Code: "UNKNOWN_METHOD", Message: "unknown method: " + req.Method}
		}

		out, _ := json.Marshal(resp)
		binary.Write(os.Stdout, binary.BigEndian, uint32(len(out)))
		os.Stdout.Write(out)
	}
}
