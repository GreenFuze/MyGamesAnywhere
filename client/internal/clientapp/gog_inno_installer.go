package clientapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
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
	"unicode/utf16"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

const (
	gogPublisherName                    = "GOG Sp. z o.o."
	gogInnoLogName                      = "installer.log"
	gogInnoFailureMarkerName            = ".mga-failed-install.json"
	gogInnoFailureMarkerDirectory       = "failed-installs"
	maxGogInnoLogTailBytes        int64 = 1024 * 1024
)

// ADR-0023 raises successful owned manifests to schema 4 and owned failed-install
// markers to schema 3. Legacy schema-3 manifests and schema-1/2 markers remain
// readable under the fail-closed single-binding ownership migration.

type AuthenticodeVerifier interface {
	VerifyGOG(path string) (subject string, thumbprint string, err error)
}

type InnoFamilyDetector interface {
	IsInnoSetup(path string) (bool, error)
}

type UninstallConfirmationDetails struct {
	GameTitle   string
	Publisher   string
	InstallPath string
	Server      string
	Warning     string
}

type CleanupConfirmationDetails struct {
	GameTitle   string
	InstallPath string
	Server      string
	Warning     string
}

type LocalConfirmer interface {
	ConfirmUninstall(context.Context, UninstallConfirmationDetails, time.Duration) error
	ConfirmCleanup(context.Context, CleanupConfirmationDetails, time.Duration) error
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

// RegisteredProgramInspector is the narrow Windows Add/Remove Programs safety
// boundary used before MGA directly deletes a failed game folder.
type RegisteredProgramInspector interface {
	HasAssociation(installPath string) (bool, error)
}

type RegisteredProgramObservation struct {
	ProductID    string
	DisplayName  string
	Version      string
	Publisher    string
	CanUninstall bool
}

type RegisteredProgramObserver interface {
	RegisteredProgramInspector
	Associations(installPath string) ([]RegisteredProgramObservation, error)
}

type GogInnoInstaller interface {
	Install(context.Context, string, devicev1.GogInnoInstallRequest, CommandProgressReporter) (devicev1.GogInnoInstallResult, error)
	Uninstall(context.Context, devicev1.GogInnoUninstallRequest, CommandProgressReporter) (devicev1.GogInnoUninstallResult, error)
	CleanupFailed(context.Context, devicev1.GogInnoFailedCleanupRequest, CommandProgressReporter) (devicev1.GogInnoFailedCleanupResult, error)
}

type ManagedGogInnoInstaller struct {
	serverURL string
	client    *http.Client
	now       func() time.Time
	verifier  AuthenticodeVerifier
	detector  InnoFamilyDetector
	confirmer LocalConfirmer
	runner    InstallerProcessRunner
	programs  RegisteredProgramInspector
	ownership *InstallationOwnership
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
	SchemaVersion       int                           `json:"schema_version"`
	GameID              string                        `json:"game_id"`
	SourceGameID        string                        `json:"source_game_id"`
	InstallRoot         string                        `json:"install_root"`
	InstallPath         string                        `json:"install_path"`
	InstallerFamily     string                        `json:"installer_family"`
	PrimarySHA256       string                        `json:"primary_sha256"`
	TotalBytes          uint64                        `json:"total_package_bytes"`
	PackageFiles        []devicev1.GogInnoPackageFile `json:"package_files"`
	SignerSubject       string                        `json:"signer_subject"`
	SignerThumbprint    string                        `json:"signer_thumbprint"`
	InvocationMode      string                        `json:"invocation_mode"`
	UninstallTarget     string                        `json:"uninstall_target"`
	LaunchTarget        string                        `json:"launch_target,omitempty"`
	LaunchCandidates    []string                      `json:"launch_candidates,omitempty"`
	ProcessID           int                           `json:"process_id,omitempty"`
	ExitCode            *int                          `json:"exit_code,omitempty"`
	DiagnosticRef       string                        `json:"diagnostic_ref,omitempty"`
	InstalledAt         time.Time                     `json:"installed_at"`
	CompletionBasis     string                        `json:"completion_basis"`
	LocalInstallationID string                        `json:"local_installation_id,omitempty"`
	OwnerBindingID      string                        `json:"owner_binding_id,omitempty"`
	OwnershipState      string                        `json:"ownership_state,omitempty"`
}

func NewOwnedManagedGogInnoInstaller(serverURL string, verifier AuthenticodeVerifier, detector InnoFamilyDetector, confirmer LocalConfirmer, runner InstallerProcessRunner, ownership *InstallationOwnership) (*ManagedGogInnoInstaller, error) {
	installer, err := NewManagedGogInnoInstaller(serverURL, verifier, detector, confirmer, runner)
	if err != nil {
		return nil, err
	}
	if ownership == nil {
		return nil, errors.New("installation ownership is required")
	}
	installer.ownership = ownership
	return installer, nil
}

type gogInnoFailureMarker struct {
	SchemaVersion   int       `json:"schema_version"`
	MarkerID        string    `json:"marker_id"`
	CommandID       string    `json:"command_id"`
	GameID          string    `json:"game_id"`
	SourceGameID    string    `json:"source_game_id"`
	InstallRoot     string    `json:"install_root"`
	InstallPath     string    `json:"install_path"`
	InstallerFamily string    `json:"installer_family"`
	PrimarySHA256   string    `json:"primary_sha256"`
	DestinationID   string    `json:"destination_identity,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	OwnerBindingID  string    `json:"owner_binding_id,omitempty"`
}

type gogInnoFailureMarkerRecord struct {
	Marker gogInnoFailureMarker
	Path   string
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
		programs:  newRegisteredProgramInspector(),
	}, nil
}

func newPlatformGogInnoInstaller(serverURL string) (*ManagedGogInnoInstaller, error) {
	return NewManagedGogInnoInstaller(serverURL, newAuthenticodeVerifier(), newInnoFamilyDetector(), newLocalConfirmer(), newInstallerProcessRunner())
}

func newPlatformOwnedGogInnoInstaller(serverURL string, ownership *InstallationOwnership) (*ManagedGogInnoInstaller, error) {
	return NewOwnedManagedGogInnoInstaller(serverURL, newAuthenticodeVerifier(), newInnoFamilyDetector(), newLocalConfirmer(), newInstallerProcessRunner(), ownership)
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
	installRoot = i.ownership.NamespacedRoot(installRoot)
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		return result, fmt.Errorf("create install root: %w", err)
	}
	installPath := filepath.Join(installRoot, request.DestinationName)
	var ownershipOperation *OwnedInstallOperation
	if i.ownership != nil {
		productIdentity := "gog-inno:" + strings.ToLower(strings.TrimSpace(request.Installer.FileName))
		ownershipOperation, err = i.ownership.BeginInstall(devicev1.InstallKindGogInno, request.GameID, request.SourceGameID, request.Title, installRoot, installPath, productIdentity)
		if err != nil {
			return result, gogError("installation_conflict", nil, "%v", err)
		}
		defer func() {
			if ownershipOperation != nil {
				_ = ownershipOperation.Abort()
			}
		}()
	}
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
	markerCreated := false
	markerPath := ""
	defer func() {
		// Before process start, clear staging on any failure. After the native
		// installer terminates successfully, clear staging. After a started
		// installer fails or times out, keep staging so the bounded log at
		// diagnostic_ref remains available for attention_required review.
		if !processStarted || installSucceeded {
			_ = os.RemoveAll(stage)
		}
		if markerCreated && !processStarted {
			_ = removeUnstartedFailureMarker(installPath, markerPath)
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
	ownerBindingID := ""
	if i.ownership != nil {
		ownerBindingID = i.ownership.bindingID
	}
	marker, err := newGogInnoFailureMarker(commandID, request, installRoot, installPath, result.PrimarySHA256, ownerBindingID, i.now().UTC())
	if err != nil {
		return result, gogError("install_validation_failed", nil, "%v", err)
	}
	markerRecord, err := createGogInnoFailureMarker(installRoot, installPath, marker)
	if err != nil {
		return result, gogError("install_validation_failed", nil, "%v", err)
	}
	marker = markerRecord.Marker
	markerPath = markerRecord.Path
	markerCreated = true
	result.CleanupMarkerID = marker.MarkerID
	if err := reportProgress(report, "starting", "Starting installer", 45, "install", 20); err != nil {
		return result, err
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
		result = i.inventoryFailedInstall(result, marker)
		return result, gogError("installer_timeout", result, "installer started but progress reporting failed: %v", err)
	}
	exitCode, err := process.Wait(ctx, devicev1.GogInnoInstallerProcessTimeout)
	if err != nil {
		result = i.inventoryFailedInstall(result, marker)
		return result, normalizeWaitError("installer_timeout", err, result)
	}
	result.ExitCode = intPointer(exitCode)
	if exitCode == 0 {
		result.CompletionBasis = devicev1.GogInnoCompletionExitZero
	} else {
		accepted, evidenceErr := isValidatedPostSuccessCrash(exitCode, filepath.Join(stage, gogInnoLogName))
		if evidenceErr != nil || !accepted {
			result = i.inventoryFailedInstall(result, marker)
			if evidenceErr != nil {
				return result, gogError("installer_exit_nonzero", result, "installer exited with code %d (0x%08X); success evidence was unavailable: %v", exitCode, uint32(exitCode), evidenceErr)
			}
			return result, gogError("installer_exit_nonzero", result, "installer exited with code %d (0x%08X)", exitCode, uint32(exitCode))
		}
		result.CompletionBasis = devicev1.GogInnoCompletionValidatedPostSuccessCrash
	}

	if err := reportProgress(report, "checking", "Checking installed game", 90, "install", 85); err != nil {
		return result, err
	}
	info, err := os.Stat(installPath)
	if err != nil || !info.IsDir() {
		result = i.inventoryFailedInstall(result, marker)
		return result, gogError("install_validation_failed", result, "installed destination is missing")
	}
	uninstallTarget, err := discoverInnoUninstaller(installPath)
	if err != nil {
		result = i.inventoryFailedInstall(result, marker)
		return result, gogError("uninstaller_missing", result, "%v", err)
	}
	candidates, launchTarget, err := discoverLaunchTargets(installPath, request.Title)
	if err != nil {
		result = i.inventoryFailedInstall(result, marker)
		return result, gogError("install_validation_failed", result, "%v", err)
	}
	result.UninstallTarget = uninstallTarget
	result.LaunchCandidates = candidates
	result.LaunchTarget = launchTarget
	result.InstalledAt = i.now().UTC()
	manifestSchema := devicev1.LegacyExecutableInstallManifestSchemaVersion
	localInstallationID, ownerBindingID := "", ""
	if ownershipOperation != nil {
		manifestSchema = devicev1.ExecutableInstallManifestSchemaVersion
		localInstallationID = ownershipOperation.LocalInstallationID()
		ownerBindingID = ownershipOperation.OwnerBindingID()
	}
	manifest := gogInnoManifest{
		SchemaVersion: manifestSchema, GameID: result.GameID, SourceGameID: result.SourceGameID,
		InstallRoot: result.InstallRoot, InstallPath: result.InstallPath, InstallerFamily: result.InstallerFamily,
		PrimarySHA256: result.PrimarySHA256, TotalBytes: result.TotalPackageBytes, PackageFiles: result.PackageFiles,
		SignerSubject: result.SignerSubject, SignerThumbprint: result.SignerThumbprint, InvocationMode: result.InvocationMode,
		UninstallTarget: result.UninstallTarget, LaunchTarget: result.LaunchTarget, LaunchCandidates: result.LaunchCandidates, InstalledAt: result.InstalledAt,
		ProcessID: result.ProcessID, ExitCode: result.ExitCode, DiagnosticRef: result.DiagnosticRef, CompletionBasis: result.CompletionBasis,
		LocalInstallationID: localInstallationID, OwnerBindingID: ownerBindingID,
		OwnershipState: func() string {
			if ownerBindingID != "" {
				return string(OwnershipOwned)
			}
			return ""
		}(),
	}
	if err := writeGogInnoManifest(installPath, manifest); err != nil {
		result = i.inventoryFailedInstall(result, marker)
		return result, gogError("install_validation_failed", result, "%v", err)
	}
	if err := result.Validate(); err != nil {
		result = i.inventoryFailedInstall(result, marker)
		return result, gogError("install_validation_failed", result, "%v", err)
	}
	if err := os.Remove(markerPath); err != nil {
		return result, gogError("install_validation_failed", result, "remove failed-install marker after commit: %v", err)
	}
	result.CleanupMarkerID = ""
	markerCreated = false
	if ownershipOperation != nil {
		if err := ownershipOperation.Complete(); err != nil {
			ownershipOperation.LeavePending()
			ownershipOperation = nil
			return result, gogError("install_validation_failed", result, "finalize installation ownership: %v", err)
		}
		ownershipOperation = nil
	}
	if err := reportProgress(report, "complete", "Installed", 100, "install", 100); err != nil {
		return result, err
	}
	installSucceeded = true
	return result, nil
}

// innoPathArgument returns one logical argv element. os/exec and ShellExecute
// own Windows command-line quoting; embedding literal Inno quotes here would
// pass backslash-escaped quotes to the installer and break its path parser.
func innoPathArgument(prefix, path string) string {
	return prefix + path
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
	if (manifest.SchemaVersion != devicev1.LegacyExecutableInstallManifestSchemaVersion && manifest.SchemaVersion != devicev1.ExecutableInstallManifestSchemaVersion) ||
		manifest.GameID != request.GameID || manifest.SourceGameID != request.SourceGameID ||
		manifest.InstallerFamily != devicev1.GogInnoInstallerFamily ||
		!sameRelativePath(manifest.UninstallTarget, request.UninstallTarget) {
		return result, gogError("uninstaller_mismatch", nil, "installation manifest does not match the uninstall request")
	}
	inside, boundaryErr := pathWithinRoot(manifest.InstallRoot, request.InstallPath)
	if boundaryErr != nil || !inside || filepath.Clean(request.InstallPath) == filepath.Clean(manifest.InstallRoot) {
		return result, gogError("uninstaller_mismatch", nil, "install path is outside its recorded MGA root")
	}
	common, commonErr := readInstallManifest(request.InstallPath)
	if commonErr != nil {
		return result, gogError("installation_owner_mismatch", nil, "%v", commonErr)
	}
	common, commonErr = ensureInstallationManifestOwnership(i.ownership, request.InstallPath, common)
	if commonErr != nil {
		return result, gogError("installation_owner_mismatch", nil, "%v", commonErr)
	}
	manifest.LocalInstallationID, manifest.OwnerBindingID, manifest.OwnershipState = common.LocalInstallationID, common.OwnerBindingID, common.OwnershipState
	mutation, err := i.ownership.AuthorizeMutation(manifest.LocalInstallationID, manifest.OwnerBindingID, request.InstallPath)
	if err != nil {
		return result, gogError("installation_owner_mismatch", nil, "%v", err)
	}
	if mutation != nil {
		defer mutation.Close()
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
	if mutation != nil {
		if err := mutation.Removed(); err != nil {
			return result, gogError("installation_owner_mismatch", result, "remove installation ownership: %v", err)
		}
	}
	if err := reportProgress(report, "complete", "Uninstalled", 100, "install", 100); err != nil {
		return result, err
	}
	return result, nil
}

func (i *ManagedGogInnoInstaller) CleanupFailed(ctx context.Context, request devicev1.GogInnoFailedCleanupRequest, report CommandProgressReporter) (devicev1.GogInnoFailedCleanupResult, error) {
	result := devicev1.GogInnoFailedCleanupResult{
		GameID: request.GameID, SourceGameID: request.SourceGameID, SystemChangesMayRemain: true,
	}
	if err := request.Validate(); err != nil {
		return result, gogError("cleanup_marker_mismatch", nil, "%v", err)
	}
	inside, boundaryErr := pathWithinRoot(request.InstallRoot, request.InstallPath)
	if boundaryErr != nil || !inside || filepath.Clean(request.InstallRoot) == filepath.Clean(request.InstallPath) {
		return result, gogError("cleanup_boundary_failed", nil, "failed install path is outside its recorded root")
	}
	if reparse, err := isFilesystemReparsePoint(request.InstallRoot); err != nil || reparse {
		return result, gogError("cleanup_boundary_failed", nil, "install root is unavailable or a reparse point")
	}
	if reparse, err := isFilesystemReparsePoint(request.InstallPath); err != nil || reparse {
		return result, gogError("cleanup_boundary_failed", nil, "failed install path is unavailable or a reparse point")
	}
	markerRecord, err := readGogInnoFailureMarker(request.InstallRoot, request.InstallPath, request.CleanupMarkerID)
	if err != nil {
		return result, gogError("cleanup_marker_missing", nil, "%v", err)
	}
	if err := validateFailureMarker(markerRecord.Marker, request); err != nil {
		return result, gogError("cleanup_marker_mismatch", nil, "%v", err)
	}
	if i.ownership != nil {
		if markerRecord.Marker.OwnerBindingID == "" && i.ownership.bindingCount != 1 {
			return result, gogError("cleanup_marker_mismatch", nil, "legacy failed-install ownership is ambiguous across MGA servers")
		}
		if markerRecord.Marker.OwnerBindingID != "" && !strings.EqualFold(markerRecord.Marker.OwnerBindingID, i.ownership.bindingID) {
			return result, gogError("cleanup_marker_mismatch", nil, "failed install is managed by another MGA server")
		}
	}
	if err := i.confirmer.ConfirmCleanup(ctx, CleanupConfirmationDetails{
		GameTitle: request.GameID, InstallPath: request.InstallPath, Server: i.serverURL,
		Warning: "Files in this failed game folder will be removed. Windows components may remain.",
	}, devicev1.GogInnoLocalConfirmationTimeout); err != nil {
		return result, normalizeConfirmationError(err, nil)
	}
	if err := reportProgress(report, "cleanup", "Cleaning up failed install", 20, "install", 10); err != nil {
		return result, err
	}
	if request.UninstallTarget != "" {
		target := filepath.Join(request.InstallPath, filepath.FromSlash(request.UninstallTarget))
		member, memberErr := pathWithinRoot(request.InstallPath, target)
		info, statErr := os.Lstat(target)
		if memberErr != nil || !member || statErr != nil || !info.Mode().IsRegular() {
			return result, gogError("cleanup_marker_mismatch", nil, "recorded failed-install uninstaller is unavailable")
		}
		if reparse, reparseErr := isFilesystemReparsePoint(target); reparseErr != nil || reparse {
			return result, gogError("cleanup_boundary_failed", nil, "recorded failed-install uninstaller is a reparse point")
		}
		process, startErr := i.runner.Start(ctx, InstallerProcessSpec{
			Path: target, Arguments: []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART"}, WorkingDirectory: request.InstallPath,
		})
		if startErr != nil {
			return result, normalizeStartError(startErr, result)
		}
		result.PublisherUninstallerUsed = true
		result.ProcessID = process.PID()
		exitCode, waitErr := process.Wait(ctx, devicev1.GogInnoUninstallerProcessTimeout)
		result.ExitCode = intPointer(exitCode)
		if waitErr != nil {
			return result, gogError("cleanup_uninstaller_failed", result, "publisher uninstaller failed: %v", waitErr)
		}
		if exitCode != 0 {
			return result, gogError("cleanup_uninstaller_failed", result, "publisher uninstaller exited with code %d (0x%08X)", exitCode, uint32(exitCode))
		}
	}
	if _, statErr := os.Lstat(request.InstallPath); statErr == nil {
		if i.programs == nil {
			return result, gogError("cleanup_registered_program_present", result, "registered-program inspection is unavailable")
		}
		registered, registryErr := i.programs.HasAssociation(request.InstallPath)
		if registryErr != nil {
			return result, gogError("cleanup_registered_program_present", result, "inspect Windows registered programs: %v", registryErr)
		}
		if registered {
			return result, gogError("cleanup_registered_program_present", result, "Windows still has a registered program for this game folder; use its publisher uninstaller")
		}
		if err := boundedRemoveTree(request.InstallRoot, request.InstallPath); err != nil {
			result.LeftoverDirectory = true
			return result, gogError("cleanup_failed", result, "%v", err)
		}
		result.BoundedDeleteUsed = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return result, gogError("cleanup_failed", result, "inspect failed install after cleanup: %v", statErr)
	}
	_ = os.Remove(markerRecord.Path)
	_, statErr := os.Lstat(request.InstallPath)
	result.LeftoverDirectory = statErr == nil
	if result.LeftoverDirectory {
		return result, gogError("cleanup_failed", result, "failed install directory remains")
	}
	result.Removed = true
	result.Summary = "Failed game folder removed; shared Windows components may remain."
	if err := reportProgress(report, "complete", "Cleaned up", 100, "install", 100); err != nil {
		return result, err
	}
	return result, nil
}

func newGogInnoFailureMarker(commandID string, request devicev1.GogInnoInstallRequest, installRoot, installPath, primarySHA256, ownerBindingID string, createdAt time.Time) (gogInnoFailureMarker, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return gogInnoFailureMarker{}, fmt.Errorf("create failed-install marker ID: %w", err)
	}
	schema := 2
	if ownerBindingID != "" {
		schema = 3
	}
	return gogInnoFailureMarker{
		SchemaVersion: schema, MarkerID: base64.RawURLEncoding.EncodeToString(random), CommandID: commandID,
		GameID: request.GameID, SourceGameID: request.SourceGameID, InstallRoot: installRoot, InstallPath: installPath,
		InstallerFamily: devicev1.GogInnoInstallerFamily, PrimarySHA256: primarySHA256, CreatedAt: createdAt, OwnerBindingID: ownerBindingID,
	}, nil
}

func failureMarkerSidecarPath(installRoot, markerID string) (string, error) {
	if strings.TrimSpace(markerID) == "" || filepath.Base(markerID) != markerID || strings.ContainsAny(markerID, `/\\:`) {
		return "", errors.New("failed-install marker ID is unsafe")
	}
	path := filepath.Join(installRoot, ".mga", gogInnoFailureMarkerDirectory, markerID+".json")
	inside, err := pathWithinRoot(installRoot, path)
	if err != nil || !inside {
		return "", errors.New("failed-install marker sidecar is outside the install root")
	}
	return path, nil
}

func createGogInnoFailureMarker(installRoot, installPath string, marker gogInnoFailureMarker) (gogInnoFailureMarkerRecord, error) {
	if err := os.Mkdir(installPath, 0o700); err != nil {
		return gogInnoFailureMarkerRecord{}, fmt.Errorf("create failed-install destination: %w", err)
	}
	destinationID, err := filesystemObjectIdentity(installPath)
	if err != nil {
		_ = os.Remove(installPath)
		return gogInnoFailureMarkerRecord{}, fmt.Errorf("identify failed-install destination: %w", err)
	}
	marker.DestinationID = destinationID
	markerPath, err := failureMarkerSidecarPath(installRoot, marker.MarkerID)
	if err != nil {
		_ = os.Remove(installPath)
		return gogInnoFailureMarkerRecord{}, err
	}
	markerDirectory := filepath.Dir(markerPath)
	if err := os.MkdirAll(markerDirectory, 0o700); err != nil {
		_ = os.Remove(installPath)
		return gogInnoFailureMarkerRecord{}, fmt.Errorf("create failed-install marker directory: %w", err)
	}
	if reparse, err := isFilesystemReparsePoint(markerDirectory); err != nil || reparse {
		_ = os.Remove(installPath)
		return gogInnoFailureMarkerRecord{}, errors.New("failed-install marker directory is unavailable or a reparse point")
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		_ = os.Remove(installPath)
		return gogInnoFailureMarkerRecord{}, err
	}
	temporary := markerPath + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		_ = os.Remove(installPath)
		return gogInnoFailureMarkerRecord{}, fmt.Errorf("write failed-install marker: %w", err)
	}
	if err := os.Rename(temporary, markerPath); err != nil {
		_ = os.Remove(temporary)
		_ = os.Remove(installPath)
		return gogInnoFailureMarkerRecord{}, fmt.Errorf("commit failed-install marker: %w", err)
	}
	return gogInnoFailureMarkerRecord{Marker: marker, Path: markerPath}, nil
}

func readGogInnoFailureMarkerAt(markerPath string) (gogInnoFailureMarker, error) {
	info, err := os.Lstat(markerPath)
	if err != nil || !info.Mode().IsRegular() {
		return gogInnoFailureMarker{}, errors.New("failed-install cleanup marker is missing or irregular")
	}
	if reparse, reparseErr := isFilesystemReparsePoint(markerPath); reparseErr != nil || reparse {
		return gogInnoFailureMarker{}, errors.New("failed-install cleanup marker is a reparse point")
	}
	data, err := os.ReadFile(markerPath)
	if err != nil {
		return gogInnoFailureMarker{}, fmt.Errorf("read failed-install marker: %w", err)
	}
	var marker gogInnoFailureMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return gogInnoFailureMarker{}, fmt.Errorf("decode failed-install marker: %w", err)
	}
	return marker, nil
}

func readGogInnoFailureMarker(installRoot, installPath, markerID string) (gogInnoFailureMarkerRecord, error) {
	sidecar, sidecarErr := failureMarkerSidecarPath(installRoot, markerID)
	if sidecarErr == nil {
		marker, err := readGogInnoFailureMarkerAt(sidecar)
		if err == nil {
			return gogInnoFailureMarkerRecord{Marker: marker, Path: sidecar}, nil
		}
	}
	// Schema-1 records are intentionally retained only for cleanup compatibility.
	legacy := filepath.Join(installPath, gogInnoFailureMarkerName)
	marker, err := readGogInnoFailureMarkerAt(legacy)
	if err != nil {
		return gogInnoFailureMarkerRecord{}, err
	}
	return gogInnoFailureMarkerRecord{Marker: marker, Path: legacy}, nil
}

func validateFailureMarker(marker gogInnoFailureMarker, request devicev1.GogInnoFailedCleanupRequest) error {
	if (marker.SchemaVersion != 1 && marker.SchemaVersion != 2 && marker.SchemaVersion != 3) || marker.MarkerID != request.CleanupMarkerID || marker.GameID != request.GameID ||
		marker.SourceGameID != request.SourceGameID || marker.InstallerFamily != request.InstallerFamily ||
		strings.TrimSpace(marker.CommandID) == "" || filepath.Base(marker.CommandID) != marker.CommandID || strings.ContainsAny(marker.CommandID, `/\:`) || marker.CreatedAt.IsZero() ||
		!strings.EqualFold(marker.PrimarySHA256, request.PrimarySHA256) ||
		!strings.EqualFold(filepath.Clean(marker.InstallRoot), filepath.Clean(request.InstallRoot)) ||
		!strings.EqualFold(filepath.Clean(marker.InstallPath), filepath.Clean(request.InstallPath)) {
		return errors.New("failed-install cleanup marker does not match the persisted installation")
	}
	if marker.SchemaVersion >= 2 {
		currentID, err := filesystemObjectIdentity(request.InstallPath)
		if err != nil || currentID != marker.DestinationID || strings.TrimSpace(marker.DestinationID) == "" {
			return errors.New("failed-install destination identity does not match the cleanup marker")
		}
	}
	return nil
}

func removeUnstartedFailureMarker(installPath, markerPath string) error {
	if markerPath != "" {
		_ = os.Remove(markerPath)
	}
	entries, err := os.ReadDir(installPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || len(entries) != 0 {
		return err
	}
	return os.Remove(installPath)
}

func (i *ManagedGogInnoInstaller) inventoryFailedInstall(result devicev1.GogInnoInstallResult, marker gogInnoFailureMarker) devicev1.GogInnoInstallResult {
	persistedRecord, err := readGogInnoFailureMarker(result.InstallRoot, result.InstallPath, marker.MarkerID)
	persisted := persistedRecord.Marker
	if err != nil || persisted.SchemaVersion != marker.SchemaVersion || persisted.MarkerID != marker.MarkerID || persisted.CommandID != marker.CommandID ||
		persisted.GameID != marker.GameID || persisted.SourceGameID != marker.SourceGameID || persisted.InstallerFamily != marker.InstallerFamily || persisted.DestinationID != marker.DestinationID ||
		!strings.EqualFold(persisted.PrimarySHA256, marker.PrimarySHA256) || !strings.EqualFold(filepath.Clean(persisted.InstallRoot), filepath.Clean(marker.InstallRoot)) ||
		!strings.EqualFold(filepath.Clean(persisted.InstallPath), filepath.Clean(marker.InstallPath)) || !persisted.CreatedAt.Equal(marker.CreatedAt) {
		result.CleanupMarkerID = ""
		return result
	}
	result.CleanupMarkerID = marker.MarkerID
	if target, err := discoverInnoUninstaller(result.InstallPath); err == nil {
		result.UninstallTarget = target
	}
	return result
}

func isValidatedPostSuccessCrash(exitCode int, logPath string) (bool, error) {
	if uint32(exitCode) != uint32(0xC000041D) {
		return false, nil
	}
	text, err := readBoundedInnoLogTail(logPath)
	if err != nil {
		return false, err
	}
	lower := strings.ToLower(text)
	if !strings.Contains(lower, strings.ToLower("Installation process succeeded")) {
		return false, nil
	}
	for _, failure := range []string{"Installation process failed", "Rolling back changes", "Rollback failed"} {
		if strings.Contains(lower, strings.ToLower(failure)) {
			return false, nil
		}
	}
	return true, nil
}

func readBoundedInnoLogTail(logPath string) (string, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("installer diagnostic log is unavailable")
	}
	size := min(info.Size(), maxGogInnoLogTailBytes)
	data := make([]byte, size)
	if _, err := file.ReadAt(data, info.Size()-size); err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if len(data) >= 2 && ((data[0] == 0xff && data[1] == 0xfe) || looksLikeUTF16LE(data)) {
		if len(data)%2 != 0 {
			data = data[1:]
		}
		words := make([]uint16, 0, len(data)/2)
		for index := 0; index+1 < len(data); index += 2 {
			words = append(words, binary.LittleEndian.Uint16(data[index:index+2]))
		}
		return string(utf16.Decode(words)), nil
	}
	return string(data), nil
}

func looksLikeUTF16LE(data []byte) bool {
	limit := min(len(data), 256)
	if limit < 4 {
		return false
	}
	nulls := 0
	for index := 1; index < limit; index += 2 {
		if data[index] == 0 {
			nulls++
		}
	}
	return nulls*2 >= limit/2
}

func boundedRemoveTree(root, target string) error {
	inside, err := pathWithinRoot(root, target)
	if err != nil || !inside || filepath.Clean(root) == filepath.Clean(target) {
		return errors.New("refusing cleanup outside the recorded install root")
	}
	return removeTreeEntryNoFollow(target)
}

func removeTreeEntryNoFollow(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	reparse, err := isFilesystemReparsePoint(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || reparse {
		return os.Remove(path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := removeTreeEntryNoFollow(filepath.Join(path, entry.Name())); err != nil {
			return err
		}
	}
	return os.Remove(path)
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
