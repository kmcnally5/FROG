# kLex Release Notes

---

## v0.3.37 — WASM Browser Port, Shared UI Toolkit, Playground CI/CD

> Week of 2026-05-25 → 2026-05-30

This release is the largest single-week drop in kLex history. The headline is a complete port of the kLex UI toolkit to the browser via WebAssembly and Canvas2D — every widget that runs on the desktop OpenGL backend now runs identically in a browser tab, with no JavaScript written by the user. On top of that: a rebuilt playground, GitHub Actions deployment, self-hosting infrastructure, and a comprehensive doc sweep.

---

### WASM — Browser UI Toolkit (Canvas2D)

kLex now ships a **dual-target UI renderer**. The same FROG script that opens a native OpenGL window on the desktop runs pixel-for-pixel in the browser via Canvas2D. 35+ widgets are fully shared across both targets through a thin renderer interface (11 primitives). No widget code was duplicated.

**Widgets ported to browser (all shared with desktop):**

| Category | Widgets |
|---|---|
| Core | `button`, `label`, `checkbox`, `toggle`, `radio` |
| Input | `textInput`, `textArea`, `slider`, `numericStepper`, `getTypedChars` |
| Selection | `list`, `listMulti`, `dropdown`, `tabs` |
| Layout | `scrollArea`, `splitter`, `accordion`, layout cursors (`uiBeginRow`/`uiBeginCol` family) |
| Data | `table`, `treeView`, `sparkline`, `lineChart`, `barChart`, `pieChart` |
| Overlay | `tooltip`, `toast`, `contextMenu`, `modal` |
| Display | `progressBar`, `colorPicker` |
| Clip/State | `pushClip`/`popClip`, `pushDisabled`/`popDisabled` |
| Theme | `makeTheme`, `uiTheme`, `setTheme`, `lineHeight` |

**Canvas2D renderer primitives:** `fillRoundedRect`, `strokeRoundedRect`, `drawText`, `textWidth`, `lineHeight`, `drawLine`, `fillPolygon`, `fillArc`, `drawImage`, `pushClip`, `popClip`.

**Text editing** — both `textInput` and `textArea` support full cursor navigation, multi-line editing, click-to-place, shift+click selection, word-jump (Ctrl/Cmd+Arrow), Home/End, clipboard (Cmd/Ctrl+C/X/V), undo/redo (Cmd/Ctrl+Z/Y, 100-step per-widget stack), and auto-scroll to track the cursor.

**Theming** — 14-slot RGBA palette with four preset themes: `"nebula"` (default), `"light"`, `"dark"`, `"highContrast"`. Call `setTheme(name)` for one-liner switching or `uiTheme(palette)` for fine-grained control.

**DPR scaling** — all Canvas2D drawing respects the device pixel ratio; Retina/HiDPI displays render crisply.

**Font rendering fix** — Safari returns a negative `alphabeticBaseline` (spec-correct); Chrome returns positive. `textTopInset` now calls `math.Abs()` so vertical text centering is accurate on both browsers.

---

### WASM — New Builtins

Four new builtins available on both desktop and browser:

| Builtin | Signature | Description |
|---|---|---|
| `pushDisabled` | `pushDisabled(disabled: bool)` | Push a disabled-state frame; all widgets between push/pop render at half opacity and ignore hover/click |
| `popDisabled` | `popDisabled()` | Pop the top disabled frame |
| `setTheme` | `setTheme(name: string)` | Install a preset theme by name |
| `lineHeight` | `lineHeight([scale: float]) → int` | Pixel height of one line of widget text; use for vertical centering: `y + (h - lineHeight()) / 2` |

**`progressBar` signature corrected** — now takes 7 arguments: `progressBar(x, y, w, h, value, min, max)`. The `min` parameter was missing from the LSP, language docs, and grammar docs — all four have been updated. Existing call sites were already passing `min` correctly.

---

### WASM — Worker Bridge

JS Web Workers are now a first-class bridge transport in the browser. Any JS library can be called from kLex FROG code via the same `bridgeOpen` / `bridgeCall` / `bridgeClose` API used by subprocess bridges on the desktop:

```frog
let bridge, err = bridgeOpen({"kind": "worker", "script": "my_worker.js"})
let result, err = bridgeCall(bridge, "compute", [data])
bridgeClose(bridge)
```

The worker-side helper lives at `stdlib/worker/klex_bridge_worker.js`. It implements the full kLex bridge protocol (hello handshake, schema introspection, streaming, cancellation, backpressure) in pure JS with no Node dependency.

