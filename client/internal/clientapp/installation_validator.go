package clientapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type InstallationValidator interface {
	Validate(context.Context, devicev1.InstallationValidationRequest, CommandProgressReporter) (devicev1.InstallationValidationResult, error)
}

type LocalInstallationValidator struct {
	programs RegisteredProgramInspector
	now      func() time.Time
}

func NewLocalInstallationValidator(programs RegisteredProgramInspector) (*LocalInstallationValidator, error) {
	if programs == nil {
		return nil, errors.New("registered program inspector is required")
	}
	return &LocalInstallationValidator{programs: programs, now: time.Now}, nil
}

func (v *LocalInstallationValidator) Validate(ctx context.Context, request devicev1.InstallationValidationRequest, report CommandProgressReporter) (devicev1.InstallationValidationResult, error) {
	var result devicev1.InstallationValidationResult
	if v == nil || v.programs == nil || v.now == nil {
		return result, errors.New("installation validator is unavailable")
	}
	if err := request.Validate(); err != nil {
		return result, err
	}
	for index, item := range request.Items {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		percent := uint8(index * 100 / len(request.Items))
		if err := reportProgress(report, "checking", fmt.Sprintf("Checking installed game %d of %d", index+1, len(request.Items)), percent, "", 0); err != nil {
			return result, err
		}
		checked, err := v.validateOne(item)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, checked)
		switch checked.State {
		case devicev1.InstallStateInstalled:
			result.Installed++
		case devicev1.InstallStateMissing:
			result.Missing++
		case devicev1.InstallStateNeedsRepair:
			result.NeedsRepair++
		}
	}
	if err := reportProgress(report, "complete", "Installed games checked", 100, "", 0); err != nil {
		return result, err
	}
	return result, result.Validate()
}

