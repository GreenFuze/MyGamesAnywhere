package clientapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

// InstallationPreflightEvaluator performs read-only, allow-listed checks on
// the target endpoint. It never installs software or executes a requirement.
type InstallationPreflightEvaluator struct {
	inventory InventoryCollector
	diskFree  func(string) (uint64, error)
}

func NewInstallationPreflightEvaluator(inventory InventoryCollector) *InstallationPreflightEvaluator {
	return &InstallationPreflightEvaluator{inventory: inventory, diskFree: availableDiskBytes}
}

func (e *InstallationPreflightEvaluator) Evaluate(ctx context.Context, request devicev1.InstallationPreflightRequest) (devicev1.InstallationPreflightResult, error) {
	if err := request.Validate(); err != nil {
		return devicev1.InstallationPreflightResult{}, err
	}
	root, err := expandInstallRoot(request.DestinationRoot)
	if err != nil {
		return devicev1.InstallationPreflightResult{}, fmt.Errorf("resolve destination folder: %w", err)
	}
	checks := []devicev1.InstallationPreflightCheck{e.storageCheck(root, request.RequiredStorageBytes)}
	switch request.Category {
	case devicev1.InstallationCategoryNativeInstaller:
		checks = append(checks, devicev1.InstallationPreflightCheck{
			ID: "prerequisites", Name: "Game components", Kind: "prerequisites",
			Status: devicev1.PreflightCheckInstallerManaged, Message: "The game installer will handle its own components.",
		})
	case devicev1.InstallationCategoryManagedArchive:
		checks = append(checks, devicev1.InstallationPreflightCheck{
			ID: "prerequisites", Name: "Game components", Kind: "prerequisites",
			Status: devicev1.PreflightCheckUnknown, Message: "MGA cannot yet tell whether this archive needs extra components or contains another installer.",
		})
	default:
		runtimeChecks, collectErr := e.runtimeChecks(ctx, request.Requirements)
		if collectErr != nil {
			return devicev1.InstallationPreflightResult{}, collectErr
		}
		checks = append(checks, runtimeChecks...)
	}
	result := devicev1.InstallationPreflightResult{SchemaVersion: devicev1.InstallationPreflightSchemaVersion, CanInstall: true, Checks: checks}
	for _, check := range checks {
		if check.Required && check.Status == devicev1.PreflightCheckMissing {
			result.CanInstall = false
		}
	}
	if err := result.Validate(); err != nil {
		return devicev1.InstallationPreflightResult{}, err
	}
	return result, nil
}

func (e *InstallationPreflightEvaluator) storageCheck(root string, required uint64) devicev1.InstallationPreflightCheck {
	check := devicev1.InstallationPreflightCheck{ID: "storage", Name: "Free space", Kind: "storage", Required: required > 0, RequiredBytes: required}
	if required == 0 {
		check.Status = devicev1.PreflightCheckUnknown
		check.Message = "The required space is not known yet. The installer will check again before changing files."
		return check
	}
	probeRoot, err := nearestExistingDirectory(root)
	if err != nil {
		check.Status = devicev1.PreflightCheckUnknown
		check.Message = "MGA could not read free space for the selected folder."
		return check
	}
	if e == nil || e.diskFree == nil {
		check.Status = devicev1.PreflightCheckUnknown
		check.Message = "MGA could not read free space for the selected folder."
		return check
	}
	available, err := e.diskFree(probeRoot)
	if err != nil {
		check.Status = devicev1.PreflightCheckUnknown
		check.Message = "MGA could not read free space for the selected folder."
		return check
	}
	check.AvailableBytes = available
	if available < required {
		check.Status = devicev1.PreflightCheckMissing
		check.Message = "This device does not have enough free space for the package."
		return check
	}
	check.Status = devicev1.PreflightCheckReady
	check.Message = "This device has enough free space for the package download."
	return check
}

func (e *InstallationPreflightEvaluator) runtimeChecks(ctx context.Context, requirements []devicev1.PrerequisiteRequirement) ([]devicev1.InstallationPreflightCheck, error) {
	if e == nil || e.inventory == nil {
		return nil, fmt.Errorf("device inventory collector is unavailable")
	}
	inventory, err := e.inventory.Collect(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect prerequisite inventory: %w", err)
	}
	found := map[string]bool{}
	for _, runtime := range inventory.Runtimes {
		found[runtime.ID] = true
	}
	checks := make([]devicev1.InstallationPreflightCheck, 0, len(requirements))
	for _, requirement := range requirements {
		runtimeID, supported := preflightRuntimeID(requirement.ID)
		check := devicev1.InstallationPreflightCheck{ID: requirement.ID, Name: requirement.Name, Kind: string(requirement.Kind), Required: requirement.Required}
		switch {
		case !supported:
			check.Status = devicev1.PreflightCheckUnknown
			check.Message = fmt.Sprintf("MGA cannot reliably check %s on this device yet.", requirement.Name)
		case found[runtimeID]:
			check.Status = devicev1.PreflightCheckReady
			check.Message = fmt.Sprintf("%s is available on this device.", requirement.Name)
		default:
			check.Status = devicev1.PreflightCheckMissing
			check.Message = fmt.Sprintf("%s was not found for this Windows user.", requirement.Name)
		}
		checks = append(checks, check)
	}
	return checks, nil
}

func preflightRuntimeID(requirementID string) (string, bool) {
	allowed := map[string]string{
		"storefront.steam":     "steam",
		"emulator.retroarch":   "retroarch",
		"emulator.scummvm":     "scummvm",
		"emulator.dosbox":      "dosbox",
		"emulator.duckstation": "duckstation",
		"emulator.pcsx2":       "pcsx2",
	}
	runtimeID, ok := allowed[strings.TrimSpace(requirementID)]
	return runtimeID, ok
}

func nearestExistingDirectory(path string) (string, error) {
	current := filepath.Clean(path)
	for {
		info, err := os.Stat(current)
		if err == nil {
			if info.IsDir() {
				return current, nil
			}
			return filepath.Dir(current), nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing parent for %s", path)
		}
		current = parent
	}
}
