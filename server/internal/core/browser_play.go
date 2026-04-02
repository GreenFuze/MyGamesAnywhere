package core

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
