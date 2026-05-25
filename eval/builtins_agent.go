package eval

// agent_hook.go — kLex's agentic runtime hooks.
//
// The vision: the runtime exposes structured semantic events that an
// external agent (an LLM, a debugger, a telemetry sink — anything that
// can be a kLex callable) can subscribe to. Each hook is a *slot* that
// can hold at most one registered callable. When the runtime hits a
// semantic checkpoint (error bubbles, async task spawns, etc.) it
// invokes the registered hook with an event hash describing the moment.
//
// Phase 1 — `on_error_bubble`:        every internal error
// Phase 2 — `on_async_spawn` / `on_async_done`: task lifecycle
//
// Design rules across all hooks:
//
//   - Zero hot-path cost when no hook is registered (single atomic
//     load returns nil, fire returns immediately).
//   - Re-entry guarded per slot: a hook that itself triggers the same
//     event class doesn't recurse infinitely. The re-entry guard is
//     PER SLOT — an async-spawn hook can fire from inside an
//     error-bubble hook without being blocked.
//   - Synchronous, in the originating goroutine. Easier ordering;
//     errors are rare; async spawn/done aren't hot enough to matter.
//   - Observer-only. The hook can read, log, ask an LLM, write a
//     file — but cannot change the original event or alter
//     propagation. Mutation hooks are a future category.
//
// All slots share one `hookDispatcher` function pointer that wires
// `callCallable` at init time. Without the function-pointer trick,
// Go's init analyzer pulls every hook fire site into a cycle with
// the Builtins map (typeError → FireXxx → callCallable → Eval → Get
// → Builtins). The dispatcher is nil during var-init and gets set in
// init(), by which point all `var` initializers have already run.

