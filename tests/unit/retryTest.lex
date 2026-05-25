import "stdlib/retry.lex" as retry

// --- do: succeeds on first try ---
let result, err = retry.do(fn() { return 42, null }, 3)
println(err == null)    // true
println(result)         // 42

// --- do: succeeds after failures ---
let attempts = 0
result, err = retry.do(fn() {
    attempts = attempts + 1
    if attempts < 3 { return null, "not yet" }
    return "done", null
}, 5)
println(err == null)    // true
println(result)         // done
println(attempts)       // 3

// --- do: exhausts all attempts ---
result, err = retry.do(fn() { return null, "always fails" }, 3)
println(err != null)    // true
println(err)            // always fails
println(result == null) // true

// --- doWithBackoff: succeeds after failures with delay=0 ---
let n = 0
result, err = retry.doWithBackoff(fn() {
    n = n + 1
    if n < 2 { return null, "fail" }
    return "ok", null
}, 4, 0)
println(err == null)    // true
println(result)         // ok
println(n)              // 2

// --- doWithBackoff: exhausts attempts ---
result, err = retry.doWithBackoff(fn() { return null, "nope" }, 2, 0)
println(err)            // nope
println(result == null) // true


// =============================================================================
// doWith — full-control retry with classifier, jitter, deadline
// Asserts here fail loudly on regression; the println-style suite above is
// preserved as-is for the v1 API.
// =============================================================================

println("")
println("=== doWith ===")


// 1. Success on first try — no retry, no sleep.
let cnt1 = {"n": 0}
let r, e = retry.doWith(fn() {
    cnt1["n"] = cnt1["n"] + 1
    return "ok", null
})
assert(e == null,        "doWith/success: err should be null")
assert(r == "ok",        "doWith/success: result should be 'ok'")
assert(cnt1["n"] == 1,   "doWith/success: should call f once")
println("  doWith/success: PASS")


// 2. Fatal error — returned immediately, no retry.
let cnt2 = {"n": 0}
r, e = retry.doWith(fn() {
    cnt2["n"] = cnt2["n"] + 1
    return null, error("ANTHROPIC_AUTH", "invalid api key")
})
assert(e != null,                     "doWith/fatal: err must be non-null")
assert(e.code == "ANTHROPIC_AUTH",    "doWith/fatal: should propagate original err")
assert(cnt2["n"] == 1,                "doWith/fatal: must not retry fatal codes")
println("  doWith/fatal: PASS")


// 3. Retryable then success.
let cnt3 = {"n": 0}
r, e = retry.doWith(fn() {
    cnt3["n"] = cnt3["n"] + 1
    if cnt3["n"] < 3 {
        return null, error("ANTHROPIC_RATE_LIMIT", "rate limit")
    }
    return "ok", null
}, {
    "maxAttempts": 5,
    "baseDelay":   1,
    "maxDelay":    5,
    "jitter":      false,
})
assert(e == null,        "doWith/recover: err should be null")
assert(r == "ok",        "doWith/recover: result should be 'ok'")
assert(cnt3["n"] == 3,   "doWith/recover: should call f 3 times")
println("  doWith/recover: PASS")


// 4. Retryable exhausts attempts → RETRY_EXHAUSTED.
let cnt4 = {"n": 0}
r, e = retry.doWith(fn() {
    cnt4["n"] = cnt4["n"] + 1
    return null, error("OLLAMA_SERVER", "boom")
}, {
    "maxAttempts": 3,
    "baseDelay":   1,
    "maxDelay":    2,
    "jitter":      false,
})
assert(e != null,                     "doWith/exhaust: err must be non-null")
assert(e.code == "RETRY_EXHAUSTED",   "doWith/exhaust: expected RETRY_EXHAUSTED, got " + e.code)
assert(cnt4["n"] == 3,                "doWith/exhaust: should call f exactly 3 times")
println("  doWith/exhaust: PASS")


// 5. Unknown error code — defaults to fatal (don't burn credits).
let cnt5 = {"n": 0}
r, e = retry.doWith(fn() {
    cnt5["n"] = cnt5["n"] + 1
    return null, error("WEIRD_THING", "nope")
})
assert(e != null,                "doWith/unknown: err must be non-null")
assert(e.code == "WEIRD_THING",  "doWith/unknown: should propagate original err code")
assert(cnt5["n"] == 1,           "doWith/unknown: must not retry unknown codes")
println("  doWith/unknown: PASS")