---

### WASM — OPFS (Origin Private File System)

The `opfs://` URL scheme gives kLex scripts persistent, sandboxed storage in the browser. Standard `fs.*` calls route transparently to OPFS when the path starts with `opfs://`:

```frog
import "stdlib/fs.lex" as fs
let data, err = fs.read("opfs://user-prefs.json")
fs.write("opfs://cache.bin", bytes)
```

No server round-trips. No user-visible file picker. Storage survives page reloads and browser restarts.

---

### Playground — Rebuilt and Deployed via CI/CD

`examples/playground/` is now the single canonical playground. `docs/playground/` has been retired.

**GitHub Actions** — `.github/workflows/deploy-playground.yml` builds `klex.wasm` fresh and deploys to GitHub Pages on every push to `main`. No binary is ever committed to the repo.

**Live URL:** `https://kmcnally5.github.io/FROG/`

The playground is a full kLex IDE built entirely in FROG on Canvas2D: code editor, tab navigation, live execution via `runScript`, output panel, and documentation links.

---

### WASM Infrastructure — kLex Self-Hosts

Python (`http.server`) has been eliminated from every WASM serve script. kLex now hosts all its own WASM applications.

**Canonical asset locations** — nothing is copied at runtime:

| Asset | Path |
|---|---|
| Go WASM runtime shim | `stdlib/wasm/wasm_exec.js` |
| Compiled kLex binary | `bin/klex.wasm` |
| Worker bridge helper | `stdlib/worker/klex_bridge_worker.js` |

**`stdlib/wasm/serve_base.lex`** — new shared module providing MIME detection, `serveFile`, and `logged` helpers. All WASM hosts import it. Each `serve.lex` uses `_scriptDir()` to compute the repo root and routes shared assets from their canonical paths with no copies.

**Test servers** — `tests/wasm_graphics/`, `tests/wasm_ui/`, and `tests/wasm_worker/` all now have proper `serve.lex` files and consistent `serve.sh` wrappers. Ports 8765 (worker), 8766 (graphics), 8767 (UI widgets), 8768 (playground).

---

### Build & Tooling

- **`tools/stdlibgen/`** — walks `stdlib/` and embeds every `.lex` file into `cmd/wasm/embeddedstdlib_gen.go` so `import "stdlib/..."` works in the browser without a filesystem.
- **`tools/fsdispatchgen/`** — generates the filesystem builtin dispatch table (`eval/builtins_fs_dispatch_gen.go`), handling `file://` and `opfs://` scheme routing.
- **`tools/wasmsmoke`** — desktop ↔ WASM parity test runner (`go run ./tools/wasmsmoke`); diffs output byte-for-byte. Baseline: 59/90 pass; 31 known platform-limitation failures.
- **`bin/wasmaudit`**, **`bin/buildtagaudit`** — new audit tools for catching WASM build tag drift before it reaches the binary.
- Tool sources (`doclinks`, `hookaudit`, `syncdocs`) moved from public `tools/` to private `stuff/tools/`. `tools/` now contains only the five committed public tools: `fsdispatchgen`, `klexfmt`, `kpkg`, `stdlibgen`, `tapetool`.

---

### LSP & Editor

- 6 builtins added to frogLSP: `lineHeight`, `pushDisabled`, `popDisabled`, `setTheme`, `openURL`, `runScript` — with full signatures and documentation extracted from Go source comments.
- VSCode syntax regex updated from 516 → 522 builtins.
- `progressBar` arity corrected in the LSP (6 → 7 args).
- syncdocs audit: 0 stale LSP entries, 0 VSCode syntax gaps.

---

### Documentation

- **`docs/WASM.MD`** — new complete WASM guide: architecture, JavaScript API, WASM-specific builtins, platform limitations, parity testing, self-hosting infrastructure.
- **`docs/BUILTINS.MD`** — regenerated (606 builtins, 49 categories).
- **`docs/KLEX_LANGUAGE.MD`** + **`docs/KLEX_GRAMMAR.MD`** — new entries for `pushDisabled`, `popDisabled`, `setTheme`, `lineHeight`; `progressBar` corrected in both.
- **`docs/STDLIB.MD`** — confirmed current (60 modules, 645 functions).
- **`docs/BRIDGE_API_DESIGN.MD`** — new bridge API design reference.
- 12 broken cross-document links fixed.
- **README.md** — playground URL updated to `https://kmcnally5.github.io/FROG/`.

