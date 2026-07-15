package clientapp

import (
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
	"sort"
	"strings"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

const (
	gogPublisherName = "GOG Sp. z o.o."
	gogInnoLogName   = "installer.log"
)

// NO_MIGRATION_NEEDED: this installer adds no client config, pairing identity,
// settings-sync, or save-sync fields. Its durable per-install data is schema 3.

type AuthenticodeVerifier interface {
	VerifyGOG(path string) (subject string, thumbprint string, err error)
}

type InnoFamilyDetector interface {
	IsInnoSetup(path string) (bool, error)
}

type InstallConfirmationDetails struct {
	GameTitle       string
	Publisher       string
	Destination     string
	Server          string
	PossibleUACNote string
}

type UninstallConfirmationDetails struct {
	GameTitle   string
	Publisher   string
	InstallPath string
	Server      string
	Warning     string
}

type LocalConfirmer interface {
	ConfirmInstall(context.Context, InstallConfirmationDetails, time.Duration) error
	ConfirmUninstall(context.Context, UninstallConfirmationDetails, time.Duration) error
}

type InstallerProcessSpec struct {
	Path             string
	Arguments        []string
	WorkingDirectory string
}

type InstallerProcess interface {
	PID() int
	Wait(context.Context, time.Duration) (int, error)
}

type InstallerProcessRunner interface {
	Start(context.Context, InstallerProcessSpec) (InstallerProcess, error)
}

type GogInnoInstaller interface {
	Install(context.Context, string, devicev1.GogInnoInstallRequest, CommandProgressReporter) (devicev1.GogInnoInstallResult, error)
	Uninstall(context.Context, devicev1.GogInnoUninstallRequest, CommandProgressReporter) (devicev1.GogInnoUninstallResult, error)
}

type ManagedGogInnoInstaller struct {
	serverURL string
	client    *http.Client
	now       func() time.Time
	verifier  AuthenticodeVerifier
	detector  InnoFamilyDetector
	confirmer LocalConfirmer
	runner    InstallerProcessRunner
}

type GogInnoCommandError struct {
	Code    string
	Message string
	Payload any
}

func (e *GogInnoCommandError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func gogError(code string, payload any, format string, values ...any) error {
	return &GogInnoCommandError{Code: code, Message: fmt.Sprintf(format, values...), Payload: payload}
}

type gogInnoManifest struct {
	SchemaVersion    int                           `json:"schema_version"`
	GameID           string                        `json:"game_id"`
	SourceGameID     string                        `json:"source_game_id"`
	InstallRoot      string                        `json:"install_root"`
	InstallPath      string                        `json:"install_path"`
	InstallerFamily  string                        `json:"installer_family"`
	PrimarySHA256    string                        `json:"primary_sha256"`
	TotalBytes       uint64                        `json:"total_package_bytes"`
	PackageFiles     []devicev1.GogInnoPackageFile `json:"package_files"`
	SignerSubject    string                        `json:"signer_subject"`
	SignerThumbprint string                        `json:"signer_thumbprint"`
	InvocationMode   string                        `json:"invocation_mode"`
	UninstallTarget  string                        `json:"uninstall_target"`
	LaunchTarget     string                        `json:"launch_target,omitempty"`
	LaunchCandidates []string                      `json:"launch_candidates,omitempty"`
	InstalledAt      time.Time                     `json:"installed_at"`
}

func NewManagedGogInnoInstaller(serverURL string, verifier AuthenticodeVerifier, detector InnoFamilyDetector, confirmer LocalConfirmer, runner InstallerProcessRunner) (*ManagedGogInnoInstaller, error) {
	parsed, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("valid MGA Server URL is required")
	}
	if verifier == nil || detector == nil || confirmer == nil || runner == nil {
		return nil, errors.New("GOG Inno installer adapters are required")
	}
	return &ManagedGogInnoInstaller{
		serverURL: parsed.Scheme + "://" + parsed.Host,
		client:    &http.Client{Timeout: 0},
		now:       time.Now,
		verifier:  verifier,
		detector:  detector,
		confirmer: confirmer,
		runner:    runner,
	}, nil
}

func newPlatformGogInnoInstaller(serverURL string) (*ManagedGogInnoInstaller, error) {
	return NewManagedGogInnoInstaller(serverURL, newAuthenticodeVerifier(), newInnoFamilyDetector(), newLocalConfirmer(), newInstallerProcessRunner())
}

func (i *ManagedGogInnoInstaller) Install(ctx context.Context, commandID string, request devicev1.GogInnoInstallRequest, report CommandProgressReporter) (devicev1.GogInnoInstallResult, error) {
	var result devicev1.GogInnoInstallResult
	if i == nil || i.client == nil || i.now == nil || i.verifier == nil || i.detector == nil || i.confirmer == nil || i.runner == nil {
		return result, gogError("unsupported_installer", nil, "GOG Inno installer is unavailable")
	}
	if strings.TrimSpace(commandID) == "" {
		return result, gogError("unsupported_installer", nil, "command_id is required")
	}
	if filepath.Base(commandID) != commandID || strings.ContainsAny(commandID, `/\:`) {
		return result, gogError("unsupported_installer", nil, "command_id is not safe for staging")
	}
	if err := request.Validate(); err != nil {
		return result, gogError("invalid_companion_set", nil, "%v", err)
	}
	if err := reportProgress(report, "preparing", "Preparing installer", 0, "install", 0); err != nil {
		return result, err
	}
	installRoot, err := expandInstallRoot(defaultInstallRoot(request.DestinationRoot))
	if err != nil {
		return result, err
	}
	if err := validateDestinationName(request.DestinationName); err != nil {
		return result, err
	}
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		return result, fmt.Errorf("create install root: %w", err)
	}
	installPath := filepath.Join(installRoot, request.DestinationName)
	if inside, boundaryErr := pathWithinRoot(installRoot, installPath); boundaryErr != nil || !inside || filepath.Clean(installPath) == filepath.Clean(installRoot) {
		return result, errors.New("destination is outside the install root")
	}
	if _, statErr := os.Stat(installPath); statErr == nil {
		return result, fmt.Errorf("destination already exists: %s", installPath)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return result, fmt.Errorf("inspect destination: %w", statErr)
	}

	descriptors := append([]devicev1.PackageTransferDescriptor{request.Installer}, request.Companions...)
	var totalExpected uint64
	for _, descriptor := range descriptors {
		if ^uint64(0)-totalExpected < descriptor.SizeBytes {
			return result, gogError("invalid_companion_set", nil, "package size overflow")
		}
		totalExpected += descriptor.SizeBytes
	}
	if free, diskErr := availableDiskBytes(installRoot); diskErr != nil {
		return result, fmt.Errorf("check free disk space: %w", diskErr)
	} else if free < totalExpected+64*1024*1024 {
		return result, fmt.Errorf("not enough free space to download installer package")
	}

	stage := filepath.Join(installRoot, ".mga", "staging", commandID)
	if err := os.RemoveAll(stage); err != nil {
		return result, fmt.Errorf("clear stale staging directory: %w", err)
	}
	if err := os.MkdirAll(stage, 0o700); err != nil {
		return result, fmt.Errorf("create staging directory: %w", err)
	}
	processStarted := false
	installSucceeded := false
	defer func() {
		// Before process start, clear staging on any failure. After the native
		// installer terminates successfully, clear staging. After a started
		// installer fails or times out, keep staging so the bounded log at
		// diagnostic_ref remains available for attention_required review.
		if !processStarted || installSucceeded {
			_ = os.RemoveAll(stage)
		}
	}()

	if err := reportProgress(report, "downloading", "Downloading installer", 0, "download", 0); err != nil {
		return result, err
	}
	files := make([]devicev1.GogInnoPackageFile, 0, len(descriptors))
	var aggregate uint64
	for _, descriptor := range descriptors {
		rawURL, resolveErr := i.resolveDownloadURL(descriptor.DownloadURL)
		if resolveErr != nil {
			return result, gogError("invalid_companion_set", nil, "%v", resolveErr)
		}
		destination := filepath.Join(stage, descriptor.FileName)
		hash, downloaded, downloadErr := i.downloadFile(ctx, rawURL, descriptor.DownloadToken, destination, descriptor.SizeBytes, aggregate, totalExpected, report)
		if downloadErr != nil {
			return result, downloadErr
		}
		if downloaded != descriptor.SizeBytes {
			return result, fmt.Errorf("package size mismatch for %s: received %d bytes, expected %d", descriptor.FileName, downloaded, descriptor.SizeBytes)
		}
		aggregate += downloaded
		files = append(files, devicev1.GogInnoPackageFile{FileName: descriptor.FileName, Role: descriptor.Role, SizeBytes: downloaded, SHA256: hash})
	}

	installerPath := filepath.Join(stage, request.Installer.FileName)
	if err := reportProgress(report, "verifying", "Verifying publisher", 42, "install", 10); err != nil {
		return result, err
	}
	subject, thumbprint, err := i.verifier.VerifyGOG(installerPath)
	if err != nil {
		if errors.Is(err, ErrUnsupportedInstallerPlatform) {
			return result, gogError("unsupported_installer", nil, "%v", err)
		}
		return result, gogError("invalid_installer_signature", nil, "verify installer signature: %v", err)
	}
	if !signerIsGOG(subject) {
		return result, gogError("invalid_installer_signature", nil, "installer publisher is not %s", gogPublisherName)
	}
	inno, err := i.detector.IsInnoSetup(installerPath)
	if err != nil || !inno {
		if err == nil {
			err = errors.New("Inno Setup marker was not found")
		}
		return result, gogError("unsupported_installer", nil, "%v", err)
	}

	result = devicev1.GogInnoInstallResult{
		GameID: request.GameID, SourceGameID: request.SourceGameID, InstallRoot: installRoot, InstallPath: installPath,
		InstallerFamily: devicev1.GogInnoInstallerFamily, PrimarySHA256: files[0].SHA256, TotalPackageBytes: aggregate,
		PackageFiles: files, SignerSubject: subject, SignerThumbprint: thumbprint,
		InvocationMode: devicev1.GogInnoInvocationFixedSilent, DiagnosticRef: filepath.ToSlash(filepath.Join(".mga", "staging", commandID, gogInnoLogName)),
	}
	if err := reportProgress(report, "confirmation", "Approve on this device", 45, "install", 20); err != nil {
		return result, err
	}
	if err := i.confirmer.ConfirmInstall(ctx, InstallConfirmationDetails{
		GameTitle: request.Title, Publisher: gogPublisherName, Destination: installPath, Server: i.serverURL,
		PossibleUACNote: "Windows may ask for permission on this device.",
	}, devicev1.GogInnoLocalConfirmationTimeout); err != nil {
		return result, normalizeConfirmationError(err, nil)
	}

	args := []string{
		"/SP-",
		"/VERYSILENT",
		"/SUPPRESSMSGBOXES",
		"/NORESTART",
		innoPathArgument("/DIR=", installPath),
		innoPathArgument("/LOG=", filepath.Join(stage, gogInnoLogName)),
	}
	process, err := i.runner.Start(ctx, InstallerProcessSpec{Path: installerPath, Arguments: args, WorkingDirectory: stage})
	if err != nil {
		return result, normalizeStartError(err, nil)
	}
	processStarted = true
	result.ProcessID = process.PID()
	if err := reportProgress(report, "installing", "Installer running", 50, "install", 0); err != nil {
		return result, gogError("installer_timeout", result, "installer started but progress reporting failed: %v", err)
	}
	exitCode, err := process.Wait(ctx, devicev1.GogInnoInstallerProcessTimeout)
	if err != nil {
		return result, normalizeWaitError("installer_timeout", err, result)
	}
	result.ExitCode = intPointer(exitCode)
	if exitCode != 0 {
		return result, gogError("installer_exit_nonzero", result, "installer exited with code %d (0x%08X)", exitCode, uint32(exitCode))
	}

	if err := reportProgress(report, "checking", "Checking installed game", 90, "install", 85); err != nil {
		return result, err
	}
	info, err := os.Stat(installPath)
	if err != nil || !info.IsDir() {
		return result, gogError("install_validation_failed", result, "installed destination is missing")
	}
	uninstallTarget, err := discoverInnoUninstaller(installPath)
	if err != nil {
		return result, gogError("uninstaller_missing", result, "%v", err)
	}
	candidates, launchTarget, err := discoverLaunchTargets(installPath, request.Title)
	if err != nil {
		return result, gogError("install_validation_failed", result, "%v", err)
	}
	result.UninstallTarget = uninstallTarget
	result.LaunchCandidates = candidates
	result.LaunchTarget = launchTarget
	result.InstalledAt = i.now().UTC()
	manifest := gogInnoManifest{
		SchemaVersion: devicev1.ExecutableInstallManifestSchemaVersion, GameID: result.GameID, SourceGameID: result.SourceGameID,
		InstallRoot: result.InstallRoot, InstallPath: result.InstallPath, InstallerFamily: result.InstallerFamily,
		PrimarySHA256: result.PrimarySHA256, TotalBytes: result.TotalPackageBytes, PackageFiles: result.PackageFiles,
		SignerSubject: result.SignerSubject, SignerThumbprint: result.SignerThumbprint, InvocationMode: result.InvocationMode,
		UninstallTarget: result.UninstallTarget, LaunchTarget: result.LaunchTarget, LaunchCandidates: result.LaunchCandidates, InstalledAt: result.InstalledAt,
	}
	if err := writeGogInnoManifest(installPath, manifest); err != nil {
		return result, gogError("install_validation_failed", result, "%v", err)
	}
	if err := result.Validate(); err != nil {
		return result, gogError("install_validation_failed", result, "%v", err)
	}
	if err := reportProgress(report, "complete", "Installed", 100, "install", 100); err != nil {
		return result, err
	}
	installSucceeded = true
	return result, nil
}

