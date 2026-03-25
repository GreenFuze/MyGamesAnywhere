package http

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
)

const defaultMediaRoot = "media"

// MediaController serves downloaded media files from the local MEDIA_ROOT directory.
type MediaController struct {
	store  core.GameStore
	config core.Configuration
	logger core.Logger
}

func NewMediaController(store core.GameStore, config core.Configuration, logger core.Logger) *MediaController {
	return &MediaController{store: store, config: config, logger: logger}
}

func (c *MediaController) mediaRootAbs() (string, error) {
	root := c.config.Get("MEDIA_ROOT")
	if root == "" {
		root = defaultMediaRoot
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root), nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(wd, root))
}

// ServeMedia streams a file from media_assets.local_path under MEDIA_ROOT (GET /api/media/{assetID}).
func (c *MediaController) ServeMedia(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "assetID")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.Error(w, "invalid asset id", http.StatusBadRequest)
		return
	}
	asset, err := c.store.GetMediaAssetByID(r.Context(), id)
	if err != nil {
		c.logger.Error("get media asset", err, "id", id)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if asset == nil || asset.LocalPath == "" {
		http.NotFound(w, r)
		return
	}

	rootAbs, err := c.mediaRootAbs()
	if err != nil {
		c.logger.Error("media root", err)
		http.Error(w, "media root unavailable", http.StatusInternalServerError)
		return
	}
	fullAbs, err := resolveUnderMediaRoot(rootAbs, asset.LocalPath)
	if err != nil {
		c.logger.Warn("rejected media path", "id", id, "local_path", asset.LocalPath, "err", err.Error())
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(fullAbs)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		c.logger.Error("open media file", err, "path", fullAbs)
		http.Error(w, "failed to open file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	if asset.MimeType != "" {
		w.Header().Set("Content-Type", asset.MimeType)
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

func resolveUnderMediaRoot(mediaRootAbs, localPath string) (string, error) {
	lp := strings.TrimSpace(localPath)
	if lp == "" {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(lp) {
		return "", fmt.Errorf("absolute local_path not allowed")
	}
	if strings.Contains(lp, "..") {
		return "", fmt.Errorf("path traversal")
	}
	root := filepath.Clean(mediaRootAbs)
	full := filepath.Join(root, filepath.Clean(lp))
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, fullAbs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("outside media root")
	}
	return fullAbs, nil
}
