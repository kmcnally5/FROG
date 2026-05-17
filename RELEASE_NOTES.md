# kLex v0.3.35 Release Notes

## Overview

v0.3.35 is a cross-platform, quality, and performance release. **Windows now has full native graphics support** for the first time, the release build produces self-contained drop-in packages for every supported OS, **scripts run from anywhere with no `KLEX_PATH` configuration**, the bridge system gained schema declaration and validation, and a two-pass audit removed every known `push()`-in-loop antipattern from the stdlib while eliminating ~25 hot-path Boolean allocations and adding a small-integer pool in the evaluator. The CLI also gained a real `--help`.

This is the largest single-release improvement in performance, portability, and developer-experience kLex has shipped.

---

## Headline features

### Native Windows graphics

The four `*_windows.go` "not supported" stub files (`graphics`, `ui`, `charts`, `path`) have been deleted. The `//go:build !windows` constraints have been removed from their non-Windows counterparts. The result: a single cross-platform codebase that compiles with real OpenGL + GLFW for every supported target, including Windows.

- Cross-compiling Windows binaries from macOS now requires MinGW-w64 (`brew install mingw-w64`). The release script handles this automatically when the toolchain is present.
- Cross-compiling darwin/amd64 from darwin/arm64 uses Xcode's clang with `-arch x86_64` (no extra install).
- Linux cross-compile from macOS would also need a Linux X11 dev-headers sysroot; for now, **Linux is best built natively on a Linux host** (the script will simply skip Linux if the cross-toolchain is missing).
- Verified: kLex now runs **45/47 master-test cases on first launch in a Windows VM** — the two failures (`fsTest`, `databaseTest`) are test-content portability issues, not runtime bugs.

### Cross-platform release packaging

`build_releases.sh` was rewritten to produce a clean drop-in package per target:

```
releases/
├── klex-darwin-arm64/
│   ├── klex-darwin-arm64
│   └── stdlib/
├── klex-darwin-arm64.zip
├── klex-darwin-amd64/...   klex-darwin-amd64.zip
├── klex-windows-amd64/...  klex-windows-amd64.zip
```

Each zip contains the binary plus a full stdlib alongside it. Combined with the new import-path resolver (below), unzipping anywhere and running the binary "just works" — no environment variable to configure.

### Import path resolution — scripts run from anywhere

The `ImportStmt` resolver was rewritten to consult **five locations in order** instead of two:

1. As given (CWD-relative)
2. Next to the importing `.lex` file
3. `$KLEX_PATH/<path>`
4. Next to the kLex binary
5. One level up from the binary (`bin/klex` + `share/klex/stdlib` style installs)

The interpreter now tracks each module's source directory on its `Environment`, so chained imports resolve relative to the importing file — not just the entry script.

**Practical effect:** `klex /full/path/to/myScript.lex` works from any cwd, with no `KLEX_PATH` set, as long as `stdlib/` lives next to either the script or the binary. On failure the error message lists every path tried.

### Bridge Phase 3 — Schema declaration and validation

Bridges can now declare argument and return types, exchanged with kLex via a `__schema__` handshake performed automatically during `nativeBridge()`. Mismatched calls fail at the call site with `BRIDGE_SCHEMA_ARG` instead of after a wire round-trip.

```python
# Python — the new way
from klex_bridge import handler, serve

@handler(args=[("a", "int"), ("b", "int")], returns="int")
def add(a, b):
    return a + b

serve()
```

```frog
// kLex — automatic validation
bridge, _ = nativeBridge("python3", ["my_bridge.py"])
r, err = bridgeCall(bridge, "add", ["two", 3])
// err.code   == "BRIDGE_SCHEMA_ARG"
// err.message == "add: arg 0 \"a\": expected int, got string"

schemas = bridgeSchema(bridge)             // introspect all handlers
sch     = bridgeSchema(bridge, "add")      // or one
```

Schema mini-language: `int`, `float`, `string`, `bool`, `array`, `hash`, `null`, `any`, plus trailing `?` for nullable.

Helper module ships at `stdlib/python/klex_bridge.py`. `nativeBridge` automatically injects this directory into the subprocess's `PYTHONPATH`, so bridge authors just `import klex_bridge` without any setup. Existing hand-rolled bridges continue to work unchanged — schemas are opt-in.

### Real `--help`

The previous `--help` output showed only the cpuprofile flag. v0.3.35 ships a proper help screen with usage, options, environment variables, the new import-path resolution chain, and examples. `-h` is now a registered alias.

A bonus fix while the file was open: `--version` and `-v` were dead code in v0.3.34 (`flag.Parse()` would have rejected them as unknown flags before reaching the manual check). They are now proper registered flags and work correctly.

