# kLex Native Bridge — Developer Guide

## What it is

The native bridge lets kLex call functions in an external subprocess. The subprocess stays alive for the duration of your work (persistent, not fire-and-forget), communicates over stdin/stdout using line-delimited JSON, and can be written in any language. Python is the natural first choice because of its library ecosystem.

Use the bridge when you need a capability that would be impractical to implement from scratch in kLex — cryptographic libraries, HTTP clients with complex auth, YARA rules, GitHub API wrappers, database drivers, ML inference, etc.

---

## The Protocol

Every message is a single line of JSON terminated by `\n`.

**kLex → bridge (stdin):**
```json
{"id": 1, "fn": "function_name", "args": [arg1, arg2, ...]}
```

**bridge → kLex (stdout), success:**
```json
{"id": 1, "result": <any JSON value>}
```

**bridge → kLex (stdout), failure:**
```json
{"id": 1, "error": "human-readable error message"}
```

Rules:
- `id` is an auto-incrementing integer managed by kLex. Echo it back exactly.
- One request, one response. Responses must arrive in request order (calls are serialised by kLex).
- Flush stdout after every response (`flush=True` in Python).
- **stderr is discarded** — the Go implementation sets `cmd.Stderr = nil`. Do not write debug output to stderr; it vanishes silently. Use a log file if you need debugging.
- Maximum response size: **1 MB**. Responses larger than this will crash the bridge read with `BRIDGE_ERROR`.

---

## kLex API

### `nativeBridge(cmd, args, opts?) → (bridge, err)`

Starts the subprocess and returns a bridge handle.

```frog
// Minimal — no options
bridge, err = nativeBridge("python3", ["path/to/my_bridge.py"])
if err != null {
    println("Failed to start bridge: " + err.message)
    return null
}

// With robustness options
bridge, err = nativeBridge("python3", ["my_bridge.py"], {
    "timeout_seconds": 30,           // default per-call timeout
    "max_response_mb": 16,           // allow up to 16MB responses
    "stderr_log":      "/tmp/x.log", // capture stderr to file
})
```

`cmd` is the executable. `args` is an array of strings — the script path and any fixed arguments. All elements must be strings.

`opts` is an optional hash:

| Key | Type | Default | Description |
|---|---|---|---|
| `timeout_seconds` | int / float | `0` (none) | Default per-call timeout |
| `max_response_mb` | int | `1` | Max response size, range `[1, 256]` |
| `stderr_log` | string | none | File path for stderr capture. Without this, last 4KB of stderr is kept in an in-memory ring buffer |

Bridges are registered globally — if kLex exits or is signalled (SIGINT/SIGTERM), every active bridge subprocess is force-killed via its process group. No more orphaned Python processes.

### `bridgeCall(bridge, fn, args, timeoutSec?) → (result, err)`

Calls a function in the subprocess. Blocks until the response arrives or the timeout fires.

```frog
// Uses the bridge's default timeout
result, err = bridgeCall(bridge, "my_function", [arg1, arg2])

// Per-call override (4th argument)
result, err = bridgeCall(bridge, "slow_function", [arg], 120)  // 120s
```

`args` is an array. Pass `[]` for a zero-argument function. `timeoutSec` of `0` means no timeout for this call.

### `bridgeClose(bridge) → null`

Closes stdin on the subprocess. A well-written bridge loop sees EOF and exits cleanly. If the subprocess does not exit within 2 seconds, the entire process group is force-killed. Always call this when you are done; safe to call multiple times.

```frog
bridgeClose(bridge)
```

### `bridgeStderr(bridge) → array of strings`

Returns the captured tail of the bridge's stderr output, one line per element. Essential for surfacing Python tracebacks or other diagnostics that would otherwise be invisible.

```frog
result, err = bridgeCall(bridge, "crash", [])
if err != null {
    lines = bridgeStderr(bridge)
    for line in lines { println("  " + line) }
}
```

---

## Type Mapping

Types are marshalled automatically in both directions.

