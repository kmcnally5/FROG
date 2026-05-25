// vmDefaultsTest.lex — locks in VM parameter-default support.
//
// Until tonight's fix, the VM compiler ignored FunctionLiteral.Defaults
// and treated every declared param as required, so `fn foo(a, b = null)`
// called as `foo(1)` would arity-error under --vm even though the
// tree-walker handled it fine. That's what made `claude.messages` in
// tadPole fail to dispatch through `stdlib/rest.lex:request(..., timeoutSec = null)`
// on the Enhance-with-Claude button.
//
// This test exercises constant-literal defaults — the supported set
// for Phase 1 of VM defaults. Non-constant defaults (e.g. `arr = []`)
// remain a compile-time error with a clear migration message.

import "assert.lex" as t

// ── 1. null default — the case that broke tadPole ──────────────────────
fn withNullDefault(a, b = null) {
    if b == null { return a }
    return a + b
}

t.assertEqual(withNullDefault(5), 5)         // omit → b is null → return a
t.assertEqual(withNullDefault(5, 10), 15)    // pass → b is 10 → return sum

// ── 2. integer default ──────────────────────────────────────────────────
fn withIntDefault(a, b = 100) {
    return a + b
}

t.assertEqual(withIntDefault(5), 105)        // 5 + 100
t.assertEqual(withIntDefault(5, 1), 6)       // 5 + 1

// ── 3. float, string, bool, bytes defaults ──────────────────────────────
fn withFloatDefault(x, scale = 2.5) {
    return x * scale
}
t.assertEqual(withFloatDefault(10), 25.0)
t.assertEqual(withFloatDefault(10, 1.0), 10.0)

fn withStringDefault(name, greeting = "hello") {
    return greeting + ", " + name
}
t.assertEqual(withStringDefault("karl"), "hello, karl")
t.assertEqual(withStringDefault("karl", "hi"), "hi, karl")

fn withBoolDefault(verbose = false) {
    if verbose { return "loud" }
    return "quiet"
}
t.assertEqual(withBoolDefault(), "quiet")
t.assertEqual(withBoolDefault(true), "loud")

// ── 4. Multiple trailing defaults — exercise the [NumRequired, NumParams] range
fn manyDefaults(a, b = 10, c = 20, d = 30) {
    return a + b + c + d
}

t.assertEqual(manyDefaults(1), 61)           // 1 + 10 + 20 + 30
t.assertEqual(manyDefaults(1, 2), 53)        // 1 + 2 + 20 + 30
t.assertEqual(manyDefaults(1, 2, 3), 36)     // 1 + 2 + 3 + 30
t.assertEqual(manyDefaults(1, 2, 3, 4), 10)  // 1 + 2 + 3 + 4

// ── 5. Arity errors at the bounds ──────────────────────────────────────
//    Too few → error; too many → error. The message should report the
//    "N to M arguments" range when defaults are in play.
let _r, tooFewErr = safe(fn() {
    return manyDefaults()    // no args at all — needs >= 1
})
t.assertTrue(tooFewErr != null)

let _r2, tooManyErr = safe(fn() {
    return manyDefaults(1, 2, 3, 4, 5)  // 5 args — max is 4
})
t.assertTrue(tooManyErr != null)

// ── 6. Default-eval inside a body — works across closures ───────────────
//    The "fn(x = null) { ... }" pattern is what the stdlib uses heavily.
fn buildOpts(name, timeoutSec = null) {
    let opts = {"name": name}
    if timeoutSec != null {
        opts["timeout"] = timeoutSec
    }
    return opts
}

let o1 = buildOpts("call_a")
t.assertEqual(hasKey(o1, "timeout"), false)
let o2 = buildOpts("call_b", 30)
t.assertEqual(o2["timeout"], 30)

t.summary()
