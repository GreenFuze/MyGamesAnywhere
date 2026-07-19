package clientapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type EmulatorLauncher interface {
	Launch(context.Context, string, devicev1.EmulatorLaunchRequest, CommandProgressReporter) (devicev1.EmulatorLaunchResult, error)
}

type emulatorProcessStarter interface {
	Start(executable string, arguments []string, workingDirectory string) (startedEmulatorProcess, error)
}

type startedEmulatorProcess struct {
	PID  int
	Done <-chan struct{}
}

type execEmulatorProcessStarter struct{}

func (execEmulatorProcessStarter) Start(executable string, arguments []string, workingDirectory string) (startedEmulatorProcess, error) {
	command := exec.Command(executable, arguments...)
	command.Dir = workingDirectory
	if err := command.Start(); err != nil {
		return startedEmulatorProcess{}, err
	}
	done := make(chan struct{})
	go func() {
		_ = command.Wait()
		close(done)
	}()
	return startedEmulatorProcess{PID: command.Process.Pid, Done: done}, nil
}

type ManagedEmulatorLauncher struct {
	serverURL string
	cacheRoot string
	client    *http.Client
	inventory InventoryCollector
	start     emulatorProcessStarter
	now       func() time.Time
	ownership *InstallationOwnership
}

func NewOwnedManagedEmulatorLauncher(serverURL string, inventory InventoryCollector, ownership *InstallationOwnership) (*ManagedEmulatorLauncher, error) {
	launcher, err := NewManagedEmulatorLauncher(serverURL, inventory)
	if err != nil {
		return nil, err
	}
	if ownership == nil || ownership.saveDomains == nil {
		return nil, errors.New("save domain authority is required")
	}
	launcher.ownership = ownership
	return launcher, nil
}

func NewManagedEmulatorLauncher(serverURL string, inventory InventoryCollector) (*ManagedEmulatorLauncher, error) {
	parsed, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("valid MGA Server URL is required")
	}
	if inventory == nil {
		return nil, errors.New("emulator inventory collector is required")
	}
	cacheBase, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(cacheBase) == "" {
		return nil, errors.New("resolve per-user emulator cache")
	}
	return &ManagedEmulatorLauncher{
		serverURL: parsed.Scheme + "://" + parsed.Host,
		cacheRoot: filepath.Join(cacheBase, "MyGamesAnywhere", "Client", "emulator-cache"),
		client:    &http.Client{Timeout: 0}, inventory: inventory, start: execEmulatorProcessStarter{}, now: time.Now,
	}, nil
}

