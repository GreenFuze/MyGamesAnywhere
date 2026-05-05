package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
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

var errDrivePathNotFound = errors.New("drive path not found")

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

func tokenFromConfig(config map[string]any) *oauth2.Token {
	raw, ok := config["tokens"]
	if !ok {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var st savedTokens
	if err := json.Unmarshal(data, &st); err != nil {
		return nil
	}
	if st.AccessToken == "" && st.RefreshToken == "" {
		return nil
	}
	return &oauth2.Token{
		AccessToken:  st.AccessToken,
		RefreshToken: st.RefreshToken,
		TokenType:    st.TokenType,
		Expiry:       st.Expiry,
	}
}

func setCachedToken(tok *oauth2.Token) {
	tokenMu.Lock()
	cachedToken = tok
	tokenMu.Unlock()
}

func tokenConfigUpdates(tok *oauth2.Token) map[string]any {
	if tok == nil || (tok.AccessToken == "" && tok.RefreshToken == "") {
		return nil
	}
	return map[string]any{
		"tokens": savedTokens{
			AccessToken:  tok.AccessToken,
			RefreshToken: tok.RefreshToken,
			TokenType:    tok.TokenType,
			Expiry:       tok.Expiry,
		},
	}
}

func driveAuthOKResponse(ctx context.Context, tok *oauth2.Token) map[string]any {
	setCachedToken(tok)
	result := map[string]any{"status": "ok"}
	if updates := tokenConfigUpdates(tok); updates != nil {
		result["config_updates"] = updates
	}
	identity, err := driveSourceIdentity(ctx)
	if err == nil {
		result["source_identity"] = identity
	}
	return result
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
			return "", fmt.Errorf("%w: folder %q not found under %s", errDrivePathNotFound, part, currentID)
		}
		currentID = result.Files[0].Id
	}

	return currentID, nil
}

func resolvePathToObjectID(srv *drive.Service, rootPath string) (string, error) {
	rootPath = sourcescope.NormalizeLogicalPath(rootPath)
	if rootPath == "" {
		return "", fmt.Errorf("path is required")
	}

	parts := strings.Split(rootPath, "/")
	currentID := "root"

	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		query := fmt.Sprintf("'%s' in parents and name = '%s' and trashed = false",
			currentID, strings.ReplaceAll(part, "'", "\\'"))
		result, err := srv.Files.List().Q(query).Fields("files(id, name, mimeType)").PageSize(50).Do()
		if err != nil {
			return "", fmt.Errorf("find object %q: %w", part, err)
		}
		if len(result.Files) == 0 {
			return "", fmt.Errorf("path component %q not found under %s", part, currentID)
		}

		if index == len(parts)-1 {
			return result.Files[0].Id, nil
		}

		nextID := ""
		for _, file := range result.Files {
			if file.MimeType == "application/vnd.google-apps.folder" {
				nextID = file.Id
				break
			}
		}
		if nextID == "" {
			return "", fmt.Errorf("path component %q is not a folder", part)
		}
		currentID = nextID
	}

	return "", fmt.Errorf("path %q could not be resolved", rootPath)
}

// --------------- File listing (source.filesystem.list) ---------------

func listFiles(ctx context.Context, includes []sourcescope.IncludePath, excludes []string) ([]map[string]any, error) {
	srv, err := getDriveService(ctx)
	if err != nil {
		return nil, err
	}

	type folderItem struct {
		id   string
		path string
	}

	seen := make(map[string]map[string]any)
	for _, include := range includes {
		folderID, err := resolvePathToFolderID(srv, include.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve root path %q: %w", include.Path, err)
		}
		log.Printf("scanning Drive folder ID %s (path=%q recursive=%t)", folderID, include.Path, include.Recursive)

		queue := []folderItem{{id: folderID, path: include.Path}}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]

			query := fmt.Sprintf("'%s' in parents and trashed = false", current.id)
			pageToken := ""
			for {
				call := srv.Files.List().Q(query).
					Fields("nextPageToken, files(id, name, mimeType, size, modifiedTime)").
					OrderBy("folder,name").
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
					entryPath = sourcescope.NormalizeLogicalPath(entryPath)
					if drivePathExcluded(entryPath, excludes) {
						continue
					}

					entry := map[string]any{
						"path":      entryPath,
						"name":      f.Name,
						"is_dir":    isDir,
						"size":      f.Size,
						"object_id": f.Id,
					}
					if f.ModifiedTime != "" {
						entry["mod_time"] = f.ModifiedTime
						entry["revision"] = fmt.Sprintf("%s:%d", f.ModifiedTime, f.Size)
					}

					seen[entryPath] = entry

					if isDir && include.Recursive {
						queue = append(queue, folderItem{id: f.Id, path: entryPath})
					}
				}

				pageToken = result.NextPageToken
				if pageToken == "" {
					break
				}
			}
		}
	}

	paths := make([]string, 0, len(seen))
	for logicalPath := range seen {
		paths = append(paths, logicalPath)
	}
	sort.Strings(paths)
	files := make([]map[string]any, 0, len(paths))
	for _, logicalPath := range paths {
		files = append(files, seen[logicalPath])
	}
	return files, nil
}

