//go:build !js

package eval

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"klex/ast"
	"os"
	"os/exec"
	"path/filepath"
	"bufio"
	"sort"
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

// bridgeWireBytesKey is the JSON object key that tags a base64-encoded binary
// payload on the bridge wire. A hash with EXACTLY this single key gets decoded
// back to *Bytes on the kLex side and to native bytes (Python bytes / Node
// Buffer) on the helper side.
//
// This sentinel form is only emitted when the `binary` capability has been
// negotiated via the __hello__ handshake. Legacy bridges that don't advertise
// `binary` reject Bytes args at the call site with a clear error rather than
// silently falling through to a useless Inspect() string.
const bridgeWireBytesKey = "__bytes__"

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
	case *Bytes:
		// Wire form: {"__bytes__": "<base64>"}.
		return map[string]interface{}{
			bridgeWireBytesKey: base64.StdEncoding.EncodeToString(val.Value),
		}
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

// treeContainsBytes walks any Object for an embedded *Bytes — used by the
// bridge call path to detect Bytes args before serialising so a clear
// "binary capability not negotiated" error can be returned at the call site
// rather than after a round trip.
func treeContainsBytes(v Object) bool {
	switch val := v.(type) {
	case *Bytes:
		return true
	case *Array:
		for _, el := range val.Elements {
			if treeContainsBytes(el) {
				return true
			}
		}
	case *Hash:
		for _, pair := range val.Pairs {
			if treeContainsBytes(pair.Value) {
				return true
			}
		}
	}
	return false
}

// checkBinaryCapability returns a BRIDGE_BINARY_UNSUPPORTED error when any
// element of callArgs contains a *Bytes and the bridge has NOT negotiated
// the `binary` capability via the __hello__ handshake. Returns nil otherwise.
// Caller is the builtin name to embed in the message ("bridgeCall" /
// "bridgeStream") so the error points to the right source line.
func checkBinaryCapability(b *Bridge, caller string, callArgs []Object) Object {
	hasBytes := false
	for _, el := range callArgs {
		if treeContainsBytes(el) {
			hasBytes = true
			break
		}
	}
	if !hasBytes {
		return nil
	}
	if b.capabilities[bridgeCapBinary] {
		return nil
	}
	return bridgeError("BRIDGE_BINARY_UNSUPPORTED", fmt.Sprintf(
		"%s: bridge has not negotiated the 'binary' capability — bytes arguments cannot be sent. "+
			"Upgrade the bridge helper to klex_bridge >= 0.7.0, or pre-encode via bytesToBase64() and declare the schema as 'string'.",
		caller))
}

