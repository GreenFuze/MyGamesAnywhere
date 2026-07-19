package clientapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var scummVMGameIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*:[a-z0-9][a-z0-9_.-]*$`)
var scummVMGameIDOutputPattern = regexp.MustCompile(`\b[a-z][a-z0-9_-]*:[a-z0-9][a-z0-9_.-]*\b`)

type ScummVMRouteDetector interface {
	Detect(context.Context, string) (string, error)
}

type localScummVMRouteDetector struct{ cacheRoot string }

func newLocalScummVMRouteDetector() (*localScummVMRouteDetector, error) {
	cacheBase, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(cacheBase) == "" {
		return nil, errors.New("resolve per-user emulator cache")
	}
	return &localScummVMRouteDetector{cacheRoot: filepath.Join(cacheBase, "MyGamesAnywhere", "Client", "emulator-cache")}, nil
}

func (d *localScummVMRouteDetector) Detect(ctx context.Context, routeFingerprint string) (string, error) {
	if d == nil || strings.TrimSpace(d.cacheRoot) == "" {
		return "", errors.New("ScummVM route detector is unavailable")
	}
	routeFingerprint = strings.ToLower(strings.TrimSpace(routeFingerprint))
	if len(routeFingerprint) != 64 {
		return "", errors.New("ScummVM route fingerprint is invalid")
	}
	contentRoot := filepath.Join(d.cacheRoot, routeFingerprint)
	if info, err := os.Stat(contentRoot); err != nil || !info.IsDir() {
		return "", errors.New("play this ScummVM copy once on this device, then set up save backup")
	}
	executable := ""
	for _, runtime := range collectKnownRuntimes(ctx) {
		if runtime.ID == "scummvm" {
			executable = runtime.Path
			break
		}
	}
	if strings.TrimSpace(executable) == "" {
		return "", errors.New("ScummVM is not available for this Windows user")
	}
	output := &boundedCommandOutput{limit: 64 * 1024}
	command := exec.CommandContext(ctx, executable, "--path="+contentRoot, "--detect")
	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("ask ScummVM to identify this copy: %w", err)
	}
	return exactScummVMGameID(output.String())
}

func exactScummVMGameID(output string) (string, error) {
	candidates := map[string]bool{}
	for _, candidate := range scummVMGameIDOutputPattern.FindAllString(strings.ToLower(output), -1) {
		candidates[candidate] = true
	}
	if len(candidates) != 1 {
		return "", errors.New("ScummVM did not identify exactly one game; save backup remains read-only")
	}
	for candidate := range candidates {
		return candidate, nil
	}
	return "", errors.New("ScummVM game identity is unavailable")
}