// --------------- Settings sync (sync.push / sync.pull) ---------------

type syncPushParams struct {
	Data   string         `json:"data"`
	Config map[string]any `json:"config"`
}

type syncPullParams struct {
	Config      map[string]any `json:"config"`
	PayloadID   string         `json:"payload_id,omitempty"`
	PayloadName string         `json:"payload_name,omitempty"`
}

type syncUpdatePayloadParams struct {
	Config map[string]any `json:"config"`
	ID     string         `json:"id"`
	Data   string         `json:"data"`
}

type saveSyncListParams struct {
	Config map[string]any `json:"config"`
	Prefix string         `json:"prefix"`
}

type saveSyncPathParams struct {
	Config map[string]any `json:"config"`
	Path   string         `json:"path"`
}

type saveSyncPutParams struct {
	Config      map[string]any `json:"config"`
	Path        string         `json:"path"`
	DataBase64  string         `json:"data_base64"`
	ContentType string         `json:"content_type"`
}

func syncPathFromConfig(cfg map[string]any) string {
	if v, ok := cfg["sync_path"].(string); ok && v != "" {
		return v
	}
	return "Games/mga_sync"
}

func saveSyncRootPathFromConfig(cfg map[string]any) string {
	if v, ok := cfg["root_path"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.Trim(strings.TrimSpace(v), "/")
	}
	return "Games/mga_save_syncs"
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

	fileID := strings.TrimSpace(params.PayloadID)
	fileName := strings.TrimSpace(params.PayloadName)
	if fileID == "" {
		if fileName == "" {
			fileName = "latest.json"
		}
		query := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", strings.ReplaceAll(fileName, "'", "\\'"), folderID)
		found, err := srv.Files.List().Q(query).Fields("files(id)").Do()
		if err != nil {
			return nil, fmt.Errorf("find %s: %w", fileName, err)
		}
		if len(found.Files) == 0 {
			return map[string]any{"status": "empty"}, nil
		}
		fileID = found.Files[0].Id
	}

	resp, err := srv.Files.Get(fileID).Download()
	if err != nil {
		if fileName == "" {
			fileName = fileID
		}
		return nil, fmt.Errorf("download %s: %w", fileName, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		if fileName == "" {
			fileName = fileID
		}
		return nil, fmt.Errorf("read %s: %w", fileName, err)
	}

	return map[string]any{
		"status": "ok",
		"data":   string(data),
	}, nil
}

func syncListPayloads(ctx context.Context, params syncPullParams) (map[string]any, error) {
	srv, err := getDriveService(ctx)
	if err != nil {
		return nil, err
	}

	syncPath := syncPathFromConfig(params.Config)
	folderID, err := resolvePathToFolderID(srv, syncPath)
	if err != nil {
		log.Printf("sync folder %q not found: %v", syncPath, err)
		return map[string]any{"status": "ok", "payloads": []map[string]any{}}, nil
	}

	query := fmt.Sprintf("(name = 'latest.json' or (name contains 'mga_sync_' and name contains '.json')) and '%s' in parents and trashed = false", folderID)
	result, err := srv.Files.List().Q(query).Fields("files(id, name, modifiedTime)").OrderBy("modifiedTime desc").PageSize(200).Do()
	if err != nil {
		return nil, fmt.Errorf("list sync payloads: %w", err)
	}

	payloads := make([]map[string]any, 0, len(result.Files))
	for _, file := range result.Files {
		resp, err := srv.Files.Get(file.Id).Download()
		if err != nil {
			return nil, fmt.Errorf("download %s: %w", file.Name, err)
		}
		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", file.Name, readErr)
		}
		payloads = append(payloads, map[string]any{
			"id":   file.Id,
			"name": file.Name,
			"data": string(data),
		})
	}
	return map[string]any{"status": "ok", "payloads": payloads}, nil
}

