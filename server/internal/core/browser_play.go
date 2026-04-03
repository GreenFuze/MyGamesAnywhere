package core

const (
	BrowserProfileEmulatorJS = "browser.emulatorjs"
	BrowserProfileJSDOS      = "browser.jsdos"
	BrowserProfileScummVM    = "browser.scummvm"
)

// EffectiveBrowserPlayPlatform matches browser-play source resolution:
// unknown source platforms inherit the canonical platform for launch/runtime decisions.
func EffectiveBrowserPlayPlatform(sourcePlatform, canonicalPlatform Platform) Platform {
	if sourcePlatform == PlatformUnknown {
		return canonicalPlatform
	}
	return sourcePlatform
}

func BrowserPlayRuntimeForPlatform(platform Platform) (string, bool) {
	switch platform {
	case PlatformNES, PlatformSNES, PlatformGB, PlatformGBC, PlatformGBA,
		PlatformGenesis, PlatformSegaMasterSystem, PlatformGameGear, PlatformSegaCD,
		PlatformSega32X, PlatformPS1, PlatformArcade:
		return "emulatorjs", true
	case PlatformMSDOS:
		return "jsdos", true
	case PlatformScummVM:
		return "scummvm", true
	default:
		return "", false
	}
}

func BrowserPlayRuntimeForSourceGame(sourcePlatform, canonicalPlatform Platform) (string, bool) {
	return BrowserPlayRuntimeForPlatform(EffectiveBrowserPlayPlatform(sourcePlatform, canonicalPlatform))
}

func BrowserPlayProfileForRuntime(runtime string) (string, bool) {
	switch runtime {
	case "emulatorjs":
		return BrowserProfileEmulatorJS, true
	case "jsdos":
		return BrowserProfileJSDOS, true
	case "scummvm":
		return BrowserProfileScummVM, true
	default:
		return "", false
	}
}

func BrowserPlayProfileForSourceGame(sourcePlatform, canonicalPlatform Platform) (string, bool) {
	runtime, ok := BrowserPlayRuntimeForSourceGame(sourcePlatform, canonicalPlatform)
	if !ok {
		return "", false
	}
	return BrowserPlayProfileForRuntime(runtime)
}
