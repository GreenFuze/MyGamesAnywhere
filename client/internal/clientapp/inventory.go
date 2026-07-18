package clientapp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type InventoryCollector interface {
	Collect(context.Context) (devicev1.DeviceInventory, error)
}

// LocalInventoryCollector owns the bounded, platform-aware inventory probes.
// It never enumerates arbitrary installed applications or environment values.
type LocalInventoryCollector struct {
	now       func() time.Time
	ownership *OwnershipCatalog
	bindingID string
}

func NewOwnedLocalInventoryCollector(ownership *OwnershipCatalog, bindingID string) *LocalInventoryCollector {
	return &LocalInventoryCollector{now: time.Now, ownership: ownership, bindingID: bindingID}
}

func NewLocalInventoryCollector() *LocalInventoryCollector {
	return &LocalInventoryCollector{now: time.Now}
}

func (c *LocalInventoryCollector) Collect(ctx context.Context) (devicev1.DeviceInventory, error) {
	if err := ctx.Err(); err != nil {
		return devicev1.DeviceInventory{}, err
	}
	storage, err := collectLocalStorage()
	if err != nil {
		return devicev1.DeviceInventory{}, err
	}
	runtimes := collectKnownRuntimes(ctx)
	inventory := devicev1.DeviceInventory{
		SchemaVersion:        devicev1.InventorySchemaVersion,
		CapturedAt:           c.now().UTC(),
		Storage:              storage,
		Runtimes:             runtimes,
		PackageManagers:      collectKnownPackageManagers(),
		SaveAdapters:         NewLocalSaveAdapterDiscoverer().Discover(runtimes),
		ManagedInstallations: c.managedInstallationObservations(),
	}.Normalize()
	if err := inventory.Validate(); err != nil {
		return devicev1.DeviceInventory{}, err
	}
	return inventory, nil
}

func (c *LocalInventoryCollector) managedInstallationObservations() []devicev1.ManagedInstallationObservation {
	if c == nil || c.ownership == nil || strings.TrimSpace(c.bindingID) == "" {
		return nil
	}
	records := c.ownership.List()
	result := make([]devicev1.ManagedInstallationObservation, 0, len(records))
	for _, record := range records {
		title := strings.TrimSpace(record.Title)
		if title == "" {
			title = "Managed game"
		}
		item := devicev1.ManagedInstallationObservation{LocalInstallationID: record.LocalInstallationID, InstallKind: record.InstallKind, Title: title}
		switch record.State {
		case OwnershipReleased:
			item.State = "released"
			item.InstallPath = record.InstallPath
			item.CanAdopt = true
		case OwnershipLegacyUnclaimed:
			item.State = "legacy_unclaimed"
			item.InstallPath = record.InstallPath
		case OwnershipInterrupted:
			if strings.EqualFold(record.OwnerBindingID, c.bindingID) {
				item.State = "interrupted"
				item.InstallPath = record.InstallPath
			} else {
				item.State = "managed_elsewhere"
			}
		case OwnershipInstalling:
			if strings.EqualFold(record.OwnerBindingID, c.bindingID) {
				item.State = "installing_here"
				item.InstallPath = record.InstallPath
			} else {
				item.State = "installing_elsewhere"
			}
		case OwnershipOwned:
			if strings.EqualFold(record.OwnerBindingID, c.bindingID) {
				item.State = "managed_here"
				item.InstallPath = record.InstallPath
				item.CanManage = true
			} else {
				item.State = "managed_elsewhere"
			}
		default:
			continue
		}
		result = append(result, item)
	}
	return result
}

type runtimeCandidate struct {
	id          string
	name        string
	commands    []string
	fixedPath   []string
	versionArgs []string
}

