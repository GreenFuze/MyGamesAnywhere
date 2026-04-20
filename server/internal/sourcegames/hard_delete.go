package sourcegames

import (
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
)

func HardDeleteEligibility(sourceGame *core.SourceGame) (bool, string) {
	if sourceGame == nil {
		return false, "source record is missing"
	}
	if !sourcescope.IsFilesystemBackedPlugin(sourceGame.PluginID) {
		return false, "this source integration does not support destructive deletion"
	}
	if strings.TrimSpace(sourceGame.RootPath) == "" {
		return false, "this source record has no rooted file scope to delete safely"
	}
	if len(sourceGame.Files) == 0 {
		return false, "this source record has no persisted file inventory to delete"
	}
	return true, ""
}
