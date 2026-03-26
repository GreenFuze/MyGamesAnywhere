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
	"os"
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

// --------------- Credentials ---------------

const (
	tokenFile  = "tokens.json"
	configFile = "config.json"
)

// Injected at build time via -ldflags "-X main.builtinClientID=... -X main.builtinClientSecret=..."
// These are the MGA app registration credentials, NOT user credentials.
var (
	builtinClientID     string
	builtinClientSecret string
)

// oauthConfig is lazily initialized in loadConfig after credentials are resolved.
var oauthConfig *oauth2.Config

// oauthPending tracks OAuth state tokens for CSRF validation. Protected by tokenMu.
var oauthPending = map[string]bool{}

// --------------- Config ---------------

type driveConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func loadConfig() error {
	// Start with built-in credentials (injected via ldflags at compile time).
	clientID := builtinClientID
	clientSecret := builtinClientSecret

	// Overlay with config.json if present (allows local dev override).
	data, err := os.ReadFile(configFile)
	if err == nil {
		var fileCfg driveConfig
		if err := json.Unmarshal(data, &fileCfg); err != nil {
			return fmt.Errorf("parse config.json: %w", err)
		}
		if fileCfg.ClientID != "" {
			clientID = fileCfg.ClientID
		}
		if fileCfg.ClientSecret != "" {
			clientSecret = fileCfg.ClientSecret
		}
	}

	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("no client credentials: set via build-time ldflags or config.json")
	}

	// Initialize the global oauth2 config with resolved credentials.
	oauthConfig = &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{drive.DriveScope},
	}

	return nil
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

// --------------- OAuth helpers ---------------

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
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

// --------------- Settings sync (sync.push / sync.pull) ---------------

type syncPushParams struct {
	Data   string         `json:"data"`
	Config map[string]any `json:"config"`
}

type syncPullParams struct {
	Config map[string]any `json:"config"`
}

func syncPathFromConfig(cfg map[string]any) string {
	if v, ok := cfg["sync_path"].(string); ok && v != "" {
		return v
	}
	return "Games/mga_sync"
}

func maxVersionsFromConfig(cfg map[string]any) int {
	if v, ok := cfg["max_versions"].(float64); ok && v > 0 {
		return int(v)
	}
	return 10
}

// ensureFolderPath creates the folder path under My Drive, returning the leaf folder ID.
func ensureFolderPath(srv *drive.Service, path string) (string, error) {
	parts := strings.Split(path, "/")
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
		if len(result.Files) > 0 {
			currentID = result.Files[0].Id
		} else {
			meta := &drive.File{
				Name:     part,
				MimeType: "application/vnd.google-apps.folder",
				Parents:  []string{currentID},
			}
			created, err := srv.Files.Create(meta).Fields("id").Do()
			if err != nil {
				return "", fmt.Errorf("create folder %q: %w", part, err)
			}
			currentID = created.Id
			log.Printf("created Drive folder %q (id=%s)", part, currentID)
		}
	}
	return currentID, nil
}

func syncPush(ctx context.Context, params syncPushParams) (map[string]any, error) {
	srv, err := getDriveService(ctx)
	if err != nil {
		return nil, err
	}

	syncPath := syncPathFromConfig(params.Config)
	maxVer := maxVersionsFromConfig(params.Config)

	folderID, err := ensureFolderPath(srv, syncPath)
	if err != nil {
		return nil, fmt.Errorf("ensure sync folder: %w", err)
	}

	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	versionedName := fmt.Sprintf("mga_sync_%s.json", timestamp)

	reader := strings.NewReader(params.Data)
	meta := &drive.File{Name: versionedName, Parents: []string{folderID}, MimeType: "application/json"}
	if _, err := srv.Files.Create(meta).Media(reader).Do(); err != nil {
		return nil, fmt.Errorf("upload versioned file: %w", err)
	}
	log.Printf("uploaded %s to Drive", versionedName)

	// Upsert latest.json
	latestQuery := fmt.Sprintf("name = 'latest.json' and '%s' in parents and trashed = false", folderID)
	existing, err := srv.Files.List().Q(latestQuery).Fields("files(id)").Do()
	if err != nil {
		return nil, fmt.Errorf("find latest.json: %w", err)
	}
	latestReader := strings.NewReader(params.Data)
	if len(existing.Files) > 0 {
		_, err = srv.Files.Update(existing.Files[0].Id, nil).Media(latestReader).Do()
	} else {
		latestMeta := &drive.File{Name: "latest.json", Parents: []string{folderID}, MimeType: "application/json"}
		_, err = srv.Files.Create(latestMeta).Media(latestReader).Do()
	}
	if err != nil {
		return nil, fmt.Errorf("upsert latest.json: %w", err)
	}

	// Prune old versions beyond maxVer.
	versionCount, err := pruneOldVersions(srv, folderID, maxVer)
	if err != nil {
		log.Printf("prune warning: %v", err)
	}

	return map[string]any{
		"status":        "ok",
		"version_count": versionCount,
		"latest":        versionedName,
	}, nil
}