// innoPathArgument formats Inno Setup path flags so values with spaces are not
// truncated. Inno documents /DIR="x:\dirname" and /LOG="x:\filename".
func innoPathArgument(prefix, path string) string {
	return prefix + `"` + path + `"`
}

func (i *ManagedGogInnoInstaller) Uninstall(ctx context.Context, request devicev1.GogInnoUninstallRequest, report CommandProgressReporter) (devicev1.GogInnoUninstallResult, error) {
	result := devicev1.GogInnoUninstallResult{GameID: request.GameID, SourceGameID: request.SourceGameID}
	if err := request.Validate(); err != nil {
		return result, gogError("uninstaller_mismatch", nil, "%v", err)
	}
	manifest, err := readGogInnoManifest(request.InstallPath)
	if err != nil {
		return result, gogError("uninstaller_mismatch", nil, "%v", err)
	}
	if manifest.SchemaVersion != devicev1.ExecutableInstallManifestSchemaVersion ||
		manifest.GameID != request.GameID || manifest.SourceGameID != request.SourceGameID ||
		manifest.InstallerFamily != devicev1.GogInnoInstallerFamily ||
		!sameRelativePath(manifest.UninstallTarget, request.UninstallTarget) {
		return result, gogError("uninstaller_mismatch", nil, "installation manifest does not match the uninstall request")
	}
	inside, boundaryErr := pathWithinRoot(manifest.InstallRoot, request.InstallPath)
	if boundaryErr != nil || !inside || filepath.Clean(request.InstallPath) == filepath.Clean(manifest.InstallRoot) {
		return result, gogError("uninstaller_mismatch", nil, "install path is outside its recorded MGA root")
	}
	target := filepath.Join(request.InstallPath, filepath.FromSlash(request.UninstallTarget))
	member, memberErr := pathWithinRoot(request.InstallPath, target)
	if memberErr != nil || !member {
		return result, gogError("uninstaller_mismatch", nil, "recorded uninstaller is outside the install path")
	}
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return result, gogError("uninstaller_missing", nil, "recorded uninstaller is missing")
	}
	if err := i.confirmer.ConfirmUninstall(ctx, UninstallConfirmationDetails{
		GameTitle: request.GameID, Publisher: gogPublisherName, InstallPath: request.InstallPath, Server: i.serverURL,
		Warning: "The game's installer will remove the game. Saves and settings may remain.",
	}, devicev1.GogInnoLocalConfirmationTimeout); err != nil {
		return result, normalizeConfirmationError(err, nil)
	}
	if err := reportProgress(report, "uninstalling", "Publisher uninstaller running", 25, "install", 0); err != nil {
		return result, err
	}
	args := []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART"}
	process, err := i.runner.Start(ctx, InstallerProcessSpec{Path: target, Arguments: args, WorkingDirectory: request.InstallPath})
	if err != nil {
		return result, normalizeStartError(err, nil)
	}
	result.ProcessID = process.PID()
	exitCode, err := process.Wait(ctx, devicev1.GogInnoUninstallerProcessTimeout)
	if err != nil {
		return result, normalizeWaitError("installer_timeout", err, result)
	}
	result.ExitCode = intPointer(exitCode)
	if exitCode != 0 {
		return result, gogError("uninstaller_exit_nonzero", result, "uninstaller exited with code %d (0x%08X)", exitCode, uint32(exitCode))
	}
	_, statErr := os.Stat(request.InstallPath)
	result.LeftoverDirectory = statErr == nil
	result.Removed = true
	if err := reportProgress(report, "complete", "Uninstalled", 100, "install", 100); err != nil {
		return result, err
	}
	return result, nil
}