func collectKnownRuntimes(ctx context.Context) []devicev1.RuntimeInventory {
	programFiles := os.Getenv("ProgramFiles")
	programFilesX86 := os.Getenv("ProgramFiles(x86)")
	localAppData := os.Getenv("LOCALAPPDATA")
	candidates := []runtimeCandidate{
		{id: "steam", name: "Steam", commands: []string{"steam.exe", "steam"}, fixedPath: []string{filepath.Join(programFilesX86, "Steam", "steam.exe"), filepath.Join(programFiles, "Steam", "steam.exe")}},
		{id: "retroarch", name: "RetroArch", commands: []string{"retroarch.exe", "retroarch"}, versionArgs: []string{"--version"}, fixedPath: []string{filepath.Join(localAppData, "Programs", "RetroArch-Win64", "retroarch.exe"), filepath.Join(programFiles, "RetroArch-Win64", "retroarch.exe"), filepath.Join(programFiles, "RetroArch", "retroarch.exe")}},
		{id: "scummvm", name: "ScummVM", commands: []string{"scummvm.exe", "scummvm"}, versionArgs: []string{"--version"}, fixedPath: []string{filepath.Join(programFiles, "ScummVM", "scummvm.exe"), filepath.Join(programFilesX86, "ScummVM", "scummvm.exe")}},
		{id: "dosbox", name: "DOSBox", commands: []string{"dosbox.exe", "dosbox", "dosbox-x.exe", "dosbox-x"}, versionArgs: []string{"--version"}, fixedPath: []string{filepath.Join(programFiles, "DOSBox-0.74-3", "DOSBox.exe"), filepath.Join(localAppData, "Programs", "DOSBox Staging", "dosbox.exe")}},
		{id: "duckstation", name: "DuckStation", commands: []string{"duckstation-qt-x64-ReleaseLTCG.exe", "duckstation"}, fixedPath: []string{filepath.Join(localAppData, "Programs", "DuckStation", "duckstation-qt-x64-ReleaseLTCG.exe"), filepath.Join(programFiles, "DuckStation", "duckstation-qt-x64-ReleaseLTCG.exe")}},
		{id: "pcsx2", name: "PCSX2", commands: []string{"pcsx2-qt.exe", "pcsx2"}, fixedPath: []string{filepath.Join(localAppData, "Programs", "PCSX2", "pcsx2-qt.exe"), filepath.Join(programFiles, "PCSX2", "pcsx2-qt.exe")}},
	}
	if runtime.GOOS != "windows" {
		for index := range candidates {
			candidates[index].fixedPath = nil
		}
	}
	result := make([]devicev1.RuntimeInventory, 0, len(candidates))
	for _, candidate := range candidates {
		if path := findRuntime(candidate); path != "" {
			runtime := devicev1.RuntimeInventory{ID: candidate.id, Name: candidate.name, Path: path, Version: probeRuntimeVersion(ctx, path, candidate.versionArgs)}
			if candidate.id == "retroarch" {
				runtime.CoreProbeState, runtime.Components = discoverRetroArchCores(path)
				firmwareState, firmware := discoverRetroArchFirmware(path)
				runtime.FirmwareProbeState = firmwareState
				runtime.Components = append(runtime.Components, firmware...)
			}
			result = append(result, runtime)
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result
}

type firmwareRequirement struct {
	id         string
	name       string
	files      []string
	requireAll bool
}

var retroArchFirmwareRequirements = []firmwareRequirement{
	{id: "ps1_bios", name: "PlayStation BIOS set", files: []string{"scph5500.bin", "scph5501.bin", "scph5502.bin"}, requireAll: true},
	{id: "segacd_bios", name: "Sega CD BIOS set", files: []string{"bios_CD_E.bin", "bios_CD_J.bin", "bios_CD_U.bin"}, requireAll: true},
}

func discoverRetroArchFirmware(executable string) (string, []devicev1.RuntimeComponentInventory) {
	systemDirectory := retroArchConfiguredDirectory(executable, "system_directory", "system")
	info, err := os.Stat(systemDirectory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "complete", nil
		}
		return "partial", nil
	}
	if !info.IsDir() {
		return "partial", nil
	}
	components := make([]devicev1.RuntimeComponentInventory, 0, len(retroArchFirmwareRequirements))
	for _, requirement := range retroArchFirmwareRequirements {
		matches := 0
		for _, fileName := range requirement.files {
			fileInfo, statErr := os.Stat(filepath.Join(systemDirectory, fileName))
			if statErr == nil && fileInfo.Mode().IsRegular() {
				matches++
			}
		}
		present := matches > 0
		if requirement.requireAll {
			present = matches == len(requirement.files)
		}
		if present {
			components = append(components, devicev1.RuntimeComponentInventory{Kind: "firmware", ID: requirement.id, Name: requirement.name})
		}
	}
	return "complete", components
}

func collectKnownPackageManagers() []devicev1.PackageManagerInventory {
	if runtime.GOOS != "windows" {
		return nil
	}
	if _, err := exec.LookPath("winget.exe"); err != nil {
		if _, err = exec.LookPath("winget"); err != nil {
			return nil
		}
	}
	return []devicev1.PackageManagerInventory{{ID: "winget", Name: "Windows Package Manager"}}
}

var runtimeVersionPattern = regexp.MustCompile(`\b\d+\.\d+(?:\.\d+){0,2}\b`)

func probeRuntimeVersion(ctx context.Context, executable string, arguments []string) string {
	if len(arguments) == 0 {
		return ""
	}
	probeContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	command := exec.CommandContext(probeContext, executable, arguments...)
	output := &boundedCommandOutput{limit: 64 * 1024}
	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil || probeContext.Err() != nil {
		return ""
	}
	return runtimeVersionPattern.FindString(output.String())
}

type boundedCommandOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (b *boundedCommandOutput) Write(value []byte) (int, error) {
	original := len(value)
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = b.buffer.Write(value)
	}
	return original, nil
}

