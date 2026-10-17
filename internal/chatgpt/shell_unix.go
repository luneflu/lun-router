//go:build !windows

package chatgpt

import (
	"os/exec"
	"syscall"
	"time"
)

// setProcessGroup puts a command in its own process group so the whole tree can
// be signalled at once. Unlike configureProcessGroup it sets no Cancel function,
// which exec.Cmd only permits for commands built with CommandContext.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessGroup terminates a process and all of its descendants.
func killProcessGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	// Negative pid targets the whole process group.
	return syscall.Kill(-pid, syscall.SIGKILL)
}

// configureProcessGroup makes a shell command killable as a group, so children
// such as `npm` -> `node` cannot survive a timeout and keep a port bound.
func configureProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid signals the whole process group.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
}
