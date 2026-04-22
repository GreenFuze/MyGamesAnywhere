package core

import "testing"

func TestBrowserPlayRuntimeForSourceGameFallsBackToCanonicalPlatform(t *testing.T) {
	runtime, ok := BrowserPlayRuntimeForSourceGame(PlatformUnknown, PlatformArcade)
	if !ok {
		t.Fatal("expected runtime for arcade canonical fallback")
	}
	if runtime != "emulatorjs" {
		t.Fatalf("runtime = %q, want %q", runtime, "emulatorjs")
	}
}

func TestBrowserPlayRuntimeForSourceGamePrefersKnownSourcePlatform(t *testing.T) {
	runtime, ok := BrowserPlayRuntimeForSourceGame(PlatformN64, PlatformUnknown)
	if !ok {
		t.Fatal("expected runtime for source platform")
	}
	if runtime != "emulatorjs" {
		t.Fatalf("runtime = %q, want %q", runtime, "emulatorjs")
	}
}

func TestBrowserPlayProfileForSourceGameArcadeUsesEmulatorJS(t *testing.T) {
	profile, ok := BrowserPlayProfileForSourceGame(PlatformArcade, PlatformUnknown)
	if !ok {
		t.Fatal("expected profile for arcade")
	}
	if profile != BrowserProfileEmulatorJS {
		t.Fatalf("profile = %q, want %q", profile, BrowserProfileEmulatorJS)
	}
}
