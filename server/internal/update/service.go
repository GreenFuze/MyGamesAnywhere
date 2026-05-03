package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/buildinfo"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const (
	assetTypeInstaller = "installer"
	assetTypePortable  = "portable"
	defaultInstallType = "portable"
	defaultManifestURL = "https://github.com/GreenFuze/MyGamesAnywhere/releases/latest/download/mga-update.json"
)

type Service struct {
	cfg        core.Configuration
	logger     core.Logger
	client     *http.Client
	mu         sync.Mutex
	lastStatus core.UpdateStatus
}

func NewService(cfg core.Configuration, logger core.Logger) *Service {
	return &Service{
		cfg:    cfg,
		logger: logger,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (s *Service) Status(ctx context.Context) (*core.UpdateStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.baseStatusLocked()
	if s.lastStatus.LatestVersion != "" {
		status.LatestVersion = s.lastStatus.LatestVersion
		status.UpdateAvailable = s.lastStatus.UpdateAvailable
		status.ReleaseNotesURL = s.lastStatus.ReleaseNotesURL
		status.DownloadedPath = s.lastStatus.DownloadedPath
		status.DownloadedSHA256 = s.lastStatus.DownloadedSHA256
		status.SelectedAsset = cloneAsset(s.lastStatus.SelectedAsset)
		status.Message = s.lastStatus.Message
	}
	_ = ctx
	return &status, nil
}

func (s *Service) Check(ctx context.Context) (*core.UpdateStatus, error) {
	manifest, err := s.fetchManifest(ctx)
	if err != nil {
		return nil, err
	}
	asset, err := s.selectAsset(manifest)
	if err != nil {
		return nil, err
	}
	status := s.statusFromManifest(manifest, asset)
	s.mu.Lock()
	s.lastStatus = status
	s.mu.Unlock()
	return &status, nil
}

func (s *Service) Download(ctx context.Context) (*core.UpdateDownloadResult, error) {
	status, err := s.Check(ctx)
	if err != nil {
		return nil, err
	}
	if status.SelectedAsset == nil {
		return nil, errors.New("no update asset selected")
	}
	if status.SelectedAsset.URL == "" {
		return nil, errors.New("selected update asset has no URL")
	}
	updatesDir := s.updatesDir()
	if err := os.MkdirAll(updatesDir, 0o755); err != nil {
		return nil, fmt.Errorf("create updates directory: %w", err)
	}
	name := status.SelectedAsset.Name
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(status.SelectedAsset.URL)
	}
	if strings.TrimSpace(name) == "." || strings.TrimSpace(name) == string(filepath.Separator) {
		name = fmt.Sprintf("mga-update-%s", status.LatestVersion)
	}
	path := filepath.Join(updatesDir, filepath.Base(name))
	hash, size, err := s.downloadAndVerify(ctx, status.SelectedAsset.URL, path, status.SelectedAsset.SHA256)
	if err != nil {
		return nil, err
	}
	status.DownloadedPath = path
	status.DownloadedSHA256 = hash
	status.Message = "Update asset downloaded and verified."
	s.mu.Lock()
	s.lastStatus = *status
	s.mu.Unlock()
	return &core.UpdateDownloadResult{
		Status: *status,
		Path:   path,
		SHA256: hash,
		Size:   size,
	}, nil
}

func (s *Service) Apply(ctx context.Context) (*core.UpdateApplyResult, error) {
	status, err := s.Status(ctx)
	if err != nil {
		return nil, err
	}
	if status.DownloadedPath == "" {
		download, err := s.Download(ctx)
		if err != nil {
			return nil, err
		}
		status = &download.Status
	}
	if s.installType() == assetTypePortable {
		return &core.UpdateApplyResult{
			Applied: false,
			Message: "Portable updates are downloaded only in this version. Stop MGA, replace the portable folder with the downloaded ZIP contents, then start MGA again.",
			Path:    status.DownloadedPath,
		}, nil
	}
	if goruntime.GOOS != "windows" {
		return nil, errors.New("installer apply is currently supported only on Windows")
	}
	cmd := exec.CommandContext(ctx, status.DownloadedPath, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/CLOSEAPPLICATIONS")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launch installer update: %w", err)
	}
	return &core.UpdateApplyResult{
		Applied: true,
		Message: "Installer update launched. MGA may stop and restart while the installer replaces app files.",
		Path:    status.DownloadedPath,
	}, nil
}