import (
	"klex/ast"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// hookSlot is a single observation point. Holds one registered
// callable plus a re-entry guard. Reads are lock-free.
type hookSlot struct {
	mu     sync.RWMutex
	fn     Object // the registered kLex callable, or nil
	active atomic.Bool
}

func (h *hookSlot) get() Object {
	h.mu.RLock()
	fn := h.fn
	h.mu.RUnlock()
	return fn
}

func (h *hookSlot) set(fn Object) {
	h.mu.Lock()
	h.fn = fn
	h.mu.Unlock()
}

// fire invokes the registered hook (if any) with `event`. eventID is
// the event's own id — pushed onto the goroutine's causal stack for
// the duration of the dispatch so any Fire*Hook calls made by the user
// callback inherit it as their caused_by.
//
// Returns immediately when: no hook is set, a hook in THIS slot is
// already firing (re-entry guard), or the dispatcher hasn't been
// wired yet (only true during package var-init).
func (h *hookSlot) fire(event *Hash, eventID uint64) {
	fn := h.get()
	if fn == nil {
		return
	}
	if !h.active.CompareAndSwap(false, true) {
		return
	}
	defer h.active.Store(false)
	dispatch := hookDispatcher
	if dispatch == nil {
		return
	}
	pushEvent(eventID)
	defer popEvent()
	// Discard the hook's return value AND any error it raised — the
	// re-entry guard protects against recursion, and a hook author
	// who lets their callback throw is on the hook for that. We must
	// not let hook errors hijack the original event's propagation.
	_, _ = dispatch(fn, []Object{event})
}

var (
	// Phase 1 hook
	errorBubbleHook hookSlot

	// Phase 2 hooks
	asyncSpawnHook hookSlot
	asyncDoneHook  hookSlot

	// Phase 3 hooks
	uiEventHook    hookSlot // user interaction with an interactive widget
	bridgeCallHook hookSlot // _bridgeCall completed (one event per call, after)

	// asyncTaskIDCounter is monotonic across the process. Each async
	// spawn increments it. IDs are unique within a process; the
	// on_async_done event carries the matching task_id so the agent
	// can pair spawn↔done events.
	asyncTaskIDCounter atomic.Uint64

	// hookDispatcher is the actual hook-invoker. See package comment
	// for the init-cycle rationale.
	hookDispatcher func(Object, []Object) (Object, Object)

	// eventIDCounter is the monotonic source for event IDs. Allocated
	// in Fire*Hook just before stamping the event hash with `id`.
	// Unique within the process; tapes carry the per-run sequence.
	eventIDCounter atomic.Uint64

	// gidContext tracks the "currently dispatching" event ID per
	// goroutine. Key = goroutine id (via goid()), value = *[]uint64
	// stack of event IDs currently mid-dispatch on that goroutine.
	// The top of the stack is the caused_by parent for any new event
	// fired by code running on that goroutine.
	//
	// Synchronous in-goroutine dispatch (UI handlers, error bubble,
	// bridge call completion) inherits causality automatically.
	// Cross-goroutine cases (async() spawning a task) are handled
	// explicitly: the async builtin captures the spawn event id and
	// pushes it on the child goroutine's stack at goroutine start.
	gidContext sync.Map
)

// nextEventID allocates a fresh monotonic event ID. Process-unique.
func nextEventID() uint64 { return eventIDCounter.Add(1) }

// currentParentID returns the top of the calling goroutine's event
// stack — i.e. the id of the event whose dispatch we're inside. Returns
// 0 when nothing is mid-dispatch (caller is at top level / root).
func currentParentID() uint64 {
	if v, ok := gidContext.Load(goid()); ok {
		stack := v.(*[]uint64)
		if len(*stack) > 0 {
			return (*stack)[len(*stack)-1]
		}
	}
	return 0
}

// pushEvent makes `id` the current event for the calling goroutine.
// Any Fire*Hook called between pushEvent and the matching popEvent
// will see `id` as currentParentID() and stamp it as caused_by.
func pushEvent(id uint64) {
	gid := goid()
	if v, ok := gidContext.Load(gid); ok {
		stack := v.(*[]uint64)
		*stack = append(*stack, id)
		return
	}
	stack := []uint64{id}
	gidContext.Store(gid, &stack)
}

// popEvent removes the top of the calling goroutine's event stack.
// Always pair with a preceding pushEvent via defer.
func popEvent() {
	gid := goid()
	if v, ok := gidContext.Load(gid); ok {
		stack := v.(*[]uint64)
		if len(*stack) > 0 {
			*stack = (*stack)[:len(*stack)-1]
		}
		if len(*stack) == 0 {
			gidContext.Delete(gid)
		}
	}
}

// goid extracts the current goroutine's ID from the runtime. Go does
// not expose this directly — we parse the first line of runtime.Stack
// which always starts with "goroutine N [state]:". A known idiom; cost
// is ~1µs per call, acceptable for hook fire sites (called per event).
//
// We use this only for the causal-ID stack; nothing else depends on
// goroutine identity, so if the runtime format ever changes we get a
// degraded chain (parent_id stays 0) rather than a crash.
func goid() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	line := string(buf[:n])
	line = strings.TrimPrefix(line, "goroutine ")
	sp := strings.IndexByte(line, ' ')
	if sp <= 0 {
		return 0
	}
	id, _ := strconv.ParseUint(line[:sp], 10, 64)
	return id
}

// stampEventIdentity adds the id + caused_by fields to an event hash.
// Call this once per Fire*Hook just after building the kind-specific
// payload, before calling notifyTape / xxxHook.fire(). parent==0 is
// rendered as caused_by:null.
func stampEventIdentity(h *Hash, id, parent uint64) {
	putInt(h, "id", int(id))
	if parent == 0 {
		h.Pairs[HashKey{Type: STRING_OBJ, Value: "caused_by"}] = HashPair{
			Key:   &String{Value: "caused_by"},
			Value: NULL,
		}
	} else {
		putInt(h, "caused_by", int(parent))
	}
}

// ── Phase 1 — on_error_bubble ─────────────────────────────────────────

// SetErrorBubbleHook installs `fn` (or nil to clear) as the
// on_error_bubble handler. Exported so the vm package can also
// install hooks for its own tests if needed.
func SetErrorBubbleHook(fn Object) { errorBubbleHook.set(fn) }

// FireErrorBubbleHook is called by the runtime's error-construction
// sites (typeError, runtimeError, typeMismatchError in typecheck.go,
// and the VM's bubbleError label in vm.go). Fires the hook at most
// ONCE per logical *Error via the err.hookFired latch — same error
// passing through both the eval helper AND the VM bubble site only
// triggers the hook on first contact.
//
// callStack is innermost-first source lines; pass nil if not
// available (tree-walker path currently does).
func FireErrorBubbleHook(err *Error, callStack []int) {
	if err == nil || err.hookFired {
		return
	}
	err.hookFired = true
	if errorBubbleHook.get() == nil && !TapeActive() {
		return
	}
	h := buildErrorBubbleEvent(err, callStack)
	eventID := nextEventID()
	parentID := currentParentID()
	stampEventIdentity(h, eventID, parentID)
	notifyTape("error", eventID, parentID, h)
	errorBubbleHook.fire(h, eventID)
}

