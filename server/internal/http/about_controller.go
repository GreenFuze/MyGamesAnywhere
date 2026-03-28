package http

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/buildinfo"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type AboutController struct {
	logger core.Logger
}

func NewAboutController(logger core.Logger) *AboutController {
	return &AboutController{logger: logger}
}

func (c *AboutController) GetAbout(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(buildinfo.AboutInfo())
}

func (c *AboutController) GetLicense(w http.ResponseWriter, r *http.Request) {
	licensePath, err := findLicensePath()
	if err != nil {
		c.logger.Error("find license", err)
		http.Error(w, "license file not found", http.StatusInternalServerError)
		return
	}
	http.ServeFile(w, r, licensePath)
}

func findLicensePath() (string, error) {
	candidates := []string{}

	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, walkLicenseCandidates(wd)...)
	}
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates, walkLicenseCandidates(filepath.Dir(exePath))...)
	}
	if _, srcFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, walkLicenseCandidates(filepath.Dir(srcFile))...)
	}

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}

func walkLicenseCandidates(start string) []string {
	var out []string
	seen := make(map[string]bool)
	current := filepath.Clean(start)
	for {
		for _, name := range []string{"LICENSE.md", "LICENSE"} {
			candidate := filepath.Join(current, name)
			if !seen[candidate] {
				seen[candidate] = true
				out = append(out, candidate)
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return out
}
