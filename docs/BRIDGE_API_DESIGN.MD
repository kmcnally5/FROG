# Bridge API Design — Transport-Polymorphic `bridgeOpen()`

> Status: **PROPOSAL** — design doc, not implemented.
> Phase 1 of the kLex WASM port roadmap.
> Date: 2026-05-27.

## Motivation

The current `nativeBridge(cmd, args, opts)` API is hard-wired to one transport:
subprocess + stdin/stdout. This is the right primitive for desktop kLex, but
two upcoming use cases need different transports:

1. **WASM target** — browser kLex can't spawn subprocesses. It needs Web Worker
   transport (`postMessage` instead of stdin/stdout).
2. **Distributed kLex** — bridge pools spanning machines need TCP/QUIC transport
   so a `pool` worker can be a remote process.

If we add `workerBridge()` and `remoteBridge()` as sibling builtins, we end up
with N×M API surface — N bridge entry points × M per-bridge operations
(call/stream/close/schema/info/metrics/notifications/stderr). That's an
explosion of redundant Go code and docs.

**Cleaner: factor transport out of `bridge`.** One constructor takes a
transport hash; every per-bridge builtin stays transport-agnostic. The
Bridge object holds an interface, not a `*exec.Cmd`.

## Current API (summary)

```
nativeBridge(cmd: string, args: array, opts?: hash) -> (bridge, error)
bridgePool(n: int, cmd: string, args: array, opts?: hash) -> (pool, error)

bridgeCall(b, fn, args, timeoutSec?)            -> (result, error)
bridgeStream(b, fn, args, timeout?)             -> (channel, error)
bridgeClose(b)                                  -> null
bridgeInfo(b)                                   -> hash
bridgeSchema(b, fn?)                            -> hash | null
bridgeMetrics(b)                                -> hash
bridgeNotifications(b)                          -> channel
bridgeStderr(b)                                 -> array

bridgePoolCall, bridgePoolStream, bridgePoolClose, bridgePoolHealth,
bridgePoolStderr  — pool versions of the above
```

Wire protocol: line-delimited JSON over stdin/stdout, with `__hello__` +
`__schema__` handshake at startup.

## Proposed API

### Single transport-polymorphic constructor

```
bridgeOpen(transport: hash) -> (bridge, error)
bridgePool(n: int, transport: hash, opts?: hash) -> (pool, error)
```

Named `bridgeOpen` (not `bridge`) so the natural variable name doesn't
shadow the builtin: `let bridge, err = bridgeOpen({...})`. Symmetric
with `bridgeClose` and consistent with the rest of the `bridgeXxx`
family (`bridgeCall`, `bridgeStream`, `bridgeInfo`, `bridgeSchema`,
`bridgeMetrics`, `bridgeNotifications`, `bridgeStderr`).

The `transport` hash carries both **what kind** of transport and the keys
that kind needs. All per-bridge operations (`bridgeCall`, `bridgeStream`,
`bridgeClose`, etc.) keep their signatures unchanged.

### Transport hash schema

Required everywhere:

| Key | Type | Notes |
|---|---|---|
| `kind` | string | One of `"subprocess"`, `"worker"`, `"remote"` (more later) |

Optional everywhere (shared opts):

| Key | Type | Default | Meaning |
|---|---|---|---|
| `timeout` | number (sec) | 0 (no timeout) | Per-call timeout default |
| `stderr_log` | string | "" | Path to mirror bridge stderr to (subprocess only) |
| `maxBytes` | int | 1 MiB | Max wire-frame size; cap at 256 MiB |

Per-kind required keys:

#### `kind: "subprocess"`
| Key | Type | Notes |
|---|---|---|
| `cmd` | string | Executable path or name on PATH |
| `args` | array of string | Optional |

```
bridgeOpen({
    "kind": "subprocess",
    "cmd": "python3",
    "args": ["yara_bridge.py"],
    "timeout_seconds": 30,
    "stderr_log": "/var/log/yara.err"
})
```

#### `kind: "worker"` (browser/WASM only)
| Key | Type | Notes |
|---|---|---|
| `script` | string | URL or relative path to the worker `.js` file |
| `type` | string | `"classic"` (default) or `"module"` (for ES modules) |

```
bridgeOpen({
    "kind": "worker",
    "script": "./workers/yara_worker.js",
    "type": "module"
})
```