func pruneOldVersions(srv *drive.Service, folderID string, maxVersions int) (int, error) {
	query := fmt.Sprintf("name contains 'mga_sync_' and name contains '.json' and '%s' in parents and trashed = false", folderID)
	result, err := srv.Files.List().Q(query).Fields("files(id, name, createdTime)").OrderBy("createdTime desc").PageSize(200).Do()
	if err != nil {
		return 0, fmt.Errorf("list versions: %w", err)
	}

	count := len(result.Files)
	if count > maxVersions {
		for _, f := range result.Files[maxVersions:] {
			if err := srv.Files.Delete(f.Id).Do(); err != nil {
				log.Printf("failed to delete old version %s: %v", f.Name, err)
			} else {
				log.Printf("pruned old version %s", f.Name)
			}
		}
		count = maxVersions
	}
	return count, nil
}

func syncPull(ctx context.Context, params syncPullParams) (map[string]any, error) {
	srv, err := getDriveService(ctx)
	if err != nil {
		return nil, err
	}

	syncPath := syncPathFromConfig(params.Config)

	folderID, err := resolvePathToFolderID(srv, syncPath)
	if err != nil {
		log.Printf("sync folder %q not found: %v", syncPath, err)
		return map[string]any{"status": "empty"}, nil
	}

	query := fmt.Sprintf("name = 'latest.json' and '%s' in parents and trashed = false", folderID)
	found, err := srv.Files.List().Q(query).Fields("files(id)").Do()
	if err != nil {
		return nil, fmt.Errorf("find latest.json: %w", err)
	}
	if len(found.Files) == 0 {
		return map[string]any{"status": "empty"}, nil
	}

	resp, err := srv.Files.Get(found.Files[0].Id).Download()
	if err != nil {
		return nil, fmt.Errorf("download latest.json: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read latest.json: %w", err)
	}

	return map[string]any{
		"status": "ok",
		"data":   string(data),
	}, nil
}

// --------------- IPC handlers ---------------

func handleInit() (any, *Error) {
	if err := loadConfig(); err != nil {
		log.Printf("WARNING: %v", err)
		return map[string]any{"status": "not_configured", "reason": err.Error()}, nil
	}

	// Try loading cached tokens (non-blocking). Auth happens via check_config + OAuth callback.
	tokenMu.Lock()
	cachedToken = loadTokens()
	tokenMu.Unlock()

	return map[string]any{"status": "ok"}, nil
}