// bridgeCapBinary is the capability name kLex (and helpers) advertise to
// signal support for the wire-level binary payload form. Centralised as a
// constant so the spelling stays consistent across the call gate, hello
// negotiation, and the bridgeCapabilities advertised set.
const bridgeCapBinary = "binary"

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
		// Wire-protocol bytes sentinel: a single-entry object keyed by
		// bridgeWireBytesKey whose value is a base64 string decodes back to
		// *Bytes. Anything else stays as a Hash. The single-key + string-value
		// shape is strict on purpose — a user hash that happens to have the
		// same key but extra fields or a non-string value is left alone.
		if len(val) == 1 {
			if encoded, ok := val[bridgeWireBytesKey].(string); ok {
				if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
					return &Bytes{Value: decoded}
				}
			}
		}
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
	pendingSnap := b.pending
	streamSnap := b.streams
	b.pending = make(map[int]chan *bridgeResponse)
	b.streams = make(map[int]*Channel)
	b.mu.Unlock()

	// Wake any blocked bridgeCall waiters with a nil signal.
	for _, ch := range pendingSnap {
		select {
		case ch <- nil:
		default:
		}
	}

	// Close any in-flight stream channels so kLex for-in consumers exit
	// rather than hanging forever waiting for items the bridge will never
	// send. The closed channel signals EOF.
	for _, sch := range streamSnap {
		close(sch.ch)
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
			raw := scanner.Bytes() // valid only until next Scan()
			byteCount := len(raw)
			var parsed map[string]interface{}
			if err := json.Unmarshal(raw, &parsed); err != nil {
				if b.stderrBuf != nil {
					_, _ = b.stderrBuf.Write([]byte("bridge: malformed line: " + string(raw) + "\n"))
				}
				continue
			}
			dispatchBridgeLine(b, &bridgeResponse{msg: parsed, bytes: byteCount})
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

// closeChannelDone idempotently closes a Channel's done signal. Both the
// consumer (eval.go's for-in break handler) and the bridge reader (on natural
// stream end) may want to close it; whoever loses the race recovers from the
// "close of closed channel" panic.
func closeChannelDone(ch *Channel) {
	defer func() { recover() }()
	close(ch.done)
}

// defaultStreamWindow is the per-stream in-flight cap when the caller doesn't
// override it. Picked so that 32 × typical-JSON-payload stays under 256 KB
// memory even in adversarial cases; large enough that fast streams never hit
// it under normal use.
const defaultStreamWindow = 32

// readIntKey pulls an integer-valued option out of a hash. Returns
// (value, true) when the key is present and integer-typed; (0, false)
// otherwise. Float values are rejected explicitly so a typo in the
// option (e.g. "window": 32.0 — accidentally a float) gets a clear error
// at the call site rather than a silently truncated value.
func readIntKey(h *Hash, key string) (int, bool) {
	hk := HashKey{Type: STRING_OBJ, Value: key}
	pair, ok := h.Pairs[hk]
	if !ok {
		return 0, false
	}
	switch v := pair.Value.(type) {
	case *Integer:
		return v.Value, true
	case *Null:
		return 0, false
	}
	return 0, false
}

// readTimeoutKey extracts a non-negative timeout value (in seconds) from a
// hash key, accepting either an integer or a float. Returns 0 when the key
// is absent or null. Negative values return -1 so the caller can reject them
// with a clear error.
func readTimeoutKey(h *Hash, key string) time.Duration {
	hk := HashKey{Type: STRING_OBJ, Value: key}
	pair, ok := h.Pairs[hk]
	if !ok {
		return 0
	}
	switch v := pair.Value.(type) {
	case *Integer:
		if v.Value < 0 {
			return -1
		}
		return time.Duration(v.Value) * time.Second
	case *Float:
		if v.Value < 0 {
			return -1
		}
		return time.Duration(v.Value * float64(time.Second))
	case *Null:
		return 0
	}
	return 0
}

// streamWatcher is the per-stream goroutine that owns the stream's shutdown
// paths: consumer break, idle timeout, total timeout, and natural end. It
// merges what used to be a single cancel-on-break watcher with the new
// timeout machinery so there's exactly one place handling lifecycle.
//
// Shutdown paths it has to cover:
//   - Consumer break       → streamCh.done closes; we still need to send
//                             {"cancel": N} to the bridge so it stops yielding.
//   - Idle timeout fires   → no item received for idle duration. Inject a
//                             BRIDGE_TIMEOUT error before close so the consumer
//                             distinguishes timeout from clean end.
//   - Total timeout fires  → same as idle but with a different message.
//   - Natural end          → dispatchBridgeLine removed the stream from
//                             b.streams and closed streamCh.done; we see the
//                             close, find !stillLive, exit without sending
//                             a stale cancel.
//
// resetCh is signalled by dispatchBridgeLine on every stream item; we drain
// it and reset the idle timer. Nil when idle timeout is 0 (no idle watch).
func streamWatcher(b *Bridge, id int, streamCh *Channel, resetCh chan struct{}, idle, total time.Duration) {
	var idleTimer *time.Timer
	var idleC <-chan time.Time
	if idle > 0 {
		idleTimer = time.NewTimer(idle)
		idleC = idleTimer.C
	}
	var totalTimer *time.Timer
	var totalC <-chan time.Time
	if total > 0 {
		totalTimer = time.NewTimer(total)
		totalC = totalTimer.C
	}

	// stopTimers releases any timer goroutines on exit. Safe to call on nils.
	stopTimers := func() {
		if idleTimer != nil && !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		if totalTimer != nil && !totalTimer.Stop() {
			select {
			case <-totalTimer.C:
			default:
			}
		}
	}

	for {
		select {
		case <-streamCh.done:
			stopTimers()
			// Consumer-break path (or watchdog already shut down). If the
			// stream is still live in b.streams, the bridge is still
			// producing — send cancel so it stops. Natural-end path has
			// already deleted from b.streams so we no-op.
			b.mu.Lock()
			_, stillLive := b.streams[id]
			closed := b.closed
			tainted := b.tainted
			delete(b.streamIdleReset, id)
			delete(b.streamUnackedCount, id)
			delete(b.streamAckThreshold, id)
			b.mu.Unlock()
			if stillLive && !closed && !tainted {
				cancelMsg, _ := json.Marshal(map[string]interface{}{"cancel": id})
				b.writeMu.Lock()
				_, _ = b.stdin.Write(append(cancelMsg, '\n'))
				b.writeMu.Unlock()
			}
			return

		case <-idleC:
			fireStreamTimeout(b, id, streamCh, "BRIDGE_TIMEOUT", fmt.Sprintf("bridgeStream: idle %s exceeded — no item received", idle))
			stopTimers()
			return

		case <-totalC:
			fireStreamTimeout(b, id, streamCh, "BRIDGE_TIMEOUT", fmt.Sprintf("bridgeStream: total duration %s exceeded", total))
			stopTimers()
			return

		case <-resetCh:
			if idleTimer != nil {
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				idleTimer.Reset(idle)
			}
		}
	}
}

// fireStreamTimeout is the timeout shutdown path. Sends cancel to the bridge,
// removes the stream from the dispatcher's map (so further items get dropped),
// delivers a BRIDGE_TIMEOUT *Error as the channel's final item, and closes
// both the data and done channels. The consumer's for-in loop sees the error
// item, can detect it via type(item) == "ERROR" + item.code == "BRIDGE_TIMEOUT",
// and exits naturally.
func fireStreamTimeout(b *Bridge, id int, streamCh *Channel, code, msg string) {
	// Send cancel first so the bridge stops producing while we tear down.
	b.mu.Lock()
	_, stillLive := b.streams[id]
	closed := b.closed
	tainted := b.tainted
	b.mu.Unlock()
	if stillLive && !closed && !tainted {
		cancelMsg, _ := json.Marshal(map[string]interface{}{"cancel": id})
		b.writeMu.Lock()
		_, _ = b.stdin.Write(append(cancelMsg, '\n'))
		b.writeMu.Unlock()
	}

	// Remove from the dispatcher's maps. After this point, dispatchBridgeLine
	// drops any further stream items addressed to this id silently.
	b.mu.Lock()
	delete(b.streams, id)
	delete(b.streamIdleReset, id)
	delete(b.streamUnackedCount, id)
	delete(b.streamAckThreshold, id)
	b.mu.Unlock()

	// Deliver the timeout error as the final channel item. Select on done so
	// we don't block forever if the consumer abandoned the channel before the
	// timeout fired.
	timeoutErr := &Error{
		Kind:        RuntimeErr,
		Code:        code,
		Message:     msg,
		IsUserError: true,
	}
	select {
	case streamCh.ch <- timeoutErr:
	case <-streamCh.done:
	}
	close(streamCh.ch)
	closeChannelDone(streamCh)
}

func dispatchBridgeLine(b *Bridge, resp *bridgeResponse) {
	msg := resp.msg

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
		// Response to a pending bridgeCall or bridgeStream
		rawID := msg["id"]
		fid, ok := rawID.(float64)
		if !ok {
			return
		}
		id := int(fid)

		_, hasStreamItem := msg["stream"]
		_, hasStreamEnd := msg["stream_end"]
		_, hasError := msg["error"]

		b.mu.Lock()
		streamCh, isStream := b.streams[id]
		respCh, isPending := b.pending[id]
		b.mu.Unlock()

		// ---- Stream path ----
		if isStream {
			if hasStreamEnd {
				// Clean end of stream — close the channel so for-in exits.
				// Also close done so the streamWatcher goroutine spawned in
				// bridgeStream() unblocks and exits without sending a stale
				// cancel for a stream that already finished.
				b.mu.Lock()
				delete(b.streams, id)
				delete(b.streamIdleReset, id)
				delete(b.streamUnackedCount, id)
				delete(b.streamAckThreshold, id)
				b.mu.Unlock()
				close(streamCh.ch)
				closeChannelDone(streamCh)
				return
			}
			if hasError {
				// Mid-stream error — deliver as an *Error value, then close.
				// Consumers see it as a final item on the channel and can
				// detect via type(item) == "ERROR".
				e := &Error{
					Kind:        RuntimeErr,
					Code:        "BRIDGE_ERROR",
					Message:     fmt.Sprintf("%v", msg["error"]),
					IsUserError: true,
				}
				if t, ok := msg["error_type"].(string); ok {
					e.ErrorType = t
				}
				if t, ok := msg["traceback"].(string); ok {
					e.Traceback = t
				}
				select {
				case streamCh.ch <- e:
				case <-streamCh.done:
				}
				b.mu.Lock()
				delete(b.streams, id)
				delete(b.streamIdleReset, id)
				delete(b.streamUnackedCount, id)
				delete(b.streamAckThreshold, id)
				b.mu.Unlock()
				close(streamCh.ch)
				closeChannelDone(streamCh)
				return
			}
			if hasStreamItem {
				val := jsonToKLex(msg["stream"])
				// Honour consumer cancellation (kLex Channel.done closed when
				// the user breaks out of a for-in loop). If cancelled, drop
				// the item silently — the bridge keeps producing but we don't
				// deliver. Backpressure happens naturally via the buffered
				// channel until the bridge finishes.
				select {
				case streamCh.ch <- val:
				case <-streamCh.done:
				}
				// Signal the streamWatcher to reset its idle timer. Buffered
				// channel + non-blocking send means rapid bursts coalesce into
				// a single reset — exactly what we want.
				// Also bump the unacked counter and emit an ack if we've
				// crossed the half-window threshold. Doing both under the
				// same lock keeps the bookkeeping cheap.
				b.mu.Lock()
				resetCh := b.streamIdleReset[id]
				threshold := b.streamAckThreshold[id]
				ackBatch := 0
				if threshold > 0 {
					b.streamUnackedCount[id]++
					if b.streamUnackedCount[id] >= threshold {
						ackBatch = b.streamUnackedCount[id]
						b.streamUnackedCount[id] = 0
					}
				}
				b.mu.Unlock()
				if resetCh != nil {
					select {
					case resetCh <- struct{}{}:
					default:
					}
				}
				if ackBatch > 0 {
					ackMsg, _ := json.Marshal(map[string]interface{}{"ack": ackBatch, "id": id})
					b.writeMu.Lock()
					_, _ = b.stdin.Write(append(ackMsg, '\n'))
					b.writeMu.Unlock()
				}
				return
			}
			// Unknown stream message — ignore.
			return
		}

		// ---- Single-response path (existing behaviour) ----
		if isPending {
			b.mu.Lock()
			delete(b.pending, id)
			b.mu.Unlock()
			select {
			case respCh <- resp:
			default:
			}
		}
		// Neither — stale response, drop silently.
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
	klexNodePathOnce   sync.Once
	klexNodePathVal    string
)

// klexHelperPath locates a stdlib/<lang> directory containing the named
// sentinel file. Caches its result in dest under the supplied sync.Once.
//
// Search order (first match wins) mirrors kLex's overall path resolution:
//   1. $KLEX_PATH/<lang>           (KLEX_PATH points at stdlib)
//   2. $CWD/stdlib/<lang>          (running from a project checkout)
//   3. <exe-dir>/stdlib/<lang>     (binary install)
//   4. <exe-parent>/stdlib/<lang>  (bin/klex-style install)
func klexHelperPath(once *sync.Once, dest *string, lang, sentinel string) string {
	once.Do(func() {
		candidates := []string{}
		if kp := os.Getenv("KLEX_PATH"); kp != "" {
			candidates = append(candidates, filepath.Join(kp, lang))
		}
		if cwd, err := os.Getwd(); err == nil {
			candidates = append(candidates, filepath.Join(cwd, "stdlib", lang))
		}
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			candidates = append(candidates,
				filepath.Join(exeDir, "stdlib", lang),
				filepath.Join(filepath.Dir(exeDir), "stdlib", lang),
			)
		}
		for _, c := range candidates {
			if _, err := os.Stat(filepath.Join(c, sentinel)); err == nil {
				if abs, err := filepath.Abs(c); err == nil {
					*dest = abs
				} else {
					*dest = c
				}
				return
			}
		}
	})
	return *dest
}

