// stdlib/agent.lex — kLex agentic runtime hooks.
// @module    agent
// @version   0.1.0
// @since     klex 0.3.36
// @author    karl
// @summary   First-class hooks for agent / telemetry / debugger subscribers
//
// The vision: kLex is the language where the runtime exposes structured
// semantic events that an external observer — an LLM, a debugger, a
// telemetry sink — can subscribe to. Not "AI in the runtime", not
// "magic auto-debugging" — a clean event stream with explicit
// interception points the language designer chose.
//
// Phase 1 ships one hook: `onErrorBubble`. More to follow as concrete
// use cases emerge (function-entry/exit, channel send/recv, async task
// lifecycle, effect boundaries). Each new hook will be a separate
// builtin + wrapper here — we don't design the catalogue up front.
//
// Performance: when no hook is registered, the runtime pays a single
// atomic pointer load + nil check per fire site. Effectively free.
// Hooks fire SYNCHRONOUSLY in the failing goroutine — fine for errors
// (rare), would need re-design for hot-path events (channel ops).

// onErrorBubble registers `fn` as the error-bubble hook. `fn` is
// invoked exactly once per internal error, at the moment the error
// begins propagating up the call stack — BEFORE any safe() / `?` /
// top-level handler sees it. The hook is observer-only: returning a
// value from it (or producing an error inside it) does not alter the
// original error or its propagation.
//
// Event hash shape:
//
//   {
//       "kind":    "TypeError" or "RuntimeError",
//       "message": "<the error message>",
//       "code":    "<user-supplied code if any, else ''>",
//       "line":    <1-based line where the error originated, 0 if unknown>,
//       "stack":   [{"line": N}, {"line": M}, ...]   // innermost frame first, capped at 64
//   }
//
// Re-entry: if the hook itself errors, the runtime detects the
// re-entry and skips the recursive call. Don't worry about producing
// a loop — kLex won't let you.
//
// Pass `null` to clear a previously-registered hook.
//
// Example — log every error to a file:
//
//   import "agent.lex" as agent
//   import "fs.lex"    as fs
//   agent.onErrorBubble(fn(evt) {
//       fs.appendFile("errors.log",
//           "[" + evt["kind"] + "] " + evt["message"] +
//           " (line " + str(evt["line"]) + ")\n")
//   })
//
// Example — ask Claude to explain (assumes claude client is wired up):
//
//   agent.onErrorBubble(fn(evt) {
//       let q = "Why might a kLex program produce this error: " +
//               evt["message"] + " at line " + str(evt["line"]) + "?"
//       let suggestion, _ = claude.ask(q)
//       println("💡 " + suggestion)
//   })
fn onErrorBubble(hookFn) {
    _setErrorHook(hookFn)
}

// clearErrorBubble removes any registered error-bubble hook. Same as
// calling onErrorBubble(null) — provided as a named alias because
// `agent.clearErrorBubble()` reads better than `agent.onErrorBubble(null)`
// at call sites that are deliberately tearing down.
fn clearErrorBubble() {
    _setErrorHook(null)
}

// onAsyncSpawn registers `hookFn` to fire whenever async(...) launches
// a new task. The hook is invoked SYNCHRONOUSLY in the spawning
// goroutine (the same goroutine that called async) BEFORE the task's
// goroutine starts running. The matching on_async_done event fires
// later, when the task body returns, carrying the same task_id.
//
// Event hash shape:
//
//   {
//       "task_id":    42,           // monotonic uint64, unique per process
//       "fn":         "enhance",    // closure's name, "<anon>" for unnamed,
//                                   // "<builtin>" for builtin callees,
//                                   // "<closure>" for VM closures
//       "argc":       1,            // arg count (not the args themselves —
//                                   // args may mutate during the task)
//       "spawned_at": 1700000000000000000   // unix nanoseconds at spawn
//   }
//
// Pass null to clear.
//
// Example — log every task launch:
//
//   import "agent.lex" as agent
//   agent.onAsyncSpawn(fn(evt) {
//       println("🚀 task #" + str(evt["task_id"]) + " (" + evt["fn"] + ")")
//   })
fn onAsyncSpawn(hookFn) {
    _setAsyncSpawnHook(hookFn)
}

// clearAsyncSpawn removes any registered on_async_spawn hook.
fn clearAsyncSpawn() {
    _setAsyncSpawnHook(null)
}

