package clientapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type GameLauncher interface {
	Launch(context.Context, devicev1.GameLaunchRequest) (devicev1.GameLaunchResult, error)
}

type gameProcessStarter interface {
	Start(executable, workingDirectory string) (int, error)
}

type execGameProcessStarter struct{}

func (execGameProcessStarter) Start(executable, workingDirectory string) (int, error) {
	command := exec.Command(executable)
	command.Dir = workingDirectory
	if err := command.Start(); err != nil {
		return 0, err
	}
	processID := command.Process.Pid
	if err := command.Process.Release(); err != nil {
		return 0, err
	}
	return processID, nil
}

type WindowsGameLauncher struct {
	now         func() time.Time
	start       gameProcessStarter
	resolvePath func(string) (string, error)
	ownership   *InstallationOwnership
}

func NewOwnedWindowsGameLauncher(ownership *InstallationOwnership) *WindowsGameLauncher {
	launcher := NewWindowsGameLauncher()
	launcher.ownership = ownership
	return launcher
}

func NewWindowsGameLauncher() *WindowsGameLauncher {
	return &WindowsGameLauncher{
		now:         time.Now,
		start:       execGameProcessStarter{},
		resolvePath: filepath.EvalSymlinks,
	}
}

func (l *WindowsGameLauncher) Launch(ctx context.Context, request devicev1.GameLaunchRequest) (devicev1.GameLaunchResult, error) {
	if l == nil || l.now == nil || l.start == nil || l.resolvePath == nil {
		return devicev1.GameLaunchResult{}, errors.New("game launcher is unavailable")
	}
	if err := request.Validate(); err != nil {
		return devicev1.GameLaunchResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return devicev1.GameLaunchResult{}, err
	}
	manifest, err := readInstallManifest(request.InstallPath)
	if err != nil {
		return devicev1.GameLaunchResult{}, err
	}
	if !isSupportedLaunchManifestVersion(manifest.SchemaVersion) || manifest.GameID != request.GameID || manifest.SourceGameID != request.SourceGameID {
		return devicev1.GameLaunchResult{}, errors.New("installation manifest does not match the requested game")
	}
	manifest, err = ensureInstallationManifestOwnership(l.ownership, request.InstallPath, manifest)
	if err != nil {
		return devicev1.GameLaunchResult{}, err
	}
	mutation, err := l.ownership.AuthorizeMutation(manifest.LocalInstallationID, manifest.OwnerBindingID, request.InstallPath)
	if err != nil {
		return devicev1.GameLaunchResult{}, err
	}
	if mutation != nil {
		defer mutation.Close()
	}
	requested := devicev1.NormalizeLaunchTarget(request.LaunchTarget)
	allowed := false
	for _, candidate := range manifest.LaunchCandidates {
		if strings.EqualFold(devicev1.NormalizeLaunchTarget(candidate), requested) {
			allowed = true
			break
		}
	}
	if !allowed {
		return devicev1.GameLaunchResult{}, errors.New("launch target is not recorded in the installation manifest")
	}
	executable := filepath.Join(request.InstallPath, filepath.FromSlash(requested))
	inside, err := pathWithinRoot(request.InstallPath, executable)
	if err != nil || !inside {
		return devicev1.GameLaunchResult{}, errors.New("launch target is outside the installation directory")
	}
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(executable), ".exe") {
		return devicev1.GameLaunchResult{}, fmt.Errorf("launch target is unavailable: %s", requested)
	}
	resolvedRoot, err := l.resolvePath(request.InstallPath)
	if err != nil {
		return devicev1.GameLaunchResult{}, fmt.Errorf("resolve installation directory: %w", err)
	}
	resolvedExecutable, err := l.resolvePath(executable)
	if err != nil {
		return devicev1.GameLaunchResult{}, fmt.Errorf("resolve launch target: %w", err)
	}
	inside, err = pathWithinRoot(resolvedRoot, resolvedExecutable)
	if err != nil || !inside {
		return devicev1.GameLaunchResult{}, errors.New("resolved launch target is outside the installation directory")
	}
	processID, err := l.start.Start(resolvedExecutable, filepath.Dir(resolvedExecutable))
	if err != nil {
		return devicev1.GameLaunchResult{}, fmt.Errorf("start game: %w", err)
	}
	return devicev1.GameLaunchResult{
		GameID: request.GameID, SourceGameID: request.SourceGameID, ProcessID: processID, StartedAt: l.now().UTC(),
	}, nil
}

func isSupportedLaunchManifestVersion(version int) bool {
	return version == devicev1.LegacyInstallManifestSchemaVersion || version == devicev1.InstallManifestSchemaVersion || version == devicev1.LegacyExecutableInstallManifestSchemaVersion || version == devicev1.ExecutableInstallManifestSchemaVersion
}
