# kLex WASM — Complete Guide

kLex compiles to WebAssembly (`GOOS=js GOARCH=wasm`) so it can run directly in a browser. This document covers everything: architecture, building, the JavaScript API, WASM-specific builtins, platform limitations, testing, and how to extend the WASM build.

For the kLex language itself see [KLEX_LANGUAGE.MD](KLEX_LANGUAGE.MD).  
For the full builtin reference see [BUILTINS.MD](BUILTINS.MD).  
For the standard library see [STDLIB.MD](STDLIB.MD).

---

## Quick Start — Running the Playground Locally

```bash
cd examples/playground
./serve.sh
# open http://localhost:8768/
```

`serve.sh` handles everything automatically:

1. Regenerates the embedded stdlib map (`go run ./tools/stdlibgen`)
2. Builds and runs the VM builtin-table generator (`vm/cmd/vmbuiltins`)
3. Compiles the WASM binary to `bin/klex.wasm`
4. Starts a kLex HTTP server on port 8768

You need nothing pre-built — `serve.sh` compiles every tool from source on a fresh clone (the `bin/` directory it writes to is generated, not checked in). The playground gives you a full REPL: type kLex/FROG code, press Enter, see output. Multi-line input is supported — the REPL waits for balanced braces before evaluating.

---

## Building klex.wasm Manually

### Prerequisites

- Go 1.26 or later
- Node.js (only needed to run the `wasmsmoke` parity tests)

> **The `bin/` directory is generated, not shipped.** It is in `.gitignore`, so a
> fresh clone has no `bin/`. Every tool below is invoked straight from source with
> `go run ./<path>` — Go compiles it on demand, so there is nothing to install or
> pre-build. `bin/klex.wasm` and friends are simply *output paths* the build writes
> to; the directory is created automatically on first build.

### One-shot build

From the repo root:

```bash
# 1. Regenerate the embedded stdlib (puts source into cmd/wasm/embeddedstdlib_gen.go)
go run ./tools/stdlibgen

# 2. Regenerate the VM builtin table (vm/builtins_gen_wasm.go)
go run ./vm/cmd/vmbuiltins

# 3. Build the WASM binary (creates bin/ if absent)
GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -o bin/klex.wasm ./cmd/wasm
```