#### `kind: "remote"` (deferred, future)
| Key | Type | Notes |
|---|---|---|
| `url` | string | `tcp://host:port` or `https://host/bridge` |
| `auth` | hash | Optional auth headers/tokens |

### Pool follows the same shape

```
bridgePool(16, {
    "kind": "subprocess",
    "cmd": "python3",
    "args": ["yara_bridge.py"]
}, {
    "timeout_seconds": 30,
    "init": fn(b) { return bridgeCall(b, "load", ["secrets.yar"]) }
})
```

Per-bridge opts (`timeout`, `stderr_log`, `maxBytes`) live inside the
**transport hash**. Pool-specific opts (`init`) live in the separate
**opts hash**. That keeps the transport hash interchangeable between
`bridgeOpen()` and `bridgePool()` — same shape, same meaning.

## Internal dispatcher design

### One Go interface, three implementations

```go
// eval/bridge_transport.go (new, no build tag)
type BridgeTransport interface {
    // Send one wire-protocol line (LF-terminated when written).
    Send(line []byte) error

    // Read the next inbound line. Blocks until a line is available
    // or the transport closes. Returns io.EOF on clean close.
    Recv() ([]byte, error)

    // Stderr returns the captured stderr ring buffer, or empty
    // for transports without stderr (worker, remote).
    Stderr() []string

    // Close terminates the underlying transport. Idempotent.
    Close() error

    // Kind returns the literal transport kind ("subprocess", etc.).
    Kind() string

    // Info returns transport-specific metadata for bridgeInfo().
    // E.g. {"pid": 12345} for subprocess; {"script": "url"} for worker.
    Info() map[string]Object
}
```

Per-transport spawn functions:

```go
// eval/bridge_transport_subprocess.go (//go:build !js)
func spawnSubprocessTransport(opts subprocessOpts) (BridgeTransport, Object) { ... }

// eval/bridge_transport_worker.go (//go:build js && wasm)
func spawnWorkerTransport(opts workerOpts) (BridgeTransport, Object) { ... }

// eval/bridge_transport_remote.go (future; no build tag — TCP works everywhere)
func spawnRemoteTransport(opts remoteOpts) (BridgeTransport, Object) { ... }
```

The dispatcher:

```go
// eval/builtins_bridge.go (no build tag now)
Builtins["bridgeOpen"] = &Builtin{Fn: func(args []Object) Object {
    if len(args) != 1 {
        return runtimeError("bridgeOpen expects 1 argument (transport hash)", ast.Pos{})
    }
    transport, ok := args[0].(*Hash)
    if !ok {
        return bridgeError("BRIDGE_OPTS_INVALID", "bridgeOpen: transport must be a hash")
    }
    kind, err := transportKind(transport)
    if err != nil {
        return err
    }
    var t BridgeTransport
    var spawnErr Object
    switch kind {
    case "subprocess":
        t, spawnErr = spawnSubprocessTransport(parseSubprocessOpts(transport))
    case "worker":
        t, spawnErr = spawnWorkerTransport(parseWorkerOpts(transport))
    default:
        return bridgeError("BRIDGE_TRANSPORT_UNKNOWN",
            fmt.Sprintf("bridgeOpen: unknown transport kind %q", kind))
    }
    if spawnErr != nil {
        return spawnErr
    }
    return newBridgeFromTransport(t)  // does __hello__ + __schema__ handshake
}}
```

### Build-tag gating

The dispatcher itself is universal. Per-transport spawn functions are
build-tagged, plus a `_stub.go` for the "wrong platform" case:

```
eval/bridge_transport.go                   — interface, no tag
eval/bridge_transport_subprocess.go        — //go:build !js
eval/bridge_transport_subprocess_stub.go   — //go:build js (returns BRIDGE_TRANSPORT_UNAVAILABLE)
eval/bridge_transport_worker.go            — //go:build js && wasm
eval/bridge_transport_worker_stub.go       — //go:build !js (returns BRIDGE_TRANSPORT_UNAVAILABLE)
```

Result: every platform compiles. Asking for a transport that's not
available on this platform returns a clean error at runtime instead of
a missing builtin.

### Bridge object refactor

`*Bridge` currently holds `cmd *exec.Cmd`, `stdin io.WriteCloser`,
`stdout *bufio.Scanner`, etc. These all become hidden inside the
transport. The `*Bridge` struct keeps:

