# kLex — FROG Language Runtime

> **A high-performance interpreted language for native tooling, scanners, terminals, IDEs, and concurrent desktop utilities.**

kLex is the reference implementation of FROG — a runtime built for a specific class of program: parallel file scanners, native GUI applications, high-throughput pipelines, and concurrent desktop utilities.

It is not a general-purpose language. It is a runtime for people who want to build real tools fast.

---

<div align="center">
  <img src="./docs/images/frog_art1.png" alt="FROG" width="260">
</div>

---

## What FROG is built for

| Pillar | What it means |
|---|---|
| **Native tooling** | File scanners, credential hunters, pipeline processors |
| **Concurrent desktop utilities** | GUI apps that stay responsive under load |
| **High-throughput scripting** | Parallel workloads without a compiled toolchain |
| **Systems-style interpreted execution** | Explicit state, explicit channels, no hidden behaviour |
| **Immediate-mode GUI** | OpenGL window + SDF rendering baked into the runtime |
| **Channel-oriented concurrency** | Channels are a language primitive, not a library add-on |

---

## Secret Hunter — credential scanner with native GUI

A full parallel security scanner built entirely in FROG. Scans codebases and git history for leaked API keys, passwords, and tokens. Ships a live OpenGL interface with real-time progress, severity distribution, and filtering.

```bash
./klex examples/SecretHunter/secretHunterUI.lex
```

40 parallel workers. Native OpenGL GUI at 60fps during scan. Built in ~900 lines of FROG.

> A scripted scanner with a native GUI and parallel async channels — in an interpreted language.

---

## Try it Online

