package clientapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

const (
	archivePackageReadyToRun = "ready_to_run"
	archivePackageGogInno    = "gog_inno"
)

type archivePackageClassification struct {
	Kind       string
	Installer  string
	Companions []string
}

type ArchivePackageInstaller interface {
	Install(context.Context, string, devicev1.ArchivePackageInstallRequest, CommandProgressReporter) (devicev1.ArchivePackageInstallResult, error)
}

// ManagedArchivePackageInstaller owns the post-extraction decision. The server
// supplies only an outer archive; inner process authority is never remote.
type ManagedArchivePackageInstaller struct {
	archive *ManagedArchiveInstaller
	gogInno *ManagedGogInnoInstaller
}

type ArchivePackageCommandError struct {
	Code    string
	Message string
	Payload any
}

func (e *ArchivePackageCommandError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func NewManagedArchivePackageInstaller(archive *ManagedArchiveInstaller, gogInno *ManagedGogInnoInstaller) (*ManagedArchivePackageInstaller, error) {
	if archive == nil || gogInno == nil {
		return nil, errors.New("archive and GOG Inno installers are required")
	}
	return &ManagedArchivePackageInstaller{archive: archive, gogInno: gogInno}, nil
}

func (i *ManagedArchivePackageInstaller) Install(ctx context.Context, commandID string, request devicev1.ArchivePackageInstallRequest, report CommandProgressReporter) (devicev1.ArchivePackageInstallResult, error) {
	if i == nil || i.archive == nil || i.gogInno == nil {
		return devicev1.ArchivePackageInstallResult{}, errors.New("archive package installer is unavailable")
	}
	prepared, err := i.archive.prepare(ctx, commandID, request, archivePackagePreparationReporter(report))
	if err != nil {
		return devicev1.ArchivePackageInstallResult{}, err
	}
	defer prepared.Cleanup()

	classification, err := classifyExtractedArchive(prepared.contentDir)
	if err != nil {
		return devicev1.ArchivePackageInstallResult{}, fmt.Errorf("cannot safely install %s: %w", request.ArchiveName, err)
	}
	switch classification.Kind {
	case archivePackageReadyToRun:
		installed, err := i.archive.commitPrepared(prepared, report)
		if err != nil {
			return devicev1.ArchivePackageInstallResult{}, err
		}
		result := devicev1.ArchivePackageInstallResult{ResolvedKind: devicev1.ArchivePackageKindManagedArchive, Archive: &installed}
		return result, result.Validate()
	case archivePackageGogInno:
		installed, err := i.installStagedGogInno(ctx, commandID, prepared, classification, archivePackageNativeReporter(report))
		if err != nil {
			var commandError *GogInnoCommandError
			if errors.As(err, &commandError) {
				var payload any
				if partial, ok := commandError.Payload.(devicev1.GogInnoInstallResult); ok {
					payload = devicev1.ArchivePackageInstallResult{
						ResolvedKind: devicev1.ArchivePackageKindGogInno,
						GogInno: &devicev1.GogInnoArchiveInstallResult{
							GogInnoInstallResult: partial,
							Container: devicev1.ArchiveContainerEvidence{
								FileName: request.ArchiveName, Format: request.ArchiveFormat,
								SizeBytes: prepared.bytes, SHA256: prepared.sha256,
							},
						},
					}
				}
				return devicev1.ArchivePackageInstallResult{}, &ArchivePackageCommandError{Code: commandError.Code, Message: commandError.Message, Payload: payload}
			}
			return devicev1.ArchivePackageInstallResult{}, err
		}
		wrapped := devicev1.GogInnoArchiveInstallResult{
			GogInnoInstallResult: installed,
			Container: devicev1.ArchiveContainerEvidence{
				FileName: request.ArchiveName, Format: request.ArchiveFormat,
				SizeBytes: prepared.bytes, SHA256: prepared.sha256,
			},
		}
		result := devicev1.ArchivePackageInstallResult{ResolvedKind: devicev1.ArchivePackageKindGogInno, GogInno: &wrapped}
		return result, result.Validate()
	default:
		return devicev1.ArchivePackageInstallResult{}, fmt.Errorf("unsupported archive package classification %q", classification.Kind)
	}
}

func (i *ManagedArchivePackageInstaller) installStagedGogInno(ctx context.Context, commandID string, prepared *preparedArchivePackage, classification archivePackageClassification, report CommandProgressReporter) (devicev1.GogInnoInstallResult, error) {
	request, transport, err := stagedGogInnoRequest(commandID, prepared.request, classification)
	if err != nil {
		return devicev1.GogInnoInstallResult{}, err
	}
	installer := *i.gogInno
	installer.client = &http.Client{Transport: transport}
	installer.packageContainer = &devicev1.ArchiveContainerEvidence{
		FileName: prepared.request.ArchiveName, Format: prepared.request.ArchiveFormat,
		SizeBytes: prepared.bytes, SHA256: prepared.sha256,
	}
	return installer.Install(ctx, commandID+"-native", request, report)
}

func classifyExtractedArchive(root string) (archivePackageClassification, error) {
	if strings.TrimSpace(root) == "" {
		return archivePackageClassification{}, errors.New("extracted archive root is required")
	}
	var setupCandidates, dangerous, companions []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("archive contains unsupported non-regular file %q", entry.Name())
		}
		base := entry.Name()
		if devicev1.IsGogInnoSetupFileName(base) {
			setupCandidates = append(setupCandidates, path)
		}
		if devicev1.IsGogInnoCompanionFileName(base) {
			companions = append(companions, path)
		}
		if isArchiveExecutableContent(base) {
			dangerous = append(dangerous, path)
		}
		return nil
	})
	if err != nil {
		return archivePackageClassification{}, err
	}
	sort.Strings(setupCandidates)
	sort.Strings(dangerous)
	sort.Strings(companions)

	if len(setupCandidates) == 0 {
		for _, path := range dangerous {
			base := strings.ToLower(filepath.Base(path))
			ext := strings.ToLower(filepath.Ext(base))
			if ext != ".exe" || strings.Contains(strings.TrimSuffix(base, ext), "setup") || strings.Contains(strings.TrimSuffix(base, ext), "install") {
				return archivePackageClassification{}, fmt.Errorf("unsupported installer or script %q", relativeArchivePath(root, path))
			}
		}
		return archivePackageClassification{Kind: archivePackageReadyToRun}, nil
	}
	if len(setupCandidates) != 1 {
		return archivePackageClassification{}, fmt.Errorf("archive contains %d setup candidates", len(setupCandidates))
	}
	installer := setupCandidates[0]
	installerDir := filepath.Dir(installer)
	stem := devicev1.GogInnoSetupStem(filepath.Base(installer))
	for _, path := range dangerous {
		if sameLocalPath(path, installer) {
			continue
		}
		return archivePackageClassification{}, fmt.Errorf("installer package is mixed with executable or script %q", relativeArchivePath(root, path))
	}
	for _, companion := range companions {
		if !sameLocalPath(filepath.Dir(companion), installerDir) || !strings.EqualFold(devicev1.GogInnoCompanionStem(filepath.Base(companion)), stem) {
			return archivePackageClassification{}, fmt.Errorf("installer companion %q does not match the setup package", relativeArchivePath(root, companion))
		}
	}
	return archivePackageClassification{Kind: archivePackageGogInno, Installer: installer, Companions: companions}, nil
}

