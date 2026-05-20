//go:build windows

package update

import (
	"os/exec"
	"syscall"
)

const (
	updateCreateNoWindow        = 0x08000000
	updateCreateNewProcessGroup = 0x00000200
	updateDetachedProcess       = 0x00000008
)

func configureDetachedUpdateCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= updateCreateNoWindow | updateCreateNewProcessGroup | updateDetachedProcess
}