func syncUpdatePayload(ctx context.Context, params syncUpdatePayloadParams) (map[string]any, error) {
	if strings.TrimSpace(params.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}
	if strings.TrimSpace(params.Data) == "" {
		return nil, fmt.Errorf("data is required")
	}
	srv, err := getDriveService(ctx)
	if err != nil {
		return nil, err
	}
	reader := strings.NewReader(params.Data)
	if _, err := srv.Files.Update(params.ID, nil).Media(reader).Do(); err != nil {
		return nil, fmt.Errorf("update payload: %w", err)
	}
	return map[string]any{"status": "ok"}, nil
}

func normalizeObjectPath(p string) (string, error) {
	cleaned := path.Clean(strings.ReplaceAll(strings.TrimSpace(p), "\\", "/"))
	if cleaned == "." || cleaned == "/" || cleaned == "" {
		return "", fmt.Errorf("path is required")
	}
	cleaned = strings.TrimPrefix(cleaned, "/")
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("invalid path")
	}
	return cleaned, nil
}

func resolveObjectFile(srv *drive.Service, rootPath, objectPath string) (string, string, error) {
	normalized, err := normalizeObjectPath(objectPath)
	if err != nil {
		return "", "", err
	}
	parentDir := path.Dir(normalized)
	if parentDir == "." {
		parentDir = ""
	}
	fileName := path.Base(normalized)

	rootFolderID, err := resolvePathToFolderID(srv, rootPath)
	if err != nil {
		if errors.Is(err, errDrivePathNotFound) {
			return "", fileName, nil
		}
		return "", "", err
	}
	parentID := rootFolderID
	if parentDir != "" {
		parentID, err = resolvePathToFolderID(srv, strings.Trim(path.Join(rootPath, parentDir), "/"))
		if err != nil {
			if errors.Is(err, errDrivePathNotFound) {
				return "", fileName, nil
			}
			return "", fileName, err
		}
	}
	query := fmt.Sprintf("'%s' in parents and name = '%s' and trashed = false", parentID, strings.ReplaceAll(fileName, "'", "\\'"))
	result, err := srv.Files.List().Q(query).Fields("files(id, name, size, modifiedTime)").PageSize(1).Do()
	if err != nil {
		return "", fileName, err
	}
	if len(result.Files) == 0 {
		return "", fileName, nil
	}
	return result.Files[0].Id, fileName, nil
}

func handleSaveSyncList(params json.RawMessage) (any, *Error) {
	var p saveSyncListParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	srv, err := getDriveService(ctx)
	if err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	rootPath := saveSyncRootPathFromConfig(p.Config)
	searchPath := strings.Trim(path.Join(rootPath, strings.TrimPrefix(strings.ReplaceAll(p.Prefix, "\\", "/"), "/")), "/")
	folderID, err := resolvePathToFolderID(srv, searchPath)
	if err != nil {
		return map[string]any{"status": "ok", "files": []map[string]any{}}, nil
	}

	type folderItem struct {
		id   string
		path string
	}
	queue := []folderItem{{id: folderID, path: strings.Trim(strings.ReplaceAll(p.Prefix, "\\", "/"), "/")}}
	files := []map[string]any{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		query := fmt.Sprintf("'%s' in parents and trashed = false", current.id)
		result, err := srv.Files.List().Q(query).Fields("files(id, name, mimeType, size, modifiedTime)").PageSize(1000).Do()
		if err != nil {
			return nil, &Error{Code: "LIST_FAILED", Message: err.Error()}
		}
		for _, f := range result.Files {
			entryPath := f.Name
			if current.path != "" {
				entryPath = current.path + "/" + f.Name
			}
			if f.MimeType == "application/vnd.google-apps.folder" {
				queue = append(queue, folderItem{id: f.Id, path: entryPath})
				continue
			}
			files = append(files, map[string]any{
				"path":     entryPath,
				"size":     f.Size,
				"mod_time": f.ModifiedTime,
			})
		}
	}
	return map[string]any{"status": "ok", "files": files}, nil
}

