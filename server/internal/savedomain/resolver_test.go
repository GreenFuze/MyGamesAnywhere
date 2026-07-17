package savedomain

import (
	"strings"
	"testing"
)

func TestResolverClassifiesRoutesConservatively(t *testing.T) {
	r := NewResolver()
	steam := Source{SourceGameID: "steam-1", PluginID: "game-source-steam", IntegrationLabel: "Steam"}
	xbox := Source{SourceGameID: "xbox-1", PluginID: "game-source-xbox"}
	local := Source{SourceGameID: "drive-1", PluginID: "game-source-google-drive", IntegrationLabel: "My Games"}

	tests := []struct {
		name       string
		got        Capability
		access     Access
		status     Status
		manager    string
		transfer   Transfer
		canRead    bool
		canWrite   bool
		detailPart string
	}{
		{"browser", r.Browser(local, "emulatorjs"), AccessMGAManaged, StatusAvailable, "mga", TransferSameDomainOnly, true, true, "can back up"},
		{"steam source", r.Source(steam), AccessProviderOpaque, StatusProviderManaged, "provider", TransferUnavailable, false, false, "cannot access"},
		{"xbox source", r.Source(xbox), AccessProviderOpaque, StatusProviderManaged, "provider", TransferUnavailable, false, false, "cannot access"},
		{"xcloud", r.XCloud(xbox), AccessProviderOpaque, StatusProviderManaged, "provider", TransferUnavailable, false, false, "cannot access"},
		{"local source", r.Source(local), AccessLocalFiles, StatusNeedsAdapter, "device", TransferConverterNeeded, false, false, "verified save adapter"},
		{"native install", r.Installed(local, "device-1"), AccessLocalFiles, StatusNeedsAdapter, "device", TransferConverterNeeded, false, false, "verified adapter"},
		{"steam install", r.Installed(steam, "device-1"), AccessProviderOpaque, StatusProviderManaged, "provider", TransferUnavailable, false, false, "cannot access"},
		{"emulator", r.Emulator(local, "device-1", "retroarch", "snes9x"), AccessLocalFiles, StatusNeedsAdapter, "device", TransferConverterNeeded, false, false, "emulator adapter"},
		{"unknown", r.Source(Source{SourceGameID: "new-1", PluginID: "game-source-new"}), AccessUnknown, StatusUnknown, "unknown", TransferUnknown, false, false, "does not yet know"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got.Access != test.access || test.got.Status != test.status || test.got.Manager != test.manager || test.got.Transfer != test.transfer || test.got.MGARead != test.canRead || test.got.MGAWrite != test.canWrite {
				t.Fatalf("capability = %#v", test.got)
			}
			if !strings.HasPrefix(test.got.DomainID, "save:") || !strings.Contains(test.got.Detail, test.detailPart) {
				t.Fatalf("capability = %#v", test.got)
			}
		})
	}
}

func TestResolverKeepsRoutesInSeparateDomains(t *testing.T) {
	r := NewResolver()
	source := Source{SourceGameID: "copy-1", PluginID: "game-source-google-drive"}
	ids := []string{
		r.Browser(source, "emulatorjs").DomainID,
		r.Installed(source, "device-1").DomainID,
		r.Emulator(source, "device-1", "retroarch", "snes9x").DomainID,
		r.Emulator(source, "device-1", "retroarch", "mesen").DomainID,
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate route domain %q", id)
		}
		seen[id] = true
	}
}
