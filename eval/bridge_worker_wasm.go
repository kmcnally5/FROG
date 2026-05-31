//go:build js && wasm

package eval

// bridge_worker_wasm.go — Web Worker transport for `bridgeOpen`.
//
// In-browser kLex has no subprocess primitive, so the worker transport
// is the way for kLex scripts to call out to JavaScript libraries.
// Same bridge protocol (line-delimited JSON), same kLex-side API
// (bridgeCall / bridgeStream / bridgeClose / bridgeInfo / etc.) — just
// a different wire.
//
// The neat trick that lets us avoid touching bridgeCall et al: the
// existing *Bridge struct uses io.WriteCloser for stdin and a
// bufio.Scanner around io.Reader for stdout. Both are interfaces. We
// implement:
//
//   workerWriteCloser  — wraps worker.postMessage(line) as io.WriteCloser
//   workerReader       — pulls onmessage events from a buffered channel
//                        and delivers them to bufio.Scanner as
//                        newline-terminated lines
//
// startBridgeReader, bridgeCall, bridgeStream, fetchBridgeHello, and
// fetchBridgeSchemas all read/write through those interfaces and never
// touch the underlying transport directly. So they work as-is.
//
// Init wires WorkerTransportSpawn so the dispatcher in builtins_bridge.go
// routes `kind: "worker"` through here.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"syscall/js"
)

func init() {
	WorkerTransportSpawn = spawnWorkerBridgeFromTransport
}

// Keys permitted in a worker transport hash. Strict validation matches
// the subprocess pattern.
var validWorkerKeys = map[string]bool{
	"kind":            true,
	"script":          true, // required
	"type":            true, // "classic" (default) or "module"
	"timeout_seconds": true,
	"max_response_mb": true,
}

func spawnWorkerBridgeFromTransport(transport *Hash) Object {
	// Strict key validation.
	for _, pair := range transport.Pairs {
		ks, ok := pair.Key.(*String)
		if !ok {
			return bridgeError("BRIDGE_TRANSPORT_MISCONFIGURED",
				fmt.Sprintf("worker transport: keys must be strings, got %s", pair.Key.Type()))
		}
		if !validWorkerKeys[ks.Value] {
			return bridgeError("BRIDGE_TRANSPORT_MISCONFIGURED",
				fmt.Sprintf("worker transport: unknown key %q (valid: kind, script, type, timeout_seconds, max_response_mb)", ks.Value))
		}
	}

	// Required: script.
	scriptObj := hashLookup(transport, "script")
	if scriptObj == nil {
		return bridgeError("BRIDGE_TRANSPORT_MISCONFIGURED",
			"worker transport: missing required key 'script'")
	}
	scriptStr, ok := scriptObj.(*String)
	if !ok {
		return bridgeError("BRIDGE_TRANSPORT_MISCONFIGURED",
			fmt.Sprintf("worker transport: 'script' must be a string, got %s", scriptObj.Type()))
	}

	// Optional: type ("classic" or "module").
	workerType := "classic"
	if t := hashLookup(transport, "type"); t != nil {
		if _, isNull := t.(*Null); !isNull {
			ts, ok := t.(*String)
			if !ok {
				return bridgeError("BRIDGE_TRANSPORT_MISCONFIGURED",
					fmt.Sprintf("worker transport: 'type' must be a string, got %s", t.Type()))
			}
			workerType = ts.Value
		}
	}
	if workerType != "classic" && workerType != "module" {
		return bridgeError("BRIDGE_TRANSPORT_MISCONFIGURED",
			fmt.Sprintf("worker transport: 'type' must be \"classic\" or \"module\", got %q", workerType))
	}

	// Shared bridge opts (timeout_seconds, max_response_mb). stderr_log
	// is silently ignored — workers have no continuous stderr.
	opts, perr := parseBridgeOpts(transport)
	if perr != nil {
		return perr
	}

	return spawnWorkerBridge(scriptStr.Value, workerType, opts)
}