// ── Phase 2 — on_async_spawn / on_async_done ──────────────────────────

// NextAsyncTaskID returns a fresh monotonic task ID. Exported so
// other event-emitting sites (future hook categories) can match the
// same ID stream if they want to correlate.
func NextAsyncTaskID() uint64 { return asyncTaskIDCounter.Add(1) }

// SetAsyncSpawnHook installs (or clears) the on_async_spawn handler.
func SetAsyncSpawnHook(fn Object) { asyncSpawnHook.set(fn) }

// SetAsyncDoneHook installs (or clears) the on_async_done handler.
func SetAsyncDoneHook(fn Object) { asyncDoneHook.set(fn) }

// FireAsyncSpawnHook is called by the async builtin immediately
// before launching the task's goroutine. spawnedAtNanos is the
// monotonic wall-clock timestamp at the spawn point (callers pass
// time.Now().UnixNano()) so events can be correlated chronologically.
// FireAsyncSpawnHook fires the spawn event and returns its event ID
// so the async builtin can pre-seed the child goroutine's causal stack
// with this id (so events fired by the task body inherit spawn as their
// caused_by). Returns 0 when no consumer is listening — callers should
// only push if the id is non-zero.
func FireAsyncSpawnHook(taskID uint64, fnName string, argc int, spawnedAtNanos int64) uint64 {
	if asyncSpawnHook.get() == nil && !TapeActive() {
		return 0
	}
	h := &Hash{Pairs: make(map[HashKey]HashPair)}
	putInt(h, "task_id", int(taskID))
	putString(h, "fn", fnName)
	putInt(h, "argc", argc)
	putInt(h, "spawned_at", int(spawnedAtNanos))
	eventID := nextEventID()
	parentID := currentParentID()
	stampEventIdentity(h, eventID, parentID)
	notifyTape("async_spawn", eventID, parentID, h)
	asyncSpawnHook.fire(h, eventID)
	return eventID
}

// FireAsyncDoneHook is called by the async builtin's goroutine
// wrapper after the task body returns. result is whatever the task
// produced (an *Error means the task threw; anything else is a
// success value). durationNanos is the goroutine's run time.
func FireAsyncDoneHook(taskID uint64, result Object, durationNanos int64) {
	if asyncDoneHook.get() == nil && !TapeActive() {
		return
	}
	h := &Hash{Pairs: make(map[HashKey]HashPair)}
	putInt(h, "task_id", int(taskID))
	putInt(h, "duration_ms", int(durationNanos/1_000_000))

	ok := true
	var errInfo Object = NULL
	if e, isErr := result.(*Error); isErr && !e.IsUserError {
		ok = false
		errH := &Hash{Pairs: make(map[HashKey]HashPair)}
		putString(errH, "kind", string(e.Kind))
		putString(errH, "message", e.Message)
		errInfo = errH
	}
	putBool(h, "ok", ok)
	h.Pairs[HashKey{Type: STRING_OBJ, Value: "error"}] = HashPair{
		Key:   &String{Value: "error"},
		Value: errInfo,
	}
	eventID := nextEventID()
	parentID := currentParentID()
	stampEventIdentity(h, eventID, parentID)
	notifyTape("async_done", eventID, parentID, h)
	asyncDoneHook.fire(h, eventID)
}

// ── Phase 3 — on_ui_event ────────────────────────────────────────────

// SetUiEventHook installs (or clears) the on_ui_event handler.
func SetUiEventHook(fn Object) { uiEventHook.set(fn) }

// UiEventHookActive returns true when ANY consumer wants UI events —
// either a kLex callback is registered, or a tape is recording.
// Producer-side widgets can guard hash construction with this to keep
// the no-consumer path allocation-free (the `_uiEventActive` builtin
// exposes this to kLex).
func UiEventHookActive() bool { return uiEventHook.get() != nil || TapeActive() }

