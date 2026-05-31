//go:build unix

package eval

import (
	"os/exec"
	"syscall"
)

// setDetached configures a Cmd so the child survives the parent's terminal
// closing. On Unix this means a new process group (Setpgid) — without it,
// the child receives SIGHUP/SIGINT routed to the controlling terminal.
func setDetached(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