func (l *ManagedEmulatorLauncher) Launch(ctx context.Context, commandID string, request devicev1.EmulatorLaunchRequest, report CommandProgressReporter) (devicev1.EmulatorLaunchResult, error) {
	if l == nil || l.client == nil || l.inventory == nil || l.start == nil || l.now == nil {
		return devicev1.EmulatorLaunchResult{}, errors.New("emulator launcher is unavailable")
	}
	if strings.TrimSpace(commandID) == "" {
		return devicev1.EmulatorLaunchResult{}, errors.New("command_id is required")
	}
	if err := request.Validate(); err != nil {
		return devicev1.EmulatorLaunchResult{}, err
	}
	if request.EmulatorID != "scummvm" && request.EmulatorID != "retroarch" {
		return devicev1.EmulatorLaunchResult{}, fmt.Errorf("unsupported typed emulator route %s for %s", request.EmulatorID, request.Platform)
	}
	_, executable, err := l.resolveRuntime(ctx, request.EmulatorID)
	if err != nil {
		return devicev1.EmulatorLaunchResult{}, err
	}
	contentRoot, err := l.prepareContent(ctx, commandID, request, report)
	if err != nil {
		return devicev1.EmulatorLaunchResult{}, err
	}
	launchName := "ScummVM"
	arguments := []string{"--path=" + contentRoot, "--auto-detect"}
	var releaseSaveLease func()
	if request.EmulatorID == "scummvm" && request.RouteFingerprint != "" && l.ownership != nil && l.ownership.saveDomains != nil {
		domain, found := l.ownership.saveDomains.FindByRoute("scummvm", strings.ToLower(request.RouteFingerprint))
		if found {
			if domain.State != SaveDomainOwned || !strings.EqualFold(domain.WriterBindingID, l.ownership.bindingID) || len(domain.ResolvedPaths) != 1 || strings.TrimSpace(domain.ScummVMGameID) == "" {
				return devicev1.EmulatorLaunchResult{}, errors.New("ScummVM save domain is not writable by this MGA Server")
			}
			savePath := filepath.Clean(domain.ResolvedPaths[0])
			if info, err := os.Stat(savePath); err != nil || !info.IsDir() {
				return devicev1.EmulatorLaunchResult{}, errors.New("managed ScummVM save folder is unavailable")
			}
			releaseSaveLease, err = l.ownership.coordinator.Reserve(l.ownership.bindingID, savePath, "save-domain:"+domain.LocalSaveDomainID)
			if err != nil {
				return devicev1.EmulatorLaunchResult{}, err
			}
			arguments = []string{"--path=" + contentRoot, "--savepath=" + savePath, domain.ScummVMGameID}
		}
	}
	if request.EmulatorID == "retroarch" {
		launchName = "RetroArch"
		corePath, err := resolveRetroArchCore(executable, request.CoreID)
		if err != nil {
			return devicev1.EmulatorLaunchResult{}, err
		}
		contentPath := filepath.Join(contentRoot, filepath.FromSlash(request.ContentPath))
		inside, err := pathWithinRoot(contentRoot, contentPath)
		if err != nil || !inside {
			return devicev1.EmulatorLaunchResult{}, errors.New("RetroArch content path escaped the MGA cache")
		}
		if info, err := os.Stat(contentPath); err != nil || !info.Mode().IsRegular() {
			return devicev1.EmulatorLaunchResult{}, errors.New("RetroArch content entry point is unavailable")
		}
		arguments = []string{"-L", corePath, contentPath}
	}
	if err := reportProgress(report, "launching", "Starting "+launchName, 98, "launch", 90); err != nil {
		return devicev1.EmulatorLaunchResult{}, err
	}
	process, err := l.start.Start(executable, arguments, contentRoot)
	if err != nil {
		if releaseSaveLease != nil {
			releaseSaveLease()
		}
		return devicev1.EmulatorLaunchResult{}, fmt.Errorf("start %s: %w", launchName, err)
	}
	if releaseSaveLease != nil {
		if process.Done == nil {
			releaseSaveLease()
			return devicev1.EmulatorLaunchResult{}, errors.New("emulator process did not provide a lifetime signal")
		}
		go func() {
			<-process.Done
			releaseSaveLease()
		}()
	}
	if err := reportProgress(report, "complete", "Started", 100, "launch", 100); err != nil {
		return devicev1.EmulatorLaunchResult{}, err
	}
	return devicev1.EmulatorLaunchResult{GameID: request.GameID, SourceGameID: request.SourceGameID, EmulatorID: request.EmulatorID, CoreID: request.CoreID, ProcessID: process.PID, StartedAt: l.now().UTC()}, nil
}

func (l *ManagedEmulatorLauncher) resolveRuntime(ctx context.Context, emulatorID string) (devicev1.RuntimeInventory, string, error) {
	inventory, err := l.inventory.Collect(ctx)
	if err != nil {
		return devicev1.RuntimeInventory{}, "", fmt.Errorf("refresh emulator inventory: %w", err)
	}
	for _, runtime := range inventory.Runtimes {
		if runtime.ID != emulatorID {
			continue
		}
		executable, err := filepath.Abs(runtime.Path)
		if err != nil || strings.TrimSpace(runtime.Path) == "" {
			return devicev1.RuntimeInventory{}, "", errors.New("detected emulator path is invalid")
		}
		info, err := os.Stat(executable)
		if err != nil || !info.Mode().IsRegular() {
			return devicev1.RuntimeInventory{}, "", errors.New("detected emulator is no longer available")
		}
		return runtime, executable, nil
	}
	return devicev1.RuntimeInventory{}, "", fmt.Errorf("%s is not installed for this device user", emulatorID)
}

