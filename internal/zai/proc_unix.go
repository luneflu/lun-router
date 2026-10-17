//go:build !windows

package zai

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the browser in its own process group so the whole
// Chromium tree can be killed as a unit. Unused on Windows.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessGroup SIGKILLs the process group led by pid.
func killProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
