package clientapp

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
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

const installManifestName = ".mga-install.json"

type CommandProgressUpdate struct {
	Phase        string
	Message      string
	Percent      uint8
	Stage        string
	StagePercent uint8
}

type CommandProgressReporter func(CommandProgressUpdate) error

type ArchiveInstaller interface {
	Install(context.Context, string, devicev1.ArchiveInstallRequest, CommandProgressReporter) (devicev1.ArchiveInstallResult, error)
	Uninstall(context.Context, devicev1.GameUninstallRequest, CommandProgressReporter) (devicev1.GameUninstallResult, error)
}

type ManagedArchiveInstaller struct {
	serverURL string
	client    *http.Client
	now       func() time.Time
}

type installManifest struct {
	SchemaVersion    int       `json:"schema_version"`
	GameID           string    `json:"game_id"`
	SourceGameID     string    `json:"source_game_id"`
	InstallRoot      string    `json:"install_root"`
	ArchiveName      string    `json:"archive_name"`
	ArchiveSHA256    string    `json:"archive_sha256"`
	ArchiveBytes     uint64    `json:"archive_bytes"`
	InstalledAt      time.Time `json:"installed_at"`
	LaunchTarget     string    `json:"launch_target,omitempty"`
	LaunchCandidates []string  `json:"launch_candidates,omitempty"`
}

func NewManagedArchiveInstaller(serverURL string) (*ManagedArchiveInstaller, error) {
	parsed, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("valid MGA Server URL is required")
	}
	return &ManagedArchiveInstaller{
		serverURL: parsed.Scheme + "://" + parsed.Host,
		client:    &http.Client{Timeout: 0},
		now:       time.Now,
	}, nil
}

func (i *ManagedArchiveInstaller) Install(ctx context.Context, commandID string, request devicev1.ArchiveInstallRequest, report CommandProgressReporter) (devicev1.ArchiveInstallResult, error) {
	if i == nil || i.client == nil || i.now == nil {
		return devicev1.ArchiveInstallResult{}, errors.New("archive installer is unavailable")
	}
	if strings.TrimSpace(commandID) == "" {
		return devicev1.ArchiveInstallResult{}, errors.New("command_id is required")
	}
	if err := request.Validate(); err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}
	extractor, err := archiveExtractorForFormat(request.ArchiveFormat)
	if err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}
	downloadURL, err := i.resolveDownloadURL(request.DownloadURL)
	if err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}
	rootTemplate := strings.TrimSpace(request.DestinationRoot)
	if rootTemplate == "" {
		rootTemplate = devicev1.DefaultInstallRootTemplate
	}
	installRoot, err := expandInstallRoot(rootTemplate)
	if err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}
	if err := validateDestinationName(request.DestinationName); err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		return devicev1.ArchiveInstallResult{}, fmt.Errorf("create install root: %w", err)
	}
	if free, err := availableDiskBytes(installRoot); err != nil {
		return devicev1.ArchiveInstallResult{}, fmt.Errorf("check free disk space: %w", err)
	} else if free < request.ArchiveSize+64*1024*1024 {
		return devicev1.ArchiveInstallResult{}, fmt.Errorf("not enough free space to download %s", request.ArchiveName)
	}

	target := filepath.Join(installRoot, request.DestinationName)
	if _, err := os.Stat(target); err == nil {
		return devicev1.ArchiveInstallResult{}, fmt.Errorf("destination already exists: %s", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return devicev1.ArchiveInstallResult{}, fmt.Errorf("inspect destination: %w", err)
	}

	stage := filepath.Join(installRoot, ".mga", "staging", commandID)
	if err := os.RemoveAll(stage); err != nil {
		return devicev1.ArchiveInstallResult{}, fmt.Errorf("clear stale staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	contentDir := filepath.Join(stage, "content")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		return devicev1.ArchiveInstallResult{}, fmt.Errorf("create staging directory: %w", err)
	}
	archivePath := filepath.Join(stage, "source."+devicev1.NormalizeArchiveFormat(request.ArchiveFormat))
	if err := reportProgress(report, "downloading", "Downloading archive", 0, "download", 0); err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}
	hash, downloaded, err := i.download(ctx, downloadURL, request.DownloadToken, archivePath, request.ArchiveSize, report)
	if err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}
	if request.ArchiveSize != downloaded {
		return devicev1.ArchiveInstallResult{}, fmt.Errorf("archive size mismatch: received %d bytes, expected %d", downloaded, request.ArchiveSize)
	}

	if err := reportProgress(report, "checking", "Checking archive", 43, "install", 5); err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}
	uncompressed, err := extractor.Validate(archivePath)
	if err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}
	if free, err := availableDiskBytes(installRoot); err != nil {
		return devicev1.ArchiveInstallResult{}, fmt.Errorf("check extraction space: %w", err)
	} else if free < uncompressed+64*1024*1024 {
		return devicev1.ArchiveInstallResult{}, fmt.Errorf("not enough free space to extract %s", request.ArchiveName)
	}
	if err := extractor.Extract(ctx, archivePath, contentDir, uncompressed, report); err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}
	launchCandidates, launchTarget, err := discoverLaunchTargets(contentDir, request.Title)
	if err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}

	installedAt := i.now().UTC()
	manifest := installManifest{
		SchemaVersion: devicev1.InstallManifestSchemaVersion,
		GameID:        request.GameID, SourceGameID: request.SourceGameID, InstallRoot: installRoot,
		ArchiveName: request.ArchiveName, ArchiveSHA256: hash, ArchiveBytes: downloaded, InstalledAt: installedAt,
		LaunchTarget: launchTarget, LaunchCandidates: launchCandidates,
	}
	if err := writeInstallManifest(contentDir, manifest); err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}
	if err := reportProgress(report, "finishing", "Finishing installation", 97, "install", 95); err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}
	if err := os.Rename(contentDir, target); err != nil {
		return devicev1.ArchiveInstallResult{}, fmt.Errorf("commit installation: %w", err)
	}
	if err := reportProgress(report, "complete", "Installed", 100, "install", 100); err != nil {
		return devicev1.ArchiveInstallResult{}, err
	}
	return devicev1.ArchiveInstallResult{
		GameID: request.GameID, SourceGameID: request.SourceGameID, InstallRoot: installRoot,
		InstallPath: target, ArchiveSHA256: hash, ArchiveBytes: downloaded, InstalledAt: installedAt,
		LaunchTarget: launchTarget, LaunchCandidates: launchCandidates,
	}, nil
}