func (b *boundedCommandOutput) String() string { return b.buffer.String() }

func discoverRetroArchCores(executable string) (string, []devicev1.RuntimeComponentInventory) {
	coreDirectory := retroArchConfiguredDirectory(executable, "libretro_directory", "cores")
	entries, err := os.ReadDir(coreDirectory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "complete", nil
		}
		return "partial", nil
	}
	components := make([]devicev1.RuntimeComponentInventory, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || len(components) >= 256 {
			continue
		}
		name := strings.ToLower(entry.Name())
		var id string
		switch {
		case strings.HasSuffix(name, "_libretro.dll"):
			id = strings.TrimSuffix(name, "_libretro.dll")
		case runtime.GOOS != "windows" && strings.HasSuffix(name, "_libretro.so"):
			id = strings.TrimSuffix(name, "_libretro.so")
		case runtime.GOOS == "darwin" && strings.HasSuffix(name, "_libretro.dylib"):
			id = strings.TrimSuffix(name, "_libretro.dylib")
		default:
			continue
		}
		if !runtimeComponentIDPattern.MatchString(id) {
			continue
		}
		components = append(components, devicev1.RuntimeComponentInventory{Kind: "core", ID: id, Name: coreDisplayName(id)})
	}
	sort.Slice(components, func(left, right int) bool { return components[left].ID < components[right].ID })
	return "complete", components
}

var runtimeComponentIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func retroArchConfiguredDirectory(executable, key, fallback string) string {
	executableDirectory := filepath.Dir(executable)
	configCandidates := []string{filepath.Join(executableDirectory, "retroarch.cfg")}
	if configRoot := strings.TrimSpace(os.Getenv("APPDATA")); configRoot != "" {
		configCandidates = append(configCandidates, filepath.Join(configRoot, "RetroArch", "retroarch.cfg"))
	}
	for _, configPath := range configCandidates {
		file, err := os.Open(configPath)
		if err != nil {
			continue
		}
		reader := bufio.NewReader(io.LimitReader(file, 1024*1024))
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			parts := strings.SplitN(scanner.Text(), "=", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) != key {
				continue
			}
			value := strings.Trim(strings.TrimSpace(parts[1]), `"`)
			_ = file.Close()
			if value == "" || value == "default" {
				break
			}
			value = os.ExpandEnv(value)
			if !filepath.IsAbs(value) {
				value = filepath.Join(executableDirectory, value)
			}
			return filepath.Clean(value)
		}
		_ = file.Close()
	}
	return filepath.Join(executableDirectory, fallback)
}

func coreDisplayName(id string) string {
	known := map[string]string{
		"beetle_psx": "Beetle PSX", "beetle_psx_hw": "Beetle PSX HW", "mednafen_psx": "Beetle PSX", "mednafen_psx_hw": "Beetle PSX HW",
		"dosbox_pure": "DOSBox Pure", "fceumm": "FCEUmm", "genesis_plus_gx": "Genesis Plus GX", "mesen_s": "Mesen-S",
		"mgba": "mGBA", "mupen64plus_next": "Mupen64Plus-Next", "parallel_n64": "ParaLLEl N64", "picodrive": "PicoDrive",
		"quicknes": "QuickNES", "sameboy": "SameBoy", "snes9x": "Snes9x", "swanstation": "SwanStation",
	}
	if name := known[id]; name != "" {
		return name
	}
	return strings.ReplaceAll(id, "_", " ")
}

func findRuntime(candidate runtimeCandidate) string {
	for _, command := range candidate.commands {
		if path, err := exec.LookPath(command); err == nil {
			if absolute, err := filepath.Abs(path); err == nil {
				return absolute
			}
			return path
		}
	}
	for _, path := range candidate.fixedPath {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}