// 6. Custom classifier — retries everything; recovers on second try.
let cnt6 = {"n": 0}
r, e = retry.doWith(fn() {
    cnt6["n"] = cnt6["n"] + 1
    if cnt6["n"] < 2 {
        return null, error("WEIRD_THING", "transient-ish")
    }
    return "ok", null
}, {
    "maxAttempts": 4,
    "baseDelay":   1,
    "maxDelay":    2,
    "jitter":      false,
    "isRetryable": fn(e2) { return true },
})
assert(e == null,        "doWith/custom: err should be null")
assert(r == "ok",        "doWith/custom: result should be 'ok'")
assert(cnt6["n"] == 2,   "doWith/custom: classifier should drive retry")
println("  doWith/custom: PASS")


// 7. onRetry callback fires once per backoff (N-1 times for N attempts).
let cnt7    = {"n": 0}
let cbCnt7  = {"n": 0}
r, e = retry.doWith(fn() {
    cnt7["n"] = cnt7["n"] + 1
    return null, error("ANTHROPIC_RATE_LIMIT", "still rate-limited")
}, {
    "maxAttempts": 3,
    "baseDelay":   1,
    "maxDelay":    2,
    "jitter":      false,
    "onRetry":     fn(att, e2, dms) {
        cbCnt7["n"] = cbCnt7["n"] + 1
    },
})
assert(e.code == "RETRY_EXHAUSTED",   "doWith/onRetry: should exhaust")
assert(cnt7["n"] == 3,                "doWith/onRetry: should call f 3 times")
assert(cbCnt7["n"] == 2,              "doWith/onRetry: callback should fire 2 times")
println("  doWith/onRetry: PASS")


// 8. defaultClassifier — direct matrix check.
assert(retry.defaultClassifier(error("ANTHROPIC_RATE_LIMIT", "x")) == true,
       "defaultClassifier: ANTHROPIC_RATE_LIMIT must be retryable")
assert(retry.defaultClassifier(error("OLLAMA_SERVER", "x")) == true,
       "defaultClassifier: OLLAMA_SERVER must be retryable")
assert(retry.defaultClassifier(error("VOYAGE_TIMEOUT", "x")) == true,
       "defaultClassifier: VOYAGE_TIMEOUT must be retryable")
assert(retry.defaultClassifier(error("ANTHROPIC_OVERLOADED", "x")) == true,
       "defaultClassifier: ANTHROPIC_OVERLOADED must be retryable")
assert(retry.defaultClassifier(error("ANTHROPIC_AUTH", "x")) == false,
       "defaultClassifier: ANTHROPIC_AUTH must be fatal")
assert(retry.defaultClassifier(error("OLLAMA_NOT_FOUND", "x")) == false,
       "defaultClassifier: OLLAMA_NOT_FOUND must be fatal")
assert(retry.defaultClassifier(error("ANTHROPIC_BAD_REQUEST", "x")) == false,
       "defaultClassifier: ANTHROPIC_BAD_REQUEST must be fatal")
assert(retry.defaultClassifier(error("WHATEVER", "x")) == false,
       "defaultClassifier: unknown codes default to fatal")
assert(retry.defaultClassifier(null) == false,
       "defaultClassifier: null err must be non-retryable")
println("  doWith/defaultClassifier: PASS")


// 9. Deadline — bails with RETRY_DEADLINE before exhausting attempts.
let cnt9 = {"n": 0}
r, e = retry.doWith(fn() {
    cnt9["n"] = cnt9["n"] + 1
    return null, error("ANTHROPIC_RATE_LIMIT", "still")
}, {
    "maxAttempts": 20,        // plenty of attempts available…
    "baseDelay":   50,
    "maxDelay":    50,
    "deadline":    30,        // …but only 30ms total
    "jitter":      false,
})
assert(e != null,                     "doWith/deadline: err must be non-null")
assert(e.code == "RETRY_DEADLINE",    "doWith/deadline: expected RETRY_DEADLINE, got " + e.code)
assert(cnt9["n"] >= 1,                "doWith/deadline: should run at least once")
assert(cnt9["n"] < 20,                "doWith/deadline: must not run all 20 attempts")
println("  doWith/deadline: PASS  (attempts before trip: " + str(cnt9["n"]) + ")")


println("")
println("ALL RETRY TESTS PASSED")
