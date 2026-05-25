// pmapSafeTest.lex — locks the new error-tolerant pmap_safe (OFI #2).
//
// Run with: ./klex tests/unit/pmapSafeTest.lex
// Exit 0 on all-pass.

import "stdlib/parallel.lex" as p
import "stdlib/stream.lex"   as s

let failures = 0

// ── 1. All-good inputs: behaves exactly like pmap ───────────────────
let inputs = makeArray(100, 0)
let i = 0
while i < 100 { inputs[i] = i + 1   i = i + 1 }

let stream = p.pmap_safe(inputs, fn(x) { return x * 2 }, 8)
let out = makeArray(100, null)
let idx = 0
for v in stream.ch {
    out[idx] = v
    idx = idx + 1
}
let errVal, _ = recv(stream.errCh)
if idx != 100 {
    println("FAIL: all-good got " + str(idx) + " outputs, expected 100")
    failures = failures + 1
} else {
    // Check first/last/random.
    if out[0] != 2 || out[99] != 200 || out[50] != 102 {
        println("FAIL: all-good values wrong (first=" + str(out[0]) +
                " last=" + str(out[99]) + " mid=" + str(out[50]) + ")")
        failures = failures + 1
    } else {
        println("ok: all-good — 100 inputs doubled cleanly")
    }
}

// ── 2. One bad input among many ─────────────────────────────────────
// Worker function divides into x. x=0 triggers a runtime error.
inputs = makeArray(50, 0)
i = 0
while i < 50 { inputs[i] = i   i = i + 1 }   // 0..49; the 0 will explode

stream = p.pmap_safe(inputs, fn(x) { return 100 / x }, 4)
let goodCount = 0
let errCount = 0
let results = makeArray(50, null)
let ri = 0
for v in stream.ch {
    results[ri] = v
    if isError(v) { errCount = errCount + 1 }
    else          { goodCount = goodCount + 1 }
    ri = ri + 1
}

if errCount != 1 {
    println("FAIL: expected 1 error (for x=0), got " + str(errCount))
    failures = failures + 1
} else if goodCount != 49 {
    println("FAIL: expected 49 successes, got " + str(goodCount))
    failures = failures + 1
} else {
    println("ok: 1 bad + 49 good inputs → 1 Error + 49 results, NO abort")
}

// ── 3. Many bad inputs scattered through ────────────────────────────
// Every 7th input crashes. We must still see every slot.
inputs = makeArray(200, 0)
i = 0
while i < 200 { inputs[i] = i + 1   i = i + 1 }   // 1..200

stream = p.pmap_safe(inputs, fn(x) {
    if x % 7 == 0 {
        // Synthetic error
        return error("EVERY_SEVENTH", "x=" + str(x) + " is divisible by 7")
    }
    return x
}, 8)
let seenErrors = 0
let seenOk = 0
for v in stream.ch {
    if isError(v) { seenErrors = seenErrors + 1 }
    else          { seenOk = seenOk + 1 }
}

// Expected errors: numbers 7, 14, 21, ..., 196 = 28 of them
let expectedErr = 28
let expectedOk = 200 - 28
if seenErrors != expectedErr || seenOk != expectedOk {
    println("FAIL: scattered errors mismatch — errs=" + str(seenErrors) +
            " (expected " + str(expectedErr) + "), ok=" + str(seenOk) +
            " (expected " + str(expectedOk) + ")")
    failures = failures + 1
} else {
    println("ok: 200 inputs with 28 scattered errors → all slots accounted for")
}

// ── 4. ALL inputs error → stream still yields N error values ───────
inputs = makeArray(20, 0)
i = 0
while i < 20 { inputs[i] = i   i = i + 1 }
stream = p.pmap_safe(inputs, fn(x) {
    return error("ALWAYS", "no good outputs from this fn")
}, 4)
let slots = 0
let errSlots = 0
for v in stream.ch {
    slots = slots + 1
    if isError(v) { errSlots = errSlots + 1 }
}
if slots != 20 || errSlots != 20 {
    println("FAIL: all-bad slots=" + str(slots) + " errs=" + str(errSlots))
    failures = failures + 1
} else {
    println("ok: 20 inputs, all error → 20 Error slots emitted")
}

// ── 5. errCh always receives null on success path ───────────────────
inputs = [1, 2, 3]
stream = p.pmap_safe(inputs, fn(x) { return x }, 2)
for v in stream.ch {
    // drain
}
// Now check errCh — pmap_safe should always send null (never an error
// tuple), even if individual elements errored.
errVal, _ = recv(stream.errCh)
if errVal != null {
    println("FAIL: errCh got non-null on success — " + str(errVal))
    failures = failures + 1
} else {
    println("ok: errCh receives null on success")
}

// ── 6. Tiny input that's smaller than numWorkers ───────────────────
inputs = [10, 20, 30]
stream = p.pmap_safe(inputs, fn(x) { return x + 1 }, 16)
let got = makeArray(3, null)
idx = 0
for v in stream.ch {
    got[idx] = v
    idx = idx + 1
}
if idx != 3 || got[0] != 11 || got[1] != 21 || got[2] != 31 {
    println("FAIL: tiny input — got=[" + str(got[0]) + "," + str(got[1]) + "," + str(got[2]) + "]")
    failures = failures + 1
} else {
    println("ok: tiny input (3 elems) with 16 workers handled cleanly")
}

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — pmap_safe: per-element errors don't abort the stream (OFI #2)")