func klexPythonPath() string {
	return klexHelperPath(&klexPythonPathOnce, &klexPythonPathVal, "python", "klex_bridge.py")
}

func klexNodePath() string {
	return klexHelperPath(&klexNodePathOnce, &klexNodePathVal, "node", "klex_bridge.js")
}

// buildBridgeEnv returns a child-process env with kLex's helper directories
// prepended to PYTHONPATH (for Python bridges) and NODE_PATH (for Node).
// Both are appended unconditionally — they're harmless for bridges in other
// languages. Returns nil when neither helper dir is locatable; the subprocess
// then inherits the parent env unchanged.
func buildBridgeEnv() []string {
	klexPy := klexPythonPath()
	klexJs := klexNodePath()
	if klexPy == "" && klexJs == "" {
		return nil
	}
	// Filter the originals out so the child sees exactly one entry each.
	base := os.Environ()
	out := make([]string, 0, len(base)+2)
	for _, e := range base {
		if strings.HasPrefix(e, "PYTHONPATH=") || strings.HasPrefix(e, "NODE_PATH=") {
			continue
		}
		out = append(out, e)
	}
	if klexPy != "" {
		existing := os.Getenv("PYTHONPATH")
		merged := klexPy
		if existing != "" {
			merged = klexPy + string(os.PathListSeparator) + existing
		}
		out = append(out, "PYTHONPATH="+merged)
	}
	if klexJs != "" {
		existing := os.Getenv("NODE_PATH")
		merged := klexJs
		if existing != "" {
			merged = klexJs + string(os.PathListSeparator) + existing
		}
		out = append(out, "NODE_PATH="+merged)
	}
	return out
}

