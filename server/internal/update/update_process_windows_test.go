//go:build windows

package update

import (
	"os/exec"
	"testing"
)

func TestConfigureDetachedUpdateCommandHidesAndDetachesProcess(t *testing.T) {
	cmd := exec.Command("mga-v0.0.10-windows-amd64-installer.exe")
	configureDetachedUpdateCommand(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow = false, want true")
	}
	for _, flag := range []uintptr{updateCreateNoWindow, updateCreateNewProcessGroup, updateDetachedProcess} {
		if cmd.SysProcAttr.CreationFlags&uint32(flag) == 0 {
			t.Fatalf("CreationFlags = %#x, missing %#x", cmd.SysProcAttr.CreationFlags, flag)
		}
	}
}