func (i *ManagedArchiveInstaller) Uninstall(ctx context.Context, request devicev1.GameUninstallRequest, report CommandProgressReporter) (devicev1.GameUninstallResult, error) {
	if err := request.Validate(); err != nil {
		return devicev1.GameUninstallResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return devicev1.GameUninstallResult{}, err
	}
	manifest, err := readInstallManifest(request.InstallPath)
	if err != nil {
		return devicev1.GameUninstallResult{}, err
	}
	if (manifest.SchemaVersion != 1 && manifest.SchemaVersion != devicev1.InstallManifestSchemaVersion) || manifest.GameID != request.GameID || manifest.SourceGameID != request.SourceGameID {
		return devicev1.GameUninstallResult{}, errors.New("installation manifest does not match the requested game")
	}
	inside, err := pathWithinRoot(manifest.InstallRoot, request.InstallPath)
	if err != nil || !inside || filepath.Clean(request.InstallPath) == filepath.Clean(manifest.InstallRoot) {
		return devicev1.GameUninstallResult{}, errors.New("install path is outside its recorded MGA root")
	}
	if err := reportProgress(report, "removing", "Removing installed files", 25, "", 0); err != nil {
		return devicev1.GameUninstallResult{}, err
	}
	if err := os.RemoveAll(request.InstallPath); err != nil {
		return devicev1.GameUninstallResult{}, fmt.Errorf("remove installation: %w", err)
	}
	if err := reportProgress(report, "complete", "Uninstalled", 100, "", 0); err != nil {
		return devicev1.GameUninstallResult{}, err
	}
	return devicev1.GameUninstallResult{GameID: request.GameID, SourceGameID: request.SourceGameID, Removed: true}, nil
}