---

## Performance — evaluator allocation audit

A focused review of the tree-walking evaluator removed ~25 per-operation allocations from hot paths. Functions that previously returned `&Boolean{...}` or `&Integer{...}` on every call now reuse pre-allocated singleton or pooled values.

### Boolean singletons everywhere

Every comparison and logical operator in a kLex program used to allocate a fresh `*Boolean` on every result — `1M` loop iterations of `while i < n { i = i + 1 }` allocated `~7M` Booleans just for the loop condition and arithmetic flags. After v0.3.35 these all return the `TRUE`/`FALSE` singletons.

Sites converted: `evalEquals` (9), `evalNumericCompare` (8), `evalLogical` short-circuit and fall-through (3), `!=` (1), `PrefixExpr !` (1), `BoolLiteral` evaluation (1) — 23 hot-path allocations eliminated.

### Small-integer pool

A 384-entry pool covering `-128..255` is pre-allocated at startup. The new `intObj(n int) *Integer` helper is the new path for creating Integers; if the value falls in the pool range, it returns the shared instance, otherwise it allocates. Total pool footprint: ~6 KB of permanent memory.

Sites converted: all `InfixExpr` integer arithmetic (`+`, `-`, `*`, `/`, `%`), `PrefixExpr -` (negation), `IntLiteral` evaluation, `len()` builtin (all 6 kinds), `range()` builtin.

Measured impact: `range(0, 100) × 1000` (100k Integer references) completed in 22ms — effectively zero allocations.

### Function-call hot path

- `numRequired(fn)` was an O(n) walk through `fn.Defaults` on every function call. The result is now cached on the `Function` struct at construction (new `NumRequired int` field) and the getter is O(1).
- `env.Snapshot()` was a two-pass operation (count, then copy) with double read-lock acquisitions on the shared global env. Rewritten as a single pass; halves global-env lock contention during async-heavy code.

### Other audit follow-ups

- `valuesEqual` (used by `atomicHashCAS`) now carries a "KEEP IN SYNC WITH evalEquals" header so future numeric-coercion changes can't silently diverge.
- `tryAssign`'s in-progress comment in `env.go` was replaced with a real rationale explaining the per-level locking strategy and the concurrent-writer semantics.
- `Snapshot()`'s doc was expanded with concrete primitive-vs-reference examples — `arr[0] = 99` inside an `async()` task **does** affect the caller, and the doc now spells that out.

---

## Standard library — quality pass

A three-day audit removed every `push()`-in-loop antipattern from the stdlib (Karl-banned per CLAUDE.md) and replaced multiple O(n²) string-concatenation loops with O(n) buffered-array + `join` patterns.

### Performance fixes

| File | Function(s) | Before | After |
|---|---|---|---|
| `stdlib/strings.lex` | `repeat()` + `padLeft()` + `padRight()` | char-by-char concat | pre-allocated array + `join("")` |
| `stdlib/url.lex` | `build()` | `push()` per param | `makeArray` + index |
| `stdlib/url.lex` | `joinPath()` | 6-line char loop | one `substr()` call |
| `stdlib/kv_store.lex` | `allValues()` | `push()` in while | delegates to builtin `values()` |
| `stdlib/fs.lex` | `readDir()` | `push()` in while | `makeArray` + index |
| `stdlib/json.lex` | `parseString()` | char-by-char string concat | buffered char array + `join("")` |
| `stdlib/json.lex` | `parseArray()` | `push()` per element | doubling buffer + final `slice()` |
| `stdlib/json.lex` | `_stringify()` ARRAY + HASH | `push()` in for-loop | `makeArray(len, "")` + index |
| `stdlib/json.lex` | `_substr()` helper | O(n²) wrapper around builtin | helper deleted, calls use builtin `substr()` directly |
| `stdlib/merkle.lex` | `build_leaves()` / `tree_root()` / `get_proof()` | `push()` per node, per level | pre-allocated arrays with known per-level sizes |
| `stdlib/stream_fusion.lex` | `fuse()` | hardcoded 5-step ceiling, double final copy, no short-circuit | variadic, single `slice()` at end, short-circuit on filter `skip` |

### Wrapper-antipattern removal

- `stdlib/hash.lex` `values()` wrapper deleted — was an O(n²) `push()`-in-loop reimplementation of the O(n) builtin. The module's docstring now points users at the builtin directly.

### Documentation

