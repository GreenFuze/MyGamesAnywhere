package http

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
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

	fullAbs, err := resolveUnderGameRoot(sourceGame.RootPath, gameFile.Path)
	if err != nil {
		c.logger.Warn("rejected play path", "game_id", id, "source_game_id", sourceGameID, "path", filePath, "err", err.Error())
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(fullAbs)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		c.logger.Error("open play file", err, "path", fullAbs)
		http.Error(w, "failed to open file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}

	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(gameFile.Path))); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, filepath.Base(gameFile.Path), st.ModTime(), f)
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
