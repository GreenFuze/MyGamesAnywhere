//go:build windows

package clientapp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestObserveSteamProductUsesExactManifestAndBoundedInstallDirectory(t *testing.T) {
	root := t.TempDir()
	steamApps := filepath.Join(root, "steamapps")
	installPath := filepath.Join(steamApps, "common", "Exact Game")
	if err := os.MkdirAll(installPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(steamApps, "appmanifest_12345.acf"), []byte(`"AppState"
{
	"appid"		"12345"
	"installdir"		"Exact Game"
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, found := observeSteamProduct([]string{root}, "12345"); !found || got != installPath {
		t.Fatalf("exact Steam observation = %q, %v", got, found)
	}
	if got, found := observeSteamProduct([]string{root}, "1234"); found || got != "" {
		t.Fatalf("partial App ID matched: %q, %v", got, found)
	}
	if err := os.WriteFile(filepath.Join(steamApps, "appmanifest_999.acf"), []byte(`"AppState"
{
	"installdir" ".."
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, found := observeSteamProduct([]string{root}, "999"); found || got != "" {
		t.Fatalf("traversing install directory accepted: %q, %v", got, found)
	}
}
