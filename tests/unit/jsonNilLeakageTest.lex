// jsonNilLeakageTest.lex — proves OFI #8 (JSON values appearing as
// Go-nil vs kLex *Null) is closed by the new Go-side parser (OFI #14).
//
// Original bug: hash values retrieved via evt["key"] sometimes flowed
// through subsequent ops as Go-nil, triggering nil-pointer derefs.
// The fix lives in jsonValueToKLex / kLexToJSON in builtins_json.go —
// every code path returns NULL (the singleton *Null), never a raw nil.
//
// This test exercises every common nil-yielding scenario and verifies
// the result is comparable to null, is type "NULL", and survives
// downstream ops without crashing.
//
// Run with: ./klex tests/unit/jsonNilLeakageTest.lex
// Exit 0 on all-pass.

import "stdlib/json.lex" as json

let failures = 0

// kLex `"{…}"` is string interpolation; literal { needs chr(123).
let lb = chr(123)
let rb = chr(125)
let dq = chr(34)

fn check(label, ok) {
    if ok { println("ok: " + label) }
    else  { println("FAIL: " + label) }
}

// ── 1. Explicit JSON null parses to NULL singleton ──────────────────
let v, _ = json.parse("null")
check("explicit null parses to NULL", v == null && type(v) == "NULL")
if v != null || type(v) != "NULL" { failures = failures + 1 }

// ── 2. Object value of null is NULL ────────────────────────────────
v, _ = json.parse(lb + dq + "k" + dq + ": null" + rb)
let val = v["k"]
check("object value null is NULL", val == null && type(val) == "NULL")
if val != null || type(val) != "NULL" { failures = failures + 1 }

// ── 3. Array element of null is NULL ───────────────────────────────
v, _ = json.parse("[1, null, 3]")
check("array element null is NULL",
    v[1] == null && type(v[1]) == "NULL" &&
    v[0] == 1 && v[2] == 3)
if v[1] != null || type(v[1]) != "NULL" { failures = failures + 1 }

// ── 4. Missing hash key returns NULL singleton ─────────────────────
//      (separate from JSON — exercises the IndexExpr Hash NULL path)
let h = {"x": 1}
let miss = h["__not_a_key__"]
check("missing hash key returns NULL", miss == null && type(miss) == "NULL")
if miss != null || type(miss) != "NULL" { failures = failures + 1 }

// ── 5. Chained access through nulls is safe ─────────────────────────
// Old bug: deeply nested null could cause Go-nil to leak.
// `data` has a null at "missing" — chained operations on that null
// must not crash.
// {"a": {"b": null}}
v, _ = json.parse(lb + dq + "a" + dq + ":" + lb + dq + "b" + dq + ": null" + rb + rb)
let inner = v["a"]
let deeper = inner["b"]
check("chained access a-b-null: inner is HASH",
    type(inner) == "HASH")
check("chained access a-b-null: deeper is NULL",
    deeper == null && type(deeper) == "NULL")
if type(inner) != "HASH" || deeper != null { failures = failures + 1 }

// `inner` lookup of a missing key — even safer than the JSON one.
let even = inner["nope"]
check("missing hash key returns NULL (deep)", even == null)
if even != null { failures = failures + 1 }

// ── 6. Equality with null works in every direction ─────────────────
v, _ = json.parse("null")
check("null == null", v == null)
check("null == v",    null == v)
check("v != 0",       v != 0)
check("v != \"\"",    v != "")
if v != null { failures = failures + 1 }

// ── 7. Round-trip null preserves nullness ──────────────────────────
let orig = {"empty": null, "filled": 42}
let s = json.stringify(orig)
v, _ = json.parse(s)
check("round-trip stringify+parse preserves null",
    v["empty"] == null && type(v["empty"]) == "NULL" && v["filled"] == 42)
if v["empty"] != null || v["filled"] != 42 { failures = failures + 1 }

// Stringified form should literally contain "null".
if indexOf(s, "null") < 0 {
    println("FAIL: stringified form missing literal null: " + s)
    failures = failures + 1
} else {
    println("ok: stringify(empty-null) → '" + s + "'")
}

// ── 8. Forwarding null through user code doesn't crash ─────────────
// Pass the parsed null through map/filter — the bug used to manifest
// as a deeper-in-the-evaluator nil-pointer panic when downstream ops
// processed it.
v, _ = json.parse("[null, 1, null, 2, null]")
let plus1 = map(v, fn(x) {
    if x == null { return 0 }
    return x + 1
})
let expected_nonnull = 0 + 2 + 0 + 3 + 0   // nulls -> 0, 1+1=2, 2+1=3
let sum = 0
let i = 0
while i < len(plus1) { sum = sum + plus1[i]   i = i + 1 }
if sum != expected_nonnull {
    println("FAIL: map over nulls sum=" + str(sum) + " expected " + str(expected_nonnull))
    failures = failures + 1
} else {
    println("ok: map over [null, 1, null, 2, null] sums to " + str(sum) + " — no nil-deref")
}

// ── 9. hasKey distinguishes "absent" from "present-but-null" ───────
// {"present_null": null, "present_int": 0}
v, _ = json.parse(lb + dq + "present_null" + dq + ": null," +
                       dq + "present_int" + dq + ": 0" + rb)
check("hasKey on present-but-null is TRUE", hasKey(v, "present_null"))
check("hasKey on present-int-zero is TRUE", hasKey(v, "present_int"))
check("hasKey on absent is FALSE",          !hasKey(v, "completely_absent"))
if !hasKey(v, "present_null") || hasKey(v, "completely_absent") {
    failures = failures + 1
}

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — null/NULL singleton is consistent across JSON parse, hash lookup, round-trip (OFI #8 — closed by #14)")