// FireUiEventHook builds the event hash and invokes the hook. Producer
// sites (UI widget functions in stdlib/ui.lex) pass the new value as
// `value`; pass NULL for plain clicks where there's no new state. x/y
// are mouse coordinates at the moment of the event.
func FireUiEventHook(kind, widget, label string, value Object, x, y int) {
	if uiEventHook.get() == nil && !TapeActive() {
		return
	}
	h := &Hash{Pairs: make(map[HashKey]HashPair)}
	putString(h, "kind", kind)
	putString(h, "widget", widget)
	putString(h, "label", label)
	if value == nil {
		h.Pairs[HashKey{Type: STRING_OBJ, Value: "value"}] = HashPair{
			Key:   &String{Value: "value"},
			Value: NULL,
		}
	} else {
		h.Pairs[HashKey{Type: STRING_OBJ, Value: "value"}] = HashPair{
			Key:   &String{Value: "value"},
			Value: value,
		}
	}
	putInt(h, "x", x)
	putInt(h, "y", y)
	eventID := nextEventID()
	parentID := currentParentID()
	stampEventIdentity(h, eventID, parentID)
	notifyTape("ui_event", eventID, parentID, h)
	uiEventHook.fire(h, eventID)
}

// ── Phase 3 — on_bridge_call ─────────────────────────────────────────

// SetBridgeCallHook installs (or clears) the on_bridge_call handler.
func SetBridgeCallHook(fn Object) { bridgeCallHook.set(fn) }

// FireBridgeCallHook is called by the _bridgeCall builtin AFTER each
// bridge call returns (synchronous round-trip from kLex's view), with
// timing + success info. `result` may be an *Error to signal the call
// failed (e.g. bridge crashed, JSON unmarshal failed); user-error
// values from the remote process come back as a (null, error) tuple
// and don't mark the call itself as failed.
//
// Single-event design (no spawn/done split) — bridge calls block the
// caller, so the agent's hook fires once per call at the natural
// semantic boundary. Pairs nicely with on_async_done's shape.
func FireBridgeCallHook(fnName string, argc int, durationNanos int64, result Object) {
	if bridgeCallHook.get() == nil && !TapeActive() {
		return
	}
	h := &Hash{Pairs: make(map[HashKey]HashPair)}
	putString(h, "fn", fnName)
	putInt(h, "argc", argc)
	putInt(h, "duration_ms", int(durationNanos/1_000_000))

	ok := true
	var errInfo Object = NULL
	if e, isErr := result.(*Error); isErr && !e.IsUserError {
		ok = false
		errH := &Hash{Pairs: make(map[HashKey]HashPair)}
		putString(errH, "kind", string(e.Kind))
		putString(errH, "message", e.Message)
		putString(errH, "code", e.Code)
		errInfo = errH
	}
	putBool(h, "ok", ok)
	h.Pairs[HashKey{Type: STRING_OBJ, Value: "error"}] = HashPair{
		Key:   &String{Value: "error"},
		Value: errInfo,
	}
	eventID := nextEventID()
	parentID := currentParentID()
	stampEventIdentity(h, eventID, parentID)
	notifyTape("bridge_call", eventID, parentID, h)
	bridgeCallHook.fire(h, eventID)
}

// AsyncCalleeName extracts a printable name for the function passed
// to async(). Used to build the spawn event. Falls back to coarse
// labels for shapes the eval package can't introspect directly (VM
// CompiledFunctions live in another package).
func AsyncCalleeName(callee Object) string {
	switch fn := callee.(type) {
	case *Function:
		if fn.Name != "" {
			return fn.Name
		}
		return "<anon>"
	case *Builtin:
		return "<builtin>"
	}
	if callee != nil && callee.Type() == COMPILED_FUNCTION_OBJ {
		return "<closure>"
	}
	return "<external>"
}

// ── Event-builder helpers (shared across hooks) ───────────────────────

func buildErrorBubbleEvent(err *Error, callStack []int) *Hash {
	h := &Hash{Pairs: make(map[HashKey]HashPair)}
	putString(h, "kind", string(err.Kind))
	putString(h, "message", err.Message)
	putString(h, "code", err.Code)
	putInt(h, "line", err.Pos.Line)

	stackArr := &Array{Elements: make([]Object, len(callStack))}
	for i, line := range callStack {
		frame := &Hash{Pairs: make(map[HashKey]HashPair)}
		putInt(frame, "line", line)
		stackArr.Elements[i] = frame
	}
	h.Pairs[HashKey{Type: STRING_OBJ, Value: "stack"}] = HashPair{
		Key:   &String{Value: "stack"},
		Value: stackArr,
	}
	return h
}

