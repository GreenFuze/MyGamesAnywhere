package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

var githubReleasesAPIBase = "https://api.github.com/repos"
var startDetachedCommand = func(cmd *exec.Cmd) error { return cmd.Start() }

type Service struct {
	cfg         core.Configuration
	logger      core.Logger
	client      *http.Client
	mu          sync.Mutex
	lastStatus  core.UpdateStatus
	exitProcess func(int)
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func NewService(cfg core.Configuration, logger core.Logger) *Service {
	return &Service{
		cfg:         cfg,
		logger:      logger,
		client:      &http.Client{Timeout: 60 * time.Second},
		exitProcess: os.Exit,
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
	if goruntime.GOOS != "windows" {
		return nil, errors.New("auto-update apply is currently supported only on Windows")
	}
	if s.installType() == assetTypePortable {
		if err := s.applyPortable(status); err != nil {
			return nil, err
		}
		return &core.UpdateApplyResult{
			Applied: true,
			Message: "Portable update started. MGA will restart shortly while the updater replaces app files.",
			Path:    status.DownloadedPath,
		}, nil
	}
	if err := s.applyInstaller(status); err != nil {
		return nil, err
	}
	return &core.UpdateApplyResult{
		Applied: true,
		Message: "Installer update started. MGA will restart shortly while the installer replaces app files.",
		Path:    status.DownloadedPath,
	}, nil
}

func (s *Service) applyInstaller(status *core.UpdateStatus) error {
	paths, err := s.resolveUpdatePaths()
	if err != nil {
		return err
	}
	flavor := s.installFlavor()
	args := []string{
		"/VERYSILENT",
		"/SUPPRESSMSGBOXES",
		"/NORESTART",
		"/CLOSEAPPLICATIONS",
		"/MGAUPDATE=1",
		"/MGAINSTALLTYPE=" + flavor,
		"/MGAAPPDIR=" + paths.AppDir,
		"/MGADATADIR=" + paths.DataDir,
		"/MGACONFIG=" + paths.ConfigPath,
		fmt.Sprintf("/MGAPID=%d", os.Getpid()),
	}
	if flavor == "service" {
		args = append(args, "/ALLUSERS")
	} else {
		args = append(args, "/CURRENTUSER")
	}
	cmd := exec.Command(status.DownloadedPath, args...)
	if err := startDetachedCommand(cmd); err != nil {
		return fmt.Errorf("launch installer update: %w", err)
	}
	return nil
}

func (s *Service) applyPortable(status *core.UpdateStatus) error {
	if err := validatePortableZip(status.DownloadedPath); err != nil {
		return err
	}
	paths, err := s.resolveUpdatePaths()
	if err != nil {
		return err
	}
	helper := filepath.Join(paths.AppDir, "mga_update.ps1")
	if _, err := os.Stat(helper); err != nil {
		return fmt.Errorf("portable updater helper is unavailable: %w", err)
	}
	planPath, err := s.writePortableUpdatePlan(status, paths)
	if err != nil {
		return err
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", helper, "-PlanPath", planPath)
	if err := startDetachedCommand(cmd); err != nil {
		return fmt.Errorf("launch portable updater: %w", err)
	}
	go func() {
		time.Sleep(1500 * time.Millisecond)
		s.exitProcess(0)
	}()
	return nil
}

func (s *Service) fetchManifest(ctx context.Context) (*core.UpdateManifest, error) {
	url := s.manifestURL()
	manifest, err := s.fetchManifestURL(ctx, url)
	if err == nil {
		return manifest, nil
	}
	if !isGitHubLatestManifestURL(url) {
		return nil, err
	}
	fallbackManifest, fallbackErr := s.fetchNewestGitHubReleaseManifest(ctx, url)
	if fallbackErr == nil {
		return fallbackManifest, nil
	}
	return nil, fmt.Errorf("%w; GitHub release fallback failed: %v", err, fallbackErr)
}

func (s *Service) fetchManifestURL(ctx context.Context, manifestURL string) (*core.UpdateManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
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

func (s *Service) fetchNewestGitHubReleaseManifest(ctx context.Context, manifestURL string) (*core.UpdateManifest, error) {
	apiURL, err := githubReleasesAPIURL(manifestURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create GitHub releases request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch GitHub releases: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch GitHub releases: unexpected status %d", res.StatusCode)
	}
	var releases []githubRelease
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&releases); err != nil {
		return nil, fmt.Errorf("parse GitHub releases: %w", err)
	}

	var selected *core.UpdateManifest
	for _, release := range releases {
		if release.Draft {
			continue
		}
		assetURL := githubReleaseManifestAssetURL(release)
		if assetURL == "" {
			continue
		}
		manifest, err := s.fetchManifestURL(ctx, assetURL)
		if err != nil {
			s.logger.Warn("skip update manifest from GitHub release", "tag", release.TagName, "error", err)
			continue
		}
		if selected == nil {
			selected = manifest
			continue
		}
		if cmp, ok := compareVersions(manifest.Version, selected.Version); ok && cmp > 0 {
			selected = manifest
		}
	}
	if selected == nil {
		return nil, errors.New("no mga-update.json asset found in GitHub releases")
	}
	return selected, nil
}

func githubReleaseManifestAssetURL(release githubRelease) string {
	for _, asset := range release.Assets {
		if strings.EqualFold(asset.Name, "mga-update.json") {
			return strings.TrimSpace(asset.BrowserDownloadURL)
		}
	}
	return ""
}

func isGitHubLatestManifestURL(value string) bool {
	u, err := parseHTTPSURL(value)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, "github.com") &&
		strings.Contains(u.Path, "/releases/latest/download/")
}

func githubReleasesAPIURL(value string) (string, error) {
	u, err := parseHTTPSURL(value)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("cannot infer GitHub repository from %q", value)
	}
	return fmt.Sprintf("%s/%s/%s/releases?per_page=20", strings.TrimRight(githubReleasesAPIBase, "/"), parts[0], parts[1]), nil
}

func parseHTTPSURL(value string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("expected absolute https URL, got %q", value)
	}
	return u, nil
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

func (s *Service) installFlavor() string {
	value := strings.ToLower(strings.TrimSpace(s.cfg.Get("APP_INSTALL_TYPE")))
	switch value {
	case "service", "machine", "installed", "installer":
		return "service"
	case "user":
		return "user"
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

type updatePaths struct {
	AppDir     string `json:"app_dir"`
	DataDir    string `json:"data_dir"`
	ConfigPath string `json:"config_path"`
}

type portableUpdatePlan struct {
	Version    string `json:"version"`
	AssetPath  string `json:"asset_path"`
	AppDir     string `json:"app_dir"`
	DataDir    string `json:"data_dir"`
	ConfigPath string `json:"config_path"`
	ServerPID  int    `json:"server_pid"`
}

func (s *Service) resolveUpdatePaths() (updatePaths, error) {
	exePath, err := os.Executable()
	if err != nil {
		return updatePaths{}, fmt.Errorf("resolve executable path: %w", err)
	}
	appDir := filepath.Dir(exePath)
	if pluginsDir := strings.TrimSpace(s.cfg.Get("PLUGINS_DIR")); pluginsDir != "" {
		appDir = filepath.Dir(filepath.Clean(pluginsDir))
	}
	if abs, err := filepath.Abs(appDir); err == nil {
		appDir = abs
	}

	updatesDir := s.updatesDir()
	dataDir := filepath.Dir(filepath.Clean(updatesDir))
	if s.installFlavor() == assetTypePortable {
		dataDir = appDir
	}
	if abs, err := filepath.Abs(dataDir); err == nil {
		dataDir = abs
	}

	configPath := filepath.Join(dataDir, "config.json")
	if s.installFlavor() == assetTypePortable {
		configPath = filepath.Join(appDir, "config.json")
	}
	if abs, err := filepath.Abs(configPath); err == nil {
		configPath = abs
	}
	if strings.TrimSpace(appDir) == "" || strings.TrimSpace(dataDir) == "" || strings.TrimSpace(configPath) == "" {
		return updatePaths{}, errors.New("update paths could not be resolved")
	}
	return updatePaths{AppDir: appDir, DataDir: dataDir, ConfigPath: configPath}, nil
}

func (s *Service) writePortableUpdatePlan(status *core.UpdateStatus, paths updatePaths) (string, error) {
	updatesDir := s.updatesDir()
	if err := os.MkdirAll(updatesDir, 0o755); err != nil {
		return "", fmt.Errorf("create updates directory: %w", err)
	}
	plan := portableUpdatePlan{
		Version:    status.LatestVersion,
		AssetPath:  status.DownloadedPath,
		AppDir:     paths.AppDir,
		DataDir:    paths.DataDir,
		ConfigPath: paths.ConfigPath,
		ServerPID:  os.Getpid(),
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal portable update plan: %w", err)
	}
	planPath := filepath.Join(updatesDir, "mga_update_plan.json")
	if err := os.WriteFile(planPath, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write portable update plan: %w", err)
	}
	return planPath, nil
}

func validatePortableZip(path string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open portable update ZIP: %w", err)
	}
	defer reader.Close()
	roots := map[string]map[string]bool{}
	for _, file := range reader.File {
		name := strings.TrimLeft(filepath.ToSlash(file.Name), "/")
		if name == "" {
			continue
		}
		parts := strings.Split(name, "/")
		root := ""
		rel := name
		if len(parts) > 1 {
			root = parts[0] + "/"
			rel = strings.TrimPrefix(name, root)
		}
		if roots[root] == nil {
			roots[root] = map[string]bool{}
		}
		if rel == "mga_server.exe" {
			roots[root]["server"] = true
		}
		if rel == "frontend/dist/index.html" {
			roots[root]["frontend"] = true
		}
		if strings.HasPrefix(rel, "plugins/") && rel != "plugins/" {
			roots[root]["plugins"] = true
		}
	}
	for _, found := range roots {
		if found["server"] && found["frontend"] && found["plugins"] {
			return nil
		}
	}
	return errors.New("portable update ZIP is missing mga_server.exe, plugins, or frontend/dist/index.html")
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
		if l.core[i] > c.core[i] {
			return 1, true
		}
		if l.core[i] < c.core[i] {
			return -1, true
		}
	}
	if l.prerelease == c.prerelease {
		return 0, true
	}
	if l.prerelease == "" {
		return 1, true
	}
	if c.prerelease == "" {
		return -1, true
	}
	return comparePrerelease(l.prerelease, c.prerelease), true
}

type semverVersion struct {
	core       [3]int
	prerelease string
}

func parseVersion(value string) (semverVersion, bool) {
	var out semverVersion
	value = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(value)), "v")
	if value == "" {
		return out, false
	}
	if beforeBuild, _, ok := strings.Cut(value, "+"); ok {
		value = beforeBuild
	}
	corePart, prerelease, hasPrerelease := strings.Cut(value, "-")
	if hasPrerelease && prerelease == "" {
		return out, false
	}
	parts := strings.Split(corePart, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, part := range parts {
		if part == "" {
			return out, false
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, false
		}
		out.core[i] = n
	}
	if hasPrerelease {
		identifiers := strings.Split(prerelease, ".")
		for _, identifier := range identifiers {
			if identifier == "" {
				return out, false
			}
		}
		out.prerelease = prerelease
	}
	return out, true
}

func comparePrerelease(latest, current string) int {
	lParts := strings.Split(latest, ".")
	cParts := strings.Split(current, ".")
	for i := 0; i < len(lParts) && i < len(cParts); i++ {
		cmp := comparePrereleaseIdentifier(lParts[i], cParts[i])
		if cmp != 0 {
			return cmp
		}
	}
	if len(lParts) > len(cParts) {
		return 1
	}
	if len(lParts) < len(cParts) {
		return -1
	}
	return 0
}

func comparePrereleaseIdentifier(latest, current string) int {
	lNumber, lNumeric := parsePrereleaseNumber(latest)
	cNumber, cNumeric := parsePrereleaseNumber(current)
	switch {
	case lNumeric && cNumeric:
		if lNumber > cNumber {
			return 1
		}
		if lNumber < cNumber {
			return -1
		}
		return 0
	case lNumeric:
		return -1
	case cNumeric:
		return 1
	default:
		if latest > current {
			return 1
		}
		if latest < current {
			return -1
		}
		return 0
	}
}

func parsePrereleaseNumber(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	return n, err == nil
}

func cloneAsset(asset *core.UpdateAsset) *core.UpdateAsset {
	if asset == nil {
		return nil
	}
	copied := *asset
	return &copied
}
