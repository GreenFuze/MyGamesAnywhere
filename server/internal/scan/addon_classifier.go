package scan

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/pkg/titlematch"
)

var nonAlnumSeparatorRE = regexp.MustCompile(`[^a-z0-9]+`)

// InstallerAddOnClassifier identifies file-backed installer records that are
// clearly add-on content rather than standalone games.
type InstallerAddOnClassifier struct{}

func NewInstallerAddOnClassifier() *InstallerAddOnClassifier {
	return &InstallerAddOnClassifier{}
}

func (c *InstallerAddOnClassifier) ClassifyAll(games []*core.Game) {
	for _, game := range games {
		if kind, ok := c.Classify(game); ok {
			game.Kind = kind
		}
	}
}

func (c *InstallerAddOnClassifier) Classify(game *core.Game) (core.GameKind, bool) {
	if !c.hasPackedWindowsInstallerEvidence(game) {
		return "", false
	}
	tokens := addOnTitleTokens(game.RawTitle)
	if len(tokens) == 0 {
		tokens = addOnTitleTokens(game.Title)
	}
	if containsToken(tokens, "expansion") || containsPhrase(tokens, "expansion", "pack") {
		return core.GameKindExpansion, true
	}
	if containsToken(tokens, "dlc") ||
		containsToken(tokens, "addon") ||
		containsPhrase(tokens, "add", "on") ||
		containsPhrase(tokens, "level", "pack") ||
		containsPhrase(tokens, "story", "pack") ||
		containsPhrase(tokens, "mission", "pack") ||
		containsPhrase(tokens, "map", "pack") ||
		containsPhrase(tokens, "character", "pack") ||
		containsPhrase(tokens, "costume", "pack") ||
		containsPhrase(tokens, "skin", "pack") ||
		containsPhrase(tokens, "content", "pack") ||
		containsPhrase(tokens, "bundle", "pack") ||
		containsPhrase(tokens, "season", "pass") {
		return core.GameKindDLC, true
	}
	return "", false
}

func (c *InstallerAddOnClassifier) ShouldAutoArchive(game *core.Game) bool {
	if game == nil {
		return false
	}
	if game.Kind != core.GameKindDLC && game.Kind != core.GameKindAddon && game.Kind != core.GameKindExpansion {
		return false
	}
	_, ok := c.Classify(game)
	return ok
}

func (c *InstallerAddOnClassifier) hasPackedWindowsInstallerEvidence(game *core.Game) bool {
	if game == nil || game.Platform != core.PlatformWindowsPC || game.GroupKind != core.GroupKindPacked {
		return false
	}
	title := strings.ToLower(strings.TrimSpace(game.RawTitle))
	if title == "" {
		title = strings.ToLower(strings.TrimSpace(game.Title))
	}
	if strings.HasPrefix(title, "setup_") || strings.HasPrefix(title, "setup-") || strings.HasPrefix(title, "setup ") {
		return true
	}
	for _, file := range game.Files {
		name := strings.ToLower(file.FileName)
		if name == "" {
			name = strings.ToLower(filepath.Base(file.Path))
		}
		if strings.HasPrefix(name, "setup_") || strings.HasPrefix(name, "setup-") || strings.HasPrefix(name, "setup ") {
			return true
		}
		if strings.EqualFold(file.FileKind, "executable") ||
			strings.EqualFold(file.FileKind, "dos_executable") ||
			strings.EqualFold(filepath.Ext(name), ".exe") ||
			strings.EqualFold(filepath.Ext(name), ".msi") {
			return true
		}
	}
	return false
}

func addOnTitleTokens(value string) []string {
	normalized := titlematch.NormalizeLookupTitle(value)
	normalized = nonAlnumSeparatorRE.ReplaceAllString(normalized, " ")
	return strings.Fields(normalized)
}

func containsToken(tokens []string, token string) bool {
	for _, current := range tokens {
		if current == token {
			return true
		}
	}
	return false
}

func containsPhrase(tokens []string, phrase ...string) bool {
	if len(phrase) == 0 || len(tokens) < len(phrase) {
		return false
	}
	for i := 0; i <= len(tokens)-len(phrase); i++ {
		matches := true
		for j, token := range phrase {
			if tokens[i+j] != token {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}