func handleCheckConfig(params json.RawMessage) (any, *Error) {
	var p struct {
		Config      map[string]any `json:"config"`
		RedirectURI string         `json:"redirect_uri"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	// Check for valid cached token.
	tokenMu.Lock()
	if cachedToken == nil {
		cachedToken = loadTokens()
	}
	tok := cachedToken
	tokenMu.Unlock()

	if tok != nil && tok.Valid() {
		return map[string]any{"status": "ok"}, nil
	}

	// Try silent refresh if we have a refresh token.
	if tok != nil && tok.RefreshToken != "" {
		log.Println("refreshing Google token...")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		src := oauthConfig.TokenSource(ctx, tok)
		newTok, err := src.Token()
		if err == nil {
			tokenMu.Lock()
			cachedToken = newTok
			tokenMu.Unlock()
			saveToken(newTok)
			log.Printf("refreshed Google token (expires %s)", newTok.Expiry.Format(time.RFC3339))
			return map[string]any{"status": "ok"}, nil
		}
		log.Printf("refresh failed: %v, requesting consent", err)
	}

	// OAuth consent required. Build authorize URL.
	redirectURI := p.RedirectURI
	if redirectURI == "" {
		return nil, &Error{Code: "NO_REDIRECT_URI", Message: "redirect_uri is required for OAuth flow"}
	}

	state := randomState()

	tokenMu.Lock()
	oauthPending[state] = true
	tokenMu.Unlock()

	oauthConfig.RedirectURL = redirectURI
	authorizeURL := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	return map[string]any{
		"status":        "oauth_required",
		"authorize_url": authorizeURL,
		"state":         state,
	}, nil
}

func handleOAuthCallback(params json.RawMessage) (any, *Error) {
	var p struct {
		Code        string `json:"code"`
		State       string `json:"state"`
		RedirectURI string `json:"redirect_uri"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	// Verify the state token for CSRF protection.
	tokenMu.Lock()
	_, ok := oauthPending[p.State]
	if ok {
		delete(oauthPending, p.State)
	}
	tokenMu.Unlock()

	if !ok {
		return nil, &Error{Code: "INVALID_STATE", Message: "no pending OAuth flow for state: " + p.State}
	}

	// Exchange authorization code for tokens.
	oauthConfig.RedirectURL = p.RedirectURI
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tok, err := oauthConfig.Exchange(ctx, p.Code)
	if err != nil {
		return nil, &Error{Code: "TOKEN_EXCHANGE_FAILED", Message: err.Error()}
	}

	tokenMu.Lock()
	cachedToken = tok
	tokenMu.Unlock()
	saveToken(tok)

	log.Println("Google Drive OAuth callback complete")
	return map[string]any{"status": "ok"}, nil
}

// handleBrowse lists immediate child folders of the given path.
func handleBrowse(params json.RawMessage) (any, *Error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	srv, err := getDriveService(ctx)
	if err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	// Resolve the path to a folder ID.
	folderID, err := resolvePathToFolderID(srv, p.Path)
	if err != nil {
		return nil, &Error{Code: "RESOLVE_FAILED", Message: err.Error()}
	}

	// List immediate child folders only.
	query := fmt.Sprintf("'%s' in parents and mimeType = 'application/vnd.google-apps.folder' and trashed = false", folderID)
	result, err := srv.Files.List().Q(query).
		Fields("files(id, name)").
		OrderBy("name").
		PageSize(200).
		Do()
	if err != nil {
		return nil, &Error{Code: "LIST_FAILED", Message: err.Error()}
	}

	type folderEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	folders := make([]folderEntry, 0, len(result.Files))
	for _, f := range result.Files {
		entryPath := f.Name
		if p.Path != "" {
			entryPath = p.Path + "/" + f.Name
		}
		folders = append(folders, folderEntry{Name: f.Name, Path: entryPath})
	}

	return map[string]any{"folders": folders}, nil
}

func handleFileList(params json.RawMessage) (any, *Error) {
	// Read root_path from the IPC params (passed by the orchestrator from integration config).
	var p struct {
		Config map[string]any `json:"config"`
	}
	json.Unmarshal(params, &p)

	rootPath := ""
	if p.Config != nil {
		if v, ok := p.Config["root_path"].(string); ok {
			rootPath = v
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 540*time.Second)
	defer cancel()

	files, err := listFiles(ctx, rootPath)
	if err != nil {
		return nil, &Error{Code: "SCAN_FAILED", Message: err.Error()}
	}
	log.Printf("listed %d files from Drive (root_path=%q)", len(files), rootPath)
	return map[string]any{"files": files}, nil
}

func handleSyncPush(params json.RawMessage) (any, *Error) {
	var sp syncPushParams
	if err := json.Unmarshal(params, &sp); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	if sp.Data == "" {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "data is required"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := syncPush(ctx, sp)
	if err != nil {
		return nil, &Error{Code: "SYNC_PUSH_FAILED", Message: err.Error()}
	}
	return result, nil
}

func handleSyncPull(params json.RawMessage) (any, *Error) {
	var sp syncPullParams
	if err := json.Unmarshal(params, &sp); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := syncPull(ctx, sp)
	if err != nil {
		return nil, &Error{Code: "SYNC_PULL_FAILED", Message: err.Error()}
	}
	return result, nil
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
				"plugin_version": "2.1.0",
				"capabilities":   []string{"source", "sync"},
			}

		case "plugin.check_config":
			result, errObj := handleCheckConfig(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "auth.oauth.callback":
			result, errObj := handleOAuthCallback(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "source.browse":
			result, errObj := handleBrowse(req.Params)
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

		case "sync.push":
			result, errObj := handleSyncPush(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "sync.pull":
			result, errObj := handleSyncPull(req.Params)
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
