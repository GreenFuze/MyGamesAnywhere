package plugins

import (
	"os/exec"
	"testing"
)

func TestConfigurePluginCommandHidesWindowsConsole(t *testing.T) {
	cmd := exec.Command("test-plugin.exe")
	configurePluginCommand(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow = false, want true")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
}
