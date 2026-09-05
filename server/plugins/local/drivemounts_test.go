package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestATruncatedVolumeLabelStillYieldsTheAccount(t *testing.T) {
	// Windows caps a volume label at 32 characters, so the accounts most worth
	// detecting are exactly the ones that arrive cut off. Measured on a real
	// machine: "green.fuzer@gmail.com - Googl..." is what the API returns for a
	// drive whose full label would be "... - Google Drive".
	for _, test := range []struct {
		label   string
		account string
	}{
		{"green.fuzer@gmail.com - Googl...", "green.fuzer@gmail.com"},
		{"theshaharsteam@gmail.com - Go...", "theshaharsteam@gmail.com"},
		{"noharb@gmail.com - Google Drive", "noharb@gmail.com"},
		{"someone@example.com - Google", "someone@example.com"},
		{"someone@example.com - Goog", "someone@example.com"},
	} {
		account, ok := googleDriveAccount(test.label)
		if !ok || account != test.account {
			t.Errorf("googleDriveAccount(%q) = (%q, %v), want (%q, true)", test.label, account, ok, test.account)
		}
	}
}

func TestAnUnlabelledDriveMountIsStillRecognised(t *testing.T) {
	// A Drive mount that never recorded an account is still a Drive mount; it
	// just cannot be attributed, and the path has to speak for itself.
	account, ok := googleDriveAccount("Google Drive")
	if !ok || account != "" {
		t.Fatalf("account = %q, ok = %v; want an unattributed match", account, ok)
	}
}

func TestOrdinaryDrivesAreNotMistakenForGoogleDrive(t *testing.T) {
	// The cost of a false positive is offering someone's system disk as their
	// game library, so the separator alone must not be enough.
	for _, label := range []string{
		"",
		"   ",
		"Windows-SSD",
		"Backup - External",        // has the separator, no address
		"someone@example.com",      // has an address, no separator
		"someone@example.com - Gg", // tail is too short to be "Google Drive"
		"someone@example.com - Dropbox",
		"someone@example.com - Google Drive Extra", // longer than the real suffix
	} {
		if account, ok := googleDriveAccount(label); ok {
			t.Errorf("googleDriveAccount(%q) matched as %q, want no match", label, account)
		}
	}
}

func TestTheOfferedFolderIsMyDriveWhenItExists(t *testing.T) {
	// Drive for Desktop keeps personal files under "My Drive", so offering the
	// bare mount would cost a click and invite picking a root that also holds
	// shared drives.
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "My Drive"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := preferMyDrive(filepath.ToSlash(root))
	want := filepath.ToSlash(filepath.Join(root, "My Drive"))
	if got != want {
		t.Fatalf("preferMyDrive = %q, want %q", got, want)
	}

	// A mount without it is offered unchanged rather than pointed at a folder
	// that is not there.
	bare := t.TempDir()
	if got := preferMyDrive(filepath.ToSlash(bare)); got != filepath.ToSlash(bare) {
		t.Fatalf("preferMyDrive = %q, want the mount itself", got)
	}
}

func TestAMountIsNamedByAccountSoSeveralCanBeToldApart(t *testing.T) {
	// Three accounts mounted at once is the case that motivated this: "G:/My
	// Drive" alone does not say which of them it is.
	if label := describeMount("someone@example.com", "G:/My Drive"); label != "someone@example.com" {
		t.Fatalf("label = %q, want the account", label)
	}
	if label := describeMount("", "G:/My Drive"); label != "Google Drive (G:/My Drive)" {
		t.Fatalf("label = %q, want the path to stand in for an unknown account", label)
	}
}

func TestIdentityDecidesWhichRootsAreOffered(t *testing.T) {
	// One binary, two plugin ids. The whole difference between them is this
	// switch, so it is asserted rather than assumed.
	original := pluginID
	t.Cleanup(func() { pluginID = original })

	adoptPluginIdentity(googleDriveDesktopPlugin)
	if !servingGoogleDriveDesktop() {
		t.Fatal("the host said we are the Drive source and we disagreed")
	}

	adoptPluginIdentity(localPluginID)
	if servingGoogleDriveDesktop() {
		t.Fatal("the host said we are the local source and we disagreed")
	}

	// An older host sends nothing, and an unknown id is not a reason to change
	// what this process already answers as.
	adoptPluginIdentity("")
	if pluginID != localPluginID {
		t.Fatalf("pluginID = %q after an empty id, want the previous identity", pluginID)
	}
	adoptPluginIdentity("game-source-something-else")
	if pluginID != localPluginID {
		t.Fatalf("pluginID = %q after an unknown id, want the previous identity", pluginID)
	}
}

func TestDetectionNeverFailsTheCaller(t *testing.T) {
	// Finding nothing is a legitimate answer — it is the expected one on Linux,
	// where Google ships no client — and the console turns it into instructions
	// rather than an error.
	mounts := googleDriveMounts()
	for _, mount := range mounts {
		if mount.Path == "" {
			t.Errorf("a detected mount has no path: %+v", mount)
		}
		if mount.Label == "" {
			t.Errorf("a detected mount has nothing to show the user: %+v", mount)
		}
	}
}
