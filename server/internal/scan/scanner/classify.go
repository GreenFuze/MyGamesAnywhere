package scanner

import "github.com/GreenFuze/MyGamesAnywhere/server/internal/core"

// emulatedPlatforms are platforms whose game files are always self-contained
// (ROMs, disc images, extracted console directories — usable as-is by an
// emulator or engine).
var emulatedPlatforms = map[core.Platform]bool{
	core.PlatformArcade:  true,
	core.PlatformGBA:     true,
	core.PlatformPS1:     true,
	core.PlatformPS2:     true,
	core.PlatformPS3:     true,
	core.PlatformPSP:     true,
	core.PlatformXbox360: true,
	core.PlatformMSDOS:   true,
	core.PlatformScummVM: true,
}

// GroupClassifier assigns a GroupKind to each GameGroup based on platform
// and file composition.
type GroupClassifier struct{}

func NewGroupClassifier() *GroupClassifier { return &GroupClassifier{} }

// ClassifyAll assigns a GroupKind to every group in-place.
func (c *GroupClassifier) ClassifyAll(groups []GameGroup) {
	for i := range groups {
		groups[i].GroupKind = c.classify(&groups[i])
	}
}

func (c *GroupClassifier) classify(g *GameGroup) core.GroupKind {
	if emulatedPlatforms[g.Platform] {
		return classifyEmulated(g)
	}
	if g.Platform == core.PlatformWindowsPC {
		return classifyWindowsPC(g)
	}
	return classifyUnknownPlatform(g)
}

// classifyEmulated handles groups for emulated platforms. The files are
// always the game itself unless the group contains only media files
// (soundtracks, manuals, screenshots) with no game-relevant content.
func classifyEmulated(g *GameGroup) core.GroupKind {
	for _, f := range g.Files {
		if !isPureMedia(f.Kind) {
			return core.GroupKindSelfContained
		}
	}
	return core.GroupKindExtras
}

// classifyWindowsPC distinguishes extracted game directories (self-contained)
// from installers/compressed archives (packed).
func classifyWindowsPC(g *GameGroup) core.GroupKind {
	hasExe := false
	allExeOrBin := true

	for _, f := range g.Files {
		if f.Kind == FileKindExecutable {
			hasExe = true
		}
		if f.Kind != FileKindExecutable && f.Kind != FileKindUnknown {
			allExeOrBin = false
		}
	}

	if hasExe && allExeOrBin {
		// exe + .bin companion files only → installer pattern (GOG-style)
		return core.GroupKindPacked
	}
	if hasExe {
		// exe + diverse support files (images, audio, data) → extracted game
		return core.GroupKindSelfContained
	}

	return classifyByFileTypes(g)
}

// classifyUnknownPlatform uses file types when platform detection gave
// no answer.
func classifyUnknownPlatform(g *GameGroup) core.GroupKind {
	return classifyByFileTypes(g)
}

func classifyByFileTypes(g *GameGroup) core.GroupKind {
	allArchive := true
	allMedia := true
	for _, f := range g.Files {
		if f.Kind != FileKindArchive {
			allArchive = false
		}
		if !isPureMedia(f.Kind) {
			allMedia = false
		}
	}
	if allArchive {
		return core.GroupKindPacked
	}
	if allMedia {
		return core.GroupKindExtras
	}
	return core.GroupKindUnknown
}

// isPureMedia returns true for file kinds that are never game content
// on their own (audio, images, documents).
func isPureMedia(kind FileKind) bool {
	switch kind {
	case FileKindAudio, FileKindImage, FileKindDocument:
		return true
	}
	return false
}
