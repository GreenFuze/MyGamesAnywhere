package plugins

import "testing"

func TestValidPluginID(t *testing.T) {
	valid := []string{
		"game-source-smb",
		"game-source-google-drive",
		"sync-settings-google-drive",
		"game-source-mock",
		"a",
		"a1",
		"ab-cd",
	}
	for _, id := range valid {
		if !validPluginID(id) {
			t.Errorf("expected valid: %q", id)
		}
	}

	invalid := []string{
		"",
		"com.mga.drive",
		"com.example.plugin",
		"Abc",
		"-leading",
		"UPPERCASE",
	}
	for _, id := range invalid {
		if validPluginID(id) {
			t.Errorf("expected invalid: %q", id)
		}
	}
}
