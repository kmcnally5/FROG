// asyncSnapshotTest.lex — empirical lock-down of kLex async snapshot
// semantics (OFI #9). The runtime source (eval/env.go Snapshot()) makes
// a shallow copy of the env's name→Object map. Primitives in kLex are
// value-like (Integer, Float, String, Boolean, Null) — re-assigning
// the NAME in an async creates a task-local binding that doesn't
// propagate. Reference types (*Array, *Hash, *Bytes, *StructInstance)
// are pointer-shared, so MUTATIONS to the underlying object DO
// propagate.
//
// This test pins all four cases. Run after touching anything in
// eval/env.go or evalAsync — if any of these flip, the semantics
// have changed and stdlib + projects/* both depend on the current
// shape.
//
// Run with: ./klex tests/unit/asyncSnapshotTest.lex
// Exit 0 on all-pass.

import "stdlib/assert.lex" as a

let failures = 0

// ── 1. Primitives: reassignment is task-local ────────────────────────
let x = 10
let t = async(fn() { x = 999  return null })
await(t)
if x != 10 {
    println("FAIL: integer reassignment inside async leaked outside (got " + str(x) + ")")
    failures = failures + 1
} else {
    println("ok: primitive Integer reassignment is task-local")
}

let s = "before"
t = async(fn() { s = "after"  return null })
await(t)
if s != "before" {
    println("FAIL: string reassignment leaked (got '" + s + "')")
    failures = failures + 1
} else {
    println("ok: primitive String reassignment is task-local")
}

// ── 2. Hash: mutation of fields IS shared ────────────────────────────
let h = {"count": 0}
t = async(fn() { h["count"] = 42  return null })
await(t)
if h["count"] != 42 {
    println("FAIL: hash field mutation NOT shared (got " + str(h["count"]) + ")")
    failures = failures + 1
} else {
    println("ok: hash field mutation IS shared with parent")
}

// ── 3. Array: index mutation IS shared ───────────────────────────────
let arr = [1, 2, 3]
t = async(fn() { arr[0] = 99  return null })
await(t)
if arr[0] != 99 {
    println("FAIL: array index mutation NOT shared (got " + str(arr[0]) + ")")
    failures = failures + 1
} else {
    println("ok: array index mutation IS shared with parent")
}

// ── 4. Struct: field mutation IS shared ──────────────────────────────
struct Counter {
    value
}
let c = Counter { value: 0 }
t = async(fn() { c.value = 7  return null })
await(t)
if c.value != 7 {
    println("FAIL: struct field mutation NOT shared (got " + str(c.value) + ")")
    failures = failures + 1
} else {
    println("ok: struct field mutation IS shared with parent")
}

// ── 5. Hash REASSIGNMENT (vs mutation): task-local ───────────────────
let h2 = {"x": 1}
t = async(fn() {
    h2 = {"x": 999}              // REBIND the name to a new hash
    return null
})
await(t)
if h2["x"] != 1 {
    println("FAIL: hash rebind leaked (got " + str(h2["x"]) + ")")
    failures = failures + 1
} else {
    println("ok: hash NAME rebind is task-local (mutation vs rebind distinction)")
}

// ── 6. Concurrent shared mutation — DANGEROUS but observable ────────
// 10 tasks each push to a shared array via index assignment to a slot
// they own. No write collisions, so this MUST be safe. Demonstrates
// reads/writes propagating across the snapshot boundary.
let target = makeArray(10, 0)
let tasks = makeArray(10, null)
let i = 0
while i < 10 {
    let slot = i
    tasks[i] = async(fn() { target[slot] = slot * 10  return null })
    i = i + 1
}
i = 0
while i < 10 { await(tasks[i])   i = i + 1 }
let ok = true
i = 0
while i < 10 {
    if target[i] != i * 10 { ok = false }
    i = i + 1
}
if !ok {
    println("FAIL: non-overlapping index writes did not all land in shared array")
    failures = failures + 1
} else {
    println("ok: 10 parallel non-overlapping index writes land in shared array")
}

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — async snapshot semantics: primitives copied, references shared (OFI #9)")
