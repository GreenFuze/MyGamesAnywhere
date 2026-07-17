package clientapp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverRetroArchCoresUsesConfiguredBoundedDirectory(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "retroarch.exe")
	if err := os.WriteFile(executable, []byte("exe"), 0o600); err != nil {
		t.Fatal(err)
	}
	cores := filepath.Join(root, "custom-cores")
	if err := os.MkdirAll(cores, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"snes9x_libretro.dll", "mupen64plus_next_libretro.dll", "ignore.txt"} {
		if err := os.WriteFile(filepath.Join(cores, name), []byte("core"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := `libretro_directory = "custom-cores"` + "\n"
	if err := os.WriteFile(filepath.Join(root, "retroarch.cfg"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDATA", filepath.Join(root, "unused"))

	state, components := discoverRetroArchCores(executable)
	if state != "complete" || len(components) != 2 || components[0].ID != "mupen64plus_next" || components[1].ID != "snes9x" {
		t.Fatalf("state=%q components=%#v", state, components)
	}
	for _, component := range components {
		if component.Kind != "core" || component.Name == "" {
			t.Fatalf("component = %#v", component)
		}
	}
}

func TestDiscoverRetroArchCoresReportsEmptyDirectoryAsComplete(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "retroarch.exe")
	if err := os.WriteFile(executable, []byte("exe"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, components := discoverRetroArchCores(executable)
	if state != "complete" || len(components) != 0 {
		t.Fatalf("state=%q components=%#v", state, components)
	}
}

func TestDiscoverRetroArchFirmwareReportsOnlyCompleteAllowlistedSets(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "retroarch.exe")
	if err := os.WriteFile(executable, []byte("exe"), 0o600); err != nil {
		t.Fatal(err)
	}
	system := filepath.Join(root, "firmware")
	if err := os.MkdirAll(system, 0o700); err != nil {
		t.Fatal(err)
	}
	config := `system_directory = "firmware"` + "\n"
	if err := os.WriteFile(filepath.Join(root, "retroarch.cfg"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"scph5500.bin", "scph5501.bin", "scph5502.bin", "bios_CD_U.bin"} {
		if err := os.WriteFile(filepath.Join(system, name), []byte("user supplied"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	state, components := discoverRetroArchFirmware(executable)
	if state != "complete" || len(components) != 1 || components[0].ID != "ps1_bios" || components[0].Kind != "firmware" {
		t.Fatalf("state=%q components=%#v", state, components)
	}
}
