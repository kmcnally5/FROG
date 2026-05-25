//go:build windows

package eval

import (
	"os/exec"
	"syscall"
)

// setDetached configures a Cmd so the child survives the parent's terminal
// closing. On Windows, children launched without inheriting console handles
// already survive their parent, but joining a NEW process group prevents
// the child from receiving CTRL_C_EVENT sent to the parent's group.
func setDetached(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000200, // CREATE_NEW_PROCESS_GROUP
	}
}
