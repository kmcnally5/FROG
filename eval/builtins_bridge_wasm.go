//go:build js && wasm

package eval

import "os/exec"

// configureBridgeProcess / killBridgeProcess — no-op stubs for the WASM
// build. The subprocess code path in spawnBridge compiles under WASM
// (os/exec compiles fine) but would fail at runtime when exec.Command's
// Start() returns "fork/exec not implemented on js" — that's the
// expected outcome for someone trying to spawn a subprocess from a
// browser. These stubs just keep the file linkable.
//
// Worker bridges (the right transport for browser-side kLex) go through
// eval/bridge_worker_wasm.go and never touch these.

func configureBridgeProcess(cmd *exec.Cmd) {}

func killBridgeProcess(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