func (i *ManagedGogInnoInstaller) resolveDownloadURL(raw string) (string, error) {
	server, _ := url.Parse(i.serverURL)
	download, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("package download URL is invalid")
	}
	if !download.IsAbs() {
		if download.Host != "" || !strings.HasPrefix(download.Path, "/") {
			return "", errors.New("package download URL must be origin-relative")
		}
		download = server.ResolveReference(download)
	}
	if download.User != nil || download.Fragment != "" || !strings.EqualFold(server.Scheme, download.Scheme) || !strings.EqualFold(server.Host, download.Host) {
		return "", errors.New("package download URL must use the paired MGA Server origin")
	}
	return download.String(), nil
}

func (i *ManagedGogInnoInstaller) downloadFile(ctx context.Context, rawURL, token, destination string, expected, completed, total uint64, report CommandProgressReporter) (string, uint64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := i.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("download installer package: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("download installer package: MGA Server returned %s", response.Status)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", 0, err
	}
	hasher := sha256.New()
	var written uint64
	buffer := make([]byte, 1024*1024)
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return "", written, err
		}
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			copied, writeErr := io.MultiWriter(file, hasher).Write(buffer[:n])
			written += uint64(copied)
			if writeErr != nil {
				_ = file.Close()
				return "", written, writeErr
			}
			downloadPercent := uint8(0)
			if total > 0 {
				downloadPercent = uint8(min(uint64(100), (completed+written)*100/total))
			}
			if err := reportProgress(report, "downloading", "Downloading installer", uint8(uint16(downloadPercent)*40/100), "download", downloadPercent); err != nil {
				_ = file.Close()
				return "", written, err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			_ = file.Close()
			return "", written, readErr
		}
		if written > expected {
			_ = file.Close()
			return "", written, errors.New("package exceeds declared size")
		}
	}
	if err := file.Close(); err != nil {
		return "", written, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), written, nil
}