// klexProtocolVersion is the wire protocol version kLex advertises in the
// __hello__ handshake. Bumped only for incompatible breaking changes; new
// additive features are gated through capabilities, not version.
const klexProtocolVersion = 1

// klexCapabilities is the set of capability names kLex advertises during
// __hello__. The negotiated set with a bridge is the intersection of this
// and the bridge's reply. Only capabilities that gate observable behaviour
// are listed — features that have always been on the wire (stream, cancel,
// backpressure, notif, timeout) are implicitly assumed and not advertised.
var klexCapabilities = []string{"schema", bridgeCapBinary}

// fetchBridgeHello performs the __hello__ handshake right after the bridge
// subprocess starts. Negotiates protocol version + capability set so future
// features (binary payloads first) can be gated cleanly.
//
// Tolerates every failure mode silently — bridges without __hello__ are
// treated as protocol 0 with no capabilities, exactly preserving today's
// behaviour. Runs before fetchBridgeSchemas so capability data is in place
// before any other handshake step inspects it.
func fetchBridgeHello(b *Bridge) {
	const handshakeTimeout = 5 * time.Second

	respCh := make(chan *bridgeResponse, 1)

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

	caps := make([]interface{}, len(klexCapabilities))
	for i, c := range klexCapabilities {
		caps[i] = c
	}
	clientInfo := map[string]interface{}{
		"protocol":     klexProtocolVersion,
		"capabilities": caps,
		"client":       "klex",
	}
	req := map[string]interface{}{
		"id":   id,
		"fn":   "__hello__",
		"args": []interface{}{clientInfo},
	}
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

	var br *bridgeResponse
	select {
	case br = <-respCh:
	case <-timer.C:
		return // bridge didn't respond in time; proceed as v0.
	}
	if br == nil {
		return // bridge taintAll'd while we waited.
	}
	resp := br.msg
	// Old-style bridge replies "unknown function: __hello__". Silent skip —
	// the bridge stays at protocol 0 with no capabilities.
	if _, hasErr := resp["error"]; hasErr {
		return
	}
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return
	}

	// Protocol version — best effort. If absent or malformed, leave at 0.
	if v, ok := result["protocol"].(float64); ok {
		b.protocol = int(v)
	}

	// Capabilities — intersect bridge's advertised set with kLex's.
	bridgeCaps := map[string]bool{}
	if rawCaps, ok := result["capabilities"].([]interface{}); ok {
		for _, c := range rawCaps {
			if s, ok := c.(string); ok {
				bridgeCaps[s] = true
			}
		}
	}
	negotiated := make(map[string]bool, len(klexCapabilities))
	for _, c := range klexCapabilities {
		if bridgeCaps[c] {
			negotiated[c] = true
		}
	}
	b.capabilities = negotiated

	// Helper identity — purely informational; surfaced through bridgeInfo().
	if s, ok := result["helper"].(string); ok {
		b.helperInfo = s
	}
	if s, ok := result["language"].(string); ok {
		b.language = s
	}
	if s, ok := result["language_version"].(string); ok {
		b.languageVersion = s
	}
}

