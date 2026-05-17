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
./klex tests/examples/SecretHunter/secretHunterUI.lex
```

40 parallel workers. Native OpenGL GUI at 60fps during scan. Built in ~900 lines of FROG.

> A scripted scanner with a native GUI and parallel async channels — in an interpreted language.

---

## Try it Online

**[Launch the kLex REPL](https://kmcnally5.github.io/FROG/playground/)** — run kLex code directly in your browser, no installation required.

The REPL supports multi-line input (automatically detects when blocks are complete) and maintains session state — define variables and functions, then use them in subsequent lines.

---

## Three things that make it different

### 1 — Async and channels in seven lines

```lex
jobs    = channel(200)
results = channel(200)

worker = async(fn() {
    while true {
        job = recv(jobs)
        send(results, process(job))
    }
})

send(jobs, "file.txt")
result = recv(results)
```

No executors, no event loops. Each `async()` spawns a real goroutine. Channels are typed, bounded, and blocking.

### 2 — Native OpenGL GUI, built in

```lex
font = loadFont("/System/Library/Fonts/SFNS.ttf", 18)

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
x = null     // explicit null, not an error
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

// Arrays, hashes, structs
points = [{"x": 1, "y": 2}, {"x": 3, "y": 4}]

// Parallel processing
tasks = makeArray(n)
i = 0
while i < n {
    let chunk = slices[i]
    tasks[i] = async(fn() { return scan(chunk) })
    i = i + 1
}

// Error handling — Go-style, no exceptions
result, err = safe(riskyCall, [arg])
if err != null { println(err) }
```

**Types:** integer, float, boolean, string, null, array, hash, tuple, channel, function, struct, enum, task

**Concurrency:** `async` / `await` / `channel` / `send` / `recv` / `select` / `atomicIntArray` / `atomicFloatArray`

---

## Built-in tooling

| Tool | What it does |
|---|---|
| `klex file.lex` | Run a program |
| `froglsp` | LSP server — autocomplete, hover docs, diagnostics |
| VS Code extension | Syntax highlighting + LSP integration |

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

kLex is extensively tested on **macOS** — its primary development platform, where every release passes the full master test suite before tagging.

**Windows** has received initial verification: the v0.3.35 release passes 45 of 47 stdlib tests on a fresh Windows install (the two failures are test-content portability issues, not interpreter bugs), but the platform has not yet been exercised in real production workloads.

**Linux** native testing is planned but has not yet started. The runtime is expected to behave the same as macOS given the overlap in concurrency, OpenGL, and filesystem subsystems, but no firsthand verification has been done.

If you hit anything platform-specific, a GitHub issue with reproduction steps is the fastest way to get it looked at.

---

## License

MIT — Copyright © 2025 Karl McNally
