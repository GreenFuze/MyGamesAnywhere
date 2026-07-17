package clientapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

func TestSaveAdapterDiscoveryFindsScummVMGlobalAndRouteOverrideWithoutPaths(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "ScummVM", "scummvm.exe")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	configRoot := filepath.Join(root, "AppData")
	configPath := filepath.Join(configRoot, "ScummVM", "scummvm.ini")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("[scummvm]\nsavepath=C:\\\\Saves\n[monkey2]\nsavepath=C:\\\\Saves\\\\Monkey2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	discoverer := &LocalSaveAdapterDiscoverer{goos: "windows", getenv: func(key string) string {
		if key == "APPDATA" {
			return configRoot
		}
		return ""
	}}
	adapters := discoverer.Discover([]devicev1.RuntimeInventory{{ID: "scummvm", Name: "ScummVM", Path: executable}})
	if len(adapters) != 1 || adapters[0].ProbeState != "complete" || !adapters[0].RouteOverrides {
		t.Fatalf("ScummVM adapter = %#v", adapters)
	}
	encoded := strings.ToLower(strings.TrimSpace(strings.Join(adapters[0].SaveKinds, ",")))
	if encoded != "save_file" || strings.Contains(encoded, "saves") {
		t.Fatalf("adapter leaked path or wrong kinds: %#v", adapters[0])
	}
}

func TestSaveAdapterDiscoveryUsesRetroArchDefaultsAndBoundsConfig(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "RetroArch", "retroarch.exe")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	discoverer := &LocalSaveAdapterDiscoverer{goos: "windows", getenv: func(string) string { return "" }}

	adapters := discoverer.Discover([]devicev1.RuntimeInventory{{ID: "retroarch", Name: "RetroArch", Path: executable}})
	if len(adapters) != 1 || adapters[0].ProbeState != "complete" || len(adapters[0].SaveKinds) != 2 {
		t.Fatalf("default RetroArch adapter = %#v", adapters)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(executable), "retroarch.cfg"), []byte(strings.Repeat("x", int(maxEmulatorConfigBytes+1))), 0o644); err != nil {
		t.Fatal(err)
	}
	adapters = discoverer.Discover([]devicev1.RuntimeInventory{{ID: "retroarch", Name: "RetroArch", Path: executable}})
	if adapters[0].ProbeState != "partial" {
		t.Fatalf("oversized RetroArch adapter = %#v", adapters[0])
	}
}
