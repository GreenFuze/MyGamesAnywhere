package clientapp

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

const maxEmulatorConfigBytes int64 = 1024 * 1024

// LocalSaveAdapterDiscoverer reads only fixed, emulator-owned configuration
// locations and emits capability facts without paths or filenames.
type LocalSaveAdapterDiscoverer struct {
	goos   string
	getenv func(string) string
}

func NewLocalSaveAdapterDiscoverer() *LocalSaveAdapterDiscoverer {
	return &LocalSaveAdapterDiscoverer{goos: runtime.GOOS, getenv: os.Getenv}
}

func (d *LocalSaveAdapterDiscoverer) Discover(runtimes []devicev1.RuntimeInventory) []devicev1.SaveAdapterInventory {
	result := make([]devicev1.SaveAdapterInventory, 0, 2)
	for _, emulator := range runtimes {
		switch emulator.ID {
		case "retroarch":
			result = append(result, d.discoverRetroArch(emulator.Path))
		case "scummvm":
			result = append(result, d.discoverScummVM(emulator.Path))
		}
	}
	return result
}

func (d *LocalSaveAdapterDiscoverer) discoverRetroArch(executable string) devicev1.SaveAdapterInventory {
	adapter := devicev1.SaveAdapterInventory{ID: "retroarch", Name: "RetroArch", ProbeState: "complete", SaveKinds: []string{"save_ram", "save_state"}}
	configPath := firstExistingFile(retroArchConfigCandidates(executable, d.getenv))
	if configPath == "" {
		return adapter
	}
	values, _, err := readBoundedINI(configPath)
	if err != nil {
		adapter.ProbeState = "partial"
		return adapter
	}
	configDir := configuredDirectory(values["rgui_config_directory"], filepath.Dir(executable), "config")
	adapter.RouteOverrides = containsBoundedOverride(configDir, configPath)
	return adapter
}

func (d *LocalSaveAdapterDiscoverer) discoverScummVM(executable string) devicev1.SaveAdapterInventory {
	adapter := devicev1.SaveAdapterInventory{ID: "scummvm", Name: "ScummVM", ProbeState: "complete", SaveKinds: []string{"save_file"}}
	configPath := firstExistingFile(scummVMConfigCandidates(executable, d.goos, d.getenv))
	if configPath == "" {
		return adapter
	}
	_, sectionKeys, err := readBoundedINI(configPath)
	if err != nil {
		adapter.ProbeState = "partial"
		return adapter
	}
	for section, keys := range sectionKeys {
		if section != "" && section != "scummvm" && keys["savepath"] != "" {
			adapter.RouteOverrides = true
			break
		}
	}
	return adapter
}

func retroArchConfigCandidates(executable string, getenv func(string) string) []string {
	result := []string{filepath.Join(filepath.Dir(executable), "retroarch.cfg")}
	if root := strings.TrimSpace(getenv("APPDATA")); root != "" {
		result = append(result, filepath.Join(root, "RetroArch", "retroarch.cfg"))
	}
	return result
}

func scummVMConfigCandidates(executable, goos string, getenv func(string) string) []string {
	result := []string{filepath.Join(filepath.Dir(executable), "scummvm.ini")}
	switch goos {
	case "windows":
		if root := strings.TrimSpace(getenv("APPDATA")); root != "" {
			result = append(result, filepath.Join(root, "ScummVM", "scummvm.ini"))
		}
	case "darwin":
		if home := strings.TrimSpace(getenv("HOME")); home != "" {
			result = append(result, filepath.Join(home, "Library", "Preferences", "ScummVM Preferences"))
		}
	default:
		root := strings.TrimSpace(getenv("XDG_CONFIG_HOME"))
		if root == "" {
			root = filepath.Join(strings.TrimSpace(getenv("HOME")), ".config")
		}
		if root != ".config" {
			result = append(result, filepath.Join(root, "scummvm", "scummvm.ini"))
		}
	}
	return result
}

func firstExistingFile(paths []string) string {
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}

func readBoundedINI(path string) (map[string]string, map[string]map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if info.Size() > maxEmulatorConfigBytes {
		return nil, nil, errors.New("emulator configuration exceeds size limit")
	}

	global := map[string]string{}
	sections := map[string]map[string]string{"": global}
	section := ""
	scanner := bufio.NewScanner(io.LimitReader(file, maxEmulatorConfigBytes+1))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")))
			if sections[section] == nil {
				sections[section] = map[string]string{}
			}
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.Trim(strings.TrimSpace(parts[1]), `"`)
		sections[section][key] = value
		if section == "" || section == "scummvm" {
			global[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return global, sections, nil
}

func configuredDirectory(value, base, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "default" {
		return filepath.Join(base, fallback)
	}
	value = os.ExpandEnv(value)
	if !filepath.IsAbs(value) {
		value = filepath.Join(base, value)
	}
	return filepath.Clean(value)
}

func containsBoundedOverride(root, primaryConfig string) bool {
	seen := 0
	found := false
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		seen++
		if seen > 512 {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".cfg") && !strings.EqualFold(filepath.Clean(path), filepath.Clean(primaryConfig)) {
			found = true
		}
		return nil
	})
	return found
}