| kLex type | JSON wire | Back to kLex |
|---|---|---|
| `integer` | number (int) | `integer` |
| `float` | number (float) | `float` |
| `bool` | `true` / `false` | `bool` |
| `string` | string | `string` |
| `null` | `null` | `null` |
| `array` | JSON array | `array` |
| `hash` | JSON object | `hash` (string keys only) |
| anything else | `.Inspect()` string | `string` |

**Important:** Hash keys are always strings on the round-trip. Integer or boolean keys in a kLex hash become their string representation in JSON and come back as string keys.

JSON numbers that are whole (e.g. `3.0`) come back as `integer`. Numbers with a fractional part come back as `float`.

---

## Writing a Bridge Script (Python)

Use the `klex_bridge` helper — it handles the dispatch loop, the `__schema__` handshake, argument validation, and notification framing. It ships with kLex at `stdlib/python/klex_bridge.py` and is auto-discoverable: `nativeBridge` prepends that directory to `PYTHONPATH` when launching the subprocess, so `import klex_bridge` just works.

**Decorator style** (recommended for new bridges):

```python
#!/usr/bin/env python3
from klex_bridge import handler, serve

@handler(args=[("a", "int"), ("b", "int")], returns="int")
def add(a, b):
    return a + b

@handler(args=[("name", "string")], returns="string")
def greet(name):
    return f"Hello, {name}!"

serve()
```

**Imperative style** (same registry, useful when generating handlers or migrating an existing bridge):

```python
#!/usr/bin/env python3
from klex_bridge import register, serve

def add(a, b):
    return a + b

register("add", add, args=[("a", "int"), ("b", "int")], returns="int")
serve()
```

Both forms register into the same internal table and behave identically on the wire.

**Schema mini-language** (the type strings passed to `args=` and `returns=`):

| Type | Matches |
|---|---|
| `int`, `float`, `string`, `bool`, `array`, `hash`, `null` | The named kLex type |
| `any` | Anything except null |
| `"<type>?"` (trailing `?`) | That type or null. Example: `string?` accepts string or null |

**Notifications** — call `klex_bridge.notify(payload)` during a long-running handler to stream progress. The kLex side receives the payload on the bridge's notification channel.

```python
from klex_bridge import handler, notify, serve

@handler(args=[("files", "array")], returns="hash")
def scan(files):
    for i, f in enumerate(files):
        notify({"phase": "progress", "done": i, "total": len(files)})
        # ... do work ...
    return {"ok": True}
```

### Legacy (no-helper) bridges

You can still write a hand-rolled bridge that speaks the raw protocol directly. Those bridges run unchanged — they just don't get argument validation or `__schema__` introspection. The minimal template:

```python
#!/usr/bin/env python3
import json
import sys

HANDLERS = {"add": lambda a, b: a + b}

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        req     = json.loads(line)
        fn_name = req["fn"]
        if fn_name not in HANDLERS:
            resp = {"id": req["id"], "error": f"unknown function: {fn_name}"}
        else:
            resp = {"id": req["id"], "result": HANDLERS[fn_name](*req.get("args", []))}
    except Exception as e:
        resp = {"id": req.get("id", 0), "error": str(e)}
    print(json.dumps(resp), flush=True)
```

Three rules for hand-rolled loops:
1. Always `flush=True` on print — kLex blocks waiting for the newline.
2. Always catch all exceptions and return an error response — never let the loop crash silently.
3. Never write to stdout outside the response loop — it will corrupt the protocol.

---

## Error Handling in kLex

`bridgeCall` returns `(result, err)`. The `err` object has two fields:
- `err.message` — the error string (includes the last 500 bytes of stderr when relevant)
- `err.code` — one of the codes below

| Code | Meaning | Bridge state after |
|---|---|---|
| `BRIDGE_OPTS_INVALID` | Bad value in the `nativeBridge` opts hash | n/a — bridge never started |
| `BRIDGE_SCHEMA_ARG` | Argument failed kLex-side schema validation (request never hit the wire) | usable |
| `BRIDGE_ERROR` | Protocol or logic error returned by the bridge | usable |
| `BRIDGE_CLOSED` | Subprocess closed unexpectedly | unusable — close and restart |
| `BRIDGE_TIMEOUT` | Call exceeded the timeout | tainted — close and restart |
| `BRIDGE_TAINTED` | Bridge is unusable after a prior fatal error | close and restart |

