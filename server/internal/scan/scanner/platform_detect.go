package scanner

import (
	"path/filepath"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// PlatformDetector assigns a platform to each GameGroup using path-based
// directory name hints and file structure signals. When neither source
// provides a confident answer, the platform stays core.PlatformUnknown.
type PlatformDetector struct {
	pathRules []pathRule
}

type pathRule struct {
	pattern  string
	platform core.Platform
	exact    bool // true = match full segment; false = substring
}

func NewPlatformDetector() *PlatformDetector {
	return &PlatformDetector{
		pathRules: []pathRule{
			// Exact matches (high specificity, checked first per-segment)
			{pattern: "mame", platform: core.PlatformArcade, exact: true},
			{pattern: "arcade", platform: core.PlatformArcade, exact: true},
			{pattern: "scummvm", platform: core.PlatformScummVM, exact: true},
			{pattern: "nes", platform: core.PlatformNES, exact: true},
			{pattern: "snes", platform: core.PlatformSNES, exact: true},
			{pattern: "gb", platform: core.PlatformGB, exact: true},
			{pattern: "gbc", platform: core.PlatformGBC, exact: true},
			{pattern: "ps3", platform: core.PlatformPS3, exact: true},
			{pattern: "ps2", platform: core.PlatformPS2, exact: true},
			{pattern: "ps1", platform: core.PlatformPS1, exact: true},
			{pattern: "psx", platform: core.PlatformPS1, exact: true},
			{pattern: "psp", platform: core.PlatformPSP, exact: true},
			{pattern: "gba", platform: core.PlatformGBA, exact: true},
			{pattern: "n64", platform: core.PlatformN64, exact: true},
			{pattern: "genesis", platform: core.PlatformGenesis, exact: true},
			{pattern: "megadrive", platform: core.PlatformGenesis, exact: true},
			{pattern: "mastersystem", platform: core.PlatformSegaMasterSystem, exact: true},
			{pattern: "sms", platform: core.PlatformSegaMasterSystem, exact: true},
			{pattern: "gamegear", platform: core.PlatformGameGear, exact: true},
			{pattern: "segacd", platform: core.PlatformSegaCD, exact: true},
			{pattern: "megacd", platform: core.PlatformSegaCD, exact: true},
			{pattern: "sega32x", platform: core.PlatformSega32X, exact: true},
			{pattern: "xbox360", platform: core.PlatformXbox360, exact: true},
			{pattern: "msdos", platform: core.PlatformMSDOS, exact: true},

			// Substring matches (longer/more specific patterns first to
			// prevent "playstation" from shadowing "playstation 2")
			{pattern: "nintendo game boy color", platform: core.PlatformGBC},
			{pattern: "game boy color", platform: core.PlatformGBC},
			{pattern: "game boy advance", platform: core.PlatformGBA},
			{pattern: "game boy advanced", platform: core.PlatformGBA},
			{pattern: "nintendo 64", platform: core.PlatformN64},
			{pattern: "super nintendo", platform: core.PlatformSNES},
			{pattern: "nintendo entertainment system", platform: core.PlatformNES},
			{pattern: "master system", platform: core.PlatformSegaMasterSystem},
			{pattern: "game gear", platform: core.PlatformGameGear},
			{pattern: "sega cd", platform: core.PlatformSegaCD},
			{pattern: "mega cd", platform: core.PlatformSegaCD},
			{pattern: "mega drive", platform: core.PlatformGenesis},
			{pattern: "sega genesis", platform: core.PlatformGenesis},
			{pattern: "playstation portable", platform: core.PlatformPSP},
			{pattern: "playstation 3", platform: core.PlatformPS3},
			{pattern: "playstation 2", platform: core.PlatformPS2},
			{pattern: "playstation", platform: core.PlatformPS1},
			{pattern: "xbox 360", platform: core.PlatformXbox360},
			{pattern: "32x", platform: core.PlatformSega32X},
			{pattern: "game boy", platform: core.PlatformGB},
			{pattern: "ms dos", platform: core.PlatformMSDOS},
			{pattern: "ms-dos", platform: core.PlatformMSDOS},
			{pattern: "dosbox", platform: core.PlatformMSDOS},
		},
	}
}

// DetectAll assigns a platform to every group in-place.
func (d *PlatformDetector) DetectAll(groups []GameGroup) {
	for i := range groups {
		groups[i].Platform = d.detect(&groups[i])
	}
}

func (d *PlatformDetector) detect(g *GameGroup) core.Platform {
	if p := d.detectFromPath(g.RootDir); p != core.PlatformUnknown {
		return p
	}
	if p := detectFromFiles(g.Files); p != core.PlatformUnknown {
		return p
	}
	return core.PlatformUnknown
}

// detectFromPath checks each segment of the group's root directory against
// the path rules. Earlier (closer-to-root) segments take precedence.
func (d *PlatformDetector) detectFromPath(rootDir string) core.Platform {
	if rootDir == "" {
		return core.PlatformUnknown
	}
	segments := strings.Split(filepath.ToSlash(rootDir), "/")
	for _, seg := range segments {
		lower := strings.ToLower(seg)
		for _, rule := range d.pathRules {
			if rule.exact {
				if lower == rule.pattern {
					return rule.platform
				}
			} else {
				if strings.Contains(lower, rule.pattern) {
					return rule.platform
				}
			}
		}
	}
	return core.PlatformUnknown
}

// detectFromFiles uses file-level signals when path gives no answer.
func detectFromFiles(files []AnnotatedFile) core.Platform {
	for _, f := range files {
		if platform, ok := romExtensionPlatforms[f.Extension]; ok {
			return platform
		}
	}

	// PS3 structure: PS3_DISC.SFB at group root or files under PS3_GAME/
	for _, f := range files {
		upper := strings.ToUpper(f.Name)
		if upper == "PS3_DISC.SFB" {
			return core.PlatformPS3
		}
	}
	for _, f := range files {
		norm := strings.ToUpper(filepath.ToSlash(f.Path))
		if strings.Contains(norm, "/PS3_GAME/") {
			return core.PlatformPS3
		}
	}

	hasCOM := false
	hasBAT := false
	hasConf := false
	hasEXE := false
	for _, f := range files {
		switch f.Kind {
		case FileKindDOSExecutable:
			hasCOM = true
		case FileKindScript:
			if f.Extension == ".bat" {
				hasBAT = true
			}
		case FileKindExecutable:
			hasEXE = true
		}
		if f.Extension == ".conf" {
			hasConf = true
		}
	}

	// .com executables are a strong DOS signal
	if hasCOM {
		return core.PlatformMSDOS
	}
	// dosbox .conf + batch files → DOS
	if hasConf && hasBAT {
		return core.PlatformMSDOS
	}
	// Windows executables without other platform indicators
	if hasEXE {
		return core.PlatformWindowsPC
	}

	return core.PlatformUnknown
}

var romExtensionPlatforms = map[string]core.Platform{
	".nes": core.PlatformNES,
	".fds": core.PlatformNES,
	".smc": core.PlatformSNES,
	".sfc": core.PlatformSNES,
	".gb":  core.PlatformGB,
	".gbc": core.PlatformGBC,
	".gba": core.PlatformGBA,
	".z64": core.PlatformN64,
	".n64": core.PlatformN64,
	".v64": core.PlatformN64,
	".gen": core.PlatformGenesis,
	".md":  core.PlatformGenesis,
	".smd": core.PlatformGenesis,
	".sms": core.PlatformSegaMasterSystem,
	".gg":  core.PlatformGameGear,
	".32x": core.PlatformSega32X,
}
