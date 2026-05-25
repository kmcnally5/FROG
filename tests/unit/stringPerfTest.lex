// stringPerfTest.lex — guards against O(n²) regressions in string indexing
// and len(). The trigger case is json.parse on a ~400KB body (Forge's
// /sdapi/v1/txt2img response with an embedded base64 PNG). Before the lazy
// rune cache on *String, this parse took 1-2 minutes; the budget below
// fails the test if a future change brings the regression back.

import "stdlib/json.lex" as json

let failed = 0
let passed = 0

fn check(name, cond) {
    if cond {
        println("  PASS  " + name)
        passed = passed + 1
    } else {
        println("  FAIL  " + name)
        failed = failed + 1
    }
}

println("── string perf test ─────────────────────────────────────────────")

// Build a ~400KB JSON payload: { "images": ["<400KB of base64-like ASCII>"] }
// We don't need real base64 — any long ASCII string exercises the same
// O(n²) hazard in json.parse (per-char peek/next via s[i] + len(s)).
let chunk = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789AB"
let big   = chunk
let i     = 0
// 13 doublings = 64 * 2^13 = ~524KB of payload. Close to a real Forge response.
while i < 13 {
    big = big + big
    i = i + 1
}

// Build the JSON via json.stringify so we don't tangle with kLex's
// `"{…}"` interpolation syntax (which `chr(123)` would also dodge, but
// stringify is closer to what a real Forge response looks like).
let payload = json.stringify({"images": [big], "info": ""})
println("  payload size: " + str(len(payload)) + " chars")

let start = elapsedTime()
let parsed, err = json.parse(payload)
let dur = elapsedTime() - start
println("  parse time:   " + str(dur) + " seconds")

check("json.parse succeeded on big payload", err == null)
check("parse time under 2 seconds (was 60-120s with O(n²) indexing)", dur < 2.0)
check("parsed shape is correct",
    parsed != null && hasKey(parsed, "images") && len(parsed["images"]) == 1)
check("inner string round-trips byte-for-byte",
    parsed != null && parsed["images"][0] == big)

// Spot-check string indexing is O(1) after first access. Walk a ~100K-char
// string char-by-char — should finish in milliseconds, not seconds.
let walkStart = elapsedTime()
let acc = 0
let j   = 0
let n   = len(big)
while j < n {
    let c = big[j]
    if c == "A" { acc = acc + 1 }
    j = j + 1
}
let walkDur = elapsedTime() - walkStart
println("  walk time:    " + str(walkDur) + " seconds (" + str(n) + " indexings)")
check("indexing a big string is sub-second", walkDur < 1.0)
check("walk found expected 'A' count", acc > 0)

println("")
println("── result ──────────────────────────────────────────────────────")
println("  passed: " + str(passed))
println("  failed: " + str(failed))
if failed > 0 { _osExit(1) }