---

### Bug Fixes

| Area | Fix |
|---|---|
| `progressBar` | Missing `min` parameter — Go source accepted 7 args; LSP, language doc, and grammar doc all said 6 |
| Canvas2D text | `textTopInset` now uses `math.Abs()` on `alphabeticBaseline` — fixes vertical centering on Safari |
| `serve.sh` (all) | All four WASM serve scripts were copying `wasm_exec.js` from `docs/playground/` (retired) — now use `stdlib/wasm/wasm_exec.js` |
| `tests/wasm_worker/` | Committed `klex.wasm` binary removed from the repo |
| `tests/wasm_worker/` | Duplicate `klex_bridge_worker.js` removed — canonical: `stdlib/worker/klex_bridge_worker.js` |

---

### Breaking Changes

None. All existing desktop kLex scripts run unchanged. WASM scripts written against the REPL (`klex_eval`) continue to work. The `progressBar` `min` parameter was always required by the Go implementation — callers already passed it.

---

## v0.3.36 Release Notes

## Overview

v0.3.36 is a **bridge, IDE, and ecosystem** release. The bridge system gains **streaming**, **worker pools**, and **per-bridge observability metrics**. The LSP/IDE story expands substantially with **code actions**, **document formatting**, **signature help**, and a **cross-file parsed-AST cache**. The standard library gains a **Node.js bridge helper** mirroring Python's, an **ultra-fast Go-backed JSONL helper** for million-line catalogs, and a new **stream_fusion** module. **tadPole** — the multi-provider AI image-generation + agentic-chat desktop app — enters the tree for the first time, bringing the project's flagship demo into the release. **SecretHunter** (the showcase example) gets a significant rewrite to use the new bridge pool primitives.

This release pushes kLex's positioning further: a language that ships with everything you need to drive AI tooling and live-extend applications, not just a runtime.

---

## Headline features

### Bridge Phase 4 — streaming, pools, metrics

Builds on v0.3.35's schema phase. Three new pillars and a Node.js companion to the existing Python helper.

**`bridgeStream(bridge, fn, args, timeout?)` — streaming results.**

Bridges can now yield multiple values per call (generators, scan results, log lines). Subprocess emits `{"id": N, "stream": item}` per yielded value, then `{"id": N, "stream_end": true}`. The kLex consumer receives them through a channel; breaking the `for-in` cancels the stream cleanly (subprocess gets `{"cancel": id}` and stops producing). Optional idle and total timeouts via the 4th argument hash:

```frog
ch, err = bridgeStream(bridge, "scan_batch_stream", [files], {"idle": 30})
for item in ch {
    if isError(item) && item.code == "BRIDGE_TIMEOUT" { break }
    if item.kind == "finding" { ... }
}
```

**`bridgePool(n, cmd, args)` — round-robin worker pools.**

N pre-started bridges with a round-robin counter. Routing skips dead members (failed init or tainted). SecretHunter uses this to fan 16 Python workers across YARA + entropy scanning of a whole repo. New primitives:

- `bridgePoolCall(pool, fn, args, timeout?)` — non-stream call on next alive bridge
- `bridgePoolStream(pool, fn, args, timeout?)` — streaming variant
- `bridgePoolHealth(pool) -> {size, alive, dead}` — snapshot for diagnostic panels
- `bridgePoolStderr(pool) -> array` — concatenated stderr ring buffer, prefixed by member index
- `bridgePoolClose(pool)` — close every member (idempotent)

Members that fail init or get tainted mid-session are latched dead. The picker routes around them automatically — callers never see `BRIDGE_TAINTED` from a tainted pool member.

**`bridgeMetrics(bridge) -> hash` — per-bridge observability.**

Single mutex acquisition + a sort over a 256-sample latency buffer = cheap enough for a dashboard polling every second. Returns:

```frog
{
  "calls_total":     230, "calls_inflight": 1, "calls_failed": 2,
  "streams_total":    18, "streams_active": 0, "streams_failed": 0,
  "bytes_sent":  1240488, "bytes_received": 2810451,
  "errors_by_code": {"BRIDGE_TIMEOUT": 4},
  "per_function": {
    "scan_file": {"count": 220, "errors": 1, "p50_ms": 12.0, "p95_ms": 84.0, "p99_ms": 230.0}
  }
}
```

**Node.js bridge helper.** `stdlib/node/klex_bridge.js` ships alongside the existing Python helper, mirroring the API:

