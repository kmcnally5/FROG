// concatAllTest.lex — locks the concatAll builtin (OFI #10).
//
// Run with: ./klex tests/unit/concatAllTest.lex
// Exit 0 on all-pass.

let failures = 0

// 1. Basic merge.
let got = concatAll([[1, 2], [3, 4], [5]])
if len(got) != 5 || got[0] != 1 || got[4] != 5 {
    println("FAIL: basic merge")
    failures = failures + 1
} else {
    println("ok: basic 3-array merge")
}

// 2. Empty outer → empty array.
got = concatAll([])
if type(got) != "ARRAY" || len(got) != 0 {
    println("FAIL: empty outer")
    failures = failures + 1
} else {
    println("ok: empty outer → empty array")
}

// 3. Empty inners are fine.
got = concatAll([[], [1], [], [2, 3], []])
if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
    println("FAIL: mixed-empty")
    failures = failures + 1
} else {
    println("ok: empty inners skipped correctly")
}

// 4. Mixed-type elements — preserves identity (Object copy).
got = concatAll([[1, "a"], [true, null], [3.14]])
if len(got) != 5 || got[0] != 1 || got[1] != "a" || got[2] != true ||
   got[3] != null || type(got[4]) != "FLOAT" {
    println("FAIL: mixed element types")
    failures = failures + 1
} else {
    println("ok: mixed element types preserved")
}

// 5. Non-array element → typed error.
let _, e = safe(fn() { return concatAll([[1], "not an array", [2]]) })
if e == null {
    println("FAIL: non-array inner did not error")
    failures = failures + 1
} else if indexOf(e.message, "element 1") < 0 {
    println("FAIL: error doesn't name index — " + e.message)
    failures = failures + 1
} else {
    println("ok: non-array inner errored at correct index — " + e.message)
}

// 6. Wrong outer type → error.
_, e = safe(fn() { return concatAll("not an array") })
if e == null {
    println("FAIL: non-array outer did not error")
    failures = failures + 1
} else {
    println("ok: non-array outer rejected — " + e.message)
}

// 7. Arity check.
_, e = safe(fn() { return concatAll() })
if e == null {
    println("FAIL: zero-arg did not error")
    failures = failures + 1
} else {
    println("ok: zero-arg rejected — " + e.message)
}

// 8. Performance proof: 1000 batches of 100 each = 100k elements.
//    concatAll should be O(total) = ~instant. The equivalent loop of
//    concat() calls would be O(N²) = noticeably slow.
let batches = makeArray(1000, null)
let i = 0
while i < 1000 {
    let b = makeArray(100, i)   // each batch is 100 copies of its index
    batches[i] = b
    i = i + 1
}

let t0 = _timeNanos()
let merged = concatAll(batches)
let elapsedMs = (_timeNanos() - t0) / 1000000

if len(merged) != 100000 {
    println("FAIL: merged length = " + str(len(merged)) + " expected 100000")
    failures = failures + 1
} else if merged[0] != 0 || merged[99999] != 999 {
    println("FAIL: merged contents wrong at boundary")
    failures = failures + 1
} else {
    println("ok: concatAll on 1000 batches × 100 = 100k elements in " + str(elapsedMs) + "ms")
}
if elapsedMs > 1000 {
    println("FAIL: concatAll took " + str(elapsedMs) + "ms (>1s) — looks O(n²) not O(total)")
    failures = failures + 1
}

// 9. Equivalence with concat() chain — for a small case where the
//    chain isn't catastrophically slow, the results must match.
let arr1 = [1, 2, 3]
let arr2 = [4, 5]
let arr3 = [6]
let chained  = concat(concat(arr1, arr2), arr3)
let oneShot  = concatAll([arr1, arr2, arr3])
if len(chained) != len(oneShot) {
    println("FAIL: equivalence lengths differ")
    failures = failures + 1
} else {
    let same = true
    i = 0
    while i < len(chained) {
        if chained[i] != oneShot[i] { same = false }
        i = i + 1
    }
    if !same {
        println("FAIL: equivalence contents differ")
        failures = failures + 1
    } else {
        println("ok: concatAll equivalent to chained concat for 3-array case")
    }
}

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — concatAll: O(total) flatten replacing O(n^2) concat-in-loop (OFI #10)")
