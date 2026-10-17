//go:build windows

package zai

import (
	"os"
	"os/exec"
)

func setProcessGroup(cmd *exec.Cmd) {}

// killProcessGroup falls back to killing the direct child, which is the only
// option without a process-group API.
func killProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}