Minimal pattern:
```frog
result, err = bridgeCall(bridge, "fn", [arg])
if err != null {
    println(err.code + ": " + err.message)
    return null
}
```

**Tainted-on-timeout semantics:** when a call times out, the bridge is marked tainted. Subsequent calls fail fast with `BRIDGE_TAINTED` instead of attempting to use a known-broken subprocess. To recover, call `bridgeClose(bridge)` and start a new bridge. This avoids the classic JSON-RPC pitfall where a late response to a timed-out call gets matched to the wrong request ID.

**Wrapping a multi-argument call in `safe()`:** `safe(fn, singleArg)` passes `singleArg` as one argument, so you cannot use `safe(myFn, [a,b,c])` to call `myFn(a,b,c)`. Use a zero-arg closure instead:

```frog
result, err = safe(fn() { return myFunctionThatMightThrow(a, b, c) })
if err != null { println(err.message) }
```

---

## Known Limitations at This Point in kLex's Life

These are real constraints verified from the Go source, not speculation.

**1. No streaming responses.**
The protocol is strictly request/response. There is no way for a bridge function to send back partial results or progress updates while it is working. If you need progress during a long operation, split it into multiple calls (e.g. `start_job` → poll `get_progress` → `get_result`), or have the bridge write intermediate results to a file and poll that from kLex.

**2. Calls are fully serialised per bridge.**
`bridgeCall` holds a mutex for the entire duration of the call. Two async tasks calling the same bridge concurrently will queue — they will not deadlock, but they will not run in parallel. If you need parallel bridge calls, start multiple bridge instances (one per worker), as the YARA and entropy scanners do.

**3. Response size capped by `max_response_mb`.**
Default 1 MB. Configurable up to 256 MB via the opts hash. Any response larger than the configured cap causes a `BRIDGE_CLOSED` read error. For very large datasets, paginate across multiple calls or write to a temp file and return the path.

**4. The bridge process has no access to kLex's environment.**
It runs as a plain subprocess. It cannot read kLex variables, call kLex functions, or send unsolicited messages to kLex. Communication is strictly kLex-initiated request/response.

**5. No binary data.**
Everything is JSON. Binary data (images, compiled artifacts, etc.) must be base64-encoded on the bridge side and decoded on the kLex side — or, better, written to a temp file with the path returned as a string.

**6. Bridge must be on the local machine.**
`nativeBridge` uses `exec.Command`. The subprocess runs locally. There is no built-in support for a remote bridge over a socket or HTTP.

---

## Phase 2 — Operational Maturity

### Notifications (server-push messages)

A bridge subprocess can emit unsolicited messages at any time without a request pending. Write a line with a `notif` key and no `id`:

```json
{"notif": {"phase": "progress", "done": 42, "total": 100}}
```

Add this helper to every Python bridge that uses notifications:

```python
def notify(data):
    print(json.dumps({"notif": data}), flush=True)
```

On the kLex side, get the notification channel and drain it in a parallel async task:

```frog
notifCh = bridgeNotifications(bridge)

async(fn() {
    msg, ok = recv(notifCh)
    while ok {
        println("progress: " + str(msg["done"]))
        msg, ok = recv(notifCh)
    }
})

result, err = bridgeCall(bridge, "long_job", [arg])
```

The channel is closed when the bridge closes — `recv` returns `false` for `ok` and the loop exits cleanly. Buffer: 256 items, drop-newest on overflow (notifications are inherently lossy progress signals).

**Call `bridgeNotifications` before starting the long operation** so no early notifications are lost into the buffer.

---

### Concurrent Calls

Multiple `async` tasks may call the same bridge at the same time. Phase 2 replaced the per-bridge serialisation mutex with id-based response routing. Each `bridgeCall` gets its own response channel; the reader goroutine routes by `id`.