// fetchBridgeSchemas performs the __schema__ handshake right after the bridge
// subprocess starts. Tolerates every failure mode silently — older bridges
// without __schema__ continue to work, just with no kLex-side validation.
// Uses a short timeout so a misbehaving bridge can't block nativeBridge.
func fetchBridgeSchemas(b *Bridge) {
	const handshakeTimeout = 5 * time.Second

	respCh := make(chan *bridgeResponse, 1)

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

	var br *bridgeResponse
	select {
	case br = <-respCh:
	case <-timer.C:
		return // bridge didn't respond in time; proceed without schemas
	}
	if br == nil {
		return // bridge taintAll'd while we waited
	}
	resp := br.msg
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

// spawnBridge starts one subprocess and wires up the kLex-side plumbing
// (stdin/stdout/stderr pipes, scanner, reader goroutine, registry, schema
// handshake). Returns the bridge on success or a kLex Error on failure.
//
// Shared between nativeBridge() (one bridge) and bridgePool() (N bridges),
// so both paths produce identical Bridge objects — same schema validation,
// same stderr capture, same cleanup behaviour.
func spawnBridge(cmdName string, cmdArgs []string, opts bridgeOpts) (*Bridge, Object) {
	cmd := exec.Command(cmdName, cmdArgs...)
	configureBridgeProcess(cmd)

	// Make kLex's stdlib/python discoverable so Python bridges can
	// `import klex_bridge` without setting PYTHONPATH themselves.
	// Harmless for non-Python bridges — extra env vars are ignored.
	if env := buildBridgeEnv(); env != nil {
		cmd.Env = env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, bridgeError("BRIDGE_ERROR", "spawnBridge: failed to open stdin: "+err.Error())
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, bridgeError("BRIDGE_ERROR", "spawnBridge: failed to open stdout: "+err.Error())
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, bridgeError("BRIDGE_ERROR", "spawnBridge: failed to open stderr: "+err.Error())
	}
	if err := cmd.Start(); err != nil {
		return nil, bridgeError("BRIDGE_ERROR", "spawnBridge: failed to start process: "+err.Error())
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, opts.maxBytes), opts.maxBytes)

	notifCh := &Channel{
		ch:   make(chan Object, notifBufSize),
		done: make(chan struct{}),
	}

	b := &Bridge{
		Cmd:                cmd,
		stdin:              stdin,
		stdout:             scanner,
		timeout:            opts.timeout,
		stderrLog:          opts.stderrLog,
		pending:            make(map[int]chan *bridgeResponse),
		streams:            make(map[int]*Channel),
		streamIdleReset:    make(map[int]chan struct{}),
		streamUnackedCount: make(map[int]int),
		streamAckThreshold: make(map[int]int),
		notifCh:            notifCh,
		metrics:            &bridgeMetrics{},
	}

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

	startBridgeReader(b)
	registerBridge(b)

	// Best-effort hello handshake — runs first so any future per-feature gating
	// (binary payloads, bidirectional streams) sees the negotiated capability
	// set before subsequent setup. Silently no-ops for bridges without
	// __hello__; those bridges stay at protocol 0 with no capabilities.
	fetchBridgeHello(b)

	// Best-effort schema handshake. Silently no-ops for bridges that don't
	// implement __schema__ — backward-compatible with every existing bridge.
	fetchBridgeSchemas(b)

	return b, nil
}

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

		b, errObj := spawnBridge(cmdArg.Value, cmdArgs, opts)
		if errObj != nil {
			return errObj
		}
		return &Tuple{Elements: []Object{b, NULL}}
	}}

	// ── bridgeCall(bridge, fn, args, timeoutSec?) → (result, err) ────────────
	//
	// Concurrent-call safe: multiple async tasks may call the same bridge
	// simultaneously. Writes are serialised (writeMu) but waits are per-call.
	Builtins["bridgeCall"] = &Builtin{Fn: func(args []Object) (result Object) {
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

		// Metrics instrumentation. We start tracking AS SOON AS the call
		// arguments type-check — every subsequent failure (schema rejection,
		// capability gate, lifecycle taint, marshal failure, write error,
		// timeout, error response) is a real user-facing call result and
		// counts in the totals + errors_by_code breakdown.
		callStart := time.Now()
		var (
			sentBytes int64
			recvBytes int64
		)
		fnName := fnArg.Value
		b.metrics.mu.Lock()
		b.metrics.callsTotal++
		b.metrics.callsInflight++
		b.metrics.mu.Unlock()
		defer func() {
			elapsedNs := time.Since(callStart).Nanoseconds()
			elapsedMs := float64(elapsedNs) / 1_000_000.0
			errCode := ""
			// bridgeCall is contracted to return (result, err). A *Tuple with
			// a user-error in slot 1, or a bare user-error, are the two error
			// shapes we count toward metrics.
			if t, ok := result.(*Tuple); ok && len(t.Elements) == 2 {
				if e, ok := t.Elements[1].(*Error); ok && e.IsUserError {
					errCode = e.Code
				}
			} else if e, ok := result.(*Error); ok && e.IsUserError {
				errCode = e.Code
			}
			b.metrics.mu.Lock()
			b.metrics.bytesSent += sentBytes
			b.metrics.bytesReceived += recvBytes
			b.metrics.mu.Unlock()
			b.metrics.recordCall(fnName, elapsedMs, errCode)

			// Phase 3 agentic hook: fire on_bridge_call once per call,
			// AFTER completion, with timing + ok/err info. Same defer
			// block as metrics so we get the final `result` value.
			// FireBridgeCallHook gates internally on whether a hook is
			// registered (no allocation when off) and inspects result
			// for *Error to populate ok/error fields. User errors come
			// through as a (null, err) tuple — those don't mark the
			// call itself as failed.
			FireBridgeCallHook(fnName, len(callArgs.Elements), elapsedNs, result)
		}()

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

		// Binary capability gate — reject Bytes args if the bridge didn't
		// negotiate the `binary` capability via __hello__. Without this check
		// bridgeToJSON would emit {"__bytes__": ...} that the bridge can't
		// decode, leading to confusing downstream failures.
		if errObj := checkBinaryCapability(b, "bridgeCall", callArgs.Elements); errObj != nil {
			return errObj
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
		respCh := make(chan *bridgeResponse, 1)

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
		sentBytes = int64(len(data) + 1) // +1 for the newline framer

		b.writeMu.Lock()
		_, werr := b.stdin.Write(append(data, '\n'))
		b.writeMu.Unlock()

		if werr != nil {
			taintAllBridge(b, "BRIDGE_CLOSED", "stdin write failed: "+werr.Error())
			return bridgeError("BRIDGE_CLOSED", withStderrTail(b, "bridgeCall: write failed: "+werr.Error()))
		}

		// Wait for the reader goroutine to deliver the response. The reader
		// has already parsed the line into a map, so we receive a wrapper
		// instead of raw bytes — no second Unmarshal needed.
		var br *bridgeResponse
		if callTimeout > 0 {
			timer := time.NewTimer(callTimeout)
			defer timer.Stop()
			select {
			case br = <-respCh:
			case <-timer.C:
				taintAllBridge(b, "BRIDGE_TIMEOUT", fmt.Sprintf("call to %q timed out after %s", fnArg.Value, callTimeout))
				killBridgeProcess(b.Cmd)
				return bridgeError("BRIDGE_TIMEOUT",
					withStderrTail(b, fmt.Sprintf("bridgeCall: call to %q exceeded %s timeout", fnArg.Value, callTimeout)))
			}
		} else {
			br = <-respCh
		}

		if br == nil {
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
		recvBytes = int64(br.bytes + 1) // +1 for the newline framer

		resp := br.msg
		if errMsg, hasErr := resp["error"]; hasErr {
			e := &Error{
				Kind:        RuntimeErr,
				Code:        "BRIDGE_ERROR",
				Message:     fmt.Sprintf("%v", errMsg),
				IsUserError: true,
			}
			// Structured-error extras from klex_bridge.py serve(): the
			// originating exception class name and the full Python traceback.
			// Hand-rolled bridges that send plain {"error": "..."} just leave
			// these empty — backward compatible.
			if t, ok := resp["error_type"].(string); ok {
				e.ErrorType = t
			}
			if t, ok := resp["traceback"].(string); ok {
				e.Traceback = t
			}
			// bridgeCall is contracted to return a (result, err) tuple; wrap
			// the error in the standard shape rather than returning it bare.
			return &Tuple{Elements: []Object{NULL, e}}
		}
		callResult := jsonToKLex(resp["result"])

		// Schema return-value validation. The Python helper validates this
		// too, but checking on the kLex side as well means hand-rolled bridges
		// (which don't use the helper) still get the safety net.
		if b.schemas != nil {
			if fnSch, ok := b.schemas[fnArg.Value]; ok {
				if vErr := ValidateValue(callResult, fnSch.Returns); vErr != nil {
					return bridgeError("BRIDGE_SCHEMA_RETURN",
						fmt.Sprintf("%s: return value: %s", fnArg.Value, vErr.Error()))
				}
			}
		}

		return &Tuple{Elements: []Object{callResult, NULL}}
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

	// ── bridgeInfo(bridge) → hash ────────────────────────────────────────────
	//
	// Returns the protocol metadata negotiated during the __hello__ handshake:
	// protocol version, the set of capabilities both sides agreed on, and
	// optional helper identity fields. A bridge that didn't reply to __hello__
	// is reported as protocol 0 with an empty capabilities array — backward
	// compatible by construction.
	//
	// Shape:
	//   {
	//     "protocol":         1,
	//     "capabilities":     ["schema", "binary"],
	//     "helper":           "klex_bridge.py/0.7.0",
	//     "language":         "python",
	//     "language_version": "3.12.4"
	//   }
	Builtins["bridgeInfo"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bridgeInfo expects 1 argument (bridge)", ast.Pos{})
		}
		b, ok := args[0].(*Bridge)
		if !ok {
			return typeError(fmt.Sprintf("bridgeInfo: argument must be a bridge, got %s", args[0].Type()), ast.Pos{})
		}

		// Sort capabilities so the array order is stable across runs —
		// makes tests deterministic and output easier to eyeball.
		caps := make([]string, 0, len(b.capabilities))
		for c := range b.capabilities {
			caps = append(caps, c)
		}
		sort.Strings(caps)
		capArr := make([]Object, len(caps))
		for i, c := range caps {
			capArr[i] = &String{Value: c}
		}

		h := &Hash{Pairs: make(map[HashKey]HashPair, 5)}
		h.Pairs[HashKey{Type: STRING_OBJ, Value: "protocol"}] = HashPair{
			Key:   &String{Value: "protocol"},
			Value: &Integer{Value: b.protocol},
		}
		h.Pairs[HashKey{Type: STRING_OBJ, Value: "capabilities"}] = HashPair{
			Key:   &String{Value: "capabilities"},
			Value: &Array{Elements: capArr},
		}
		h.Pairs[HashKey{Type: STRING_OBJ, Value: "helper"}] = HashPair{
			Key:   &String{Value: "helper"},
			Value: &String{Value: b.helperInfo},
		}
		h.Pairs[HashKey{Type: STRING_OBJ, Value: "language"}] = HashPair{
			Key:   &String{Value: "language"},
			Value: &String{Value: b.language},
		}
		h.Pairs[HashKey{Type: STRING_OBJ, Value: "language_version"}] = HashPair{
			Key:   &String{Value: "language_version"},
			Value: &String{Value: b.languageVersion},
		}
		return h
	}}

	// ── bridgeMetrics(bridge) → hash ─────────────────────────────────────────
	//
	// Snapshot the bridge's observability counters and per-function latency
	// percentiles. Cheap to call (one mutex acquisition + one sort per
	// function on the latency samples) so a dashboard polling every second
	// adds no measurable overhead.
	//
	// Shape:
	//   {
	//     "calls_total":     N,
	//     "calls_inflight":  M,
	//     "calls_failed":    K,
	//     "streams_total":   S,
	//     "bytes_sent":      bytesOut,
	//     "bytes_received":  bytesIn,
	//     "errors_by_code":  {"BRIDGE_TIMEOUT": 4, ...},
	//     "per_function":    {
	//        "add":   {"count": 230, "errors": 2, "p50_ms": 12.0, "p95_ms": 84.0, "p99_ms": 230.0},
	//        ...
	//     }
	//   }
	//
	// Percentiles are computed from a 256-sample circular buffer of recent
	// call latencies (in milliseconds). With fewer than 256 calls, the
	// percentiles use what's available.
	Builtins["bridgeMetrics"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bridgeMetrics expects 1 argument (bridge)", ast.Pos{})
		}
		b, ok := args[0].(*Bridge)
		if !ok {
			return typeError(fmt.Sprintf("bridgeMetrics: argument must be a bridge, got %s", args[0].Type()), ast.Pos{})
		}
		m := b.metrics
		m.mu.Lock()
		defer m.mu.Unlock()

		intPair := func(k string, v int64) (HashKey, HashPair) {
			return HashKey{Type: STRING_OBJ, Value: k},
				HashPair{Key: &String{Value: k}, Value: &Integer{Value: int(v)}}
		}
		floatPair := func(k string, v float64) (HashKey, HashPair) {
			return HashKey{Type: STRING_OBJ, Value: k},
				HashPair{Key: &String{Value: k}, Value: &Float{Value: v}}
		}

		// Top-level counters.
		out := &Hash{Pairs: make(map[HashKey]HashPair, 12)}
		for _, kv := range []struct {
			k string
			v int64
		}{
			{"calls_total", m.callsTotal},
			{"calls_inflight", m.callsInflight},
			{"calls_failed", m.callsFailed},
			{"streams_total", m.streamsTotal},
			{"bytes_sent", m.bytesSent},
			{"bytes_received", m.bytesReceived},
		} {
			hk, hp := intPair(kv.k, kv.v)
			out.Pairs[hk] = hp
		}

		// errors_by_code: hash of code → count, sorted for stable output.
		errH := &Hash{Pairs: make(map[HashKey]HashPair, len(m.errorsByCode))}
		codes := make([]string, 0, len(m.errorsByCode))
		for c := range m.errorsByCode {
			codes = append(codes, c)
		}
		sort.Strings(codes)
		for _, c := range codes {
			hk, hp := intPair(c, m.errorsByCode[c])
			errH.Pairs[hk] = hp
		}
		out.Pairs[HashKey{Type: STRING_OBJ, Value: "errors_by_code"}] = HashPair{
			Key:   &String{Value: "errors_by_code"},
			Value: errH,
		}

		// per_function: each fn → {count, errors, p50_ms, p95_ms, p99_ms}.
		perFn := &Hash{Pairs: make(map[HashKey]HashPair, len(m.perFunction))}
		fnNames := make([]string, 0, len(m.perFunction))
		for name := range m.perFunction {
			fnNames = append(fnNames, name)
		}
		sort.Strings(fnNames)
		for _, name := range fnNames {
			fm := m.perFunction[name]
			n := bridgeMetricsSampleN
			if !fm.wrapped {
				n = fm.wrIdx
			}
			p50, p95, p99 := 0.0, 0.0, 0.0
			if n > 0 {
				sorted := make([]float64, n)
				copy(sorted, fm.samples[:n])
				sort.Float64s(sorted)
				p50 = sorted[(n-1)*50/100]
				p95 = sorted[(n-1)*95/100]
				p99 = sorted[(n-1)*99/100]
			}
			entry := &Hash{Pairs: make(map[HashKey]HashPair, 5)}
			hk, hp := intPair("count", fm.count)
			entry.Pairs[hk] = hp
			hk, hp = intPair("errors", fm.errors)
			entry.Pairs[hk] = hp
			hk, hp = floatPair("p50_ms", p50)
			entry.Pairs[hk] = hp
			hk, hp = floatPair("p95_ms", p95)
			entry.Pairs[hk] = hp
			hk, hp = floatPair("p99_ms", p99)
			entry.Pairs[hk] = hp
			perFn.Pairs[HashKey{Type: STRING_OBJ, Value: name}] = HashPair{
				Key:   &String{Value: name},
				Value: entry,
			}
		}
		out.Pairs[HashKey{Type: STRING_OBJ, Value: "per_function"}] = HashPair{
			Key:   &String{Value: "per_function"},
			Value: perFn,
		}
		return out
	}}

	// ── bridgeStream(bridge, fn, args) → (channel, err) ─────────────────────
	//
	// Calls a STREAMING handler on the bridge subprocess. Returns immediately
	// with a kLex channel; the bridge produces items in the background and
	// the reader goroutine delivers each to the channel.
	//
	// Consumers drain via `for item in ch { ... }`. The channel closes on
	// clean end-of-stream. Mid-stream errors are delivered as the final
	// item — an *Error value the consumer can detect with `type(item) ==
	// "ERROR"`. Breaking out of the for-in loop cancels delivery (further
	// items are dropped) but does not currently signal the bridge to stop
	// producing — that's a Phase 3.5 follow-up.
	//
	// The handler on the bridge side must be registered with @stream_handler
	// (or register(..., stream=True)). Calling a non-streaming handler via
	// bridgeStream returns a BRIDGE_ERROR with a clear message.
	//
	// Errors returned in the second tuple element (before any items arrive):
	//   BRIDGE_CLOSED   — bridge already closed or stdin write failed
	//   BRIDGE_TAINTED  — bridge is unusable after a prior fatal error
	//   BRIDGE_ERROR    — marshal failed
	Builtins["bridgeStream"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 3 || len(args) > 4 {
			return runtimeError("bridgeStream expects 3 or 4 arguments (bridge, fn, args, timeout?)", ast.Pos{})
		}
		b, ok := args[0].(*Bridge)
		if !ok {
			return typeError(fmt.Sprintf("bridgeStream: first argument must be a bridge, got %s", args[0].Type()), ast.Pos{})
		}
		fnArg, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("bridgeStream: fn must be string, got %s", args[1].Type()), ast.Pos{})
		}
		callArgs, ok := args[2].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("bridgeStream: args must be array, got %s", args[2].Type()), ast.Pos{})
		}

		// Parse optional 4th arg. Number = idle seconds (the useful default —
		// "fail if no item arrives for N s"). Hash = explicit options
		// {"idle": N, "total": M, "window": W}. Absent = no timeout and the
		// default backpressure window (32).
		var idleTimeout, totalTimeout time.Duration
		window := defaultStreamWindow
		if len(args) == 4 {
			switch v := args[3].(type) {
			case *Integer:
				if v.Value < 0 {
					return typeError("bridgeStream: idle timeout must be >= 0", ast.Pos{})
				}
				idleTimeout = time.Duration(v.Value) * time.Second
			case *Float:
				if v.Value < 0 {
					return typeError("bridgeStream: idle timeout must be >= 0", ast.Pos{})
				}
				idleTimeout = time.Duration(v.Value * float64(time.Second))
			case *Hash:
				idleTimeout = readTimeoutKey(v, "idle")
				totalTimeout = readTimeoutKey(v, "total")
				if idleTimeout < 0 || totalTimeout < 0 {
					return typeError("bridgeStream: timeout values must be >= 0", ast.Pos{})
				}
				if w, ok := readIntKey(v, "window"); ok {
					if w < 0 {
						return typeError("bridgeStream: window must be >= 0", ast.Pos{})
					}
					window = w
				}
			case *Null:
				// explicit null → no timeout, default window
			default:
				return typeError(fmt.Sprintf("bridgeStream: timeout must be number, hash, or null, got %s", v.Type()), ast.Pos{})
			}
		}

		// Same schema validation as bridgeCall — catch arg mismatches before
		// they hit the wire.
		if b.schemas != nil {
			if fnSch, ok := b.schemas[fnArg.Value]; ok {
				if vErr := validateArgs(fnArg.Value, fnSch, callArgs.Elements); vErr != nil {
					return bridgeError("BRIDGE_SCHEMA_ARG", vErr.Error())
				}
			}
		}

		// Same binary-capability gate as bridgeCall — reject before allocating
		// any per-stream state.
		if errObj := checkBinaryCapability(b, "bridgeStream", callArgs.Elements); errObj != nil {
			return errObj
		}

		// Allocate the consumer-facing channel. Buffer 256 is generous enough
		// to let the bridge run ahead of the consumer without immediately
		// blocking, but small enough to keep memory bounded if the consumer
		// stalls.
		streamCh := &Channel{
			ch:   make(chan Object, 256),
			done: make(chan struct{}),
		}

		// Atomically reserve a request id and register the stream channel
		// against it. Match bridgeCall's lifecycle checks.
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
		b.streams[id] = streamCh
		// Reset channel for the idle-timer watchdog. dispatchBridgeLine signals
		// it once per item; the watcher drains it and resets the idle timer.
		var resetCh chan struct{}
		if idleTimeout > 0 {
			resetCh = make(chan struct{}, 1)
			b.streamIdleReset[id] = resetCh
		}
		// Backpressure threshold = half the window. Sending acks at half-
		// window means the bridge's in-flight count never has to wait for an
		// ack we haven't sent yet, while halving the ack-message overhead
		// vs. acking each item. window=0 disables backpressure entirely
		// (back-compat for hand-rolled bridges that don't speak ack).
		if window > 0 {
			threshold := window / 2
			if threshold < 1 {
				threshold = 1
			}
			b.streamAckThreshold[id] = threshold
			b.streamUnackedCount[id] = 0
		}
		b.mu.Unlock()

		// Build and write the request — `stream: true` selects the streaming
		// path on the Python helper's serve() loop.
		jsonArgs := make([]interface{}, len(callArgs.Elements))
		for i, el := range callArgs.Elements {
			jsonArgs[i] = bridgeToJSON(el)
		}
		req := map[string]interface{}{
			"id":     id,
			"fn":     fnArg.Value,
			"args":   jsonArgs,
			"stream": true,
			"window": window,
		}
		data, err := json.Marshal(req)
		if err != nil {
			b.mu.Lock()
			delete(b.streams, id)
			delete(b.streamIdleReset, id)
			delete(b.streamUnackedCount, id)
			delete(b.streamAckThreshold, id)
			b.mu.Unlock()
			close(streamCh.ch)
			return bridgeError("BRIDGE_ERROR", "bridgeStream: marshal error: "+err.Error())
		}

		b.writeMu.Lock()
		_, werr := b.stdin.Write(append(data, '\n'))
		b.writeMu.Unlock()
		if werr != nil {
			taintAllBridge(b, "BRIDGE_CLOSED", "stdin write failed: "+werr.Error())
			return bridgeError("BRIDGE_CLOSED", withStderrTail(b, "bridgeStream: write failed: "+werr.Error()))
		}

		// Stream metrics — count the start and the request bytes. Per-stream
		// duration and items-received aren't tracked here because their natural
		// observation point is the reader goroutine / consumer; surfacing them
		// would need a separate lifecycle hook.
		b.metrics.mu.Lock()
		b.metrics.streamsTotal++
		b.metrics.bytesSent += int64(len(data) + 1)
		b.metrics.mu.Unlock()

		// Combined stream watcher — handles three shutdown paths in one goroutine:
		//   1. Consumer break       → streamCh.done closes; send cancel to bridge.
		//   2. Idle timeout fires   → no item for N seconds; inject BRIDGE_TIMEOUT,
		//                              cancel the bridge, close the channels.
		//   3. Total timeout fires  → stream exceeded total duration; same as idle.
		//   4. Natural end          → dispatchBridgeLine closes done; we see it,
		//                              find stream gone from b.streams, exit clean.
		go streamWatcher(b, id, streamCh, resetCh, idleTimeout, totalTimeout)

		// Hand the channel to the caller. The reader goroutine takes over
		// from here, delivering items as they arrive on stdout.
		return &Tuple{Elements: []Object{streamCh, NULL}}
	}}
}
