# FROG Agentic Hooks

> **A structured, causal event stream baked into the kLex runtime — designed for AI agents, debuggers, telemetry sinks, and any external observer that needs to understand what a FROG program is actually doing.**

---

## What It Is

kLex exposes five semantic interception points — called *hooks* — directly in the runtime. When a hook fires, the runtime packages the relevant moment into a typed event hash and delivers it to any registered observer: a kLex callback, a `.lextape` recording file, or both simultaneously.

This is not an add-on library. The hooks are instrumented in `eval/builtins_agent.go` at the Go level and fire unconditionally at the exact semantic moment they describe. An LLM, a debugger, or a telemetry pipeline receives the same structured view of runtime behaviour that the kLex evaluator itself has.

---

## The Five Hook Surfaces

| Hook | When it fires | Key event fields |
|---|---|---|
| `error` | The moment any internal error begins propagating — before `safe()` or any caller sees it | `kind`, `message`, `code`, `line`, `stack[]` |
| `async_spawn` | Synchronously in the spawning goroutine, before the task goroutine starts | `task_id`, `fn`, `argc`, `spawned_at` |
| `async_done` | In the task's own goroutine, immediately after its body returns | `task_id`, `duration_ms`, `ok`, `error` |
| `ui_event` | Synchronously from the widget function when the user causes a state change | `kind`, `widget`, `label`, `value`, `x`, `y` |
| `bridge_call` | After every external bridge call completes (Python, Node, MCP, etc.) | `fn`, `argc`, `duration_ms`, `ok`, `error` |

Every event also carries a monotonic process-unique `id` and a `caused_by` field that links it to the parent event whose dispatch triggered it — building a causal graph automatically from the goroutine context, with no manual ID stamping required.

---

## Two Ways to Consume Events

### 1 — Passive recording with `--record-tape`

```bash
./klex --record-tape=/tmp/run.lextape your_program.lex
```

Records every hook event to a `.lextape` file as it happens. The program runs exactly as normal; the tape is a side-channel. No code changes required.

### 2 — In-process callbacks via `stdlib/agent.lex`

```lex
import "stdlib/agent.lex" as agent

agent.onErrorBubble(fn(evt) {
    println("[" + evt["kind"] + "] " + evt["message"] + " at line " + str(evt["line"]))
})

agent.onAsyncSpawn(fn(evt) {
    println("spawned task #" + str(evt["task_id"]) + " (" + evt["fn"] + ")")
})

agent.onAsyncDone(fn(evt) {
    let status = "ok"
    if !evt["ok"] { status = "FAILED" }
    println("task #" + str(evt["task_id"]) + " " + status +
            " in " + str(evt["duration_ms"]) + "ms")
})
```

Both modes can be active simultaneously. A live callback and a tape recording are independent consumers.

---

## The `.lextape` Format

A `.lextape` file is **JSON Lines** — one JSON object per line, readable with any JSON tooling. Every file has three structural sections:

### Header (first line)

```json
{
  "type":                "header",
  "tape_version":        1,
  "klex_version":        "v0.3.36",
  "program":             "/path/to/program.lex",
  "program_sha256":      "e3b0c44298fc1c149...",
  "started_at":          "2026-05-24T19:00:00.000000000Z",
  "args":                [],
  "gesture_aggregation": false
}
```

`program_sha256` is the SHA-256 of the program source at the time of the run. Combined with a lockfile, a tape + lockfile pair is sufficient to identify a bit-exact reproduction of the run.

### Event lines (one per event)

```json
{
  "type":      "event",
  "id":        7,
  "caused_by": 3,
  "t_ms":      142,
  "kind":      "async_done",
  "data": {
    "task_id":     2,
    "duration_ms": 138,
    "ok":          true,
    "error":       null
  }
}
```

`t_ms` is wall-clock milliseconds since the tape started. `caused_by` is the `id` of the event whose dispatch triggered this one, or `null` at the root level.

### Footer (last line, written on clean shutdown)

```json
{
  "type":           "footer",
  "ended_at":       "2026-05-24T19:00:05.000000000Z",
  "duration_ms":    5021,
  "event_count":    143,
  "counts_by_kind": {
    "async_spawn": 40,
    "async_done":  40,
    "error":        3,
    "ui_event":    60
  }
}
```

---

## Event Payload Reference

### `error`

Fires exactly once per logical error. Fires before `safe()`, before the `?` operator, before any caller-level handler.

```
kind        string   "TypeError" or "RuntimeError"
message     string   the human-readable error message
code        string   user-supplied error code, or "" if none
line        int      1-based source line where the error originated; 0 if unknown
stack       array    [{line: N}, {line: M}, ...]  innermost frame first, capped at 64
```

### `async_spawn`

Fires in the spawning goroutine before the child goroutine starts. The child goroutine's events inherit this event's `id` as their `caused_by`.