func isArchiveExecutableContent(name string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(name))) {
	case ".exe", ".msi", ".com", ".scr", ".bat", ".cmd", ".ps1", ".vbs", ".js", ".wsf":
		return true
	default:
		return false
	}
}

func relativeArchivePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(relative)
}

type stagedPackageTransfer struct {
	path  string
	token string
	size  int64
}

type stagedPackageTransport struct {
	files map[string]stagedPackageTransfer
}

func (t *stagedPackageTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.Method != http.MethodGet {
		return nil, errors.New("staged package transfer requires GET")
	}
	transfer, ok := t.files[request.URL.Path]
	if !ok || request.Header.Get("Authorization") != "Bearer "+transfer.token {
		return &http.Response{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized", Header: make(http.Header), Body: http.NoBody, Request: request}, nil
	}
	file, err := os.Open(transfer.path)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: file,
		ContentLength: transfer.size, Request: request,
	}, nil
}

func stagedGogInnoRequest(commandID string, source devicev1.ArchiveInstallRequest, classification archivePackageClassification) (devicev1.GogInnoInstallRequest, *stagedPackageTransport, error) {
	paths := append([]string{classification.Installer}, classification.Companions...)
	descriptors := make([]devicev1.PackageTransferDescriptor, 0, len(paths))
	transport := &stagedPackageTransport{files: make(map[string]stagedPackageTransfer, len(paths))}
	for index, path := range paths {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
			return devicev1.GogInnoInstallRequest{}, nil, fmt.Errorf("inspect staged installer file %q", filepath.Base(path))
		}
		keyBytes := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", commandID, index, path)))
		token := hex.EncodeToString(keyBytes[:])
		urlPath := fmt.Sprintf("/__mga/archive-package/%d", index)
		role := devicev1.PackageTransferRoleCompanion
		if index == 0 {
			role = devicev1.PackageTransferRoleInstaller
		}
		descriptors = append(descriptors, devicev1.PackageTransferDescriptor{
			FileName: filepath.Base(path), Role: role, SizeBytes: uint64(info.Size()), DownloadURL: urlPath, DownloadToken: token,
		})
		transport.files[urlPath] = stagedPackageTransfer{path: path, token: token, size: info.Size()}
	}
	request := devicev1.GogInnoInstallRequest{
		GameID: source.GameID, SourceGameID: source.SourceGameID, Title: source.Title,
		DestinationRoot: source.DestinationRoot, DestinationName: source.DestinationName,
		Installer: descriptors[0], Companions: descriptors[1:],
	}
	if err := request.Validate(); err != nil {
		return devicev1.GogInnoInstallRequest{}, nil, err
	}
	return request, transport, nil
}

func archivePackagePreparationReporter(report CommandProgressReporter) CommandProgressReporter {
	return func(update CommandProgressUpdate) error {
		if update.Stage == "download" {
			update.Percent = uint8(uint16(update.StagePercent) * 30 / 100)
			return reportProgress(report, update.Phase, update.Message, update.Percent, "download", update.StagePercent)
		}
		stagePercent := uint8(5 + uint16(update.StagePercent)*25/100)
		return reportProgress(report, update.Phase, update.Message, 30+stagePercent/2, "install", stagePercent)
	}
}

func archivePackageNativeReporter(report CommandProgressReporter) CommandProgressReporter {
	return func(update CommandProgressUpdate) error {
		if update.Stage == "download" {
			stagePercent := uint8(30 + uint16(update.StagePercent)*10/100)
			return reportProgress(report, "preparing", "Preparing installer package", 45+uint8(uint16(update.StagePercent)*5/100), "install", stagePercent)
		}
		stagePercent := uint8(40 + uint16(update.StagePercent)*60/100)
		return reportProgress(report, update.Phase, update.Message, 50+uint8(uint16(update.StagePercent)*50/100), "install", stagePercent)
	}
}
