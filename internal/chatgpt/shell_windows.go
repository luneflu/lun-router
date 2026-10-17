//go:build windows

package chatgpt

import (
	"os"
	"os/exec"
	"time"
)

// configureProcessGroup on Windows relies on exec's default process kill.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
}

// setProcessGroup on Windows is a no-op here; killProcessGroup kills the process.
func setProcessGroup(cmd *exec.Cmd) {}

// killProcessGroup terminates a process; Windows has no process-group kill here.
func killProcessGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