func discoverInnoUninstaller(root string) (string, error) {
	var matches []string
	err := filepath.WalkDir(root, func(candidatePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".exe") || !strings.HasPrefix(strings.ToLower(entry.Name()), "unins") {
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
		if err := devicev1.ValidateUninstallTarget(relative); err != nil {
			return err
		}
		matches = append(matches, relative)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("discover Inno uninstaller: %w", err)
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return "", errors.New("installed game has no Inno uninstaller")
	}
	if len(matches) != 1 {
		return "", errors.New("installed game has an ambiguous Inno uninstaller set")
	}
	return matches[0], nil
}

func writeGogInnoManifest(directory string, manifest gogInnoManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, installManifestName), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write MGA executable installation manifest: %w", err)
	}
	return nil
}

func readGogInnoManifest(directory string) (gogInnoManifest, error) {
	data, err := os.ReadFile(filepath.Join(directory, installManifestName))
	if err != nil {
		return gogInnoManifest{}, fmt.Errorf("read MGA executable installation manifest: %w", err)
	}
	var manifest gogInnoManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return gogInnoManifest{}, fmt.Errorf("decode MGA executable installation manifest: %w", err)
	}
	return manifest, nil
}

func signerIsGOG(subject string) bool {
	subject = strings.TrimSpace(subject)
	if strings.EqualFold(subject, gogPublisherName) {
		return true
	}
	for _, component := range strings.Split(subject, ",") {
		parts := strings.SplitN(strings.TrimSpace(component), "=", 2)
		if len(parts) == 2 && (strings.EqualFold(strings.TrimSpace(parts[0]), "O") || strings.EqualFold(strings.TrimSpace(parts[0]), "CN")) && strings.EqualFold(strings.TrimSpace(parts[1]), gogPublisherName) {
			return true
		}
	}
	return false
}