```
task_id     uint64   monotonic, process-unique task identifier
fn          string   callee name, "<anon>", "<builtin>", or "<closure>"
argc        int      argument count at spawn time
spawned_at  int64    unix nanoseconds at the spawn point
```

### `async_done`

Fires in the task's own goroutine after the body returns. `task_id` matches the earlier `async_spawn` event.

```
task_id     uint64   matches the spawning event
duration_ms int      wall-clock goroutine run time in milliseconds
ok          bool     false only for internal errors (TypeError/RuntimeError);
                     user errors returned via error() leave ok=true
error       hash     {kind, message} when ok=false; null when ok=true
```

### `ui_event`

Fires synchronously from the widget function in the same frame as the user interaction. Pure-display widgets (panels, labels) and hover events do not fire.

```
kind        string   "click" | "drag" | "toggle" | "select" | "text" | ...
widget      string   "button" | "slider" | "checkbox" | "dropdown" |
                     "textInput" | "toggle" | "splitter" | "colorPicker"
label       string   the widget's label or identifier; "" if none
value       any      the new value produced by the interaction, or null for plain clicks
x           int      mouse x at event time
y           int      mouse y at event time
```

### `bridge_call`

Fires once per external bridge call, after the round-trip completes. A remote function returning a user error tuple is a successful call (`ok=true`); only bridge-level failures (crash, JSON parse failure) set `ok=false`.

```
fn          string   the remote function name
argc        int      argument count
duration_ms int      wall-clock round-trip time in milliseconds
ok          bool     false if the bridge call itself failed
error       hash     {kind, message, code} when ok=false; null when ok=true
```

---

## The `stdlib/agent.lex` API

```lex
import "stdlib/agent.lex" as agent

// Register / clear each hook
agent.onErrorBubble(hookFn)     agent.clearErrorBubble()
agent.onAsyncSpawn(hookFn)      agent.clearAsyncSpawn()
agent.onAsyncDone(hookFn)       agent.clearAsyncDone()
agent.onUiEvent(hookFn)         agent.clearUiEvent()
agent.onBridgeCall(hookFn)      agent.clearBridgeCall()
```

Each `hookFn` receives one argument: the event hash described above. Pass `null` or call the matching `clear*()` function to deregister.

**Re-entry is protected automatically.** If a hook callback itself triggers the same hook class, the recursive dispatch is silently dropped. A different hook class fires normally — an `onAsyncDone` callback can trigger `onErrorBubble` without any issue.

**Hooks are observer-only.** A hook's return value is discarded. An error raised inside a hook does not alter the original event or its propagation.

---

## Performance Characteristics

When no hook is registered and no tape is active, every fire site costs a single `atomic.Pointer` load and a nil check — effectively zero overhead. Hash construction at producer sites is short-circuited before any allocation occurs.

When a tape is open, the hot path is: one atomic load (non-nil), one mutex lock, one `json.Encoder.Encode` call, one mutex unlock. Write failures are handled best-effort — a single error is printed to stderr and further writes are silently dropped. A tape failure never affects program execution.

---

## What This Means for AI Agents

A running kLex program is a **fully observable runtime artifact**. An AI agent co-operating with a kLex program can:

- **React to errors in real time** — `onErrorBubble` fires before any error handler, giving the agent first sight of every failure at the exact source location.
- **Track parallel task execution** — `async_spawn` + `async_done` pairs with matching `task_id` give a complete lifecycle view of every goroutine: what it ran, how long it took, whether it succeeded.
- **Observe user intent directly** — `onUiEvent` delivers widget interactions as structured events. An agent knows what the user clicked, dragged, or typed the moment it happens, without polling or screen-scraping.
- **Monitor external calls** — `onBridgeCall` reports every round-trip to Python, Node, or any MCP-connected process: which function, how many arguments, how long it took, whether it succeeded.
- **Replay and diff executions** — `.lextape` files are stable JSON Lines with monotonic IDs. Two tapes from the same program can be `diff`-ed directly. The `program_sha256` header field ties each tape to the exact source that produced it.

The causal `caused_by` chain means an agent does not need to correlate events by timestamp or guess which task produced which error — the runtime constructs the causal graph by construction, from goroutine context, and writes it into every event line.

---

## `tapetool`

`go run ./tools/tapetool` is the companion command-line tool for working with `.lextape` files. It can filter, pretty-print, summarise, and query tape recordings.

---

## Source Locations

| File | Purpose |
|---|---|
| `eval/builtins_agent.go` | Hook slots, fire functions, causal ID stack, builtin registration |
| `eval/tape.go` | `.lextape` writer, format serialisation, header/footer |
| `stdlib/agent.lex` | Public kLex API — `onErrorBubble`, `onAsyncSpawn`, etc. |
| `tools/tapetool/` | Source for the `tapetool` companion binary |
