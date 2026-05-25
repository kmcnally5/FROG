// hashConcurrentTest.lex — proves OFI #3 (Hash concurrent-write panic).
//
// Before the fix: N goroutines mutating the same *Hash triggered Go's
//   "fatal error: concurrent map writes"
// crash — UNRECOVERABLE from kLex code. The Hash struct now carries a
// sync.Mutex and every user-facing eval site (h[k] = v, h[k], for-in,
// keys/values/len/hasKey/delete) serialises through it. This test
// hammers the locked paths from 50 goroutines × 500 writes each
// (25,000 writes total) and verifies (a) no crash, (b) all writes
// are present afterward.
//
// Run with: ./klex tests/unit/hashConcurrentTest.lex
//   (and: go run -race . tests/unit/hashConcurrentTest.lex to confirm
//    race-detector cleanness)
// Exit 0 on all-pass.

let failures = 0

// ── 1. Concurrent writes to a shared hash — must not crash ──────────
let shared = {}
let nWorkers = 50
let nWritesPerWorker = 500
let tasks = makeArray(nWorkers, null)

let w = 0
while w < nWorkers {
    let me = w
    let n  = nWritesPerWorker
    tasks[w] = async(fn() {
        let i = 0
        while i < n {
            // Each worker writes into its own keyspace ("w<id>-<i>").
            // No two workers ever write the same key, so the FINAL
            // entry count is exactly nWorkers × nWritesPerWorker.
            let key = "w" + str(me) + "-" + str(i)
            shared[key] = me * 1000 + i
            i = i + 1
        }
        return null
    })
    w = w + 1
}

// Wait for all.
w = 0
while w < nWorkers { await(tasks[w])  w = w + 1 }

let expectedTotal = nWorkers * nWritesPerWorker
let gotTotal = len(shared)
if gotTotal != expectedTotal {
    println("FAIL: got " + str(gotTotal) + " entries, expected " + str(expectedTotal))
    failures = failures + 1
} else {
    println("ok: " + str(expectedTotal) + " parallel non-overlapping writes all landed in shared hash")
}

// Spot-check a few values.
let v = shared["w0-0"]
if v != 0 {
    println("FAIL: w0-0 = " + str(v) + ", expected 0")
    failures = failures + 1
}
v = shared["w7-42"]
if v != 7042 {
    println("FAIL: w7-42 = " + str(v) + ", expected 7042")
    failures = failures + 1
}
v = shared["w49-499"]
if v != 49499 {
    println("FAIL: w49-499 = " + str(v) + ", expected 49499")
    failures = failures + 1
}
println("ok: spot-checked values match the writer-encoded pattern")

// ── 2. Concurrent reads + writes on the same key ────────────────────
// Workers race on a single counter slot. The final value is non-
// deterministic (without CAS), but the test must NOT crash. Last-
// writer-wins is fine — what matters is that the runtime survives.
let race = {"counter": 0}
let nRacers = 30
tasks = makeArray(nRacers, null)
w = 0
while w < nRacers {
    let me = w
    tasks[w] = async(fn() {
        let i = 0
        while i < 200 {
            race["counter"] = me            // Pure write race.
            v = race["counter"]              // Pure read race.
            // Touch v just so the optimiser doesn't elide.
            if v < 0 { println("impossible") }
            i = i + 1
        }
        return null
    })
    w = w + 1
}
w = 0
while w < nRacers { await(tasks[w])  w = w + 1 }
println("ok: " + str(nRacers) + " readers/writers raced on a single key — no panic")

// ── 3. Iteration during writes (keys/values/for-in) ─────────────────
// One goroutine continuously iterates; another continuously mutates.
// The iterator snapshots under the lock so it sees a consistent view
// each pass, regardless of what the writer does between passes.
let shared2 = makeArray(1, null)
shared2[0] = {"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}

// Channel for clean writer→reader completion signaling. Avoids the
// bool-array race that would otherwise show up under `go run -race`
// — separate concern from the hash-concurrency we're testing.
let doneCh = channel(1)
let writer = async(fn() {
    let i = 0
    while i < 1000 {
        let h = shared2[0]
        h["x" + str(i)] = i
        if hasKey(h, "x" + str(i - 100)) {
            delete(h, "x" + str(i - 100))
        }
        i = i + 1
    }
    send(doneCh, true)
    return null
})

let reader = async(fn() {
    let rounds = 0
    while true {
        // Non-blocking check on the completion channel each loop.
        let msg = recvNonBlock(doneCh)
        if msg != null { return rounds }
        let h = shared2[0]
        let ks = keys(h)              // snapshot under lock
        let vs = values(h)            // snapshot under lock
        // Iterate the snapshot — independent of any further writes.
        let i = 0
        while i < len(ks) {
            let k = ks[i]
            v = vs[i]
            if len(k) < 0 { println("impossible") }
            if type(v) != "INTEGER" && type(v) != "STRING" { /* fine */ }
            i = i + 1
        }
        // for-in path too.
        for k, v in h {
            if len(k) < 0 { println("impossible") }
        }
        rounds = rounds + 1
    }
    return rounds
})

await(writer)
let rounds = await(reader)
println("ok: iterator ran " + str(rounds) + " rounds concurrently with writer — no panic")

// ── 4. delete() under concurrency ───────────────────────────────────
let shared3 = {}
let i = 0
while i < 1000 {
    shared3["k" + str(i)] = i
    i = i + 1
}
let beforeDel = len(shared3)
// 10 goroutines each delete a non-overlapping range.
tasks = makeArray(10, null)
w = 0
while w < 10 {
    let me = w
    tasks[w] = async(fn() {
        i = me * 100
        let end = (me + 1) * 100
        while i < end {
            delete(shared3, "k" + str(i))
            i = i + 1
        }
        return null
    })
    w = w + 1
}
w = 0
while w < 10 { await(tasks[w])  w = w + 1 }
let afterDel = len(shared3)
if afterDel != 0 {
    println("FAIL: deletes left " + str(afterDel) + " entries (expected 0)")
    failures = failures + 1
} else {
    println("ok: parallel non-overlapping deletes drained 1000-entry hash (before=" +
            str(beforeDel) + " after=0)")
}

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — Hash survives concurrent writes/reads/iterations/deletes (OFI #3)")