func spawnWorkerBridge(scriptURL, workerType string, opts bridgeOpts) Object {
	workerCtor := js.Global().Get("Worker")
	if workerCtor.IsUndefined() {
		return bridgeError("BRIDGE_TRANSPORT_UNAVAILABLE",
			"worker transport: global Worker constructor not present in this WASM host")
	}

	// Worker options ({type: "classic" | "module"}). Pass even for
	// classic because some browsers treat the default differently from
	// an explicit "classic".
	workerOpts := js.Global().Get("Object").New()
	workerOpts.Set("type", workerType)
	worker := workerCtor.New(scriptURL, workerOpts)

	// Stderr ring buffer — fed by onerror events. Workers don't have a
	// continuous stderr stream like subprocesses, but this gives the
	// same surface for error diagnostics via bridgeStderr().
	stderrBuf := NewBridgeRingBuffer(stderrRingSize)

	// Buffered channel between the JS onmessage callback and the Go
	// reader goroutine. 256 matches notifBufSize — well above the
	// in-flight depth a sane bridge ever reaches.
	msgCh := make(chan string, 256)
	doneCh := make(chan struct{})

	reader := &workerReader{ch: msgCh, done: doneCh}

	// onmessage delivers each message as one event. We push the
	// payload onto msgCh; the reader goroutine drains it into the
	// bridge's bufio.Scanner.
	//
	// Note: js.Func instances are intentionally not Released here
	// because there is no portable hook point after worker.terminate()
	// where it's safe to do so (a stray queued message would crash a
	// released Func). One Func per bridge — acceptable for v1.
	onmessage := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		data := args[0].Get("data")
		var line string
		if data.Type() == js.TypeString {
			line = data.String()
		} else {
			// Coerce non-string payloads through JSON.stringify so the
			// kLex side always sees a parseable line.
			line = js.Global().Get("JSON").Call("stringify", data).String()
		}
		select {
		case msgCh <- line:
		default:
			// Buffer overflow — record but don't block the JS thread.
			stderrBuf.Write([]byte("worker: msgCh full, dropped message\n"))
		}
		return nil
	})
	worker.Set("onmessage", onmessage)

	// onerror captures worker-side runtime errors into the stderr
	// ring so they surface via bridgeStderr() and error message tails.
	onerror := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		evt := args[0]
		msg := evt.Get("message")
		if !msg.IsUndefined() {
			stderrBuf.Write([]byte("worker error: " + msg.String() + "\n"))
		}
		return nil
	})
	worker.Set("onerror", onerror)

	writer := &workerWriteCloser{
		worker: worker,
		done:   doneCh,
	}

	scanner := bufio.NewScanner(reader)
	bufSize := opts.maxBytes
	if bufSize <= 0 {
		bufSize = defaultMaxBytes
	}
	scanner.Buffer(make([]byte, bufSize), bufSize)

	notifCh := &Channel{
		ch:   make(chan Object, notifBufSize),
		done: make(chan struct{}),
	}

	b := &Bridge{
		Cmd:                nil, // no subprocess for workers
		stdin:              writer,
		stdout:             scanner,
		timeout:            opts.timeout,
		stderrBuf:          stderrBuf,
		stderrLog:          "",
		pending:            make(map[int]chan *bridgeResponse),
		streams:            make(map[int]*Channel),
		streamIdleReset:    make(map[int]chan struct{}),
		streamUnackedCount: make(map[int]int),
		streamAckThreshold: make(map[int]int),
		notifCh:            notifCh,
		metrics:            &bridgeMetrics{},
	}

	startBridgeReader(b)

	// Best-effort handshakes. Same code as subprocess — talks through
	// b.stdin / b.stdout which are our worker shims.
	fetchBridgeHello(b)
	fetchBridgeSchemas(b)

	return &Tuple{Elements: []Object{b, NULL}}
}

// ─────────────────────────────────────────────────────────────────────────────
// io.WriteCloser over postMessage
// ─────────────────────────────────────────────────────────────────────────────

type workerWriteCloser struct {
	worker js.Value
	done   chan struct{} // closed by Close() — signals workerReader to EOF

	mu     sync.Mutex
	closed bool
}

func (w *workerWriteCloser) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errors.New("worker bridge: write to closed worker")
	}
	// Strip the trailing newline that bridgeCall appends — postMessage
	// delivers each call as one discrete event; the line delimiter is
	// meaningless on this wire.
	line := strings.TrimRight(string(p), "\n")
	w.worker.Call("postMessage", line)
	return len(p), nil
}

func (w *workerWriteCloser) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	// Signal the reader to EOF, then terminate the worker.
	close(w.done)
	w.worker.Call("terminate")
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// io.Reader over a message channel
// ─────────────────────────────────────────────────────────────────────────────

type workerReader struct {
	ch      chan string
	done    chan struct{}
	pending []byte
}

func (r *workerReader) Read(p []byte) (int, error) {
	if len(r.pending) == 0 {
		select {
		case msg, ok := <-r.ch:
			if !ok {
				return 0, io.EOF
			}
			// Append \n so bufio.Scanner sees one line per message.
			r.pending = []byte(msg + "\n")
		case <-r.done:
			return 0, io.EOF
		}
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}