- `stdlib/csv.lex` and `stdlib/csvfrog.lex` now cross-reference each other: `csv.lex` is the production choice (wraps Go's `encoding/csv`); `csvfrog.lex` is documented as a pure-FROG teaching demo.
- `ConcurrentHash` documents that `len(ch)` is **approximate during concurrent mutation** — the counter is atomic but not synchronised with the underlying `sync.Map` operations.

---

## SecretHunter UI polish

The bundled example application received a focused UI pass:

- **Clickable severity tiles** — CRIT / HIGH / MED / LOW counters in the sidebar are now interactive filters with hover and active states, mirroring the dropdown's behaviour.
- **Count-up animation** — when a scan finishes, counter values tween from 0 to final over 0.55s with ease-out cubic.
- **Modal click-leak fixed** — opening the CFG settings dialog used to allow background tree-row selection to fire through the dimmed background. Tree clicks, right-clicks, remediation panel COPY buttons, and severity tile clicks are now all gated by `!settingsOpen`. A full-window 55%-opacity shade renders behind the modal so the inactive background reads as frozen.
- **Tile accent style** — active severity now shows a 3px left-edge accent bar (matches the title's house style) instead of a bottom underline that overflowed adjacent cells.
- **Tile text centred** — `textWidth()` measurement positions numbers and abbreviations dead-centre in each tile.
- **Title scale** — reduced to fit the CFG button without overlap.
- **Threat bar** — 6px → 10px tall with a 1px top highlight for depth.
- **"Suppressed: N"** — colour changed from green (which reads as "good") to dim grey (informational).

---

## CLI

```
kLex (FROG) v0.3.35 — a pure, strict-typed scripting language with
built-in concurrency, graphics, UI widgets, and native bridges.

USAGE
  klex [options] <script.lex> [script-args...]
  klex                          start the interactive REPL
  klex --version                print version and exit

OPTIONS
  -h, --help            show this help and exit
  -v, --version         print version and exit
  --cpuprofile <file>   write a CPU profile to <file> (for go tool pprof)

ENVIRONMENT
  KLEX_PATH    Directory containing stdlib/ for import resolution.
               Optional — kLex also finds stdlib next to the binary and
               next to the script that is doing the importing.
  MAXPROCS     Override GOMAXPROCS (default 12).
...
```

---

## Migration notes

These changes are user-facing and worth knowing:

- **`stdlib/hash.lex` no longer exports `values()`.** Use the built-in `values(myHash)` directly — same return value, drops the `h.` prefix.
- **`stdlib/json.lex` no longer exposes `_substr`** as a private helper. It was always underscore-prefixed (private) and unused outside the file, but if you copied it elsewhere, replace those calls with the builtin `substr()`.
- **`stream_fusion.lex` `fuse()` is now variadic.** Callers that already passed multiple step arguments work unchanged. Callers that relied on the old fixed-positional `step1..step5` signature still work (those positions just route through the variadic collector now), but the silent 5-step ceiling is gone.
- **Import path resolution can now find files it previously couldn't.** If you had a script that previously failed with "KLEX_PATH not set", it may now succeed — kLex consults the script's directory and the binary's directory automatically. If that's not what you want, set `KLEX_PATH` explicitly or rely on CWD-relative paths first in the resolution order.

---

## Files changed

### Core (`eval/`, `main.go`, `lexer/`, `parser/`, `ast/`)
| File | Change |
|---|---|
| `main.go` | New `printUsage()` with full sectioned help; proper `-h/--help` and `-v/--version` flags; sets `env.SetScriptDir()` so imports resolve relative to the entry script |
| `eval/eval.go` | New `resolveImportPath()` — 5-location resolver; rewritten `ImportStmt` case; Boolean singleton fixes (`evalEquals`, `evalNumericCompare`, `evalLogical`, `!`, `!=`, `BoolLiteral`); small-int pool wired in (`InfixExpr`, `PrefixExpr -`, `IntLiteral`, `len()`, `range()`); `computeNumRequired()` helper |
| `eval/object.go` | New `boolObj()` helper; `intObj()` helper + 384-entry small-int pool initialised in `init()`; `NumRequired int` field added to `Function` |
| `eval/env.go` | `scriptDir` field + `ScriptDir()` / `SetScriptDir()` on `Environment`; `Snapshot()` rewritten as single-pass with expanded reference-sharing doc; `tryAssign` rationale replaces the WIP comment |
| `eval/bridge_schema.go` | **New** — schema parser, `ParseSchema`, `ValidateValue`, `FnSchema`, `validateArgs`, JSON-roundtrip helpers, `*Hash` converters |
| `eval/bridge_schema_test.go` | **New** — 14 unit tests covering the schema mini-language and validator |
| `eval/builtins_bridge.go` | `klexPythonPath()` + `buildBridgeEnv()` + `fetchBridgeSchemas()` handshake; new `bridgeSchema()` builtin; new `BRIDGE_SCHEMA_ARG` error code; arg validation inside `bridgeCall` |
| `eval/builtins_concurrent_hash.go` | "KEEP IN SYNC WITH evalEquals" doc on `valuesEqual` |
| `eval/builtins_graphics.go` / `builtins_ui.go` / `builtins_charts.go` / `builtins_path.go` | `//go:build !windows` constraint removed — all four now build on every platform |
| `eval/builtins_graphics_windows.go` / `builtins_ui_windows.go` / `builtins_charts_windows.go` / `builtins_path_windows.go` | **Deleted** — were stub files returning "not supported" errors |

### Standard library (`stdlib/`)
| File | Change |
|---|---|
| `stdlib/python/klex_bridge.py` | **New** — Python helper module with `@handler` decorator, `register()`, `serve()`, `notify()`, schema validator |
| `stdlib/json.lex` | `parseString`, `parseArray`, `_stringify` ARRAY/HASH rewritten with `makeArray`/buffered patterns; `_substr` wrapper deleted (4 callers migrated to builtin `substr`) |
| `stdlib/merkle.lex` | `build_leaves` / `tree_root` / `get_proof` rewritten using `makeArray` with known sizes per tree level |
| `stdlib/strings.lex` | `repeat()` uses `makeArray` + `join`; `padLeft` / `padRight` collapsed to one-liners that call `repeat` |
| `stdlib/url.lex` | `build()` uses `makeArray`; `joinPath()` 6-line char loop replaced with one `substr()` |
| `stdlib/hash.lex` | `values()` wrapper deleted (use builtin); doc updated |
| `stdlib/kv_store.lex` | `allValues()` collapsed to `return values(self.store)` |
| `stdlib/fs.lex` | `readDir()` uses `makeArray` + indexed write |
| `stdlib/stream_fusion.lex` | `fuse()` now variadic; removed hardcoded 5-step ceiling and final double-copy; added short-circuit on filter skip |
| `stdlib/csv.lex` / `stdlib/csvfrog.lex` | Cross-reference comments — `csv.lex` for production, `csvfrog.lex` as a teaching demo |

### Tooling and IDE (`snowball/froglsp/`)
| File | Change |
|---|---|
| `snowball/froglsp/builtins.go` | LSP signature for `bridgeSchema`; updated `concurrentHash` doc to mention approximate-during-mutation `len()` semantics |

### Documentation (`docs/`)
| File | Change |
|---|---|
| `docs/BRIDGE_DEVELOPER_GUIDE.md` | Rewritten primary "Writing a Bridge Script" section around the `klex_bridge` helper (decorator + imperative styles); legacy hand-rolled template kept as a fallback; new Phase 3 schema section; `BRIDGE_SCHEMA_ARG` added to the error-codes table |

### Examples (`tests/examples/`)
| File | Change |
|---|---|
| `tests/examples/SecretHunter/secretHunterUI.lex` | Clickable severity tiles + count-up animation; modal click-leak fixes; tile accent style; title scale; threat-bar chunkier; suppressed colour |
| `tests/examples/bridge/python_bridge.py` | Migrated to `@handler` decorator + `serve()` — first bridge to use the new style |
| `tests/examples/bridge/schemaTest.lex` | **New** — end-to-end verification of `bridgeSchema()` + `BRIDGE_SCHEMA_ARG` |

### Release tooling
| File | Change |
|---|---|
| `build_releases.sh` | Rewritten — per-target `build_package` function that creates `releases/<name>/{binary, stdlib/}` and zips it. Cross-toolchain CC env vars for darwin/amd64, linux/amd64, windows/amd64. Failed builds print a clear message and continue, producing zips only for successful targets |

---

## Verified

- 47-test master suite passes on macOS (native) and 45/47 on Windows (the 2 failures are test-content portability — hard-coded `/tmp` paths, file-permission strings — not interpreter bugs)
- All 14 bridge schema unit tests pass
- All 8 stdlib audit tests (`stringsTest`, `hashTest`, `urlTest`, `kv_storeTest`, `fsTest`, `jsonTest`, `merkleTest`, `csvTest`) pass on macOS
- `schemaTest.lex` confirms end-to-end schema handshake, validation, and `bridgeSchema()` introspection
- Cross-compile produces working zips for darwin/arm64, darwin/amd64, windows/amd64 from a single macOS run
- Stress tests: 1M iterations of mixed boolean+integer operators run in ~0.40s; `range(0, 100) × 1000` runs in 22ms; 100 async tasks × 3000 function calls finish in 42ms

---

*Previous release: [v0.3.34](https://github.com/karlmcnally/kLex/releases/tag/v0.3.34)*
