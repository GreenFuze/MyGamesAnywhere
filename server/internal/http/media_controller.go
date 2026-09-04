package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	store    core.GameStore
	config   core.Configuration
	logger   core.Logger
	mediaSvc core.MediaDownloadService
}

func NewMediaController(store core.GameStore, config core.Configuration, logger core.Logger, mediaSvc core.MediaDownloadService) *MediaController {
	return &MediaController{store: store, config: config, logger: logger, mediaSvc: mediaSvc}
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
	if asset == nil {
		http.NotFound(w, r)
		return
	}
	if asset.LocalPath == "" {
		if redirectToOriginalMedia(w, r, asset.URL) {
			return
		}
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
			c.handleMissingLocalFile(r, id, fullAbs)
			if redirectToOriginalMedia(w, r, asset.URL) {
				return
			}
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
	// A frontend caches thousands of covers and revalidates them on every
	// refresh. Without an ETag the only validator was Last-Modified, so a client
	// sending If-None-Match got a full redownload of a byte-identical file.
	// http.ServeContent answers the conditional request itself once the header
	// is set, and honours it for Range and If-Range too.
	w.Header().Set("ETag", mediaETag(asset, st))
	// Private, not public: this asset is served only to an authorized profile,
	// so a shared cache must not be allowed to hand it to a different one.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

// mediaETag returns a validator a client can revalidate against.
//
// The stored hash is a SHA-256 of the downloaded bytes, written in the same
// update as the local path, so it is a strong validator: equal hash means equal
// file. Rows whose download never recorded one fall back to a weak size and
// modification-time validator rather than to no validator at all — one missing
// checksum should not cost a client every byte of its artwork again.
func mediaETag(asset *core.MediaAsset, info os.FileInfo) string {
	if hash := strings.TrimSpace(asset.Hash); hash != "" {
		return strconv.Quote(hash)
	}
	return fmt.Sprintf("W/%s", strconv.Quote(fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano())))
}

// redirectToOriginalMedia keeps artwork usable while MGA repairs a missing
// local cache entry. Restricting the target to absolute HTTP(S) URLs prevents
// the media endpoint from becoming an open redirect for arbitrary schemes.
func redirectToOriginalMedia(w http.ResponseWriter, r *http.Request, rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, parsed.String(), http.StatusTemporaryRedirect)
	return true
}

func (c *MediaController) QueueStatus(w http.ResponseWriter, r *http.Request) {
	status, err := c.mediaSvc.Status(r.Context())
	if err != nil {
		c.logger.Error("media queue status", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(status)
}

func (c *MediaController) RetryFailed(w http.ResponseWriter, r *http.Request) {
	status, err := c.mediaSvc.RetryFailed(r.Context())
	if err != nil {
		c.logger.Error("retry failed media downloads", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(status)
}

func (c *MediaController) ClearCache(w http.ResponseWriter, r *http.Request) {
	status, err := c.mediaSvc.ClearCache(r.Context())
	if err != nil {
		c.logger.Error("clear media cache", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(status)
}

func (c *MediaController) handleMissingLocalFile(r *http.Request, assetID int, fullAbs string) {
	if c.mediaSvc == nil {
		return
	}
	if err := c.mediaSvc.MarkLocalFileMissing(r.Context(), assetID); err != nil {
		c.logger.Warn("reset missing media asset state failed", "asset_id", assetID, "path", fullAbs, "error", err)
		return
	}
	c.logger.Warn("media file missing; queued redownload", "asset_id", assetID, "path", fullAbs)
}

type updateMediaMetadataRequest struct {
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	MimeType string `json:"mime_type,omitempty"`
}

func (c *MediaController) UpdateMediaMetadata(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "assetID")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.Error(w, "invalid asset id", http.StatusBadRequest)
		return
	}

	var req updateMediaMetadataRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Width <= 0 || req.Height <= 0 {
		http.Error(w, "width and height must be positive", http.StatusBadRequest)
		return
	}

	if err := c.store.UpdateMediaAssetMetadata(r.Context(), id, req.Width, req.Height, req.MimeType); err != nil {
		c.logger.Error("update media metadata", err, "id", id)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
