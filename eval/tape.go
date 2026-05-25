package eval

// tape.go — record-side of the .lextape file format.
//
// When the kLex binary is invoked with --record-tape=PATH, OpenTape is
// called at startup. From that point on, every Fire*Hook callsite in
// builtins_agent.go calls notifyTape with the event kind + payload hash,
// and the writer serializes it to disk as a JSON line.
//
// Design notes:
//
//   - Writes are best-effort. A tape-write failure must not crash the
//     program — the runtime stays functional even if the disk is full
//     or the file becomes unwritable mid-run. Errors are written once
//     to stderr and further writes are silently dropped.
//   - Lock-free read path. notifyTape() loads an atomic.Pointer; when
//     no tape is open, the cost is one nil check. When a tape IS open,
//     a per-write mutex serializes the encoder.
//   - The Hash event payload is converted to a Go map via objectToJSON
//     and emitted as the "data" field of the line. Causal ID fields
//     (id, caused_by) are placeholder for now; the runtime will start
//     populating them in a follow-up. v1 tapes always have caused_by:null.
//
// Format spec: docs/AGENTIC_HOOKS_ROADMAP.md

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const tapeFormatVersion = 1

// TapeWriter holds a single open .lextape file plus its serialization
// machinery. One per process; multi-write would be a misconfiguration.
//
// Event IDs come from the RUNTIME's atomic.Uint64 (nextEventID in
// builtins_agent.go) so the tape's id/caused_by chain matches what
// kLex callbacks see — every event has one canonical identity.
type TapeWriter struct {
	mu          sync.Mutex
	f           *os.File
	enc         *json.Encoder
	startMs     int64
	countsKind  map[string]int
	totalEvents int
	disabled    bool // set true after first write error to suppress spam
}

// tapeWriter holds the active writer (or nil). Loaded lock-free on the
// hot path; replaced atomically by OpenTape / CloseTape.
var tapeWriter atomic.Pointer[TapeWriter]

// TapeActive returns true if a tape is currently being recorded.
// FireXxxHook callsites can use this to short-circuit the hash-build
// when neither a kLex hook nor the tape is consuming events.
func TapeActive() bool { return tapeWriter.Load() != nil }

// OpenTape opens PATH for writing and starts a new tape. Returns an
// error if the file can't be created; in that case no tape is opened
// and notifyTape stays a no-op. programPath / args populate the header.
func OpenTape(path, programPath string, args []string) error {
	if tapeWriter.Load() != nil {
		return fmt.Errorf("tape already open")
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("open tape %s: %w", path, err)
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)

	tw := &TapeWriter{
		f:          f,
		enc:        enc,
		startMs:    time.Now().UnixMilli(),
		countsKind: map[string]int{},
	}

	// Compute SHA-256 of the program source. Best-effort; missing/
	// unreadable program means an empty hash field, not an error.
	programSha := ""
	if programPath != "" {
		if data, rerr := os.ReadFile(programPath); rerr == nil {
			sum := sha256.Sum256(data)
			programSha = fmt.Sprintf("%x", sum)
		}
	}

	header := map[string]interface{}{
		"type":                 "header",
		"tape_version":         tapeFormatVersion,
		"klex_version":         KLexVersion,
		"program":              programPath,
		"program_sha256":       programSha,
		"started_at":           time.Now().UTC().Format(time.RFC3339Nano),
		"args":                 args,
		"gesture_aggregation":  false, // v1 — raw events only
	}
	if err := enc.Encode(header); err != nil {
		f.Close()
		return fmt.Errorf("write tape header: %w", err)
	}

	tapeWriter.Store(tw)
	return nil
}

