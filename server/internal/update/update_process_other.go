//go:build !windows

package update

import "os/exec"

func configureDetachedUpdateCommand(cmd *exec.Cmd) {
	_ = cmd
}
