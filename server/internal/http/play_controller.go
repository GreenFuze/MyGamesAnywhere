package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/hirochachacha/go-smb2"
)

// ServePlayFile streams one launchable/source-owned file for a canonical game
// (GET /api/games/{id}/play?file_id=...).
func (c *GameController) ServePlayFile(w http.ResponseWriter, r *http.Request) {
	id, err := decodedPathParam(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	fileID := r.URL.Query().Get("file_id")
	sourceGameID, filePath, err := decodeGameFileID(fileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	game, err := c.gameStore.GetCanonicalGameByID(r.Context(), id)
	if err != nil {
		c.logger.Error("get game for play", err, "id", id, "file_id", fileID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}

	sourceGame, gameFile := findSourceGameFile(game, sourceGameID, filePath)
	if sourceGame == nil || gameFile == nil {
		http.NotFound(w, r)
		return
	}

	playFile, contentName, err := c.openPlayFile(r.Context(), sourceGame, gameFile)
	if err != nil {
		c.logger.Warn("open play file rejected", "game_id", id, "source_game_id", sourceGameID, "path", filePath, "err", err.Error())
		http.NotFound(w, r)
		return
	}
	defer playFile.Close()

	st, err := playFile.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}

	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(gameFile.Path))); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, contentName, st.ModTime(), playFile)
}

type playReadSeekCloser interface {
	io.ReadSeeker
	io.Closer
	Stat() (os.FileInfo, error)
}

type smbPlayConfig struct {
	Host     string `json:"host"`
	Share    string `json:"share"`
	Username string `json:"username"`
	Password string `json:"password"`
	Path     string `json:"path"`
}

type smbPlayFile struct {
	file   *smb2.File
	share  *smb2.Share
	conn   net.Conn
	logoff func() error
}

func (f *smbPlayFile) Read(p []byte) (int, error) {
	return f.file.Read(p)
}

func (f *smbPlayFile) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence)
}

func (f *smbPlayFile) Stat() (os.FileInfo, error) {
	return f.file.Stat()
}

func (f *smbPlayFile) Close() error {
	var firstErr error
	closeWithFirst := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if f.file != nil {
		closeWithFirst(f.file.Close())
	}
	if f.share != nil {
		closeWithFirst(f.share.Umount())
	}
	if f.logoff != nil {
		closeWithFirst(f.logoff())
	}
	if f.conn != nil {
		closeWithFirst(f.conn.Close())
	}
	return firstErr
}

func (c *GameController) openPlayFile(
	ctx context.Context,
	sourceGame *core.SourceGame,
	gameFile *core.GameFile,
) (playReadSeekCloser, string, error) {
	if sourceGame == nil || gameFile == nil {
		return nil, "", fmt.Errorf("missing source game or file")
	}

	if sourceGame.PluginID == "game-source-smb" {
		return c.openSMBPlayFile(ctx, sourceGame, gameFile)
	}

	fullAbs, err := resolveUnderGameRoot(sourceGame.RootPath, gameFile.Path)
	if err != nil {
		return nil, "", err
	}

	f, err := os.Open(fullAbs)
	if err != nil {
		return nil, "", err
	}
	return f, filepath.Base(gameFile.Path), nil
}

func (c *GameController) openSMBPlayFile(
	ctx context.Context,
	sourceGame *core.SourceGame,
	gameFile *core.GameFile,
) (playReadSeekCloser, string, error) {
	if c.integrationRepo == nil {
		return nil, "", fmt.Errorf("integration repository unavailable")
	}
	integration, err := c.integrationRepo.GetByID(ctx, sourceGame.IntegrationID)
	if err != nil || integration == nil {
		return nil, "", fmt.Errorf("integration not found")
	}

	var cfg smbPlayConfig
	if err := json.Unmarshal([]byte(integration.ConfigJSON), &cfg); err != nil {
		return nil, "", fmt.Errorf("invalid smb config: %w", err)
	}
	if strings.TrimSpace(cfg.Host) == "" || strings.TrimSpace(cfg.Share) == "" {
		return nil, "", fmt.Errorf("incomplete smb config")
	}

	sharePath, err := resolveSMBSharePath(cfg.Path, gameFile.Path)
	if err != nil {
		return nil, "", err
	}

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:445", cfg.Host))
	if err != nil {
		return nil, "", err
	}

	dialer := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     cfg.Username,
			Password: cfg.Password,
		},
	}
	session, err := dialer.Dial(conn)
	if err != nil {
		_ = conn.Close()
		return nil, "", err
	}

	share, err := session.Mount(cfg.Share)
	if err != nil {
		_ = session.Logoff()
		_ = conn.Close()
		return nil, "", err
	}

	file, err := share.Open(sharePath)
	if err != nil {
		_ = share.Umount()
		_ = session.Logoff()
		_ = conn.Close()
		return nil, "", err
	}

	return &smbPlayFile{
		file:   file,
		share:  share,
		conn:   conn,
		logoff: session.Logoff,
	}, filepath.Base(gameFile.Path), nil
}

func findSourceGameFile(
	game *core.CanonicalGame,
	sourceGameID string,
	filePath string,
) (*core.SourceGame, *core.GameFile) {
	if game == nil {
		return nil, nil
	}
	normalizedPath := filepath.ToSlash(strings.TrimSpace(filePath))
	for _, sg := range game.SourceGames {
		if sg == nil || sg.ID != sourceGameID || sg.Status != "found" {
			continue
		}
		for i := range sg.Files {
			if filepath.ToSlash(sg.Files[i].Path) == normalizedPath {
				return sg, &sg.Files[i]
			}
		}
		return sg, nil
	}
	return nil, nil
}

func resolveUnderGameRoot(rootPath, relativePath string) (string, error) {
	root := strings.TrimSpace(rootPath)
	if root == "" {
		return "", fmt.Errorf("empty root path")
	}
	if !filepath.IsAbs(root) {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return "", err
		}
		root = absRoot
	}
	root = filepath.Clean(root)

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

	full := filepath.Join(root, filepath.Clean(path))
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, fullAbs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("outside game root")
	}
	return fullAbs, nil
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