// CloseTape flushes the footer and closes the file. Safe to call when
// no tape is open (no-op). Intended for the main-program exit path.
func CloseTape() {
	tw := tapeWriter.Swap(nil)
	if tw == nil {
		return
	}
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.disabled {
		tw.f.Close()
		return
	}
	footer := map[string]interface{}{
		"type":           "footer",
		"ended_at":       time.Now().UTC().Format(time.RFC3339Nano),
		"duration_ms":    time.Now().UnixMilli() - tw.startMs,
		"event_count":    tw.totalEvents,
		"counts_by_kind": tw.countsKind,
	}
	_ = tw.enc.Encode(footer)
	tw.f.Close()
}

// notifyTape writes an event line to the active tape, if any.
// Called from Fire*Hook callsites in builtins_agent.go AFTER they've
// allocated a runtime event id and a parent (via currentParentID()),
// so the tape's id/caused_by exactly mirror what user callbacks see.
// Best-effort: a write error disables further writes and prints once
// to stderr.
//
// The `data` hash already contains "id" and "caused_by" because
// stampEventIdentity stamped them for the kLex callback; we strip them
// during serialisation so they appear only at the top level of the
// tape line (not duplicated inside data).
func notifyTape(kind string, id, parent uint64, data *Hash) {
	tw := tapeWriter.Load()
	if tw == nil {
		return
	}
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.disabled {
		return
	}
	var causedBy interface{} = nil
	if parent != 0 {
		causedBy = parent
	}
	event := map[string]interface{}{
		"type":      "event",
		"id":        id,
		"caused_by": causedBy,
		"t_ms":      time.Now().UnixMilli() - tw.startMs,
		"kind":      kind,
		"data":      hashToJSONExcept(data, "id", "caused_by"),
	}
	if err := tw.enc.Encode(event); err != nil {
		fmt.Fprintf(os.Stderr, "kLex tape: write failed (%v) — disabling further tape output\n", err)
		tw.disabled = true
		return
	}
	tw.countsKind[kind]++
	tw.totalEvents++
}

// hashToJSON converts a kLex *Hash to a Go map[string]interface{}
// suitable for json.Encoder. Recursive over nested arrays/hashes.
func hashToJSON(h *Hash) map[string]interface{} {
	out := make(map[string]interface{}, len(h.Pairs))
	for _, pair := range h.Pairs {
		key, ok := pair.Key.(*String)
		if !ok {
			continue
		}
		out[key.Value] = objectToJSON(pair.Value)
	}
	return out
}

// hashToJSONExcept is hashToJSON with a list of keys to skip. Used by
// notifyTape to avoid duplicating id/caused_by inside the data field
// (those live at the top level of the tape line).
func hashToJSONExcept(h *Hash, skip ...string) map[string]interface{} {
	skipSet := make(map[string]bool, len(skip))
	for _, k := range skip {
		skipSet[k] = true
	}
	out := make(map[string]interface{}, len(h.Pairs))
	for _, pair := range h.Pairs {
		key, ok := pair.Key.(*String)
		if !ok {
			continue
		}
		if skipSet[key.Value] {
			continue
		}
		out[key.Value] = objectToJSON(pair.Value)
	}
	return out
}

// objectToJSON converts a kLex Object to its JSON equivalent.
// Handles all primitives plus arrays and nested hashes; falls back
// to Inspect() for anything exotic so the tape never silently drops
// a value (the consumer can still parse it as a string).
func objectToJSON(o Object) interface{} {
	switch v := o.(type) {
	case *String:
		return v.Value
	case *Integer:
		return v.Value
	case *Float:
		return v.Value
	case *Boolean:
		return v.Value
	case *Null:
		return nil
	case nil:
		return nil
	case *Array:
		out := make([]interface{}, len(v.Elements))
		for i, e := range v.Elements {
			out[i] = objectToJSON(e)
		}
		return out
	case *Hash:
		return hashToJSON(v)
	case *Error:
		return map[string]interface{}{
			"kind":    string(v.Kind),
			"message": v.Message,
			"code":    v.Code,
		}
	default:
		return v.Inspect()
	}
}