```frog
// These four calls run in parallel — all four wait simultaneously.
t1 = async(fn() { return bridgeCall(bridge, "work", ["A"]) })
t2 = async(fn() { return bridgeCall(bridge, "work", ["B"]) })
t3 = async(fn() { return bridgeCall(bridge, "work", ["C"]) })
t4 = async(fn() { return bridgeCall(bridge, "work", ["D"]) })
r1, _ = await(t1)
r2, _ = await(t2)
r3, _ = await(t3)
r4, _ = await(t4)
```

**Python side:** a sequential bridge just processes them one at a time (no issues). An asyncio bridge can handle them in parallel. The kLex side doesn't care — it routes by id regardless.

---

### stdlib/bridge.lex — Resilient Bridge

The standard library wrapper eliminates the boilerplate every bridge user writes:

```frog
import "stdlib/bridge.lex" as br

b, err = br.newResilient("python3", ["my_bridge.py"], {
    "timeout_seconds": 30,
    "max_retries":     3,        // circuit opens after 3 consecutive failures
    "require_deps":    true,     // calls check_deps() on startup
})
if err != null { println(err.message)  return }

result, err = b.call("my_function", [arg])
if err != null {
    if br.isCircuitOpen(err) {
        println("bridge failed too many times — manual intervention required")
    } else {
        println(err.message)
    }
    return
}

b.close()
```

**Circuit breaker:** after `max_retries` consecutive `BRIDGE_CLOSED` or `BRIDGE_TAINTED` failures, all calls immediately return `CIRCUIT_OPEN`. Call `b.reset()` to clear it and try again.

**Auto-restart:** on `BRIDGE_CLOSED` or `BRIDGE_TAINTED`, the wrapper closes the dead bridge, starts a new one, optionally calls `health_check()` to verify it is alive, then retries the original call once.

**`health_check()` convention:** if your bridge implements `health_check` it is called after each restart. Return `{"ok": true}`. Bridges that don't implement it are assumed alive (`BRIDGE_ERROR: unknown function` is silently accepted).

**`check_deps()` convention:** if your bridge implements `check_deps` and you pass `require_deps: true`, it is called on startup. Return `{"ok": bool, "missing": [...]}`. Missing deps surface as `MISSING_DEPS` error before any work starts.

**Notification forwarding:** `b.notifications()` returns the underlying `bridgeNotifications(bridge)` channel.

**One-shot with auto-close:**

```frog
result, err = br.withBridge("python3", ["tool.py"], null, fn(b) {
    return bridgeCall(b, "process", [input])
})
// bridge is closed automatically, even if the callback throws
```

---

## Phase 3 — Schema Declaration & Validation

Bridges declare each handler's argument and return types so kLex catches mismatches at the call site instead of as cryptic runtime errors after a round-trip.

**On the wire.** When `nativeBridge` succeeds, kLex immediately sends a `__schema__` call:
```json
{"id": 1, "fn": "__schema__", "args": []}
```
The bridge replies with the full handler map:
```json
{"id": 1, "result": {
  "add":   {"args": [["a","int"], ["b","int"]], "returns": "int"},
  "greet": {"args": [["name","string"]],         "returns": "string"}
}}
```
kLex caches this on the `Bridge` and validates every `bridgeCall` against it. Bridges that don't implement `__schema__` (or that respond with an error) work unchanged — they just don't get validation. Fully backward-compatible.

**Where validation happens.** Two layers:
1. **kLex side, before marshalling.** A type mismatch returns `BRIDGE_SCHEMA_ARG` immediately, at the `bridgeCall` site, with no round-trip.
2. **Python side, before invoking the handler.** `klex_bridge`'s `serve()` revalidates the same schema as defence in depth — so the error is consistent whether or not kLex pulled the schema during handshake.

**Introspection from kLex.** Use `bridgeSchema()` to read the cached map:
```frog
schemas = bridgeSchema(bridge)
// hash of { fnName -> { args: [[name,type],...], returns: type } }

s = bridgeSchema(bridge, "add")
// { args: [["a","int"], ["b","int"]], returns: "int" }
// or null if the handler isn't declared
```
Returns `null` overall when the bridge didn't expose any schemas.

