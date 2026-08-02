//go:build windows

package clientapp

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unsafe"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var steamVDFPairPattern = regexp.MustCompile(`^\s*"([^"]+)"\s+"(.*)"\s*$`)

type windowsStorefrontObserver struct{}
type windowsStorefrontLauncher struct{}

func newStorefrontProductObserver() StorefrontProductObserver { return windowsStorefrontObserver{} }
func newStorefrontRouteLauncher() StorefrontRouteLauncher     { return windowsStorefrontLauncher{} }

func (windowsStorefrontObserver) Observe(ctx context.Context, candidates []devicev1.StorefrontProductCandidate) ([]devicev1.StorefrontProductObservation, error) {
	for _, candidate := range candidates {
		if err := candidate.Validate(); err != nil {
			return nil, err
		}
	}
	steamRoots, err := registeredSteamRoots()
	if err != nil {
		return nil, err
	}
	result := make([]devicev1.StorefrontProductObservation, 0)
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if candidate.Provider != devicev1.StorefrontProviderSteam {
			continue
		}
		path, found := observeSteamProduct(steamRoots, candidate.ProductID)
		if !found {
			continue
		}
		result = append(result, devicev1.StorefrontProductObservation{StorefrontProductCandidate: candidate, InstallPath: path, ObservedAt: timeNowUTC()})
	}
	return result, nil
}

func registeredSteamRoots() ([]string, error) {
	roots := []string{}
	for _, spec := range []struct {
		root registry.Key
		path string
		view uint32
	}{
		{registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE},
		{registry.LOCAL_MACHINE, `Software\WOW6432Node\Valve\Steam`, registry.QUERY_VALUE | registry.WOW64_32KEY},
	} {
		key, err := registry.OpenKey(spec.root, spec.path, spec.view)
		if err != nil {
			continue
		}
		value, _, valueErr := key.GetStringValue("SteamPath")
		if valueErr != nil {
			value, _, valueErr = key.GetStringValue("InstallPath")
		}
		_ = key.Close()
		if valueErr == nil {
			roots = append(roots, filepath.Clean(filepath.FromSlash(value)))
		}
	}
	if len(roots) == 0 {
		return nil, nil
	}
	primary := roots[0]
	libraryFile := filepath.Join(primary, "steamapps", "libraryfolders.vdf")
	if file, err := os.Open(libraryFile); err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			parts := steamVDFPairPattern.FindStringSubmatch(scanner.Text())
			if len(parts) != 3 {
				continue
			}
			key := strings.ToLower(parts[1])
			if key == "path" || allDigits(key) {
				roots = append(roots, filepath.Clean(filepath.FromSlash(strings.ReplaceAll(parts[2], `\\`, `\`))))
			}
		}
		_ = file.Close()
	}
	seen := map[string]bool{}
	filtered := roots[:0]
	for _, root := range roots {
		root = filepath.Clean(root)
		key := strings.ToLower(root)
		if seen[key] || validateInstallRootStorage(root) != nil {
			continue
		}
		seen[key] = true
		filtered = append(filtered, root)
	}
	sort.Strings(filtered)
	return filtered, nil
}

func observeSteamProduct(roots []string, productID string) (string, bool) {
	for _, root := range roots {
		manifestPath := filepath.Join(root, "steamapps", "appmanifest_"+productID+".acf")
		file, err := os.Open(manifestPath)
		if err != nil {
			continue
		}
		installDir := ""
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			parts := steamVDFPairPattern.FindStringSubmatch(scanner.Text())
			if len(parts) == 3 && strings.EqualFold(parts[1], "installdir") {
				installDir = strings.TrimSpace(parts[2])
				break
			}
		}
		_ = file.Close()
		if installDir == "" || installDir == "." || installDir == ".." || filepath.Base(installDir) != installDir {
			continue
		}
		installPath := filepath.Join(root, "steamapps", "common", installDir)
		if info, err := os.Stat(installPath); err == nil && info.IsDir() {
			return installPath, true
		}
	}
	return "", false
}

func (windowsStorefrontLauncher) Launch(ctx context.Context, provider, productID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := (devicev1.StorefrontLaunchRequest{GameID: "launch", SourceGameID: "launch", Provider: provider, ProductID: productID}).Validate(); err != nil {
		return err
	}
	uri, err := windows.UTF16PtrFromString(fmt.Sprintf("steam://run/%s", productID))
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("open")
	info := shellExecuteInfo{Mask: seeMaskNoCloseProcess, Verb: verb, File: uri, Show: swShowNormal}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return fmt.Errorf("open Steam game: %w", callErr)
	}
	if info.Process != 0 {
		_ = windows.CloseHandle(info.Process)
	}
	return nil
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

var timeNowUTC = func() time.Time { return time.Now().UTC() }