func normalizeConfirmationError(err error, payload any) error {
	if errors.Is(err, ErrUnsupportedInstallerPlatform) {
		return gogError("unsupported_installer", payload, "%v", err)
	}
	code := "local_confirmation_declined"
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrLocalConfirmationTimeout) {
		code = "local_confirmation_timeout"
	}
	return gogError(code, payload, "%v", err)
}

func normalizeStartError(err error, payload any) error {
	code := "installer_start_failed"
	if errors.Is(err, ErrUACDeclined) {
		code = "uac_declined"
	}
	if errors.Is(err, ErrUnsupportedInstallerPlatform) {
		code = "unsupported_installer"
	}
	return gogError(code, payload, "%v", err)
}

func normalizeWaitError(defaultCode string, err error, payload any) error {
	if errors.Is(err, ErrUACDeclined) {
		return gogError("uac_declined", payload, "%v", err)
	}
	return gogError(defaultCode, payload, "%v", err)
}

func defaultInstallRoot(value string) string {
	if strings.TrimSpace(value) == "" {
		return devicev1.DefaultInstallRootTemplate
	}
	return value
}

func sameRelativePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(filepath.FromSlash(a)), filepath.Clean(filepath.FromSlash(b)))
}

func intPointer(value int) *int { return &value }

var (
	ErrLocalConfirmationTimeout     = errors.New("local confirmation timed out")
	ErrLocalConfirmationDeclined    = errors.New("local confirmation declined")
	ErrUACDeclined                  = errors.New("Windows permission request was declined")
	ErrUnsupportedInstallerPlatform = errors.New("installer platform is unsupported")
)
