# kLex Async Guide

The single source of truth for kLex's async / await / channel system: the
snapshot model that powers it, the primitives, the patterns that make it
fast and safe, and the mistakes to avoid.

---

## Contents

1. [Cheat sheet](#1-cheat-sheet)
2. [Concepts — the snapshot model](#2-concepts--the-snapshot-model)
3. [Primitives](#3-primitives) — `async`, `await`, channels, `select`, `safe`
4. [Patterns](#4-patterns) — pure async, worker pools, pipelines, cancel, timeout, error handling
5. [Anti-patterns](#5-anti-patterns) — what to avoid and why
6. [Performance](#6-performance)
7. [FAQ](#7-faq)
8. [Testing async code](#8-testing-async-code)

---

## 1. Cheat sheet

**One-line summary:**

> `async` launches a task in a snapshot of the current scope. Return data
> via `await()`, communicate via channels, never mutate shared globals.

**The basic pattern** (preallocate — never `push()` in a loop):

```lex
fn worker(x) { return expensive(x) }

n     = 10
tasks = makeArray(n, null)
for i in range(n) {
    tasks[i] = async(worker, i)
}

results = makeArray(n, null)
for i, t in tasks {
    results[i] = await(t)
}
```

**The two rules:**

1. **Return data, don't mutate globals.** Scalar mutations inside an async
   task stay inside the task. (See §5 for the array/hash exception — it's
   a footgun.)
2. **Use `await()` or channels for results.** Don't poll a global flag —
   it won't update.

**What works:**

| ✅ | ❌ |
|---|---|
| Read globals (snapshotted at launch) | Mutate global scalars and expect the caller to see it |
| Read closure variables (snapshotted) | Reassign global arrays/hashes to new arrays in the task |
| Use local variables / locals mutate freely | Use a global flag to signal task completion |
| Pass arguments at launch time | Expect post-launch global mutations to flow inward |
| Return any value | Mutate the *contents* of arrays/hashes the caller can also see — that's a data race (use `concurrentHash` / `atomicIntArray` / channels) |
| Communicate via channels |   |

---

## 2. Concepts — the snapshot model

### 2.1 What `async` actually does

When you call `async(fn, args)`:

1. **Snapshot taken.** kLex copies the current environment chain — every
   name visible from the call site — into the task's own scope.
2. **Goroutine launched.** The function starts running on a real Go
   goroutine. The caller does not wait.
3. **Reads see the snapshot.** Inside the task, looking up any name
   resolves against the snapshot, not the live caller environment.
4. **Scalar writes are local.** Re-binding a name inside the task only
   updates the snapshot. The caller's binding is untouched.
5. **The return value flows back via `await()`** when the caller asks for it.

```lex
x = 100
task = async(fn() {
    x = 999            // task-local — visible only inside this task's snapshot
    return x + 1       // returns 1000
})
x = 200                // caller's binding — does NOT affect the running task
result = await(task)   // 1000
println(x)             // 200 (caller's value, never touched by the task)
```

### 2.1.1 Cheat sheet — what crosses the snapshot boundary

The shapshot copies the NAME → OBJECT map shallowly. Whether the task
can mutate the caller's data depends on the value's kind:

| Type                                    | Reassign name in task | Mutate object in place |
| --------------------------------------- | --------------------- | ---------------------- |
| Integer / Float / String / Boolean / Null | task-local            | n/a (immutable)        |
| Array                                   | task-local            | **shared** (`arr[i] = …`) |
| Hash                                    | task-local            | **shared** (`h["k"] = …`) |
| Struct instance                         | task-local            | **shared** (`s.field = …`) |
| Bytes                                   | task-local            | n/a (no mutation today)  |
| Channel                                 | task-local            | **shared** (send/recv affect both ends) |

In short: **rebinding the name is safe; mutating the pointed-at
object is shared**. The 6-case verification lives in
`tests/unit/asyncSnapshotTest.lex` — run it after any change to
`eval/env.go` or the async path.

### 2.2 The footgun — reference types are shared by pointer

The snapshot copies *bindings*, not the heap objects they point at.
**Arrays, hashes, and struct instances live in shared memory**:

```lex
arr = [1, 2, 3]
async(fn() {
    arr[0] = 99       // ⚠ ALSO mutates the caller's arr
})
sleep(100)
println(arr[0])       // → 99 (data race territory if anyone else writes too)
```

This is a real concurrent-mutation hazard. **Do not pass an array or hash
to an async task as a mutable target.** If you need shared mutable state:

- For numeric arrays: `atomicIntArray(n)` / `atomicFloatArray(n)` — every
  read and write is lock-free atomic.
- For hash maps: `concurrentHash()` — thread-safe get/set/CAS.
- For message passing: channels (the rest of this guide).

Reading a shared array is fine; mutating it from multiple goroutines is not.

### 2.3 Why this design

**Benefits**
- **Zero mutex contention** for the common case — task-local mutations
  hit no locks.
- **No race in scalar state** — tasks can't accidentally trample globals.
- **Performance** — ~89% less overhead than the prior shared-mutex model.
- **Clear semantics** — "what happens in async, stays in async (for scalars)."

**Trade-off**
- Tasks cannot push scalar updates back to the caller. All flow back goes
  through `await()` or channels. **This is a feature, not a bug** —
  enforced data flow > implicit aliasing.

---

## 3. Primitives

All of these are zero-import — available everywhere in kLex with no
`import` line.

### 3.1 `async(fn, ...args)` → Task

Launches `fn(...args)` on a new goroutine with its own scope snapshot.
Returns a `Task` value (type `"TASK"`).

```lex
t = async(myFn, arg1, arg2)        // launches immediately
t2 = async(fn(x) { return x * 2 }, 21)
```

Anonymous functions, named functions, and builtins are all valid first
arguments. `args` are passed by value (scalars copy; arrays/hashes pass
by pointer per §2.2).

### 3.2 `await(task)` → any

Blocks until the task finishes; returns whatever the function returned.

```lex
result = await(t)
```

If the task panicked or raised a `runtimeError`, the error propagates
through `await` and crashes the caller. Wrap with `safe` (§3.6) if the
task may fail:

```lex
result, err = safe(await, t)
if err != null {
    println("task failed: " + err.message)
}
```

If the task returns a `(value, err)` tuple, the caller unpacks normally —
no `safe` needed:

```lex
fn fetch() { return http.get(url) }   // returns (resp, err)
t = async(fetch)
resp, err = await(t)
```

### 3.3 Channels — `channel`, `send`, `recv`, `recvNonBlock`

Channels are typed conduits for passing values between goroutines.

**Create:**

```lex
ch = channel()        // unbuffered — synchronous handoff
ch = channel(64)      // buffered — sender doesn't block until buffer is full
```

**Send:**

```lex
send(ch, value)
```

Blocks if the buffer is full (or, for unbuffered channels, until a
receiver is ready). **Returns `false` if the channel has been closed or
cancelled** — use this to break worker loops cleanly:

```lex
while moreWork() {
    if send(ch, nextItem()) == false { break }   // channel gone
}
```

**Blocking receive:**

```lex
val, ok = recv(ch)
if !ok {
    // channel was closed AND drained — no more values will arrive
    break
}
```

**Non-blocking receive:**

```lex
msg = recvNonBlock(ch)        // returns the value, or null if nothing's ready
if msg != null {
    handle(msg)
}
```

Useful for polling channels inside a draw loop or a periodic ticker —
never blocks the calling goroutine.

**Iterate until close** — the for-in pattern handles closed channels for you:

```lex
for v in ch {
    handle(v)            // loop exits cleanly when ch is closed and drained
}
```

### 3.4 `close(ch)` vs `cancel(ch)`

Both signal "no more receives from this channel will succeed", but the
semantics differ:

| Operation | Buffered values | Blocked receivers | Use when |
|---|---|---|---|
| `close(ch)` | Drained first, then `ok == false` | Wake AFTER buffer is drained | Producer finished gracefully |
| `cancel(ch)` | Discarded immediately | Wake immediately with `ok == false` | Caller-initiated shutdown |

`cancel` is the standard pattern for cooperative cancellation across a
worker pool — one writer cancels the `done` channel, every worker
blocked on it wakes at once.

### 3.5 `select` — multi-channel coordination

`select` waits on multiple channel operations at once and runs the first
ready case. Four case forms:

```lex
select {
    case recv(ch1)              { … }   // receive, discard the value
    case val = recv(ch2)        { … }   // receive, bind value
    case val, ok = recv(ch3)    { … }   // bind value + closed-flag
    case send(ch4, value)       { … }   // try to send
    default                     { … }   // (optional) runs when nothing else is ready
}
```

Without `default`, `select` blocks until at least one case is ready. With
`default`, it never blocks — `select` becomes a poll.

The standard "worker with stop signal" loop:

```lex
while true {
    select {
        case msg = recv(work) { process(msg) }
        case _ = recv(done)   { return }       // caller asked us to stop
    }
}
```

### 3.6 `safe(fn, ...args)` → (result, err)

Calls `fn(...args)` and catches any panic or runtime error, returning a
`(result, err)` tuple. The single best companion to `await` when a task
may fail:

```lex
val, err = safe(await, t)
```

Without `safe`, an error inside the task crashes the caller; with it,
the caller gets to handle the failure.

---

## 4. Patterns

### 4.1 Pure async (preferred)

**When:** the task needs nothing from the caller's environment except its
arguments. Fastest, safest, easiest.

```lex
fn compute(n) {
    sum = 0
    i = 0
    while i < n {
        sum = sum + i
        i = i + 1
    }
    return sum
}

n     = 10
tasks = makeArray(n, null)
for i in range(n) {
    tasks[i] = async(compute, i * 1000)
}

results = makeArray(n, null)
for i, t in tasks {
    results[i] = await(t)
}
```

### 4.2 Read-only globals

**When:** all tasks need shared config and never mutate it. Snapshot makes
this safe and cheap.

```lex
config = {"max_retries": 3, "timeout": 5000, "api_key": env("API_KEY")}

fn fetch(url) {
    return http.get(url, config["api_key"], config["timeout"])
}

urls  = ["…", "…", "…"]
tasks = makeArray(len(urls), null)
for i, u in urls {
    tasks[i] = async(fetch, u)
}
responses = makeArray(len(urls), null)
for i, t in tasks {
    responses[i] = await(t)
}
```

If `config` is reassigned mid-flight, **already-launched tasks still see
their own snapshot.** Tasks launched after the reassignment see the new
value. (Mutating `config` *in place* — `config["timeout"] = 9999` — IS
visible everywhere, per §2.2. Prefer reassignment if you want snapshot
isolation.)

### 4.3 Fan-out / fan-in

A specific shape of pure async — launch N parallel tasks, collect their
results in order.

```lex
fn process(item) { … }

items = […]
tasks = makeArray(len(items), null)
for i, x in items {
    tasks[i] = async(process, x)
}
results = makeArray(len(items), null)
for i, t in tasks {
    results[i] = await(t)
}
```

Wall-clock time ≈ duration of the slowest task, not the sum.

For convenience, `stdlib/async.lex` exposes `await_all(tasks)` which does
the second loop for you:

```lex
import "stdlib/async.lex" as a
results = a.await_all(tasks)
```

### 4.4 Worker pool with a shared work channel

**When:** many items, fixed number of workers, items can be processed in
any order.

```lex
fn worker(workCh, resultCh) {
    while true {
        item, ok = recv(workCh)
        if !ok { return }                  // channel closed — work done
        send(resultCh, process(item))
    }
}

items    = […]
workCh   = channel(64)
resultCh = channel(64)

N = 4
workers = makeArray(N, null)
for i in range(N) {
    workers[i] = async(worker, workCh, resultCh)
}

// Feed work, then close so workers exit when drained.
for item in items { send(workCh, item) }
close(workCh)

// Wait for every worker to finish.
for w in workers { await(w) }

// Drain the result channel.
results = makeArray(len(items), null)
i = 0
while i < len(items) {
    r, _ = recv(resultCh)
    results[i] = r
    i = i + 1
}
```

### 4.5 Pipeline with channels

Each stage is a task; stages chain via channels. Each downstream stage
closes its output when its input closes — propagates shutdown
automatically.

```lex
fn stage1(out) {
    for v in produceValues() { send(out, v) }
    close(out)
}

fn stage2(in_, out) {
    while true {
        v, ok = recv(in_)
        if !ok { close(out)  return }
        send(out, transform(v))
    }
}

a = channel(32)
b = channel(32)
async(stage1, a)
async(stage2, a, b)

for v in b {
    println(v)               // exits when stage2 closes b
}
```

### 4.6 Cancellation via a done channel

Workers `select` on both their work channel and a `done` channel. Closing
or cancelling `done` shuts them all down immediately.

```lex
fn worker(work, done) {
    while true {
        select {
            case item = recv(work) { process(item) }
            case _    = recv(done) { return }
        }
    }
}

work = channel(64)
done = channel()
async(worker, work, done)
…
cancel(done)   // every worker blocked on done wakes and returns
```

`cancel` over `close` here because cancellation should not wait for
buffered values to drain.

### 4.7 Timeout — race against a timer task

There's no built-in `select`-on-task primitive; race against a timer
goroutine via a shared "winner" channel:

```lex
fn doWork()     { return expensiveJob() }
fn timeoutAfter(ms, winner) {
    sleep(ms)
    send(winner, {"who": "timeout"})
}

winner = channel(1)
workCh = channel(1)

async(fn() { send(workCh, doWork()) })
async(timeoutAfter, 5000, winner)
async(fn() {
    v, _ = recv(workCh)
    send(winner, {"who": "work", "value": v})
})

result, _ = recv(winner)
if result["who"] == "timeout" {
    println("timed out")
} else {
    println(result["value"])
}
```

For most production cases, set timeouts on the underlying primitive
(HTTP, DB) rather than wrapping a task — narrower scope, cleaner cancel.

### 4.8 Error handling — `safe(await, t)`

Any panic or runtime error inside the task surfaces through `await`. If
the task can fail unpredictably, wrap the `await`:

```lex
fn risky() { … }

t = async(risky)
val, err = safe(await, t)
if err != null {
    println("task failed: " + err.message)
} else {
    use(val)
}
```

If your task returns a `(value, err)` tuple by convention (the kLex
standard pattern for fallible functions), the caller unpacks the tuple
directly — `safe` isn't needed unless a panic is also possible:

```lex
fn fetch() { return http.get(url) }   // returns (resp, err)
t            = async(fetch)
resp, err    = await(t)               // (resp, err) tuple
```

For chains of fallible calls, the `?` postfix operator inside the task
propagates errors up to the task's own return — the result then flows
back to the caller normally:

```lex
fn pipeline() {
    raw  = readFile(path)?
    obj  = json.parse(raw)?
    return obj["data"], null
}
```

---

## 5. Anti-patterns

### 5.1 Mutating a global array or hash from inside async

Two distinct mistakes lurk here — be careful which one applies:

**Mistake A: reassigning a global to a new array, expecting the caller
to see it.**

```lex
results = []
fn worker(id) {
    results = push(results, compute(id))  // ❌ creates a NEW array, stays in snapshot
}
for i in range(100) { async(worker, i) }
println(results)                           // still empty
```

The reassignment `results = push(...)` rebinds `results` in the task's
snapshot. The caller's `results` is untouched. Fix: return the value and
collect in the main scope.

**Mistake B: mutating the contents of a shared array.**

```lex
arr = makeArray(100, 0)
for i in range(100) {
    async(fn(idx) { arr[idx] = compute(idx) }, i)   // ⚠ multiple writers — DATA RACE
}
```

This *does* update the caller's array (arrays are shared by pointer per
§2.2), but it's a **data race** — concurrent unsynchronised writes have
undefined behaviour. Fix: use `atomicIntArray(n)` / `atomicFloatArray(n)`
if the values are numbers, or have each task return its value and let
the main scope write to `arr[i]`.

### 5.2 Coordination via a global flag

```lex
done = false
fn worker() { … ; done = true }     // ❌ only sets the task's snapshot
async(worker)
while !done { }                      // ❌ infinite loop
```

`done` in the task is a scalar — its rebinding stays in the snapshot.
Fix: use `await`, or use a channel for fine-grained signalling.

### 5.3 `push()` in a loop (even outside async)

```lex
tasks = []
for i in range(n) {
    tasks = push(tasks, async(worker, i))   // ❌ O(n²) — allocates each iteration
}
```

`push` returns a fresh array; calling it in a loop is quadratic.
**Always preallocate:**

```lex
tasks = makeArray(n, null)
for i in range(n) {
    tasks[i] = async(worker, i)             // ✅ O(n)
}
```

### 5.4 Expecting post-launch mutations to propagate inward

```lex
counter = 0
task    = async(fn() { return counter + 1 })
counter = 100                                // ❌ snapshot already taken
await(task)                                  // returns 1, not 101
```

Snapshot is taken at `async` call time. Any later mutation by the caller
is invisible inside the task. Fix: pass values as arguments at launch:

```lex
task = async(fn(start) { return start + 1 }, 100)
await(task)   // 101
```

### 5.5 Sharing a regular hash across goroutines for mutable state

Same data race as 5.1B, but for hashes. If multiple tasks need to write
into a shared map, use `concurrentHash()`:

```lex
state = concurrentHash()
for i in range(N) {
    async(fn(id) { state.set("k" + str(id), compute(id)) }, i)
}
```

`concurrentHash` has internal synchronisation; plain `{}` hashes do not.

---

## 6. Performance

Approximate wall-clock numbers from a 2026 Mac mini — your hardware will
vary, but the relative shape is the point:

| Pattern | 10 tasks | 100 tasks | Notes |
|---|---|---|---|
| Pure async, no global access | ~380 ms | ~3.8 s | Linear — no contention |
| Read-only global access | ~420 ms | ~4.2 s | Snapshot read overhead is small |
| *(Old design — shared globals + mutex)* | 690 ms | bottlenecked | Replaced for a reason |

The snapshot design delivers **~89% less contention overhead** than the
prior shared-mutex model. Don't reach for shared mutable state unless
you genuinely need a single-writer/many-reader pattern best served by a
manager task + channel.

Snapshot cost is roughly **O(globals) per `async` call**. Programs with
thousands of module-level constants will see a measurable per-launch
overhead — keep module-level state lean if you're launching many short
tasks.

For shared-mutation hot paths, the lock-free primitives
(`atomicIntArray`, `atomicFloatArray`, `concurrentHash`) outperform any
manager-task pattern but only fit specific access shapes.

---

## 7. FAQ

**Can I modify arrays/hashes inside an async task?**
*Locally — yes.* If you create a new array inside the task and mutate
it, that's fine. If the array existed in the caller's scope (and was
captured into the snapshot as a pointer), mutations to its contents are
visible to the caller AND are a data race if any other goroutine also
writes to it. See §2.2.

**Can I access closure variables?**
Yes. Closures are part of the snapshot. Scalar closure variables behave
like any other binding (snapshotted). Captured arrays/hashes share by
pointer (§2.2).

**What about `const` values?**
Snapshotted like any binding, and the const-ness still applies inside
the task — assigning to a `const` is a runtime error regardless of
context.

**Can two async tasks see each other's bindings?**
No, not via scalars. Each task has its own snapshot. Use channels to
communicate, or have the main scope collect both results and combine
them.

**What if I genuinely need shared mutable state?**
You have three options, in order of preference:

1. **Channels + a manager task** — one goroutine owns the state; everyone
   else talks to it via channel messages. Single-threaded by construction.
2. **`concurrentHash()`** — for hash-shaped state. Thread-safe get/set/CAS.
3. **`atomicIntArray()` / `atomicFloatArray()`** — for numeric arrays.
   Every read and write is lock-free atomic.

Avoid plain `{}` hashes and `[]` arrays for shared mutation.

**Does `async` work inside async?**
Yes. Nested asyncs are fine. Each gets its own snapshot.

**Are tasks garbage-collected if I forget to `await`?**
The underlying goroutine runs to completion regardless. The Task object
is GC'd when nothing references it. **Forgotten tasks leak goroutines** —
keep references and `await` (or use a context-channel for cancellation).

**What's the difference between `close(ch)` and `cancel(ch)`?**
- `close` says "no more sends" — receivers drain remaining buffered
  values, then see `ok == false`.
- `cancel` is immediate — every blocked receiver wakes right now with
  `ok == false`, buffered values are discarded.

**Why does `send` return a value?**
It returns `false` when the channel has been closed or cancelled. Used
to break producer loops cleanly without panicking.

**Why does `recv` return a tuple but `recvNonBlock` doesn't?**
Pragmatic ergonomics. `recv` blocks until a value arrives, so the
closed-flag is the only way to signal "channel done" — hence `(val, ok)`.
`recvNonBlock` returns immediately and can use `null` as the
"nothing-ready" sentinel because real values aren't usually null.

**Can I `select` on a task completion?**
Not directly. The Task type doesn't expose its internal "done" channel.
If you need to race a task against something else, have the task
send-on-completion to a channel you create:

```lex
done = channel(1)
async(fn() { result = work()  send(done, result) })

select {
    case r = recv(done)        { use(r) }
    case _ = recv(otherSignal) { … }
}
```

---

## 8. Testing async code

### Test the function synchronously first

`async` should add concurrency, not correctness — so verify behaviour
in a plain call before introducing scheduling.

```lex
fn worker(id) { return id * 2 }

// Logic check, no async involved.
assert(worker(3) == 6, "worker logic")

// Async wrapper check.
t = async(worker, 3)
assert(await(t) == 6, "worker via async")
```

### Verify isolation explicitly

Prove the snapshot semantics in test code — both the protection (scalars)
and the leak (reference types):

```lex
state = 100

fn checkIsolation() {
    state = 999
    return state
}

t      = async(checkIsolation)
result = await(t)

assert(result == 999,  "task saw its own scalar mutation")
assert(state  == 100,  "caller's scalar unchanged")
```

### Race-detect via shape, not via timing

The snapshot model makes data races structurally impossible for scalar
state. If your test relies on `sleep()` to "wait for the right thing to
happen", it's testing scheduling, not behaviour. Restructure to use
`await` or channel signalling instead.

### When testing shared-mutation primitives

`concurrentHash`, `atomicIntArray`, and `atomicFloatArray` have their
own deterministic correctness guarantees. Use them as the test target
when you're verifying concurrent-write behaviour; don't try to make a
plain `{}` hash behave correctly under multi-writer load.

---

## Related references

- `eval/env.go` — implementation of the snapshot mechanism (with caveats
  on reference types prominently documented in the source).
- `eval/eval.go` builtins — `async`, `await`, `channel`, `send`, `recv`,
  `recvNonBlock`, `close`, `cancel` definitions.
- `stdlib/async.lex` — small set of helpers (`await_all`, `parallel`)
  built on the primitives.
- `eval/builtins_atomic.go` + `eval/builtins_concurrent_hash.go` — the
  lock-free primitives for shared-mutation patterns.
