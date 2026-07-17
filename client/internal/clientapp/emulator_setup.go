package clientapp

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type EmulatorSetupManager interface {
	Setup(context.Context, devicev1.EmulatorSetupRequest, CommandProgressReporter) (devicev1.EmulatorSetupResult, error)
}

type packageSetupRunner interface {
	Run(context.Context, string, string) error
}

type wingetSetupRunner struct{}

func (wingetSetupRunner) Run(ctx context.Context, packageID, action string) error {
	verb := "install"
	if action == "update" {
		verb = "upgrade"
	}
	arguments := []string{verb, "--id", packageID, "--exact", "--source", "winget", "--silent", "--disable-interactivity", "--accept-package-agreements", "--accept-source-agreements"}
	if action == "update" {
		arguments = append(arguments, "--include-unknown")
	}
	command := exec.CommandContext(ctx, "winget.exe", arguments...)
	output := &boundedCommandOutput{limit: 256 * 1024}
	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(output.String())
		if message == "" {
			return fmt.Errorf("winget %s failed: %w", action, err)
		}
		return fmt.Errorf("winget %s failed: %w: %s", action, err, message)
	}
	return nil
}

type ManagedEmulatorSetupManager struct {
	inventory InventoryCollector
	runner    packageSetupRunner
	packages  map[string]string
}

func NewManagedEmulatorSetupManager(inventory InventoryCollector) (*ManagedEmulatorSetupManager, error) {
	if inventory == nil {
		return nil, errors.New("emulator inventory collector is required")
	}
	return &ManagedEmulatorSetupManager{
		inventory: inventory,
		runner:    wingetSetupRunner{},
		packages: map[string]string{
			"retroarch":   "Libretro.RetroArch",
			"scummvm":     "ScummVM.ScummVM",
			"dosbox":      "DOSBox.DOSBox",
			"duckstation": "Stenzek.DuckStation",
			"pcsx2":       "PCSX2Team.PCSX2",
		},
	}, nil
}

func (m *ManagedEmulatorSetupManager) Setup(ctx context.Context, request devicev1.EmulatorSetupRequest, report CommandProgressReporter) (devicev1.EmulatorSetupResult, error) {
	if m == nil || m.inventory == nil || m.runner == nil {
		return devicev1.EmulatorSetupResult{}, errors.New("emulator setup manager is unavailable")
	}
	if err := request.Validate(); err != nil {
		return devicev1.EmulatorSetupResult{}, err
	}
	packageID := m.packages[request.EmulatorID]
	if packageID == "" {
		return devicev1.EmulatorSetupResult{}, fmt.Errorf("emulator %s is not allow-listed for managed setup", request.EmulatorID)
	}
	if err := reportProgress(report, "checking", "Checking current emulator", 5, "setup", 5); err != nil {
		return devicev1.EmulatorSetupResult{}, err
	}
	before, err := m.inventory.Collect(ctx)
	if err != nil {
		return devicev1.EmulatorSetupResult{}, fmt.Errorf("collect inventory before setup: %w", err)
	}
	previous, wasDetected := detectedRuntime(before, request.EmulatorID)
	phase := "Installing emulator"
	phaseID := "installing"
	if request.Action == "update" {
		phase = "Checking for emulator updates"
		phaseID = "updating"
	}
	if err := reportProgress(report, phaseID, phase, 20, "setup", 20); err != nil {
		return devicev1.EmulatorSetupResult{}, err
	}
	if err := m.runner.Run(ctx, packageID, request.Action); err != nil {
		return devicev1.EmulatorSetupResult{}, err
	}
	if err := reportProgress(report, "refreshing", "Refreshing emulator details", 90, "setup", 90); err != nil {
		return devicev1.EmulatorSetupResult{}, err
	}
	after, err := m.inventory.Collect(ctx)
	if err != nil {
		return devicev1.EmulatorSetupResult{}, fmt.Errorf("collect inventory after setup: %w", err)
	}
	current, detected := detectedRuntime(after, request.EmulatorID)
	if !detected {
		return devicev1.EmulatorSetupResult{}, errors.New("package manager completed but the emulator is not detectable for this device user")
	}
	result := devicev1.EmulatorSetupResult{EmulatorID: request.EmulatorID, Action: request.Action, PreviousVersion: previous.Version, CurrentVersion: current.Version, State: "completed"}
	switch {
	case request.Action == "install" && !wasDetected:
		result.State, result.Changed = "installed", true
	case request.Action == "update" && previous.Version != "" && current.Version != "" && previous.Version != current.Version:
		result.State, result.Changed = "updated", true
	case previous.Version != "" && current.Version == previous.Version:
		result.State = "already_current"
	}
	if err := reportProgress(report, "complete", "Emulator setup complete", 100, "setup", 100); err != nil {
		return devicev1.EmulatorSetupResult{}, err
	}
	return result, result.Validate()
}

func detectedRuntime(inventory devicev1.DeviceInventory, emulatorID string) (devicev1.RuntimeInventory, bool) {
	for _, runtime := range inventory.Runtimes {
		if runtime.ID == emulatorID {
			return runtime, true
		}
	}
	return devicev1.RuntimeInventory{}, false
}
