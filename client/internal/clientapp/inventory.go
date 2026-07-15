package clientapp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
	now func() time.Time
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
	inventory := devicev1.DeviceInventory{
		SchemaVersion: devicev1.InventorySchemaVersion,
		CapturedAt:    c.now().UTC(),
		Storage:       storage,
		Runtimes:      collectKnownRuntimes(),
	}.Normalize()
	if err := inventory.Validate(); err != nil {
		return devicev1.DeviceInventory{}, err
	}
	return inventory, nil
}

type runtimeCandidate struct {
	id        string
	name      string
	commands  []string
	fixedPath []string
}

func collectKnownRuntimes() []devicev1.RuntimeInventory {
	programFiles := os.Getenv("ProgramFiles")
	programFilesX86 := os.Getenv("ProgramFiles(x86)")
	candidates := []runtimeCandidate{
		{id: "steam", name: "Steam", commands: []string{"steam.exe", "steam"}, fixedPath: []string{filepath.Join(programFilesX86, "Steam", "steam.exe"), filepath.Join(programFiles, "Steam", "steam.exe")}},
		{id: "retroarch", name: "RetroArch", commands: []string{"retroarch.exe", "retroarch"}, fixedPath: []string{filepath.Join(programFiles, "RetroArch-Win64", "retroarch.exe"), filepath.Join(programFiles, "RetroArch", "retroarch.exe")}},
		{id: "scummvm", name: "ScummVM", commands: []string{"scummvm.exe", "scummvm"}, fixedPath: []string{filepath.Join(programFiles, "ScummVM", "scummvm.exe"), filepath.Join(programFilesX86, "ScummVM", "scummvm.exe")}},
		{id: "dosbox", name: "DOSBox", commands: []string{"dosbox.exe", "dosbox", "dosbox-x.exe", "dosbox-x"}},
		{id: "duckstation", name: "DuckStation", commands: []string{"duckstation-qt-x64-ReleaseLTCG.exe", "duckstation"}},
		{id: "pcsx2", name: "PCSX2", commands: []string{"pcsx2-qt.exe", "pcsx2"}},
	}
	if runtime.GOOS != "windows" {
		for index := range candidates {
			candidates[index].fixedPath = nil
		}
	}
	result := make([]devicev1.RuntimeInventory, 0, len(candidates))
	for _, candidate := range candidates {
		if path := findRuntime(candidate); path != "" {
			result = append(result, devicev1.RuntimeInventory{ID: candidate.id, Name: candidate.name, Path: path})
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result
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
