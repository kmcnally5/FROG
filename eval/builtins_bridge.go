//go:build !js

package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"klex/ast"
	"os"
	"os/exec"
	"path/filepath"
	"bufio"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Active-bridge registry
// ─────────────────────────────────────────────────────────────────────────────

var (
	activeBridgesMu sync.Mutex
	activeBridges   = make(map[*Bridge]struct{})
)

func registerBridge(b *Bridge) {
	activeBridgesMu.Lock()
	activeBridges[b] = struct{}{}
	activeBridgesMu.Unlock()
}

func unregisterBridge(b *Bridge) {
	activeBridgesMu.Lock()
	delete(activeBridges, b)
	activeBridgesMu.Unlock()
}

// CleanupAllBridges force-kills every active bridge.
// Called from main.go on SIGINT / SIGTERM / normal exit.
func CleanupAllBridges() {
	activeBridgesMu.Lock()
	toKill := make([]*Bridge, 0, len(activeBridges))
	for b := range activeBridges {
		toKill = append(toKill, b)
	}
	activeBridges = make(map[*Bridge]struct{})
	activeBridgesMu.Unlock()

	for _, b := range toKill {
		if b.stdin != nil {
			_ = b.stdin.Close()
		}
		killBridgeProcess(b.Cmd)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// JSON ↔ kLex marshalling
// ─────────────────────────────────────────────────────────────────────────────

func bridgeToJSON(v Object) interface{} {
	switch val := v.(type) {
	case *Integer:
		return val.Value
	case *Float:
		return val.Value
	case *Boolean:
		return val.Value
	case *String:
		return val.Value
	case *Null:
		return nil
	case *Array:
		out := make([]interface{}, len(val.Elements))
		for i, el := range val.Elements {
			out[i] = bridgeToJSON(el)
		}
		return out
	case *Hash:
		out := make(map[string]interface{}, len(val.Pairs))
		for _, pair := range val.Pairs {
			out[pair.Key.Inspect()] = bridgeToJSON(pair.Value)
		}
		return out
	default:
		return val.Inspect()
	}
}

func jsonToKLex(v interface{}) Object {
	if v == nil {
		return NULL
	}
	switch val := v.(type) {
	case bool:
		if val {
			return TRUE
		}
		return FALSE
	case float64:
		if float64(int(val)) == val {
			return &Integer{Value: int(val)}
		}
		return &Float{Value: val}
	case string:
		return &String{Value: val}
	case []interface{}:
		elements := make([]Object, len(val))
		for i, el := range val {
			elements[i] = jsonToKLex(el)
		}
		return &Array{Elements: elements}
	case map[string]interface{}:
		h := &Hash{Pairs: make(map[HashKey]HashPair, len(val))}
		for k, v := range val {
			key := &String{Value: k}
			hk := HashKey{Type: STRING_OBJ, Value: k}
			h.Pairs[hk] = HashPair{Key: key, Value: jsonToKLex(v)}
		}
		return h
	default:
		return &String{Value: fmt.Sprintf("%v", val)}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Options-hash parsing
// ─────────────────────────────────────────────────────────────────────────────

type bridgeOpts struct {
	timeout   time.Duration
	maxBytes  int
	stderrLog string
}

const (
	defaultMaxBytes = 1024 * 1024
	maxAllowedBytes = 256 * 1024 * 1024
	stderrRingSize  = 4096
	notifBufSize    = 256
)

func parseBridgeOpts(opts Object) (bridgeOpts, Object) {
	out := bridgeOpts{timeout: 0, maxBytes: defaultMaxBytes, stderrLog: ""}
	if opts == nil || opts == NULL {
		return out, nil
	}
	h, ok := opts.(*Hash)
	if !ok {
		return out, bridgeError("BRIDGE_OPTS_INVALID",
			fmt.Sprintf("nativeBridge: opts must be hash, got %s", opts.Type()))
	}

	for _, pair := range h.Pairs {
		keyStr, ok := pair.Key.(*String)
		if !ok {
			continue
		}
		switch keyStr.Value {
		case "timeout_seconds":
			switch v := pair.Value.(type) {
			case *Integer:
				if v.Value < 0 {
					return out, bridgeError("BRIDGE_OPTS_INVALID", "nativeBridge: timeout_seconds must be >= 0")
				}
				out.timeout = time.Duration(v.Value) * time.Second
			case *Float:
				if v.Value < 0 {
					return out, bridgeError("BRIDGE_OPTS_INVALID", "nativeBridge: timeout_seconds must be >= 0")
				}
				out.timeout = time.Duration(v.Value * float64(time.Second))
			case *Null:
				out.timeout = 0
			default:
				return out, bridgeError("BRIDGE_OPTS_INVALID",
					fmt.Sprintf("nativeBridge: timeout_seconds must be number, got %s", v.Type()))
			}
		case "max_response_mb":
			v, ok := pair.Value.(*Integer)
			if !ok {
				return out, bridgeError("BRIDGE_OPTS_INVALID",
					fmt.Sprintf("nativeBridge: max_response_mb must be integer, got %s", pair.Value.Type()))
			}
			if v.Value < 1 || v.Value > 256 {
				return out, bridgeError("BRIDGE_OPTS_INVALID",
					fmt.Sprintf("nativeBridge: max_response_mb must be in [1, 256], got %d", v.Value))
			}
			out.maxBytes = v.Value * 1024 * 1024
		case "stderr_log":
			v, ok := pair.Value.(*String)
			if !ok {
				return out, bridgeError("BRIDGE_OPTS_INVALID",
					fmt.Sprintf("nativeBridge: stderr_log must be string, got %s", pair.Value.Type()))
			}
			out.stderrLog = v.Value
		}
	}
	return out, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Error helpers
// ─────────────────────────────────────────────────────────────────────────────

func bridgeError(code, message string) Object {
	return &Tuple{Elements: []Object{NULL, &Error{
		IsUserError: true,
		Code:        code,
		Message:     message,
	}}}
}

func withStderrTail(b *Bridge, message string) string {
	if b == nil || b.stderrBuf == nil {
		return message
	}
	tail := b.stderrBuf.Snapshot()
	if len(tail) == 0 {
		return message
	}
	const maxTail = 500
	if len(tail) > maxTail {
		tail = tail[len(tail)-maxTail:]
	}
	trimmed := strings.TrimSpace(string(tail))
	if trimmed == "" {
		return message
	}
	return message + "\n\n--- bridge stderr (tail) ---\n" + trimmed
}

// ─────────────────────────────────────────────────────────────────────────────
// Bridge lifecycle helpers
// ─────────────────────────────────────────────────────────────────────────────

// taintAllBridge marks the bridge as tainted, snapshots and clears the pending
// map, sends nil to every waiting bridgeCall, and closes the notification
// channel. Safe to call multiple times (idempotent via the tainted flag).
//
// code is the error code that in-flight calls receive when they unblock via
// nil. The first waiting call gets this code (e.g. "BRIDGE_CLOSED" when the
// subprocess dies, "BRIDGE_TIMEOUT" on a per-call timeout). Subsequent calls
// that check b.tainted before registering always get "BRIDGE_TAINTED".
func taintAllBridge(b *Bridge, code, reason string) {
	b.mu.Lock()
	if b.tainted {
		b.mu.Unlock()
		return
	}
	b.tainted = true
	b.taintMsg = reason
	b.taintCode = code
	snapshot := b.pending
	b.pending = make(map[int]chan []byte)
	b.mu.Unlock()

	for _, ch := range snapshot {
		select {
		case ch <- nil:
		default:
		}
	}

	// Close notification channel — signals EOF to kLex for-in loops.
	b.notifClose.Do(func() {
		if b.notifCh != nil {
			close(b.notifCh.ch)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Reader goroutine
// ─────────────────────────────────────────────────────────────────────────────
// Owns stdout exclusively. Runs for the bridge's entire lifetime.
// Routes:
//   {"notif": <val>}       → b.notifCh  (drop-newest if full)
//   {"id": N, ...}         → pending[N] (response for bridgeCall N)
//   malformed              → logged to stderrBuf, skipped
// When stdout closes (subprocess died), calls taintAllBridge.

func startBridgeReader(b *Bridge) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				taintAllBridge(b, "BRIDGE_CLOSED", fmt.Sprintf("reader goroutine panicked: %v", r))
			}
		}()

		scanner := b.stdout
		for scanner.Scan() {
			line := make([]byte, len(scanner.Bytes()))
			copy(line, scanner.Bytes())
			dispatchBridgeLine(b, line)
		}
		// stdout closed = subprocess died or was killed
		scanErr := scanner.Err()
		msg := "subprocess stdout closed"
		if scanErr != nil && scanErr != io.EOF {
			msg = "reader error: " + scanErr.Error()
		}
		taintAllBridge(b, "BRIDGE_CLOSED", msg)
	}()
}

func dispatchBridgeLine(b *Bridge, line []byte) {
	var msg map[string]interface{}
	if err := json.Unmarshal(line, &msg); err != nil {
		if b.stderrBuf != nil {
			_, _ = b.stderrBuf.Write([]byte("bridge: malformed line: " + string(line) + "\n"))
		}
		return
	}

	_, hasID    := msg["id"]
	notifData, hasNotif := msg["notif"]

	if hasNotif && !hasID {
		// Server-push notification
		if b.notifCh != nil {
			val := jsonToKLex(notifData)
			select {
			case b.notifCh.ch <- val:
			default:
				// Channel full — drop newest (notification channels are lossy by design)
			}
		}
		return
	}

	if hasID {
		// Response to a pending bridgeCall
		rawID := msg["id"]
		fid, ok := rawID.(float64)
		if !ok {
			return
		}
		id := int(fid)

		b.mu.Lock()
		respCh, exists := b.pending[id]
		if exists {
			delete(b.pending, id)
		}
		b.mu.Unlock()

		if !exists {
			// Stale response (from a call that already timed out) — drop silently.
			return
		}
		select {
		case respCh <- line:
		default:
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Stderr drain goroutine
// ─────────────────────────────────────────────────────────────────────────────

// ─────────────────────────────────────────────────────────────────────────────
// Schema handshake — PYTHONPATH injection + __schema__ fetch
// ─────────────────────────────────────────────────────────────────────────────

// klexPythonPath locates the kLex stdlib/python directory so bridges can
// `import klex_bridge` without setting up PYTHONPATH themselves. The result is
// cached after the first lookup.
//
// Search order (first match wins):
//  1. $KLEX_PATH/python                (when KLEX_PATH points at stdlib)
//  2. $CWD/stdlib/python               (running from a project checkout)
//  3. <exe-dir>/stdlib/python          (binary install)
//  4. <exe-parent>/stdlib/python       (bin/klex-style install)
//
// Returns "" if none of those exist with a klex_bridge.py inside — in that
// case bridges that import klex_bridge will see a clear ImportError on stderr,
// captured in bridgeStderr().
var (
	klexPythonPathOnce sync.Once
	klexPythonPathVal  string
)

func klexPythonPath() string {
	klexPythonPathOnce.Do(func() {
		candidates := []string{}
		if kp := os.Getenv("KLEX_PATH"); kp != "" {
			candidates = append(candidates, filepath.Join(kp, "python"))
		}
		if cwd, err := os.Getwd(); err == nil {
			candidates = append(candidates, filepath.Join(cwd, "stdlib", "python"))
		}
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			candidates = append(candidates,
				filepath.Join(exeDir, "stdlib", "python"),
				filepath.Join(filepath.Dir(exeDir), "stdlib", "python"),
			)
		}
		for _, c := range candidates {
			if _, err := os.Stat(filepath.Join(c, "klex_bridge.py")); err == nil {
				if abs, err := filepath.Abs(c); err == nil {
					klexPythonPathVal = abs
				} else {
					klexPythonPathVal = c
				}
				return
			}
		}
	})
	return klexPythonPathVal
}

// buildBridgeEnv returns a child-process env with kLex's stdlib/python prepended
// to PYTHONPATH. Returns nil when the helper dir can't be located, so the
// subprocess inherits the parent env unchanged.
func buildBridgeEnv() []string {
	klexPy := klexPythonPath()
	if klexPy == "" {
		return nil
	}
	existing := os.Getenv("PYTHONPATH")
	newPyPath := klexPy
	if existing != "" {
		newPyPath = klexPy + string(os.PathListSeparator) + existing
	}
	// Filter the original PYTHONPATH out and append the merged value so the
	// child sees exactly one entry rather than two.
	base := os.Environ()
	out := make([]string, 0, len(base)+1)
	for _, e := range base {
		if !strings.HasPrefix(e, "PYTHONPATH=") {
			out = append(out, e)
		}
	}
	return append(out, "PYTHONPATH="+newPyPath)
}

// fetchBridgeSchemas performs the __schema__ handshake right after the bridge
// subprocess starts. Tolerates every failure mode silently — older bridges
// without __schema__ continue to work, just with no kLex-side validation.
// Uses a short timeout so a misbehaving bridge can't block nativeBridge.
func fetchBridgeSchemas(b *Bridge) {
	const handshakeTimeout = 5 * time.Second

	respCh := make(chan []byte, 1)

	b.mu.Lock()
	if b.closed || b.tainted {
		b.mu.Unlock()
		return
	}
	b.nextID++
	id := b.nextID
	b.pending[id] = respCh
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
	}()

	req := map[string]interface{}{"id": id, "fn": "__schema__", "args": []interface{}{}}
	data, err := json.Marshal(req)
	if err != nil {
		return
	}
	b.writeMu.Lock()
	_, werr := b.stdin.Write(append(data, '\n'))
	b.writeMu.Unlock()
	if werr != nil {
		return
	}

	timer := time.NewTimer(handshakeTimeout)
	defer timer.Stop()

	var rawLine []byte
	select {
	case rawLine = <-respCh:
	case <-timer.C:
		return // bridge didn't respond in time; proceed without schemas
	}
	if rawLine == nil {
		return // bridge taintAll'd while we waited
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rawLine, &resp); err != nil {
		return
	}
	// Old-style bridge replies "unknown function: __schema__". Silent skip.
	if _, hasErr := resp["error"]; hasErr {
		return
	}
	schemas, perr := parseSchemaResponse(resp["result"])
	if perr != nil {
		return
	}
	b.schemas = schemas
}

func drainStderr(r io.Reader, ring *BridgeRingBuffer, file io.Writer) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if ring != nil {
				_, _ = ring.Write(chunk)
			}
			if file != nil {
				_, _ = file.Write(chunk)
			}
		}
		if err != nil {
			return
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Builtins
// ─────────────────────────────────────────────────────────────────────────────

func init() {

	// ── nativeBridge(cmd, args, opts?) → (bridge, err) ───────────────────────
	Builtins["nativeBridge"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 && len(args) != 3 {
			return runtimeError("nativeBridge expects 2 or 3 arguments (cmd, args, opts?)", ast.Pos{})
		}
		cmdArg, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("nativeBridge: cmd must be string, got %s", args[0].Type()), ast.Pos{})
		}
		argsArr, ok := args[1].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("nativeBridge: args must be array, got %s", args[1].Type()), ast.Pos{})
		}
		cmdArgs := make([]string, len(argsArr.Elements))
		for i, el := range argsArr.Elements {
			s, ok := el.(*String)
			if !ok {
				return typeError(fmt.Sprintf("nativeBridge: args[%d] must be string, got %s", i, el.Type()), ast.Pos{})
			}
			cmdArgs[i] = s.Value
		}

		var opts bridgeOpts
		if len(args) == 3 {
			parsed, perr := parseBridgeOpts(args[2])
			if perr != nil {
				return perr
			}
			opts = parsed
		} else {
			parsed, _ := parseBridgeOpts(nil)
			opts = parsed
		}

		cmd := exec.Command(cmdArg.Value, cmdArgs...)
		configureBridgeProcess(cmd)

		// Make kLex's stdlib/python discoverable so Python bridges can
		// `import klex_bridge` without setting PYTHONPATH themselves.
		// Harmless for non-Python bridges — extra env vars are ignored.
		if env := buildBridgeEnv(); env != nil {
			cmd.Env = env
		}

		stdin, err := cmd.StdinPipe()
		if err != nil {
			return bridgeError("BRIDGE_ERROR", "nativeBridge: failed to open stdin: "+err.Error())
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return bridgeError("BRIDGE_ERROR", "nativeBridge: failed to open stdout: "+err.Error())
		}
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			return bridgeError("BRIDGE_ERROR", "nativeBridge: failed to open stderr: "+err.Error())
		}
		if err := cmd.Start(); err != nil {
			return bridgeError("BRIDGE_ERROR", "nativeBridge: failed to start process: "+err.Error())
		}

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, opts.maxBytes), opts.maxBytes)

		// Notification channel — always created so early notifications are not lost.
		notifCh := &Channel{
			ch:   make(chan Object, notifBufSize),
			done: make(chan struct{}),
		}

		b := &Bridge{
			Cmd:       cmd,
			stdin:     stdin,
			stdout:    scanner,
			timeout:   opts.timeout,
			stderrLog: opts.stderrLog,
			pending:   make(map[int]chan []byte),
			notifCh:   notifCh,
		}

		// Stderr: ring buffer always; additionally to a file if requested.
		ring := NewBridgeRingBuffer(stderrRingSize)
		b.stderrBuf = ring

		var fileSink io.Writer
		if opts.stderrLog != "" {
			f, ferr := os.OpenFile(opts.stderrLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if ferr == nil {
				fileSink = f
			}
		}
		go drainStderr(stderrPipe, ring, fileSink)

		// Reader goroutine owns stdout for the bridge lifetime.
		startBridgeReader(b)

		registerBridge(b)

		// Best-effort schema handshake. Silently no-ops for bridges that
		// don't implement __schema__ — backward-compatible with every
		// existing kLex bridge.
		fetchBridgeSchemas(b)

		return &Tuple{Elements: []Object{b, NULL}}
	}}

	// ── bridgeCall(bridge, fn, args, timeoutSec?) → (result, err) ────────────
	//
	// Concurrent-call safe: multiple async tasks may call the same bridge
	// simultaneously. Writes are serialised (writeMu) but waits are per-call.
	Builtins["bridgeCall"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 && len(args) != 4 {
			return runtimeError("bridgeCall expects 3 or 4 arguments (bridge, fn, args, timeoutSec?)", ast.Pos{})
		}
		b, ok := args[0].(*Bridge)
		if !ok {
			return typeError(fmt.Sprintf("bridgeCall: first argument must be a bridge, got %s", args[0].Type()), ast.Pos{})
		}
		fnArg, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("bridgeCall: fn must be string, got %s", args[1].Type()), ast.Pos{})
		}
		callArgs, ok := args[2].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("bridgeCall: args must be array, got %s", args[2].Type()), ast.Pos{})
		}

		// Schema validation — fail fast before marshalling and writing to
		// stdin so the user gets the error at the call site, not as a
		// generic protocol error after a round-trip. Skipped when the
		// bridge didn't expose __schema__ (b.schemas == nil) or when this
		// particular function isn't in the schema map.
		if b.schemas != nil {
			if fnSch, ok := b.schemas[fnArg.Value]; ok {
				if vErr := validateArgs(fnArg.Value, fnSch, callArgs.Elements); vErr != nil {
					return bridgeError("BRIDGE_SCHEMA_ARG", vErr.Error())
				}
			}
		}

		// Resolve per-call timeout.
		callTimeout := b.timeout
		if len(args) == 4 {
			switch v := args[3].(type) {
			case *Integer:
				if v.Value < 0 {
					return typeError("bridgeCall: timeoutSec must be >= 0", ast.Pos{})
				}
				callTimeout = time.Duration(v.Value) * time.Second
			case *Float:
				if v.Value < 0 {
					return typeError("bridgeCall: timeoutSec must be >= 0", ast.Pos{})
				}
				callTimeout = time.Duration(v.Value * float64(time.Second))
			case *Null:
				callTimeout = b.timeout
			default:
				return typeError(fmt.Sprintf("bridgeCall: timeoutSec must be number or null, got %s", v.Type()), ast.Pos{})
			}
		}

		// Per-call response channel (buffered 1 — reader never blocks on delivery).
		respCh := make(chan []byte, 1)

		// Atomically check lifecycle state and register response channel.
		// taintAllBridge also holds b.mu when it clears pending, so this is
		// race-free: either taint happens before registration (we see tainted=true
		// here and bail) or after registration (taintAll sends nil to respCh).
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return bridgeError("BRIDGE_CLOSED", "bridge has been closed")
		}
		if b.tainted {
			b.mu.Unlock()
			return bridgeError("BRIDGE_TAINTED", "bridge is tainted: "+b.taintMsg+"  — call bridgeClose() and start a new bridge")
		}
		b.nextID++
		id := b.nextID
		b.pending[id] = respCh
		b.mu.Unlock()

		// Ensure we clean up the pending entry even on early return.
		defer func() {
			b.mu.Lock()
			delete(b.pending, id)
			b.mu.Unlock()
		}()

		// Marshal and write the request.
		jsonArgs := make([]interface{}, len(callArgs.Elements))
		for i, el := range callArgs.Elements {
			jsonArgs[i] = bridgeToJSON(el)
		}
		req := map[string]interface{}{"id": id, "fn": fnArg.Value, "args": jsonArgs}
		data, err := json.Marshal(req)
		if err != nil {
			return bridgeError("BRIDGE_ERROR", "bridgeCall: marshal error: "+err.Error())
		}

		b.writeMu.Lock()
		_, werr := b.stdin.Write(append(data, '\n'))
		b.writeMu.Unlock()

		if werr != nil {
			taintAllBridge(b, "BRIDGE_CLOSED", "stdin write failed: "+werr.Error())
			return bridgeError("BRIDGE_CLOSED", withStderrTail(b, "bridgeCall: write failed: "+werr.Error()))
		}

		// Wait for the reader goroutine to deliver the response.
		var rawLine []byte
		if callTimeout > 0 {
			timer := time.NewTimer(callTimeout)
			defer timer.Stop()
			select {
			case line := <-respCh:
				rawLine = line
			case <-timer.C:
				taintAllBridge(b, "BRIDGE_TIMEOUT", fmt.Sprintf("call to %q timed out after %s", fnArg.Value, callTimeout))
				killBridgeProcess(b.Cmd)
				return bridgeError("BRIDGE_TIMEOUT",
					withStderrTail(b, fmt.Sprintf("bridgeCall: call to %q exceeded %s timeout", fnArg.Value, callTimeout)))
			}
		} else {
			rawLine = <-respCh
		}

		if rawLine == nil {
			// Sent by taintAllBridge — bridge became unavailable while we waited.
			// Use the taintCode so the first failing call reports the actual cause
			// (BRIDGE_CLOSED, BRIDGE_TIMEOUT, etc.) rather than the generic
			// BRIDGE_TAINTED that subsequent calls receive.
			code := b.taintCode
			if code == "" {
				code = "BRIDGE_TAINTED"
			}
			return bridgeError(code, withStderrTail(b, "bridge became unavailable: "+b.taintMsg))
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rawLine, &resp); err != nil {
			return bridgeError("BRIDGE_ERROR",
				withStderrTail(b, "bridgeCall: invalid response: "+strings.TrimSpace(string(rawLine))))
		}
		if errMsg, hasErr := resp["error"]; hasErr {
			return bridgeError("BRIDGE_ERROR", fmt.Sprintf("%v", errMsg))
		}
		return &Tuple{Elements: []Object{jsonToKLex(resp["result"]), NULL}}
	}}

	// ── bridgeClose(bridge) → null ────────────────────────────────────────────
	Builtins["bridgeClose"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bridgeClose expects 1 argument (bridge)", ast.Pos{})
		}
		b, ok := args[0].(*Bridge)
		if !ok {
			return typeError(fmt.Sprintf("bridgeClose: argument must be a bridge, got %s", args[0].Type()), ast.Pos{})
		}

		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return NULL
		}
		b.closed = true
		b.mu.Unlock()

		// Close stdin — a well-written bridge loop sees EOF and exits.
		if b.stdin != nil {
			_ = b.stdin.Close()
		}

		// Wait up to 2s for clean exit; force-kill the process group otherwise.
		done := make(chan error, 1)
		go func() { done <- b.Cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			killBridgeProcess(b.Cmd)
			<-done
		}

		// Taint to unblock any bridgeCalls that are still in flight (race with
		// a concurrent call that had already passed the lifecycle check).
		taintAllBridge(b, "BRIDGE_CLOSED", "bridge closed by caller")

		unregisterBridge(b)
		return NULL
	}}

	// ── bridgeNotifications(bridge) → channel ────────────────────────────────
	//
	// Returns the notification channel for this bridge. The channel receives
	// every {"notif": ...} message the bridge subprocess emits. The channel is
	// closed when the bridge closes (clean EOF for for-in loops).
	//
	// Call this before starting the long operation; notifications emitted
	// before the first call are buffered (256 items, drop-newest).
	//
	// Example:
	//   notifCh = bridgeNotifications(bridge)
	//   async(fn() {
	//       msg, ok = recv(notifCh)
	//       while ok {
	//           println(msg["done"])
	//           msg, ok = recv(notifCh)
	//       }
	//   })
	//   result, err = bridgeCall(bridge, "long_job", [arg])
	Builtins["bridgeNotifications"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bridgeNotifications expects 1 argument (bridge)", ast.Pos{})
		}
		b, ok := args[0].(*Bridge)
		if !ok {
			return typeError(fmt.Sprintf("bridgeNotifications: argument must be a bridge, got %s", args[0].Type()), ast.Pos{})
		}
		if b.notifCh == nil {
			// Shouldn't happen with Phase 2 nativeBridge, but guard anyway.
			b.notifCh = &Channel{
				ch:   make(chan Object, notifBufSize),
				done: make(chan struct{}),
			}
		}
		return b.notifCh
	}}

	// ── bridgeStderr(bridge) → array of strings ───────────────────────────────
	Builtins["bridgeStderr"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bridgeStderr expects 1 argument (bridge)", ast.Pos{})
		}
		b, ok := args[0].(*Bridge)
		if !ok {
			return typeError(fmt.Sprintf("bridgeStderr: argument must be a bridge, got %s", args[0].Type()), ast.Pos{})
		}
		if b.stderrBuf == nil {
			return &Array{Elements: []Object{}}
		}
		tail := b.stderrBuf.Snapshot()
		if len(tail) == 0 {
			return &Array{Elements: []Object{}}
		}
		lines := strings.Split(strings.TrimRight(string(tail), "\n"), "\n")
		out := make([]Object, len(lines))
		for i, l := range lines {
			out[i] = &String{Value: l}
		}
		return &Array{Elements: out}
	}}

	// ── bridgeSchema(bridge, fn?) → hash | null ───────────────────────────────
	//
	// Returns the schema map declared by the bridge (via __schema__) for
	// introspection. With one argument, returns a hash of every handler keyed
	// by name. With two arguments, returns the single handler's schema or
	// null if it isn't declared. Returns null overall when the bridge
	// doesn't expose schemas.
	//
	// Each schema hash is shaped:
	//   { "args": [[name, type], ...], "returns": type }
	Builtins["bridgeSchema"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 && len(args) != 2 {
			return runtimeError("bridgeSchema expects 1 or 2 arguments (bridge, fn?)", ast.Pos{})
		}
		b, ok := args[0].(*Bridge)
		if !ok {
			return typeError(fmt.Sprintf("bridgeSchema: first argument must be a bridge, got %s", args[0].Type()), ast.Pos{})
		}
		if b.schemas == nil {
			return NULL
		}
		if len(args) == 1 {
			return fnSchemaMapToHash(b.schemas)
		}
		fnName, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("bridgeSchema: fn must be string, got %s", args[1].Type()), ast.Pos{})
		}
		fn, ok := b.schemas[fnName.Value]
		if !ok {
			return NULL
		}
		return fnSchemaToHash(fn)
	}}
}