func handleSaveSyncGet(params json.RawMessage) (any, *Error) {
	var p saveSyncPathParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	srv, err := getDriveService(ctx)
	if err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	rootPath := saveSyncRootPathFromConfig(p.Config)
	fileID, _, err := resolveObjectFile(srv, rootPath, p.Path)
	if err != nil {
		return nil, &Error{Code: "LOOKUP_FAILED", Message: err.Error()}
	}
	if fileID == "" {
		return map[string]any{"status": "not_found"}, nil
	}

	resp, err := srv.Files.Get(fileID).Download()
	if err != nil {
		return nil, &Error{Code: "DOWNLOAD_FAILED", Message: err.Error()}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
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
	data, err := base64.StdEncoding.DecodeString(p.DataBase64)
	if err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	normalized, err := normalizeObjectPath(p.Path)
	if err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	srv, err := getDriveService(ctx)
	if err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	rootPath := saveSyncRootPathFromConfig(p.Config)
	parentPath := path.Dir(normalized)
	if parentPath == "." {
		parentPath = ""
	}
	parentID, err := ensureFolderPath(srv, strings.Trim(path.Join(rootPath, parentPath), "/"))
	if err != nil {
		return nil, &Error{Code: "CREATE_PATH_FAILED", Message: err.Error()}
	}
	fileName := path.Base(normalized)

	existingID, _, err := resolveObjectFile(srv, rootPath, normalized)
	if err != nil {
		return nil, &Error{Code: "LOOKUP_FAILED", Message: err.Error()}
	}

	reader := bytes.NewReader(data)
	if existingID != "" {
		_, err = srv.Files.Update(existingID, nil).Media(reader).Do()
	} else {
		meta := &drive.File{Name: fileName, Parents: []string{parentID}}
		if p.ContentType != "" {
			meta.MimeType = p.ContentType
		}
		_, err = srv.Files.Create(meta).Media(reader).Do()
	}
	if err != nil {
		return nil, &Error{Code: "UPLOAD_FAILED", Message: err.Error()}
	}
	return map[string]any{"status": "ok"}, nil
}

func handleSaveSyncDelete(params json.RawMessage) (any, *Error) {
	var p saveSyncPathParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	srv, err := getDriveService(ctx)
	if err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	rootPath := saveSyncRootPathFromConfig(p.Config)
	fileID, _, err := resolveObjectFile(srv, rootPath, p.Path)
	if err != nil {
		return nil, &Error{Code: "LOOKUP_FAILED", Message: err.Error()}
	}
	if fileID == "" {
		return map[string]any{"status": "not_found"}, nil
	}
	if err := srv.Files.Delete(fileID).Do(); err != nil {
		return nil, &Error{Code: "DELETE_FAILED", Message: err.Error()}
	}
	return map[string]any{"status": "ok"}, nil
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

	configToken := tokenFromConfig(p.Config)
	if configToken != nil {
		setCachedToken(configToken)
	}

	// Check for valid profile-owned or cached token.
	tokenMu.Lock()
	if cachedToken == nil {
		cachedToken = loadTokens()
	}
	tok := cachedToken
	tokenMu.Unlock()

	if tok != nil && tok.Valid() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return driveAuthOKResponse(ctx, tok), nil
	}

	// Try silent refresh if we have a refresh token.
	if tok != nil && tok.RefreshToken != "" {
		log.Println("refreshing Google token...")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		src := oauthConfig.TokenSource(ctx, tok)
		newTok, err := src.Token()
		if err == nil {
			setCachedToken(newTok)
			saveToken(newTok)
			log.Printf("refreshed Google token (expires %s)", newTok.Expiry.Format(time.RFC3339))
			return driveAuthOKResponse(ctx, newTok), nil
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

func driveSourceIdentity(ctx context.Context) (string, error) {
	srv, err := getDriveService(ctx)
	if err != nil {
		return "", err
	}
	about, err := srv.About.Get().Fields("user(emailAddress,permissionId)").Do()
	if err != nil {
		return "", err
	}
	if about.User == nil {
		return "", fmt.Errorf("drive account identity unavailable")
	}
	if about.User.PermissionId != "" {
		return "gdrive:" + about.User.PermissionId, nil
	}
	if about.User.EmailAddress != "" {
		return "gdrive:" + strings.ToLower(strings.TrimSpace(about.User.EmailAddress)), nil
	}
	return "", fmt.Errorf("drive account identity unavailable")
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

	setCachedToken(tok)
	saveToken(tok)

	log.Println("Google Drive OAuth callback complete")
	return driveAuthOKResponse(ctx, tok), nil
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
	var config map[string]any
	if err := json.Unmarshal(params, &config); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	if nestedConfig, ok := config["config"].(map[string]any); ok {
		config = nestedConfig
	}
	includes := filesystemIncludePathsFromConfig(config)
	excludes := filesystemExcludePathsFromConfig(config)

	ctx, cancel := context.WithTimeout(context.Background(), 540*time.Second)
	defer cancel()

	files, err := listFiles(ctx, includes, excludes)
	if err != nil {
		return nil, &Error{Code: "SCAN_FAILED", Message: err.Error()}
	}
	log.Printf("listed %d files from Drive (include_paths=%d)", len(files), len(includes))
	return map[string]any{"files": files}, nil
}

func filesystemIncludePathsFromConfig(config map[string]any) []sourcescope.IncludePath {
	normalized := sourcescope.NormalizeConfig("game-source-google-drive", config)
	return sourcescope.ReadIncludePaths("game-source-google-drive", normalized)
}

func filesystemExcludePathsFromConfig(config map[string]any) []string {
	raw, ok := config["exclude_paths"]
	if !ok {
		return nil
	}
	var values []string
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok {
				if normalized := sourcescope.NormalizeLogicalPath(s); normalized != "" {
					values = append(values, normalized)
				}
			}
		}
	case []string:
		for _, item := range typed {
			if normalized := sourcescope.NormalizeLogicalPath(item); normalized != "" {
				values = append(values, normalized)
			}
		}
	}
	return values
}

func drivePathExcluded(logicalPath string, excludes []string) bool {
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

func handleFileMaterialize(params json.RawMessage) (any, *Error) {
	var p struct {
		Config   map[string]any `json:"config"`
		Path     string         `json:"path"`
		ObjectID string         `json:"object_id"`
		Revision string         `json:"revision"`
		Profile  string         `json:"profile"`
		DestPath string         `json:"dest_path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	if strings.TrimSpace(p.ObjectID) == "" {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "object_id is required"}
	}
	if strings.TrimSpace(p.DestPath) == "" {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "dest_path is required"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	srv, err := getDriveService(ctx)
	if err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	if err := os.MkdirAll(filepath.Dir(p.DestPath), 0o755); err != nil {
		return nil, &Error{Code: "WRITE_FAILED", Message: err.Error()}
	}

	resp, err := srv.Files.Get(p.ObjectID).Fields("id, modifiedTime, size").Download()
	if err != nil {
		return nil, &Error{Code: "DOWNLOAD_FAILED", Message: err.Error()}
	}
	defer resp.Body.Close()

	out, err := os.Create(p.DestPath)
	if err != nil {
		return nil, &Error{Code: "WRITE_FAILED", Message: err.Error()}
	}
	defer out.Close()

	size, err := io.Copy(out, resp.Body)
	if err != nil {
		return nil, &Error{Code: "WRITE_FAILED", Message: err.Error()}
	}

	meta, err := srv.Files.Get(p.ObjectID).Fields("modifiedTime, size").Do()
	if err != nil {
		return map[string]any{"size": size}, nil
	}

	result := map[string]any{"size": size}
	if meta.ModifiedTime != "" {
		result["mod_time"] = meta.ModifiedTime
		result["revision"] = fmt.Sprintf("%s:%d", meta.ModifiedTime, meta.Size)
	}
	return result, nil
}

type sourceDeleteFile struct {
	Path     string `json:"path"`
	IsDir    bool   `json:"is_dir"`
	ObjectID string `json:"object_id"`
	Size     int64  `json:"size"`
}

type sourceDeletePlanItem struct {
	Path     string `json:"path"`
	ObjectID string `json:"object_id,omitempty"`
	IsDir    bool   `json:"is_dir,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Action   string `json:"action"`
}

func handleSourceDelete(params json.RawMessage) (any, *Error) {
	var body struct {
		Config       map[string]any     `json:"config"`
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
	if !sourcescope.ScopeContainsRootPath(rootPath, filesystemIncludePathsFromConfig(body.Config)) {
		return nil, &Error{Code: "NOT_ALLOWED", Message: "root_path is outside the configured include_paths scope"}
	}
	if len(body.Files) == 0 {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "files are required"}
	}

	var srv *drive.Service
	if !body.DryRun || sourceDeletePlanNeedsResolution(body.Files) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		var err error
		srv, err = getDriveService(ctx)
		if err != nil {
			return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
		}
	}
	items, errObj := buildSourceDeletePlan(srv, rootPath, body.Config, body.Files)
	if errObj != nil {
		return nil, errObj
	}
	if body.DryRun {
		return sourceDeleteResponse(body.SourceGameID, "game-source-google-drive", "trash", items, 0), nil
	}

	for _, item := range items {
		objectID := strings.TrimSpace(item.ObjectID)
		if _, err := srv.Files.Update(objectID, &drive.File{Trashed: true}).SupportsAllDrives(true).Fields("id,trashed").Do(); err != nil {
			return nil, &Error{Code: "DELETE_FAILED", Message: err.Error()}
		}
	}
	return sourceDeleteResponse(body.SourceGameID, "game-source-google-drive", "trash", items, len(items)), nil
}

func buildSourceDeletePlan(srv *drive.Service, rootPath string, config map[string]any, files []sourceDeleteFile) ([]sourceDeletePlanItem, *Error) {
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
		if !sourcescope.ScopeContainsRootPath(filePath, filesystemIncludePathsFromConfig(config)) {
			return nil, &Error{Code: "NOT_ALLOWED", Message: fmt.Sprintf("file %q is outside the configured include_paths scope", filePath)}
		}

		objectID := strings.TrimSpace(file.ObjectID)
		if objectID == "" {
			if srv == nil {
				return nil, &Error{Code: "AUTH_FAILED", Message: "drive service is required to resolve file paths"}
			}
			var err error
			objectID, err = resolvePathToObjectID(srv, filePath)
			if err != nil {
				return nil, &Error{Code: "DELETE_FAILED", Message: err.Error()}
			}
		}
		items = append(items, sourceDeletePlanItem{
			Path:     filePath,
			ObjectID: objectID,
			Size:     file.Size,
			Action:   "trash",
		})
	}
	return items, nil
}

func sourceDeletePlanNeedsResolution(files []sourceDeleteFile) bool {
	for _, file := range files {
		if strings.TrimSpace(file.ObjectID) == "" {
			return true
		}
	}
	return false
}

func sourceDeleteResponse(sourceGameID, pluginID, action string, items []sourceDeletePlanItem, deletedCount int) map[string]any {
	return map[string]any{
		"source_game_id": sourceGameID,
		"plugin_id":      pluginID,
		"action":         action,
		"summary":        fmt.Sprintf("%d file(s) will be moved to Google Drive trash.", len(items)),
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

		case "source.filesystem.delete":
			result, errObj := handleSourceDelete(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "source.file.materialize":
			result, errObj := handleFileMaterialize(req.Params)
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

		case "sync.list_payloads":
			var p syncPullParams
			if err := json.Unmarshal(req.Params, &p); err != nil {
				resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
			} else if result, err := syncListPayloads(context.Background(), p); err != nil {
				resp.Error = &Error{Code: "SYNC_LIST_FAILED", Message: err.Error()}
			} else {
				resp.Result = result
			}

		case "sync.update_payload":
			var p syncUpdatePayloadParams
			if err := json.Unmarshal(req.Params, &p); err != nil {
				resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
			} else if result, err := syncUpdatePayload(context.Background(), p); err != nil {
				resp.Error = &Error{Code: "SYNC_UPDATE_FAILED", Message: err.Error()}
			} else {
				resp.Result = result
			}

		case "save_sync.list":
			result, errObj := handleSaveSyncList(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "save_sync.get":
			result, errObj := handleSaveSyncGet(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "save_sync.put":
			result, errObj := handleSaveSyncPut(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "save_sync.delete":
			result, errObj := handleSaveSyncDelete(req.Params)
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
