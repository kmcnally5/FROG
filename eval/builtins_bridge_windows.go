//go:build windows

package eval

import (
	"os/exec"
	"syscall"
)

// Windows-equivalent of POSIX process groups.
// CREATE_NEW_PROCESS_GROUP makes the spawned process the root of a new group
// so Ctrl-Break events (and group kills) target the bridge alone.
const createNewProcessGroup = 0x00000200

func configureBridgeProcess(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNewProcessGroup
}

// killBridgeProcess terminates the bridge and any descendants on Windows.
// taskkill /T = tree, /F = force, /PID = target.
func killBridgeProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	tk := exec.Command("taskkill", "/T", "/F", "/PID", itoa(pid))
	if err := tk.Run(); err != nil {
		_ = cmd.Process.Kill()
	}
}

// Local itoa avoids pulling strconv in for one call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