```js
const { handler, streamHandler, serve } = require('./klex_bridge');

handler({args: [['n', 'int']], returns: 'int'}, function double(n) { return n * 2; });
streamHandler({args: [['files', 'array']], yields: 'hash'}, function* scan(files) {
    for (const f of files) yield { path: f, ok: true };
});
serve();
```

Same wire protocol as Python — kLex doesn't care which side it talks to. Schema validation, streaming, cancellation, and the protocol-negotiation handshake all work identically.

**Stream cancellation hygiene.** Streams that timeout reap cleanly: cancel sent to subprocess, dispatcher state cleared, final timeout error delivered as the channel's last item before close. No leaked goroutines.

### LSP/IDE — production-grade developer experience

The LSP (`snowball/froglsp/`) gains substantial capability across hover, completion, diagnostics, and editor integration. **~+2,100 lines** of LSP code total, plus **+149 lines** of new VS Code snippets.

**New server capabilities:**

- **Code actions (quickfix family)** — the lint pass surfaces diagnostics with one-click fixes
- **Document formatting + range formatting** — Format Document and Format Selection both work
- **Signature help** — pop the param-help bubble on `(` and re-trigger on `,` so the active-parameter highlight tracks the cursor
- **Document symbols with full-body ranges** — VS Code's breadcrumb stays anchored to the function as the cursor moves through its body
- **Cross-file hover** — hovering `module.symbol` reads + parses the imported `.lex` file. Backed by new `parsed_cache.go`: mtime-keyed LRU (cap 64) of parsed ASTs, so hovering inside dot-chains doesn't re-parse the stdlib on every keystroke.

**Completion improvements:**

- User symbols sort above builtins (`SortText: "1_"` prefix)
- Function completions insert a snippet with placeholders that Tab-jump through each parameter
- Markdown-rendered hover documentation in the completion popup
- Const/var/module symbols carry their inferred type in the `Detail` field

**Hover improvements:**

- Builtin hovers render full markdown documentation (`snowball/froglsp/builtins.go` +1,260 lines of builtin descriptions)
- User-symbol hovers extract the contiguous `//` comment block above the definition and join it with the rendered signature

**VS Code extension:**

- 149 new snippets across the kLex language surface (`klex.json`)
- Extension JS updated for the new server capabilities

### tadPole — multi-provider AI image generation and agentic chat (NEW, IN-TREE)

**Tadpole** — the project's flagship demo app — enters the tree at `snowball/tadPole/`. **5,662 lines** total: 4,880 of kLex (`tadPole.lex`), 543 of Python (`ai_image_bridge.py`), plus README, fonts, and logo. Cross-platform (macOS, Linux, Windows).

**Image generation across five providers in one UI:**
- Local Stable Diffusion (AUTOMATIC1111 / Forge / reForge) over pure HTTP — no Python bridge needed
- AI Horde (free, anonymous works)
- Hugging Face (free tier with token)
- OpenAI DALL·E (paid, best prompt adherence)

**Agentic chat** backed by Claude (Anthropic API) or Ollama (qwen3 / llama3.1 etc. for tool use). Built-in tools the chat agent can invoke: `read_file`, `list_dir`, `http`, `write_file`, `shell`, `launch` (OS-aware app launcher).

**Optional MCP integration as a CLIENT** — drops in `frogMcp` so the chat agent gets first-class kLex-language navigation: `klex_search`, `klex_describe_symbol`, `klex_list_builtins`, etc.

**Self-exposes as an MCP SERVER on `:7778`** — Tadpole runs `_mcpServeHTTP` and advertises **11 tools** that any MCP client (Claude Code, Claude Desktop, the MCP Inspector, scripts) can drive:

| Category | Tools | Notes |
|---|---|---|
| Observe | `current_state`, `list_history`, `list_providers`, `tape_query` | `tape_query` whitelists `/tmp/*.lextape` and `~/.tadpole/*.lextape` |
| Chat | `chat`, `transform` | `transform` is a bounded one-shot prose transform via local ollama (stateless) |
| Image | `generate_image` | fires the active provider's pipeline |
| UI | `set_right_tab`, `set_theme`, `set_adjust`, `reset_adjust` | drive the UI from outside — switch tabs, change theme, set sliders |

**Prompt caching** wired through Anthropic's `cache_control: ephemeral` markers — system prompt + tool definitions cached for ~90% input-token discount on repeated turns within 5 minutes.