func (i *ManagedArchiveInstaller) resolveDownloadURL(raw string) (string, error) {
	server, _ := url.Parse(i.serverURL)
	download, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("archive download URL is invalid")
	}
	if !download.IsAbs() {
		if download.Host != "" || !strings.HasPrefix(download.Path, "/") {
			return "", errors.New("archive download URL must be origin-relative")
		}
		download = server.ResolveReference(download)
	}
	if !strings.EqualFold(server.Scheme, download.Scheme) || !strings.EqualFold(server.Host, download.Host) {
		return "", errors.New("archive download URL must use the paired MGA Server origin")
	}
	return download.String(), nil
}

func (i *ManagedArchiveInstaller) download(ctx context.Context, rawURL, token, destination string, expected uint64, report CommandProgressReporter) (string, uint64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("create archive request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := i.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("download archive: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("download archive: MGA Server returned %s", response.Status)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", 0, fmt.Errorf("create staged archive: %w", err)
	}
	hasher := sha256.New()
	written, copyErr := copyWithDownloadProgress(ctx, io.MultiWriter(file, hasher), response.Body, expected, report)
	closeErr := file.Close()
	if copyErr != nil {
		return "", uint64(written), copyErr
	}
	if closeErr != nil {
		return "", uint64(written), closeErr
	}
	return hex.EncodeToString(hasher.Sum(nil)), uint64(written), nil
}

func copyWithDownloadProgress(ctx context.Context, destination io.Writer, source io.Reader, total uint64, report CommandProgressReporter) (int64, error) {
	buffer := make([]byte, 1024*1024)
	var written int64
	lastPercent := uint8(0)
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			n, writeErr := destination.Write(buffer[:count])
			written += int64(n)
			if writeErr != nil {
				return written, writeErr
			}
			percent := uint8(0)
			if total > 0 {
				percent = uint8(min(uint64(100), uint64(written)*100/total))
			}
			if percent > lastPercent {
				overall := uint8(uint16(percent) * 40 / 100)
				if err := reportProgress(report, "downloading", "Downloading archive", overall, "download", percent); err != nil {
					return written, err
				}
				lastPercent = percent
			}
		}
		if errors.Is(readErr, io.EOF) {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

func validateZIPArchive(path string) (uint64, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return 0, fmt.Errorf("open ZIP archive: %w", err)
	}
	defer reader.Close()
	if len(reader.File) == 0 {
		return 0, errors.New("ZIP archive is empty")
	}
	var total uint64
	for _, entry := range reader.File {
		if _, err := safeArchivePath(entry.Name); err != nil {
			return 0, err
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return 0, fmt.Errorf("ZIP archive contains unsupported symbolic link %q", entry.Name)
		}
		if ^uint64(0)-total < entry.UncompressedSize64 {
			return 0, errors.New("ZIP uncompressed size overflow")
		}
		total += entry.UncompressedSize64
	}
	return total, nil
}

func extractZIP(ctx context.Context, archivePath, destination string, total uint64, report CommandProgressReporter) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open ZIP archive: %w", err)
	}
	defer reader.Close()
	var extracted uint64
	lastPercent := uint8(10)
	if err := reportProgress(report, "extracting", "Extracting files", 46, "install", lastPercent); err != nil {
		return err
	}
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := safeArchivePath(entry.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			source.Close()
			return err
		}
		written, copyErr := io.Copy(output, source)
		closeOutputErr := output.Close()
		closeSourceErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOutputErr != nil {
			return closeOutputErr
		}
		if closeSourceErr != nil {
			return closeSourceErr
		}
		extracted += uint64(written)
		percent := uint8(90)
		if total > 0 {
			percent = 10 + uint8(min(uint64(80), extracted*80/total))
		}
		if percent > lastPercent {
			overall := 40 + uint8(uint16(percent)*60/100)
			if err := reportProgress(report, "extracting", "Extracting files", overall, "install", percent); err != nil {
				return err
			}
			lastPercent = percent
		}
	}
	return nil
}

func safeArchivePath(name string) (string, error) {
	normalized := filepath.ToSlash(strings.TrimSpace(name))
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(normalized)))
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") || cleaned == ".." || filepath.IsAbs(filepath.FromSlash(cleaned)) || strings.Contains(cleaned, ":") {
		return "", fmt.Errorf("unsafe ZIP entry path %q", name)
	}
	return cleaned, nil
}

var windowsEnvironmentPattern = regexp.MustCompile(`%([^%]+)%`)

