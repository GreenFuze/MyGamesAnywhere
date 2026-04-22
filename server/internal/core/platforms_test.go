package core

import "testing"

func TestNormalizePlatformAliasHandlesUnderscoreCanonicalValues(t *testing.T) {
	if got := NormalizePlatformAlias("windows_pc"); got != PlatformWindowsPC {
		t.Fatalf("NormalizePlatformAlias(%q) = %q, want %q", "windows_pc", got, PlatformWindowsPC)
	}
	if got := NormalizePlatformAlias("sega_master_system"); got != PlatformSegaMasterSystem {
		t.Fatalf("NormalizePlatformAlias(%q) = %q, want %q", "sega_master_system", got, PlatformSegaMasterSystem)
	}
}