func (v *LocalInstallationValidator) validateOne(item devicev1.InstallationValidationRequestItem) (devicev1.InstallationValidationResultItem, error) {
	checked := devicev1.InstallationValidationResultItem{
		GameID: item.GameID, SourceGameID: item.SourceGameID, CheckedAt: v.now().UTC(),
	}
	inside, err := pathWithinRoot(item.InstallRoot, item.InstallPath)
	if err != nil || !inside || strings.EqualFold(filepath.Clean(item.InstallRoot), filepath.Clean(item.InstallPath)) {
		return checked, errors.New("installation path is outside its recorded MGA root")
	}

	registered := false
	if item.InstallKind == devicev1.InstallKindGogInno {
		registered, err = v.programs.HasAssociation(item.InstallPath)
		if err != nil {
			return checked, fmt.Errorf("inspect Add/Remove Programs association: %w", err)
		}
		checked.RegisteredProgram = boolPointer(registered)
	}

	info, err := os.Lstat(item.InstallPath)
	if errors.Is(err, os.ErrNotExist) {
		if registered {
			return validationRepair(checked, devicev1.ValidationReasonFilesMissingRegistrationPresent, 0), nil
		}
		checked.State = devicev1.InstallStateMissing
		checked.ReasonCode = devicev1.ValidationReasonInstallPathMissing
		return checked, nil
	}
	if err != nil {
		return checked, fmt.Errorf("inspect installation path: %w", err)
	}
	if !info.IsDir() {
		return validationRepair(checked, devicev1.ValidationReasonManifestInvalid, 0), nil
	}
	reparse, err := isFilesystemReparsePoint(item.InstallPath)
	if err != nil {
		return checked, fmt.Errorf("inspect installation boundary: %w", err)
	}
	if reparse {
		return validationRepair(checked, devicev1.ValidationReasonUnsafeReparsePoint, 0), nil
	}

	switch item.InstallKind {
	case devicev1.InstallKindManagedArchive:
		manifest, readErr := readInstallManifest(item.InstallPath)
		if readErr != nil {
			return manifestReadFailure(checked, readErr), nil
		}
		checked.ManifestSchema = manifest.SchemaVersion
		if manifest.SchemaVersion != 1 && manifest.SchemaVersion != devicev1.InstallManifestSchemaVersion {
			return validationRepair(checked, devicev1.ValidationReasonManifestSchemaUnsupported, manifest.SchemaVersion), nil
		}
		if manifest.GameID != item.GameID || manifest.SourceGameID != item.SourceGameID || !strings.EqualFold(filepath.Clean(manifest.InstallRoot), filepath.Clean(item.InstallRoot)) {
			return validationRepair(checked, devicev1.ValidationReasonManifestIdentityMismatch, manifest.SchemaVersion), nil
		}
		if item.LaunchTarget != "" && !sameRelativePath(manifest.LaunchTarget, item.LaunchTarget) {
			return validationRepair(checked, devicev1.ValidationReasonManifestIdentityMismatch, manifest.SchemaVersion), nil
		}
	case devicev1.InstallKindGogInno:
		manifest, readErr := readGogInnoManifest(item.InstallPath)
		if readErr != nil {
			return manifestReadFailure(checked, readErr), nil
		}
		checked.ManifestSchema = manifest.SchemaVersion
		if manifest.SchemaVersion != devicev1.ExecutableInstallManifestSchemaVersion {
			return validationRepair(checked, devicev1.ValidationReasonManifestSchemaUnsupported, manifest.SchemaVersion), nil
		}
		if manifest.GameID != item.GameID || manifest.SourceGameID != item.SourceGameID || manifest.InstallerFamily != devicev1.GogInnoInstallerFamily ||
			!strings.EqualFold(filepath.Clean(manifest.InstallRoot), filepath.Clean(item.InstallRoot)) || !strings.EqualFold(filepath.Clean(manifest.InstallPath), filepath.Clean(item.InstallPath)) {
			return validationRepair(checked, devicev1.ValidationReasonManifestIdentityMismatch, manifest.SchemaVersion), nil
		}
		if !sameRelativePath(manifest.UninstallTarget, item.UninstallTarget) || (item.LaunchTarget != "" && !sameRelativePath(manifest.LaunchTarget, item.LaunchTarget)) {
			return validationRepair(checked, devicev1.ValidationReasonManifestIdentityMismatch, manifest.SchemaVersion), nil
		}
		if reason, fileErr := validateRecordedRegularFile(item.InstallPath, item.UninstallTarget, devicev1.ValidationReasonUninstallTargetMissing); fileErr != nil {
			return checked, fileErr
		} else if reason != "" {
			return validationRepair(checked, reason, manifest.SchemaVersion), nil
		}
		if !registered {
			return validationRepair(checked, devicev1.ValidationReasonRegisteredProgramMissing, manifest.SchemaVersion), nil
		}
	default:
		return checked, fmt.Errorf("unsupported install kind %q", item.InstallKind)
	}

	if item.LaunchTarget != "" {
		if reason, fileErr := validateRecordedRegularFile(item.InstallPath, item.LaunchTarget, devicev1.ValidationReasonLaunchTargetMissing); fileErr != nil {
			return checked, fileErr
		} else if reason != "" {
			return validationRepair(checked, reason, checked.ManifestSchema), nil
		}
	}
	checked.State = devicev1.InstallStateInstalled
	checked.ReasonCode = devicev1.ValidationReasonHealthy
	return checked, nil
}

func manifestReadFailure(checked devicev1.InstallationValidationResultItem, err error) devicev1.InstallationValidationResultItem {
	reason := devicev1.ValidationReasonManifestInvalid
	if errors.Is(err, os.ErrNotExist) {
		reason = devicev1.ValidationReasonManifestMissing
	}
	return validationRepair(checked, reason, 0)
}

func validationRepair(checked devicev1.InstallationValidationResultItem, reason string, schema int) devicev1.InstallationValidationResultItem {
	checked.State = devicev1.InstallStateNeedsRepair
	checked.ReasonCode = reason
	checked.ManifestSchema = schema
	return checked
}

func validateRecordedRegularFile(root, relative, missingReason string) (string, error) {
	if relative == "" {
		return missingReason, nil
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	inside, err := pathWithinRoot(root, target)
	if err != nil || !inside {
		return "", errors.New("recorded installation target is outside its installation path")
	}
	current := filepath.Clean(root)
	parts := strings.Split(filepath.Clean(filepath.FromSlash(relative)), string(filepath.Separator))
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			return missingReason, nil
		}
		if statErr != nil {
			return "", statErr
		}
		reparse, reparseErr := isFilesystemReparsePoint(current)
		if reparseErr != nil {
			return "", reparseErr
		}
		if reparse {
			return devicev1.ValidationReasonUnsafeReparsePoint, nil
		}
		if current == target && !info.Mode().IsRegular() {
			return missingReason, nil
		}
	}
	return "", nil
}

func boolPointer(value bool) *bool { return &value }