func resolveRetroArchCore(executable, coreID string) (string, error) {
	if !runtimeComponentIDPattern.MatchString(coreID) {
		return "", errors.New("RetroArch core id is invalid")
	}
	directory := retroArchConfiguredDirectory(executable, "libretro_directory", "cores")
	extensions := []string{"_libretro.dll"}
	if runtime.GOOS != "windows" {
		extensions = []string{"_libretro.so"}
	}
	if runtime.GOOS == "darwin" {
		extensions = []string{"_libretro.dylib"}
	}
	for _, extension := range extensions {
		candidate := filepath.Join(directory, coreID+extension)
		inside, err := pathWithinRoot(directory, candidate)
		if err != nil || !inside {
			return "", errors.New("RetroArch core path escaped the configured core directory")
		}
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("RetroArch core %s is not installed for this device user", coreID)
}

func (l *ManagedEmulatorLauncher) prepareContent(ctx context.Context, commandID string, request devicev1.EmulatorLaunchRequest, report CommandProgressReporter) (string, error) {
	contentKey := strings.ToLower(strings.TrimSpace(request.RouteFingerprint))
	if contentKey == "" {
		keyHasher := sha256.New()
		_, _ = io.WriteString(keyHasher, request.GameID+"\x00"+request.SourceGameID+"\x00")
		for _, artifact := range request.Artifacts {
			_, _ = io.WriteString(keyHasher, artifact.Path+"\x00"+strings.ToLower(artifact.SHA256)+"\x00")
		}
		contentKey = hex.EncodeToString(keyHasher.Sum(nil))
	}
	if err := os.MkdirAll(l.cacheRoot, 0o700); err != nil {
		return "", fmt.Errorf("create emulator cache: %w", err)
	}
	contentRoot := filepath.Join(l.cacheRoot, contentKey)
	if valid, err := verifyEmulatorContent(contentRoot, request.Artifacts); err == nil && valid {
		return contentRoot, nil
	}
	commandHash := sha256.Sum256([]byte(commandID))
	stage := filepath.Join(l.cacheRoot, ".staging-"+hex.EncodeToString(commandHash[:8]))
	inside, err := pathWithinRoot(l.cacheRoot, stage)
	if err != nil || !inside {
		return "", errors.New("emulator staging path is outside the MGA cache")
	}
	if err := os.RemoveAll(stage); err != nil {
		return "", fmt.Errorf("clear emulator staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	if err := os.MkdirAll(stage, 0o700); err != nil {
		return "", fmt.Errorf("create emulator staging directory: %w", err)
	}
	for index, artifact := range request.Artifacts {
		percent := uint8((index * 90) / len(request.Artifacts))
		if err := reportProgress(report, "downloading", "Preparing game files", percent, "download", percent); err != nil {
			return "", err
		}
		destination := filepath.Join(stage, filepath.FromSlash(artifact.Path))
		inside, err := pathWithinRoot(stage, destination)
		if err != nil || !inside {
			return "", errors.New("emulator content destination escaped the MGA cache")
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return "", err
		}
		if err := l.downloadArtifact(ctx, artifact, destination); err != nil {
			return "", err
		}
	}
	if valid, err := verifyEmulatorContent(stage, request.Artifacts); err != nil || !valid {
		return "", errors.New("downloaded emulator content failed verification")
	}
	if err := os.Rename(stage, contentRoot); err != nil {
		if valid, verifyErr := verifyEmulatorContent(contentRoot, request.Artifacts); verifyErr == nil && valid {
			return contentRoot, nil
		}
		return "", fmt.Errorf("commit emulator content: %w", err)
	}
	return contentRoot, nil
}

func (l *ManagedEmulatorLauncher) downloadArtifact(ctx context.Context, artifact devicev1.EmulatorContentArtifact, destination string) error {
	server, _ := url.Parse(l.serverURL)
	download, err := url.Parse(artifact.DownloadURL)
	if err != nil {
		return errors.New("emulator content URL is invalid")
	}
	if !download.IsAbs() {
		download = server.ResolveReference(download)
	}
	if !strings.EqualFold(server.Scheme, download.Scheme) || !strings.EqualFold(server.Host, download.Host) {
		return errors.New("emulator content URL must use the paired MGA Server origin")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, download.String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+artifact.DownloadToken)
	response, err := l.client.Do(request)
	if err != nil {
		return fmt.Errorf("download emulator content: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download emulator content: MGA Server returned %s", response.Status)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(response.Body, int64(artifact.SizeBytes)+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if uint64(written) != artifact.SizeBytes || !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), artifact.SHA256) {
		return fmt.Errorf("emulator content verification failed for %s", artifact.Path)
	}
	return nil
}

func verifyEmulatorContent(root string, artifacts []devicev1.EmulatorContentArtifact) (bool, error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return false, err
	}
	for _, artifact := range artifacts {
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(artifact.Path)))
		if err != nil {
			return false, err
		}
		hasher := sha256.New()
		written, copyErr := io.Copy(hasher, file)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil || uint64(written) != artifact.SizeBytes || !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), artifact.SHA256) {
			return false, nil
		}
	}
	return true, nil
}