func (s *Service) fetchManifest(ctx context.Context) (*core.UpdateManifest, error) {
	url := s.manifestURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create manifest request: %w", err)
	}
	res, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch update manifest: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch update manifest: unexpected status %d", res.StatusCode)
	}
	var manifest core.UpdateManifest
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("parse update manifest: %w", err)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return nil, errors.New("update manifest missing version")
	}
	return &manifest, nil
}

func (s *Service) selectAsset(manifest *core.UpdateManifest) (*core.UpdateAsset, error) {
	wantType := assetTypeInstaller
	if s.installType() == assetTypePortable {
		wantType = assetTypePortable
	}
	for i := range manifest.Assets {
		asset := manifest.Assets[i]
		if !strings.EqualFold(asset.OS, goruntime.GOOS) {
			continue
		}
		if !strings.EqualFold(asset.Arch, goruntime.GOARCH) {
			continue
		}
		if !strings.EqualFold(asset.Type, wantType) {
			continue
		}
		if asset.SHA256 == "" {
			return nil, errors.New("selected update asset is missing SHA256")
		}
		asset.Version = manifest.Version
		return &asset, nil
	}
	return nil, fmt.Errorf("no %s update asset for %s/%s", wantType, goruntime.GOOS, goruntime.GOARCH)
}

func (s *Service) statusFromManifest(manifest *core.UpdateManifest, asset *core.UpdateAsset) core.UpdateStatus {
	status := s.baseStatusLocked()
	status.LatestVersion = manifest.Version
	status.ReleaseNotesURL = manifest.ReleaseNotesURL
	status.SelectedAsset = cloneAsset(asset)
	cmp, ok := compareVersions(manifest.Version, buildinfo.Version)
	if !ok {
		status.UpdateAvailable = false
		status.Message = "Current or latest version is not a semantic release version."
		return status
	}
	status.UpdateAvailable = cmp > 0
	if status.UpdateAvailable {
		status.Message = "Update available."
	} else {
		status.Message = "MGA is up to date."
	}
	return status
}

func (s *Service) baseStatusLocked() core.UpdateStatus {
	return core.UpdateStatus{
		CurrentVersion: buildinfo.Version,
		ManifestURL:    s.manifestURL(),
		InstallType:    s.installType(),
	}
}

func (s *Service) manifestURL() string {
	if value := strings.TrimSpace(s.cfg.Get("UPDATE_MANIFEST_URL")); value != "" {
		return value
	}
	return defaultManifestURL
}

func (s *Service) installType() string {
	value := strings.ToLower(strings.TrimSpace(s.cfg.Get("APP_INSTALL_TYPE")))
	switch value {
	case "user", "machine", "service", "installed", "installer":
		return assetTypeInstaller
	case assetTypePortable:
		return assetTypePortable
	default:
		return defaultInstallType
	}
}

func (s *Service) updatesDir() string {
	if value := strings.TrimSpace(s.cfg.Get("UPDATES_DIR")); value != "" {
		return value
	}
	if base, err := os.UserCacheDir(); err == nil {
		return filepath.Join(base, "MyGamesAnywhere", "updates")
	}
	return "updates"
}

func (s *Service) downloadAndVerify(ctx context.Context, url, path, expectedSHA string) (string, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, fmt.Errorf("create download request: %w", err)
	}
	res, err := s.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("download update asset: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", 0, fmt.Errorf("download update asset: unexpected status %d", res.StatusCode)
	}
	tmp := path + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return "", 0, fmt.Errorf("create update download file: %w", err)
	}
	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hasher), res.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return "", 0, fmt.Errorf("write update download: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", 0, fmt.Errorf("close update download: %w", closeErr)
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actual, strings.TrimSpace(expectedSHA)) {
		_ = os.Remove(tmp)
		return "", 0, fmt.Errorf("update SHA256 mismatch: expected %s got %s", expectedSHA, actual)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", 0, fmt.Errorf("move update download into place: %w", err)
	}
	return actual, size, nil
}

func compareVersions(latest, current string) (int, bool) {
	l, ok := parseVersion(latest)
	if !ok {
		return 0, false
	}
	c, ok := parseVersion(current)
	if !ok {
		return 0, false
	}
	for i := 0; i < 3; i++ {
		if l[i] > c[i] {
			return 1, true
		}
		if l[i] < c[i] {
			return -1, true
		}
	}
	return 0, true
}

func parseVersion(value string) ([3]int, bool) {
	var out [3]int
	value = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(value)), "v")
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func cloneAsset(asset *core.UpdateAsset) *core.UpdateAsset {
	if asset == nil {
		return nil
	}
	copied := *asset
	return &copied
}