func expandInstallRoot(template string) (string, error) {
	missing := ""
	expanded := windowsEnvironmentPattern.ReplaceAllStringFunc(template, func(match string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(match, "%"), "%")
		value, ok := os.LookupEnv(name)
		if !ok || strings.TrimSpace(value) == "" {
			missing = name
			return match
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("environment variable %%%s%% is not available", missing)
	}
	expanded = os.Expand(expanded, func(name string) string {
		value, ok := os.LookupEnv(name)
		if !ok {
			missing = name
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("environment variable %s is not available", missing)
	}
	absolute, err := filepath.Abs(strings.TrimSpace(expanded))
	if err != nil {
		return "", fmt.Errorf("resolve install root: %w", err)
	}
	if !filepath.IsAbs(absolute) {
		return "", errors.New("install root must resolve to an absolute path")
	}
	cleaned := filepath.Clean(absolute)
	if err := validateInstallRootStorage(cleaned); err != nil {
		return "", err
	}
	return cleaned, nil
}

func validateDestinationName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\:*?"<>|`) || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return errors.New("destination_name is not a safe Windows folder name")
	}
	return nil
}

func writeInstallManifest(directory string, manifest installManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, installManifestName), append(data, '\n'), 0o600)
}

func readInstallManifest(directory string) (installManifest, error) {
	data, err := os.ReadFile(filepath.Join(directory, installManifestName))
	if err != nil {
		return installManifest{}, fmt.Errorf("read MGA installation manifest: %w", err)
	}
	var manifest installManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return installManifest{}, fmt.Errorf("decode MGA installation manifest: %w", err)
	}
	return manifest, nil
}

func pathWithinRoot(root, candidate string) (bool, error) {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil {
		return false, err
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)), nil
}

func reportProgress(report CommandProgressReporter, phase, message string, percent uint8, stage string, stagePercent uint8) error {
	if report == nil {
		return nil
	}
	return report(CommandProgressUpdate{Phase: phase, Message: message, Percent: percent, Stage: stage, StagePercent: stagePercent})
}

type launchCandidate struct {
	path  string
	score int
}

func discoverLaunchTargets(root, title string) ([]string, string, error) {
	normalizedTitle := normalizeLaunchName(title)
	var scored []launchCandidate
	err := filepath.WalkDir(root, func(candidatePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".exe") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, candidatePath)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if err := devicev1.ValidateLaunchTarget(relative); err != nil {
			return err
		}
		base := normalizeLaunchName(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		score := 100 - strings.Count(relative, "/")*8
		if isBlockedLaunchExecutable(base) {
			return nil
		}
		if normalizedTitle != "" && base == normalizedTitle {
			score += 250
		} else if normalizedTitle != "" && (strings.Contains(base, normalizedTitle) || strings.Contains(normalizedTitle, base)) {
			score += 100
		}
		scored = append(scored, launchCandidate{path: relative, score: score})
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("discover launch targets: %w", err)
	}
	sort.Slice(scored, func(a, b int) bool {
		if scored[a].score != scored[b].score {
			return scored[a].score > scored[b].score
		}
		return scored[a].path < scored[b].path
	})
	candidates := make([]string, 0, len(scored))
	for _, candidate := range scored {
		candidates = append(candidates, candidate.path)
	}
	selected := ""
	if len(scored) == 1 {
		selected = scored[0].path
	} else if len(scored) > 1 && scored[0].score-scored[1].score >= 50 {
		selected = scored[0].path
	}
	return candidates, selected, nil
}

func isBlockedLaunchExecutable(normalizedBase string) bool {
	if normalizedBase == "" {
		return true
	}
	for _, fragment := range []string{"vcredist", "dxsetup", "crashhandler", "crashreporter", "crashsender"} {
		if strings.Contains(normalizedBase, fragment) {
			return true
		}
	}
	if strings.HasPrefix(normalizedBase, "unins") {
		return true
	}
	for _, suffix := range []string{"setup", "installer", "uninstall", "uninstaller", "updater", "helper"} {
		if normalizedBase == suffix || strings.HasSuffix(normalizedBase, suffix) {
			return true
		}
	}
	return false
}

func normalizeLaunchName(value string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}
