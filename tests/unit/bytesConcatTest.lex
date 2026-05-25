// bytesConcatTest.lex — locks the bytesConcat builtin contract.
//
// Run with: ./klex tests/unit/bytesConcatTest.lex
// Exit 0 on all-pass.

let failures = 0

// 1. Two pieces merge in order.
let a = strToBytes("hello ")
let b = strToBytes("world")
let merged = bytesConcat([a, b])
if type(merged) != "BYTES" {
    println("FAIL: result type = " + type(merged))
    failures = failures + 1
}
let s, _ = bytesToStr(merged)
if s != "hello world" {
    println("FAIL: two-piece merge got '" + s + "'")
    failures = failures + 1
} else {
    println("ok: two-piece merge")
}

// 2. Empty array → empty bytes.
let empty = bytesConcat([])
if type(empty) != "BYTES" {
    println("FAIL: empty array result type = " + type(empty))
    failures = failures + 1
} else if len(empty) != 0 {
    println("FAIL: empty array len = " + str(len(empty)))
    failures = failures + 1
} else {
    println("ok: empty array → 0-byte bytes")
}

// 3. Single-element array → copy of that element (defensive copy —
//    mutating the source must NOT affect the result, but since *Bytes
//    has no mutation builtin today this is mainly about identity).
let src = strToBytes("solo")
let result = bytesConcat([src])
let s2, _ = bytesToStr(result)
if s2 != "solo" {
    println("FAIL: single-element merge got '" + s2 + "'")
    failures = failures + 1
} else if len(result) != len(src) {
    println("FAIL: single-element length mismatch")
    failures = failures + 1
} else {
    println("ok: single-element merge")
}

// 4. Many pieces — exercise the cumulative copy. 1KB + 1KB + 1KB.
let piece = bytes(1024)            // 1024 zero bytes
let big = bytesConcat([piece, piece, piece])
if len(big) != 3072 {
    println("FAIL: 3x 1KB merge len = " + str(len(big)) + " expected 3072")
    failures = failures + 1
} else {
    println("ok: 3x 1KB merge → 3072 bytes")
}

// 5. Mixed-type array — first non-bytes element must surface in error.
let _, e = safe(fn() { return bytesConcat([strToBytes("ok"), "string-not-bytes", strToBytes("late")]) })
if e == null {
    println("FAIL: mixed-type array did not error")
    failures = failures + 1
} else if indexOf(e.message, "array[1]") < 0 {
    println("FAIL: error message missing index hint: " + e.message)
    failures = failures + 1
} else {
    println("ok: mixed-type array errored at correct index — " + e.message)
}

// 6. Wrong outer type → error.
_, e = safe(fn() { return bytesConcat("not an array") })
if e == null {
    println("FAIL: non-array outer did not error")
    failures = failures + 1
} else {
    println("ok: non-array outer errored cleanly — " + e.message)
}

// 7. Arity check.
_, e = safe(fn() { return bytesConcat() })
if e == null {
    println("FAIL: zero-arg did not error")
    failures = failures + 1
} else {
    println("ok: zero-arg errored cleanly — " + e.message)
}

// 8. Real use-case proof: round-trip a packed binary payload.
//    Build header (4 bytes) + 16-byte body, merge, then check the
//    full byte sequence by hex.
let hdr  = bytes([0x4B, 0x4C, 0x45, 0x58])   // "KLEX"
let body = bytes([0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15])
let pkt  = bytesConcat([hdr, body])
let hx   = bytesToHex(pkt)
if hx != "4b4c4558000102030405060708090a0b0c0d0e0f" {
    println("FAIL: round-trip hex = " + hx)
    failures = failures + 1
} else {
    println("ok: header+body round-trip via hex")
}

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — bytesConcat handles empty / single / many / mixed-type / wrong-type / arity / round-trip")
