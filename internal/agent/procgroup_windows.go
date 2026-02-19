//go:build windows

package agent

import (
	"os/exec"
	"time"
)

// setProcGroup is a no-op on Windows. exec.CommandContext already sends
// os.Kill on context cancellation, and Windows does not support Unix-style
// process groups. The WaitDelay gives child processes a grace period to drain.
func setProcGroup(cmd *exec.Cmd) {
	cmd.WaitDelay = 3 * time.Second
}