**[Launch the kLex REPL](https://kmcnally5.github.io/FROG/playground/)** — run kLex code directly in your browser, no installation required.

The REPL supports multi-line input (automatically detects when blocks are complete) and maintains session state — define variables and functions, then use them in subsequent lines.

---

## Three things that make it different

### 1 — Async and channels

```lex
let jobs    = channel(200)
let results = channel(200)

let worker = async(fn() {
    while true {
        let job, ok = recv(jobs)
        if ok == false { break }
        send(results, job + " processed")
    }
})

send(jobs, "file.txt")
let result, ok = recv(results)
println(result)
```

No executors, no event loops. Each `async()` spawns a real goroutine. Channels are typed, bounded, and blocking.

### 2 — Native OpenGL GUI, built in

```lex
let font = loadFont("/System/Library/Fonts/SFNS.ttf", 18)

window(800, 600, "App", fn(frame) {
    background(0.07, 0.07, 0.10)
    fill(0.40, 0.82, 1.00, 1.0)
    circle(mouseX(), mouseY(), 20.0)
    textFont(font, "Hello.", 40, 40, 1.2)
})
```

SDF-rendered shapes and text. 8× MSAA. Immediate-mode widget system. Runs at native speed via go-gl.

> **Platform note:** Graphics and UI run on macOS, Linux, and Windows. Cross-compiling Windows binaries from macOS requires `mingw-w64`; native builds use whatever C toolchain the host platform provides (Xcode on macOS, gcc on Linux, MSYS2 or TDM-GCC on Windows).

### 3 — No implicit behaviour, ever

```lex
1 == "1"     // TypeError — no coercion
if 1 { }     // TypeError — integer is not a boolean
let x = null     // explicit null, not an error
```

No hidden type coercion. No implicit threading. No magic. Every behaviour in a FROG program is declared.

---

## Install

Requires Go 1.22+.

```bash
git clone https://github.com/kmcnally5/FROG
cd FROG
go build -o klex .
./klex your_program.lex
```

The built `klex` binary auto-discovers `stdlib/` next to itself, so scripts that `import "stdlib/..."` work from any directory — no `KLEX_PATH` configuration needed when the binary and stdlib stay together.

---

## Core language

```lex
// Functions are first-class
fn add(a, b) { return a + b }
println(add(3, 4))   // 7

// Arrays and hashes
let points = [{"x": 1, "y": 2}, {"x": 3, "y": 4}]
println(points[0]["x"])   // 1

// Parallel processing — fan out work across async tasks
fn scan(chunk) { return len(chunk) }

let slices = ["alpha", "beta", "gamma"]
let n      = len(slices)
let tasks  = makeArray(n, null)
let i      = 0
while i < n {
    let chunk = slices[i]
    tasks[i] = async(fn() { return scan(chunk) })
    i = i + 1
}
let counts = makeArray(n, null)
let j = 0
while j < n {
    counts[j] = await(tasks[j])
    j = j + 1
}
println(counts)   // [5, 4, 5]

// Error handling — Go-style, no exceptions
fn riskyCall(x) { return x * 2 }
let result, err = safe(riskyCall, 21)
if err != null { println(err) }
println(result)   // 42
```

**Types:** integer, float, boolean, string, null, array, hash, tuple, channel, function, struct, enum, task

**Concurrency:** `async` / `await` / `channel` / `send` / `recv` / `select` / `atomicIntArray` / `atomicFloatArray`

---

## Language Server (froglsp)

`froglsp` is a full LSP server for FROG, built from the ground up for the language. Source lives in `snowball/froglsp/`.

**Capabilities:**

| Feature | Detail |
|---|---|
| Completion | Symbol, module-member (`.`), and builtin completions with snippet tab-stops |
| Signature help | Parameter hints on `(` and `,` — tracks the active argument as you type |
| Hover | Inline docs for builtins, user-defined functions, and imported module members |
| Go to definition | Jump to where a symbol is declared |
| Diagnostics | Parser errors and a static lint pass surfaced in-editor as you type |
| Document symbols | Outline view of all functions and declarations in the file |
| Formatting | Full document formatting via the built-in `klexfmt` formatter |
| Code actions | Quick-fixes for lint diagnostics |

The VS Code extension (`editors/vscode_froglsp/`) configuration wires the server up automatically — install the extension, open a `.lex` file, and all features are active with no additional configuration.

---

## Bytecode VM

kLex includes a bytecode compiler and VM. Rather than walking the AST node-by-node at runtime, the compiler translates your program to a compact instruction set that the VM dispatches in a tight flat loop — no recursion, no interpreter overhead per node.

Enable it with `--vm`:

```bash
./klex --vm your_program.lex
```

The VM delivers the most noticeable gains on compute-heavy workloads: tight arithmetic loops, deep recursion, and programs that call functions millions of times. Programs that spend most of their time in I/O, channels, or the graphics pipeline see smaller differences since those paths run in native Go regardless.

The VM is under active development. The vast majority of the language is supported; a small number of constructs (`select`, some advanced async patterns) are not yet implemented in the compiler.

---

## Why kLex exists

Most interpreted languages treat concurrency as an afterthought — async/await sugar over an event loop, or a GIL that makes threading a lie. FROG treats concurrency as the execution model.

The question kLex was built to answer:

> Can a lightweight tree-walking interpreter power real native applications — desktop tools, parallel scanners, live GUIs — without the overhead of a compiled toolchain?

The answer is yes.

The work that went into kLex is not syntax design. It is:

- **Scheduler design** — real goroutine-backed tasks, not cooperative coroutines
- **Graphics systems** — SDF rendering pipeline, MSAA, immediate-mode layout
- **Async runtime architecture** — environment snapshots eliminate mutex contention across task boundaries
- **Performant tooling pipelines** — parallel workers, atomic arrays, bounded channels
- **Coherent application model** — one concurrency model, not fourteen

The restraint is intentional. No decorators, no metaclasses, no reactive state systems, no giant framework abstractions. The simplicity is a feature. The moment kLex becomes a kitchen-sink language is the moment it stops being useful for the thing it is actually good at.

---

## Design principles

- **Explicit over implicit** — if it happens, you wrote it
- **Channels over shared memory** — coordinate by passing values
- **Strict types** — no coercion, ever
- **Array-first** — flat data structures, parallel processing
- **Low magic** — the runtime does what you can read

---

## Testing status

**macOS** is the primary development platform. Every release passes the full master test suite before tagging.

**Windows** — v0.3.35 passes 45 of 47 stdlib tests on a fresh install (the two failures are test-content portability issues, not interpreter bugs). v0.3.36 and the bytecode VM are untested on Windows.

**Linux** — v0.3.35 has been verified and is working. v0.3.36 and the bytecode VM are untested on Linux.

If you hit anything platform-specific, a GitHub issue with reproduction steps is the fastest way to get it looked at.

---

## Project Structure

| Folder | Purpose |
|---|---|
| `ast/` | AST node types |
| `cmd/` | CLI entry-point helpers |
| `docs/` | Language and library documentation |
| `editors/` | Editor integrations (VSCode extension) |
| `eval/` | Tree-walking evaluator and all builtins |
| `examples/` | Runnable example programs |
| `formatter/` | Source code formatter |
| `lexer/` | Tokeniser |
| `parser/` | Pratt parser |
| `repl/` | Interactive REPL |
| `snowball/` | Developer and build-time tooling |
| `stdlib/` | Standard library — `.lex` files |
| `tests/` | Test suite |
| `tools/` | Source for kLex-specific CLI tools |
| `vm/` | Bytecode compiler and VM (experimental) |

---

## License

MIT — Copyright © 2025 Karl McNally