// onAsyncDone registers `hookFn` to fire whenever an async task
// finishes. The hook is invoked from the task's OWN goroutine
// immediately after the body returns and `task.done` is signalled.
// Use task_id to pair with the earlier on_async_spawn event.
//
// Event hash shape:
//
//   {
//       "task_id":     42,
//       "duration_ms": 1342,        // wall time the goroutine ran
//       "ok":          true,        // false when the task threw an
//                                   // internal error (TypeError /
//                                   // RuntimeError); user errors from
//                                   // error() are still ok=true
//       "error":       null         // or {"kind": ..., "message": ...}
//                                   // when ok=false
//   }
//
// Pass null to clear.
//
// Example — print spawn/done pairs:
//
//   agent.onAsyncSpawn(fn(evt) {
//       println("🚀 #" + str(evt["task_id"]) + " " + evt["fn"])
//   })
//   agent.onAsyncDone(fn(evt) {
//       let status = "✓"
//       if !evt["ok"] { status = "✗" }
//       println(status + " #" + str(evt["task_id"]) + " (" +
//               str(evt["duration_ms"]) + "ms)")
//   })
fn onAsyncDone(hookFn) {
    _setAsyncDoneHook(hookFn)
}

// clearAsyncDone removes any registered on_async_done hook.
fn clearAsyncDone() {
    _setAsyncDoneHook(null)
}

// onUiEvent registers `hookFn` to fire when the user causes a state
// change on an interactive widget — clicking a button, dragging a
// slider, toggling a checkbox, picking a dropdown item, typing in a
// text field, etc. The hook is invoked SYNCHRONOUSLY from the widget
// function that detected the interaction, in the same frame, so the
// event's wall-clock ordering matches what the user sees.
//
// Fires only when the user causes a change. Pure-display widgets
// (panels, labels, etc.) do NOT fire — they're not interactive.
// Hover events are not emitted either.
//
// Event hash shape:
//
//   {
//       "kind":   "click" | "drag" | "toggle" | "select" | "text" | ...,
//       "widget": "button" | "slider" | "checkbox" | "dropdown" |
//                 "textInput" | "toggle" | "splitter" | "colorPicker",
//       "label":  "<the widget's label/identifier, or '' if none>",
//       "value":  <the new value the interaction produced, or null
//                  for plain clicks where there is no new state>,
//       "x":      <int mouse x at the event>,
//       "y":      <int mouse y at the event>
//   }
//
// Pass null to clear.
//
// Example — log every UI interaction:
//
//   import "agent.lex" as agent
//   agent.onUiEvent(fn(evt) {
//       println("🖱  " + evt["widget"] + ":" + evt["label"] +
//               " " + evt["kind"] + " value=" + str(evt["value"]))
//   })
fn onUiEvent(hookFn) {
    _setUiEventHook(hookFn)
}

// clearUiEvent removes any registered on_ui_event hook.
fn clearUiEvent() {
    _setUiEventHook(null)
}

// onBridgeCall registers `hookFn` to fire AFTER every _bridgeCall to
// an external process (Python / Node / MCP server / etc.). Fires
// exactly once per call, when the round-trip completes — bridge calls
// are synchronous from kLex's view, so this is the natural semantic
// boundary. Same shape as on_async_done minus task_id.
//
// Event hash shape:
//
//   {
//       "fn":          "enhance_prompt",       // remote function name
//       "argc":        2,                      // arg count
//       "duration_ms": 1342,                   // wall-clock round-trip
//       "ok":          true,                   // false if the call itself failed
//                                              // (bridge crash, JSON unmarshal, etc.)
//       "error":       null                    // or {"kind": ..., "message": ..., "code": ...}
//                                              // when ok=false
//   }
//
// Note: a remote function returning a (null, errorValue) tuple is NOT
// a bridge-call failure — it's a successful round-trip whose result
// happens to be a user error. The hook fires with ok=true; inspect
// the tuple at the call site if you care.
//
// Pass null to clear.
//
// Example — log every external call:
//
//   agent.onBridgeCall(fn(evt) {
//       let icon = "✓"
//       if !evt["ok"] { icon = "✗" }
//       println("🌉 " + icon + " " + evt["fn"] + "(" + str(evt["argc"]) +
//               " args) " + str(evt["duration_ms"]) + "ms")
//   })
fn onBridgeCall(hookFn) {
    _setBridgeCallHook(hookFn)
}

// clearBridgeCall removes any registered on_bridge_call hook.
fn clearBridgeCall() {
    _setBridgeCallHook(null)
}
