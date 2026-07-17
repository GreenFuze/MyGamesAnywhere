package v1

import "testing"

func TestEmulatorSetupRequestIsClosedToTypedIDsAndActions(t *testing.T) {
	for _, valid := range []EmulatorSetupRequest{{EmulatorID: "retroarch", Action: "install"}, {EmulatorID: "scummvm", Action: "update"}} {
		if err := valid.Validate(); err != nil {
			t.Fatalf("valid request rejected: %v", err)
		}
	}
	for _, invalid := range []EmulatorSetupRequest{{EmulatorID: "Retro Arch", Action: "install"}, {EmulatorID: "retroarch", Action: "uninstall"}, {EmulatorID: "https://example.com", Action: "install"}} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("invalid request accepted: %#v", invalid)
		}
	}
}