func putString(h *Hash, key, val string) {
	h.Pairs[HashKey{Type: STRING_OBJ, Value: key}] = HashPair{
		Key:   &String{Value: key},
		Value: &String{Value: val},
	}
}

func putInt(h *Hash, key string, val int) {
	h.Pairs[HashKey{Type: STRING_OBJ, Value: key}] = HashPair{
		Key:   &String{Value: key},
		Value: &Integer{Value: val},
	}
}

func putBool(h *Hash, key string, val bool) {
	var obj Object = FALSE
	if val {
		obj = TRUE
	}
	h.Pairs[HashKey{Type: STRING_OBJ, Value: key}] = HashPair{
		Key:   &String{Value: key},
		Value: obj,
	}
}

// ── Builtin registration ──────────────────────────────────────────────

func init() {
	// Wire the dispatcher AFTER all `var` initializers have finished
	// (init() runs post-var-init by Go's package init protocol).
	hookDispatcher = callCallable

	Builtins["_setErrorHook"] = &Builtin{Fn: func(args []Object) Object {
		return setHookBuiltin(args, "_setErrorHook", SetErrorBubbleHook)
	}}
	Builtins["_setAsyncSpawnHook"] = &Builtin{Fn: func(args []Object) Object {
		return setHookBuiltin(args, "_setAsyncSpawnHook", SetAsyncSpawnHook)
	}}
	Builtins["_setAsyncDoneHook"] = &Builtin{Fn: func(args []Object) Object {
		return setHookBuiltin(args, "_setAsyncDoneHook", SetAsyncDoneHook)
	}}
	Builtins["_setUiEventHook"] = &Builtin{Fn: func(args []Object) Object {
		return setHookBuiltin(args, "_setUiEventHook", SetUiEventHook)
	}}
	Builtins["_setBridgeCallHook"] = &Builtin{Fn: func(args []Object) Object {
		return setHookBuiltin(args, "_setBridgeCallHook", SetBridgeCallHook)
	}}

	// _uiEventActive() → BOOLEAN — used by stdlib/ui.lex widget
	// instrumentation to skip event-hash construction when no consumer
	// is registered. Lock-free atomic pointer load on the no-hook path.
	Builtins["_uiEventActive"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_uiEventActive expects 0 arguments", ast.Pos{})
		}
		if UiEventHookActive() {
			return TRUE
		}
		return FALSE
	}}

	// _uiEvent(kind: string, widget: string, label: string,
	//          value: any, x: int, y: int) → null
	// Producer-side primitive. Called by instrumented widgets in
	// stdlib/ui.lex when the user causes a state change. Builds the
	// event hash and dispatches it to the registered on_ui_event hook
	// (if any). When no hook is registered, returns immediately
	// without building the hash. Match the stdlib/agent.lex doc for
	// the kind / widget vocabulary.
	Builtins["_uiEvent"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 6 {
			return runtimeError("_uiEvent expects 6 arguments (kind, widget, label, value, x, y)", ast.Pos{})
		}
		kindS, ok1 := args[0].(*String)
		widgetS, ok2 := args[1].(*String)
		labelS, ok3 := args[2].(*String)
		xI, ok4 := args[4].(*Integer)
		yI, ok5 := args[5].(*Integer)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
			return typeError("_uiEvent: arg types must be (string, string, string, any, int, int)", ast.Pos{})
		}
		FireUiEventHook(kindS.Value, widgetS.Value, labelS.Value, args[3], xI.Value, yI.Value)
		return NULL
	}}
}

// setHookBuiltin is the shared body for all _setXxxHook builtins.
// Validates one-arg callable-or-null and installs via setter.
func setHookBuiltin(args []Object, name string, setter func(Object)) Object {
	if len(args) != 1 {
		return runtimeError(name+" expects 1 argument (callable or null)", ast.Pos{})
	}
	if _, isNull := args[0].(*Null); isNull {
		setter(nil)
		return NULL
	}
	if !IsCallable(args[0]) {
		return typeError(name+": argument must be a callable or null, got "+string(args[0].Type()), ast.Pos{})
	}
	setter(args[0])
	return NULL
}