- `transport BridgeTransport`
- `notifications *Channel`
- `pending map[int64]chan *bridgeResponse`
- `schema *Hash`
- `metrics bridgeMetrics`
- `stderrRing *BridgeRingBuffer` (read via `transport.Stderr()`)
- `tainted *atomic.Bool`
- etc.

Methods like `bridgeCall` now go through `transport.Send`/`transport.Recv`,
not `b.stdin.Write`/`b.stdout.Scan`. The protocol layer is unchanged.

## Migration plan

### Step 1 — Land the new API alongside the old one
- Add `Builtins["bridgeOpen"]` and update `Builtins["bridgePool"]` signature.
- Implement subprocess transport (matches today's behaviour exactly).
- Keep `Builtins["nativeBridge"]` and `Builtins["bridgePool"]` working.
- Tests pass with the existing call sites untouched.

### Step 2 — Automated call-site migration
- A one-shot codemod (maintainer-internal, not shipped in the public tree).
- Walks `.lex` (stdlib + tests + examples) and `.MD` (docs).
- Rewrites `nativeBridge(cmd, args, opts?)` →
  `bridgeOpen({"kind": "subprocess", "cmd": cmd, "args": args, ...opts})`.
- Same shape for `bridgePool(n, cmd, args, opts?)` →
  `bridgePool(n, {"kind": "subprocess", "cmd": cmd, "args": args}, opts)`.
- Idempotent (`--check` flag exits non-zero if anything is unmigrated).
- Has `--dry-run` for preview.

### Step 3 — Run the codemod, land the diff
- All call sites switch to `bridgeOpen()` / `bridgePool()`.
- Tests must still pass (behaviour is identical).

### Step 4 — Remove the old `nativeBridge` builtin
- Delete `Builtins["nativeBridge"]` registration.
- The codemod's check mode is now a CI gate against regression.
- LSP/docs regenerated.

### Step 5 — Phase 3 work (Worker transport)
- Implement worker transport spawn function for WASM build.
- Adapt `stdlib/node/klex_bridge.js` to support both stdio and postMessage modes (one config flag).
- No further changes to `.lex` code — `bridgeOpen({"kind": "worker", ...})` just works.

## Error codes

Existing (unchanged):
| Code | Meaning |
|---|---|
| `BRIDGE_ERROR` | Generic bridge failure |
| `BRIDGE_TIMEOUT` | Call exceeded `timeout` |
| `BRIDGE_CLOSED` | Transport closed mid-call |
| `BRIDGE_TAINTED` | Bridge has been marked dead by a prior failure |
| `BRIDGE_OPTS_INVALID` | Malformed opts hash |

New:
| Code | Meaning |
|---|---|
| `BRIDGE_TRANSPORT_UNKNOWN` | `kind` not recognised |
| `BRIDGE_TRANSPORT_UNAVAILABLE` | `kind` recognised but not built into this binary (e.g. `worker` on desktop) |
| `BRIDGE_TRANSPORT_MISCONFIGURED` | Required key missing (e.g. `subprocess` with no `cmd`) |

## Per-bridge builtins — unchanged signatures

All of these keep the exact signatures they have today:

```
bridgeCall(b, fn, args, timeoutSec?)         -> (result, error)
bridgeStream(b, fn, args, timeout?)          -> (channel, error)
bridgeClose(b)                               -> null
bridgeInfo(b)                                -> hash
bridgeSchema(b, fn?)                         -> hash | null
bridgeMetrics(b)                             -> hash
bridgeNotifications(b)                       -> channel
bridgeStderr(b)                              -> array
```

And the pool versions, **unchanged names**:

```
bridgePoolCall, bridgePoolStream, bridgePoolClose, bridgePoolHealth, bridgePoolStderr
```

(Per FROG decision #3, `bridgePool*` names stay. Only the constructor's
**signature** changes — from positional `(n, cmd, args, opts)` to
`(n, transport, opts)`. The codemod handles the call-site rewrite.)

## Future transports (sketched, not in v1)

### `kind: "remote"` — TCP/QUIC remote bridge
- Same JSON protocol over a TCP connection
- Enables distributed kLex (bridge pool members on remote machines)
- Auth via TLS + token in `auth` hash

### `kind: "inproc"` — in-process Go callable
- Mostly useful for testing
- The "transport" is a goroutine + two channels
- Skips JSON serialisation; passes Object values directly
- Open question: is the win worth the complexity?

### `kind: "tcp_relay"` — proxy through a fixed gateway
- For zero-trust environments where workers can't be reached directly
- Probably overkill for v1; flag as future

## FROG decisions (resolved)

### Decision 10 (added 2026-05-27): Builtin named `bridgeOpen`, not `bridge`

Hit during Phase 1 implementation. Initial plan named the constructor
`bridge()` returning a `*Bridge`. First migration revealed that the
natural usage `let bridge, err = bridge({...})` shadows the builtin
from line 2 onward; any later reassignment (`bridge, err = bridge({...})`)
evaluates the RHS as `<local Bridge>(...)` and fails with
`TypeError: not a function, got BRIDGE`. Renamed to `bridgeOpen()` —
symmetric with `bridgeClose`, fits the `bridgeXxx` family, no shadowing.
See [[feedback-builtin-naming-avoid-collisions]].

Per "what would the FROG choice be?" — kLex is pure, honest, explicit, no
shortcuts. Each open question landed on the FROG side.

| # | Decision | Reasoning |
|---|---|---|
| 1 | Single flat transport hash | Nested `{transport, opts}` is indirection without meaning. Flat is honest. |
| 2 | Pool: transport hash + separate opts hash | Per-bridge config and pool-only config (`init`) are genuinely different concerns. |
| 3 | **Keep `bridgePool*` names** (no rename) | `pool` alone is too generic. `bridgePool` is explicit about what it pools. |
| 4 | Remove `nativeBridge` outright (no shim) | "No shortcuts, hacks, dirty workarounds" — deprecation shims are technical debt. |
| 5 | Strict key validation on transport hash | Silent typo acceptance is anti-honest. Fail loud, name the bad key. |
| 6 | Defer `BridgeTransport` Go interface to Phase 3 | YAGNI. With one transport, interface is premature abstraction. Extract under real pressure when worker transport lands. |
| 7 | Case-sensitive `kind` values | No magic normalisation. `"Subprocess"` ≠ `"subprocess"`. |
| 8 | No default `kind` — always required | `bridge({cmd: "x"})` is a `BRIDGE_TRANSPORT_MISCONFIGURED` error, not a fall-back to subprocess. |
| 9 | Per-call `timeout` overrides transport-hash `timeout` | Unchanged from today — `bridgeCall(b, fn, args, timeoutSec)` still wins per-call. |

### Implications for the implementation plan

Decision #6 (defer interface) simplifies Step 2 of the original sequencing
significantly. The new `bridgeOpen()` builtin can be a thin dispatcher that
parses the transport hash, validates `kind`, and calls the *existing*
`spawnBridge` directly for `kind: "subprocess"`. No interface, no refactor
of the 1000-line `builtins_bridge.go`. When worker transport arrives in
Phase 3, *that's* the moment to extract `BridgeTransport` under real
pressure from a second implementation.

Decision #3 (keep `bridgePool*` names) means the codemod rewrites
`bridgePool(n, cmd, args, opts)` →
`bridgePool(n, {"kind": "subprocess", "cmd": cmd, "args": args, ...opts})`
— name unchanged, signature changes.

## Out of scope

- Bidirectional streams (host → bridge callbacks). Already deferred per
  existing [feedback_bridge_bidirectional_deferred] memory.
- A general "kLex IPC" abstraction beyond bridges. Channels are the
  in-process primitive; bridges are the cross-process primitive. We are
  not unifying them.
- Auto-respawn / resilient bridge logic. That lives in `stdlib/bridge.lex`
  (`newResilient`) and is one of the MANUAL migration sites (runtime opts
  hash) — needs a small hand refactor to use `bridgeOpen()`.

## Implementation sequencing

| Step | What | Effort |
|---|---|---|
| 1 | Design sign-off (this doc) | Discussion |
| 2 | Implement `BridgeTransport` interface + subprocess impl | 1-2 days |
| 3 | Dispatcher for `bridgeOpen()` + `bridgePool()` (subprocess only) | 1 day |
| 4 | Build the migration codemod (maintainer-internal) | 1 day |
| 5 | Run migrate; verify all tests pass | 0.5 day |
| 6 | Remove `nativeBridge` + `bridgePool` builtins | 0.5 day |
| 7 | Regen LSP/docs; update KLEX_LANGUAGE.MD | 0.5 day |
| 8 | Update `stdlib/bridge.lex`'s `newResilient` to use `bridge()` | 0.5 day |

**Total: ~1 week of focused work**, ending with a green test suite and
the API ready for Phase 3 (Worker transport).
