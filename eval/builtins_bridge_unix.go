//go:build !windows && !js

package eval

import (
	"os/exec"
	"syscall"
)

// configureBridgeProcess puts the bridge subprocess into its own process group
// on Unix-like systems. This lets us kill the entire group (including any
// children the bridge spawns, e.g. git inside github_bridge.py) cleanly.
func configureBridgeProcess(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killBridgeProcess sends SIGKILL to the bridge's entire process group.
// Negative PID = process group ID on POSIX, hence the unary minus.
// Falls back to killing just the process if the group kill fails.
func killBridgeProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