**Adjust panel** with live preview: exposure, contrast, saturation, hue shift, vignette, sepia, brightness, gamma + invert/desaturate toggles. Built on the `mtl_fx` Metal-backed filter chain (sub-ms per filter on macOS; 5–20 ms CPU fallback on Linux/Windows).

### SecretHunter — bridge pool rewrite (showcase)

The SecretHunter example (`tests/examples/SecretHunter/`) gets a significant rewrite to use the new bridge pool primitives. Spawns 16 Python workers for parallel YARA + entropy scanning of a target tree; streams findings back via `bridgePoolStream`; per-worker stderr surfaced via `bridgePoolStderr` to a diagnostic panel. The showcase app for the bridge pool feature.

Cumulative change: roughly +1,100 / −1,119 lines across `secretHunterLib.lex` and `secretHunterUI.lex` plus the YARA Python bridge.

---

## Standard library — expansions

### Performance — Go-backed JSONL fast path

New `eval/builtins_jsonl.go` with `_replaySeenFile(path)` — bypasses the kLex JSON parser entirely for streaming JSONL processing. Used by frogLight's cataloger on `catalog.jsonl` files of millions of lines. **~30 s → ~1-2 s** for 4M lines. Returns a `{path: mtime}` hash — exactly the shape `isFileFresh` needs.

The kLex stdlib `json.parse` is still the right tool for general JSON; this is the surgical optimisation for the JSONL-as-state-file pattern.

### `stdlib/parallel.lex` — pmap, parallel_reduce, chunked

High-performance parallel data processing on async Tasks. `pmap`, `parallel_reduce` with custom merge strategies, early termination. Built atop the snapshot model from v0.3.35.

```frog
let result, err = s.collect(p.pmap([1..1000], fn(x) { x * 2 }, 4))
let sum, err = p.parallel_reduce([1..1000], fn(a, x) { a + x }, fn(a, b) { a + b }, 4, 0)
```

### `stdlib/retry.lex` — exponential backoff + circuit breaker patterns

(+236 lines) — production-ready retry strategies with jitter and breaker state.

### `stdlib/stream_fusion.lex` — NEW

Stream-of-streams composition for pipeline-style data processing.

### `stdlib/cache.lex`, `stdlib/event.lex`, `stdlib/stream.lex`

Significant expansions to the cache, event bus, and stream modules. (Combined ~+300 lines.)

### `stdlib/json.lex` — parser polish (+325)

Builds on the v0.3.35 string rune-cache fix; further parser ergonomics and error reporting improvements.

### Deletions

- `stdlib/csvfrog.lex` — consolidated into `stdlib/csv.lex`

---

## Python bridge helper — robustness pass

`stdlib/python/klex_bridge.py` gains **+398 lines**. Highlights:
- `@stream_handler` decorator for streaming subprocess functions
- Auto-registration of `__hello__` and `__schema__` so kLex can introspect the bridge with no per-script wiring
- Background reader thread owns stdin; main thread reads classified lines from a queue
- Errors at any point (parse, validation, exception inside the handler) surface as structured `{"id": N, "error": "...", "error_type": "...", "traceback": "..."}` responses

---

## Documentation overhaul

The language documentation got a significant consolidation pass.

**Deleted:**
- `docs/KLEX_LANGUAGE.TXT` (−3,832 lines) — the old monolithic language reference; superseded
- `docs/LLMs_AND_FROG.md` (−307) — folded into broader docs
- `docs/ASYNC_QUICK_REFERENCE.md` (−177) — folded into ASYNC_BEST_PRACTICES.md

**Substantially expanded:**
- `docs/ASYNC_BEST_PRACTICES.md` — **+1,012 lines**. Now the single source of truth for async/await/channels: snapshot model, primitives, patterns (pure async, worker pools, pipelines, cancel, timeout, errors), anti-patterns, performance, FAQ.
- `docs/KLEX_GRAMMAR.MD` — **+517 lines**. Formal grammar reference, kept in sync with the parser.
- `docs/BRIDGE_DEVELOPER_GUIDE.md` — **+206 lines**. Bridge protocol, schema declaration, streaming, pools, metrics, Python and Node helpers, debugging.
- `docs/FROG_GRAPHICS_GUIDE.MD` — minor updates.

---

## Tests

Almost every `tests/unit/*Test.lex` file was touched — primarily test-framework format updates as part of `stdlib/test.lex`'s polish (+21 lines), plus new coverage for `bridgeStream`, `bridgePool`, `bridgeMetrics`, and updated bridge examples in `tests/examples/bridge/`.

---