`CGO_ENABLED=0` is required because CGO is unavailable in the browser. The resulting binary is typically 20–30 MB (Go's WASM runtime is not small; this is normal).

### What serve.sh automates

The script `examples/playground/serve.sh` runs all three steps above before every serve. The WASM binary is built to `bin/klex.wasm` and served from there directly — no copy is made anywhere. It also auto-builds `bin/vmbuiltins` if the binary is absent (first checkout, or after changes to the generator source).

### Shared WASM assets

All kLex WASM hosts (the playground and the three test servers) use the same shared assets served from canonical locations — nothing is ever copied at runtime:

| Asset | Canonical path |
|---|---|
| Go WASM runtime glue | `stdlib/wasm/wasm_exec.js` |
| Compiled kLex binary | `bin/klex.wasm` |
| Worker bridge helper | `stdlib/worker/klex_bridge_worker.js` |

Every `serve.lex` imports `stdlib/wasm/serve_base.lex` for the shared MIME map and file-serving helpers, then uses `_scriptDir()` to compute the repo root and build absolute paths to these assets. No external web server (e.g. Python) is needed — kLex hosts itself.

---

## Architecture

### Entry point — `cmd/wasm/main.go`

Build tag: `//go:build js && wasm`

This is the WASM `package main`. Its `init()` wires three things:

| Hook | What it does |
|---|---|
| `eval.EmbeddedImportLookup` | Routes `import "stdlib/…"` to the in-binary stdlib map (no filesystem in the browser) |
| `eval.VMRunScript` | Wires the bytecode VM for isolated full-program execution via `runScript()` |
| JS globals | Exports `klex_eval`, `klex_reset`, `klex_depth` to JavaScript |

`main()` blocks forever on a channel — the program lives as long as the page does.

### Two execution modes

| Mode | Used by | Environment | VM? |
|---|---|---|---|
| **REPL** | `klex_eval(src)` | Persistent `globalEnv` — state survives between calls | Tree-walker |
| **runScript** | `runScript(src)` builtin | Fresh `NewEnv()` per call — fully isolated | Bytecode VM (falls back to tree-walker on unsupported AST) |

The REPL uses the tree-walker so that module imports (`import "stdlib/json.lex"`) work correctly with kLex's live-binding environment semantics. The VM is reserved for `runScript` where each call is stateless and gets its own environment.

### Embedded stdlib

The browser has no filesystem, so every `.lex` file under `stdlib/` is embedded at build time. The tool `tools/stdlibgen` walks `stdlib/` and emits `cmd/wasm/embeddedstdlib_gen.go`:

```go
//go:build wasm
var embeddedStdlib = map[string]string{
    "stdlib/json.lex": `...source...`,
    ...
}
```

`eval.EmbeddedImportLookup` is set to read from this map, so `import "stdlib/json.lex"` resolves to the embedded source rather than the disk.

If you add a new stdlib file, re-run `go run ./tools/stdlibgen` before building.

### Bytecode VM in WASM

The bytecode VM (`vm/`) is compiled into the WASM binary and wired to `runScript`. When `runScript(src)` is called:

1. Source is parsed to an AST.
2. The VM tries to compile and run the AST.
3. If the compiler hits an unsupported AST node, it returns `(nil, false)` and the tree-walker takes over — keeping rollout safe while the VM matures.

The VM is intentionally **not** wired to module imports in the REPL path. Wiring it there caused import failures because the VM's environment model doesn't interoperate cleanly with the tree-walker's live-binding REPL state.

### Output capture and concurrency

`eval.Output` is a package-level `io.Writer` that `println` writes to. In the WASM REPL, each `klex_eval` call runs in a goroutine and captures output into a local `strings.Builder`. A package-level `sync.Mutex` (`jsEvalMu`) serialises concurrent calls so only one goroutine holds `eval.Output` at a time.

This is correct for a REPL: sequential input, sequential execution. It also means that if a script blocks on a bridge channel operation, the next user input waits until the current script resolves its Promise — which is what the playground UI expects.

### Builtin table split

The desktop and WASM builds have different builtin counts:

| Build | File | Count |
|---|---|---|
| Desktop (`//go:build !js`) | `vm/builtins_gen.go` | 520 |
| WASM (`//go:build js && wasm`) | `vm/builtins_gen_wasm.go` | 499 |

The difference is the set of builtins that don't exist in the WASM binary (graphics, process, bridge pool, Metal/tensor, database drivers, etc.). Both files are generated by `go run ./vm/cmd/vmbuiltins`, which evaluates every `//go:build` constraint in `eval/` against `GOOS=js GOARCH=wasm` to decide what to include.

If the table is stale, the WASM binary panics at startup with a clear message:

```
vm: generated builtin table references missing eval.Builtins["beginPath"]
```

Fix: run `go run ./vm/cmd/vmbuiltins` and rebuild.

---

## JavaScript API

The WASM binary exports three functions to JavaScript via `syscall/js`.

### `klex_eval(src) → Promise<Result>`

Evaluates `src` in the persistent REPL environment. Returns a Promise that resolves to:

```js
{
  output:  string,   // everything printed by println()
  error:   string,   // parse or runtime error message, empty on success
  isError: boolean   // true when error is non-empty
}
```

The call is asynchronous (returns a Promise) so the JS event loop remains free to deliver bridge worker responses while a script is running. Always `await` it or use `.then()`.

```js
const result = await klex_eval('println("hello")');
if (result.isError) {
    console.error(result.error);
} else {
    console.log(result.output);
}
```

State persists between calls — variables and functions defined in one call are available in the next, just like a terminal REPL.

### `klex_reset()`

Discards all REPL state. Creates a fresh `globalEnv`. Call this to start a new session without reloading the page.

```js
klex_reset();
```

### `klex_depth(src) → number`

Returns the brace/paren nesting depth of `src`. Used by the playground to decide whether to wait for more input (depth > 0) or evaluate immediately (depth == 0). Handles escaped characters and quoted strings correctly.

```js
klex_depth("fn foo() {")   // → 2
klex_depth("fn foo() {}")  // → 0
```

---

## WASM-specific Builtins

These two builtins exist only in the WASM build (`//go:build js && wasm`). They are not available on desktop.

### `runScript(src)` → `{output, error, isError}`

Evaluates `src` as a complete isolated kLex program and returns its output. Unlike the REPL (`klex_eval`), each `runScript` call gets a fresh environment — definitions do not persist.

Used by the playground's "Run" button in the IDE panel to execute user scripts without interfering with the REPL's state.

```frog
let result = runScript("println(1 + 2)")
println(result["output"])   // → 3
println(result["isError"])  // → false
```

Scripts that call `window()` are rejected — the canvas belongs to the playground IDE, not to user scripts. The check is performed on the parsed AST, so `window()` in a comment or string literal does not trigger it.

### `openURL(url)`

Opens `url` in a new browser tab via `window.open(url, "_blank")`. Used by the playground's documentation links.

```frog
openURL("https://github.com/…")
```

---

## Platform Limitations

The browser sandbox eliminates several capabilities that work on desktop. The following builtins and features are **unavailable** in WASM:

### Filesystem

All `_fs*` builtins that touch the host filesystem (`readFile`, `writeFile`, `readDir`, `deleteFile`, `_fsCopy`, `_fsMove`, etc.) are not implemented. The WASM build has stub implementations that return an error immediately.

The embedded stdlib (`import "stdlib/fs.lex"`) is available for string/path utilities, but actual disk I/O fails.

### Process / OS

`exec()`, `execSh()`, `spawn()`, `_processExec`, `_osGetenv`, `_osName`, `_osArgs` — anything that spawns a subprocess or reads OS state — is unavailable. These builtins don't exist in the WASM binary.

### Graphics / UI / Metal

OpenGL, GLFW, and Metal builtins (`window()`, `beginPath()`, `drawText()`, etc.) are not compiled into the WASM binary. There is no canvas output from kLex scripts; the playground canvas is reserved for the IDE itself.

### Database drivers

The Microsoft SQL Server and PostgreSQL drivers use CGO and network socket APIs that are unavailable in the browser. All `_db*` builtins are absent from the WASM build.

### Tensor / Metal compute

Tensor operations (`tensorCreate`, `matMul`, `cosineSim`, etc.) require macOS Metal or Linux CPU kernel paths that don't exist in the browser. They return a `RuntimeError` immediately.

### Timing

Go's WASM runtime runs in a single OS thread with cooperative scheduling. Scripts that measure wall-clock time will see different (usually slower) numbers than desktop. Tests that assert specific timings will fail under `wasmsmoke`.

### Bridge / native workers

The native bridge (Python subprocess model) is unavailable in WASM. The **worker bridge** (`bridge.new()`) works — it uses JavaScript Web Workers under the hood — but the bridge worker script must be served alongside `klex.wasm` and the kLex runtime.

---

## Testing WASM Parity

`tools/wasmsmoke` runs every test file under both the desktop tree-walker and the WASM build, diffs the output byte-for-byte, and reports mismatches. Run it straight from source — no pre-build needed:

```bash
# Run all unit tests
go run ./tools/wasmsmoke --glob='tests/unit/*Test.lex'

# Run one test verbosely
go run ./tools/wasmsmoke -v tests/unit/jsonTest.lex

# CI mode — exits 2 on any failure
go run ./tools/wasmsmoke --ci --glob='tests/unit/*Test.lex'
```

### Current baseline

As of v0.3.36: **59/90 pass, 31 fail**. All 31 failures are pre-existing platform limitations (filesystem, subprocess, database, Metal/tensor, OS env, timing-sensitive tests). Zero unexpected regressions.

### Categorising a failure

When a test fails, check the first differing line:

| wasm output starts with | Category |
|---|---|
| `RuntimeError: … not implemented on js` | Filesystem stub hit |
| `RuntimeError: undefined variable: _processExec` | Subprocess/OS builtin absent |
| `RuntimeError: tensor ops require macOS or Linux` | Tensor/Metal absent |
| `TypeError: cannot access .status on null` | HTTP/network runtime failure |
| A timing value (`1715 ms` vs `253 ms`) | Expected — cooperative scheduling |
| `exit code: 1` | OS-level failure (env, args) |

Tests that fail for the above reasons are expected and should not be fixed by patching WASM — they reflect genuine browser limitations.

---

## Build Tag Reference

All WASM-specific Go files use:

```go
//go:build js && wasm
```

`js` means the `GOOS=js` target (the Go JS/WASM runtime). `wasm` means `GOARCH=wasm`. Both are required — `//go:build wasm` alone would also match WASI targets, which kLex does not support.

Files that must be **excluded** from WASM use:

```go
//go:build !js
```

The convention in `eval/` for per-platform builtin files:

| Filename suffix | Included when |
|---|---|
| `_wasm.go` | `js && wasm` only |
| `_unix.go` | Unix desktop targets |
| `_windows.go` | Windows only |
| (no suffix) | All platforms |

---

## Extending WASM

### Adding a WASM-only builtin

1. Add the registration to a `//go:build js && wasm` file in `eval/` (or to `eval/builtins_eval_wasm.go`).
2. Regenerate the VM builtin table:
   ```bash
   go run ./vm/cmd/vmbuiltins
   ```
3. Verify the WASM build:
   ```bash
   GOOS=js GOARCH=wasm go build ./cmd/wasm/
   ```
4. Run the parity tests to confirm no regressions:
   ```bash
   go run ./tools/wasmsmoke --glob='tests/unit/*Test.lex'
   ```

### Adding a builtin that exists on all platforms

Add the registration to an untagged (or appropriately tagged) file in `eval/`. Then regenerate both builtin tables:

```bash
go run ./vm/cmd/vmbuiltins   # updates both vm/builtins_gen.go and vm/builtins_gen_wasm.go
```

The WASM table generator evaluates `//go:build` constraints against `GOOS=js GOARCH=wasm`, so it automatically includes or excludes the new builtin based on its build tags.

### Adding a new stdlib file

Add the `.lex` file under `stdlib/` then re-run:

```bash
go run ./tools/stdlibgen
```

This updates `cmd/wasm/embeddedstdlib_gen.go` so the new module is available via `import` in the browser.

### After changing the generator itself

If you modify `vm/cmd/vmbuiltins/main.go`, there is nothing extra to do —
`go run ./vm/cmd/vmbuiltins` always compiles the current source. If you run the
generator often and want to skip the recompile each time, you can cache a binary:

```bash
go build -o bin/vmbuiltins ./vm/cmd/vmbuiltins   # optional — bin/ is gitignored
```

---

## File Reference

| Path | Purpose |
|---|---|
| `cmd/wasm/main.go` | WASM entry point — JS exports, hook wiring |
| `cmd/wasm/embeddedstdlib_gen.go` | Generated stdlib map (do not edit) |
| `eval/builtins_eval_wasm.go` | `runScript` and `openURL` builtins |
| `vm/builtins_gen_wasm.go` | Generated WASM VM builtin table (do not edit) |
| `vm/module.go` | `CompileAndRunScript` — VM hook for `runScript` |
| `tools/stdlibgen/` | Stdlib embedder — `go run ./tools/stdlibgen` before every WASM build |
| `vm/cmd/vmbuiltins/` | VM builtin table generator — `go run ./vm/cmd/vmbuiltins` after adding builtins |
| `examples/playground/serve.sh` | One-shot: build + serve the playground |
| `examples/playground/serve.lex` | HTTP file server for the playground |
| `stdlib/wasm/wasm_exec.js` | Go WASM runtime glue (from Go distribution) — canonical copy, served by all WASM hosts |
| `stdlib/wasm/serve_base.lex` | Shared MIME map + `serveFile`/`logged` helpers; imported by every `serve.lex` |
| `tests/wasm_graphics/serve.sh` | Build-check + serve the Canvas2D graphics test on port 8766 |
| `tests/wasm_ui/serve.sh` | Build-check + serve the UI widgets test on port 8767 |
| `tests/wasm_worker/serve.sh` | Build-check + serve the worker bridge test on port 8765 |
| `tools/wasmsmoke/` | Desktop ↔ WASM parity test runner — `go run ./tools/wasmsmoke` |

---

## See Also

- [KLEX_LANGUAGE.MD](KLEX_LANGUAGE.MD) — complete kLex/FROG language reference
- [BUILTINS.MD](BUILTINS.MD) — all builtin functions, including WASM availability notes
- [STDLIB.MD](STDLIB.MD) — standard library modules
- [BRIDGE_DEVELOPER_GUIDE.MD](BRIDGE_DEVELOPER_GUIDE.MD) — native bridge (desktop only)
- [LINUX_QUICKSTART.MD](LINUX_QUICKSTART.MD) — desktop setup on Linux