**PYTHONPATH injection.** `nativeBridge` locates kLex's `stdlib/python/` directory and prepends it to the subprocess's `PYTHONPATH` so `import klex_bridge` works without any setup. Search order: `$KLEX_PATH/python`, `$CWD/stdlib/python`, `<klex-exe-dir>/stdlib/python`, `<klex-exe-parent>/stdlib/python`. If none is found, no injection happens and `import klex_bridge` will fail with a clear `ImportError` visible via `bridgeStderr()`.

**Migrating an existing bridge.** Drop the hand-rolled dispatch loop and rewrite as:
```python
#!/usr/bin/env python3
from klex_bridge import handler, serve

@handler(args=[("path", "string")], returns="hash")
def load(path):
    ...

serve()
```
Handler names stay the same on the wire, so existing kLex code that calls the bridge keeps working. The migration is opt-in — leave legacy bridges in place if they're working.

---

## What Phase 1 Hardening Fixed

These were limitations in earlier versions of the bridge; they are no longer constraints.

| Old limitation | Phase 1 solution |
|---|---|
| Calls could hang forever | `timeout_seconds` option + per-call timeout argument |
| stderr discarded — invisible Python tracebacks | Captured to ring buffer (default) or log file; exposed via `bridgeStderr()` and appended to error tails |
| Orphaned subprocesses on kLex crash | Process group registration + signal handler force-kills all bridges on SIGINT / SIGTERM / panic |
| Hard 1MB response limit | Configurable `max_response_mb` up to 256MB |
| No structured error codes | Five distinct codes (`BRIDGE_*`) with semantic meaning |
| Subprocess could be re-used after fatal error | Tainted-on-timeout pattern forces explicit recovery |

---

## Checklist for Adding a New Bridge

- [ ] Create `my_bridge.py` alongside the other bridge scripts
- [ ] Add a `check_deps()` function that verifies required packages and returns `{"ok": true/false, "missing": [...]}`
- [ ] Write all diagnostic output to a log file, never stderr
- [ ] Return `{"error": "message"}` from every handler that can fail — never raise uncaught exceptions from the loop
- [ ] Scrub sensitive values (tokens, passwords) from error messages before returning
- [ ] Add a safety check to any cleanup/delete handler so it only operates on paths under `/tmp`
- [ ] Test the bridge standalone before wiring it into kLex: `echo '{"id":1,"fn":"check_deps","args":[]}' | python3 my_bridge.py`
- [ ] Call `bridgeClose(bridge)` when done — don't leave orphaned processes
- [ ] If responses might exceed 1 MB, paginate or use temp files
- [ ] If you need parallelism, start one bridge per worker for independent workloads, or use concurrent `bridgeCall` on one bridge if the Python side supports it
- [ ] If emitting notifications, add the `notify()` helper and document which functions use it
- [ ] Consider using `stdlib/bridge.lex` `newResilient` instead of raw `nativeBridge` for production code

---

## Reference Implementations

Working bridges in the repo to study:

- `stdlib/python/klex_bridge.py` — the helper module itself; read it to see exactly what the dispatch loop and validator do
- `tests/examples/bridge/python_bridge.py` — minimal arithmetic example using the `@handler` decorator and schemas; ideal starting point
- `tests/examples/bridge/schemaTest.lex` — exercises `bridgeSchema()` introspection + `BRIDGE_SCHEMA_ARG` validation
- `tests/examples/SecretHunter/yara_bridge.py` — legacy (no-helper) bridge using `load` + `scan_batch`; uses an external Python library (yara-python)
- `tests/examples/SecretHunter/github_bridge.py` — full-featured: API client, subprocess (git), HTTP downloads, config file management, sensitive-value scrubbing
- `tests/examples/bridge/robustness_bridge.py` — Phase 1+2 test bridge: timeout, crash, kill, notifications, concurrent echo
- `tests/examples/bridge/robustnessTest.lex` — Phase 1 test suite (12 tests)
- `tests/examples/bridge/phase2Test.lex` — Phase 2 test suite (24 tests): notifications, concurrent calls, resilient bridge, circuit breaker