## Files changed

(The biggest movers — see git diff for the complete list.)

### Core runtime (`eval/`, `lexer/`, `parser/`, `ast/`)

```
eval/builtins_bridge.go        +1,310
eval/eval.go                     +911
eval/builtins_ui.go              +783
eval/object.go                   +621
lexer/lexer.go                   +456
eval/builtins_graphics.go        +302
parser/parser.go                 +287
eval/env.go                      +159
eval/builtins_strings.go         +123
eval/builtins_process.go         +102
eval/builtins_os.go               +89
eval/builtins_concurrent_hash.go  +65
eval/typecheck.go                 +65
eval/builtins_fs.go               +62
eval/builtins_http.go             +56
eval/builtins_parallel.go         +55
ast/ast.go                        +23
NEW: eval/builtins_bridge_pool.go
NEW: eval/builtins_jsonl.go
```

### LSP / IDE (`snowball/froglsp/`)

```
snowball/froglsp/builtins.go    +1,260
snowball/froglsp/hover.go         +315
snowball/froglsp/completion.go    +231
snowball/froglsp/diagnostics.go   +203
snowball/froglsp/analysis.go      +171
snowball/froglsp/server.go        +148
snowball/froglsp/protocol.go      +148
NEW: snowball/froglsp/parsed_cache.go
NEW: snowball/froglsp/documentSymbol.go
```

### Standard library (`stdlib/`)

```
stdlib/json.lex                   +325
stdlib/retry.lex                  +236
stdlib/ui.lex                     +198
stdlib/parallel.lex               +140
stdlib/cache.lex                  +114
stdlib/stream.lex                 +105
NEW: stdlib/stream_fusion.lex      +27
NEW: stdlib/node/klex_bridge.js
DEL: stdlib/csvfrog.lex          (−475)
+ event/functional/graph/merkle/rest/ui_themes polish
```

### Python bridge (`stdlib/python/`)

```
stdlib/python/klex_bridge.py      +398
```

### tadPole — NEW IN TREE

```
snowball/tadPole/tadPole.lex            +4,880
snowball/tadPole/ai_image_bridge.py      +543
snowball/tadPole/README.md               +239
snowball/tadPole/fonts/*.ttf            (binary)
snowball/tadPole/tadpole_logo.png       (binary)
```

### Documentation (`docs/`)

```
docs/ASYNC_BEST_PRACTICES.md    +1,012
docs/KLEX_GRAMMAR.MD              +517
docs/BRIDGE_DEVELOPER_GUIDE.md    +206
DEL: docs/KLEX_LANGUAGE.TXT     (−3,832)
DEL: docs/LLMs_AND_FROG.md        (−307)
DEL: docs/ASYNC_QUICK_REFERENCE.md (−177)
```

### Examples (`tests/examples/`)

```
SecretHunter/secretHunterLib.lex    ~+1,100 / −1,119
SecretHunter/secretHunterUI.lex     ~+774
SecretHunter/secretHunterTest.lex      +52
SecretHunter/yaraTest.lex              +22
SecretHunter/yara_bridge.py           +245
SecretHunter/github_bridge.py         +117
bridge/{schemaTest, robustnessTest, phase2Test, …}  various
```

### Editor extensions

```
editors/vscode_froglsp/klex-language/snippets/klex.json   +149
editors/vscode_froglsp/klex-language/extension.js          +11
editors/vscode_froglsp/klex-language/syntaxes/klex.tmLanguage.json  +2
```

---

## Migration notes

- **`bridgeCall` signature unchanged** — no breaking change for existing bridge users. The new primitives (`bridgeStream`, `bridgePool*`, `bridgeMetrics`) are additive.
- **`csvfrog.lex` users:** migrate imports to `csv.lex`. Same surface, less to import.
- **Old language reference removed:** if you were linking to `docs/KLEX_LANGUAGE.TXT`, switch to `docs/KLEX_GRAMMAR.MD` + `docs/ASYNC_BEST_PRACTICES.md`.

---

## Verified

- All tests pass on macOS arm64 (M4) — full master test suite
- Bridge pool stress-tested via SecretHunter on multi-thousand-file repos
- LSP cross-file hover validated against the kLex stdlib
- tadPole verified end-to-end: image gen across all five providers, MCP server reachable on `:7778`, slider chain rendering on macOS Metal path
- Windows VM smoke: image gen + bridge pool + LSP cross-file hover all functional
- Linux native build (Debian/Ubuntu fonts auto-detected)
