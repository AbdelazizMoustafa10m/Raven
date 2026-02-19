//go:build !windows

package agent

import (
	"os/exec"
	"syscall"
	"time"
)

// setProcGroup configures cmd to run in its own process group and sets up
// Cancel/WaitDelay so that context cancellation kills the entire group
// (including child processes like sleep, curl, etc.) rather than only the
// direct child.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Send SIGKILL to the entire process group (negative PID).
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	// Give child processes a short grace period to drain after the group is
	// killed before forcibly closing their pipe file descriptors.
	cmd.WaitDelay = 3 * time.Second
}
