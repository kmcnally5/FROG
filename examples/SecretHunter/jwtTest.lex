// jwtTest.lex — verifies jwt_bridge.js decode logic against hand-crafted
// fixtures covering each branch we care about.
//
// We can't easily mint signed tokens from kLex, but we don't need to —
// decoding is base64+JSON, signing is out of scope for the scanner. The
// fixtures below are real JWT-shaped strings with whatever header/payload we
// want; the signature segments are intentionally bogus.

let bridge, err = nativeBridge("node", ["examples/SecretHunter/jwt_bridge.js"])
if err != null {
    println("bridge failed to start: " + err.message)
    return
}
println("=== bridge started ===")

// 1. alg:none — the classic spec footgun (header decodes to {"alg":"none","typ":"JWT"})
let T_NONE = "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJhZG1pbiIsImlzcyI6ImFwaS5leGFtcGxlLmNvbSJ9.bogus"

// 2. HS256 — flagged as weak-alg shared-secret family
//    payload includes exp far in the past (Jan 2020 = 1577836800)
let T_EXPIRED = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJiYWNrdXAtdXNlciIsImlzcyI6ImNpLmV4YW1wbGUuY29tIiwic2NvcGUiOiJyZWFkIHdyaXRlIGFkbWluIiwiZXhwIjoxNTc3ODM2ODAwfQ.bogus"

// 3. RS256 with no exp — flagged missing_exp (no expiry = leaks live forever)
let T_NO_EXP = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJzdmMtYWNjb3VudCIsImlzcyI6Imluc2lkZS5leGFtcGxlLmNvbSIsImF1ZCI6ImFwaS1nYXRld2F5In0.bogus"

// 4. Malformed — only two segments, decode_batch must still produce a row
let T_BAD = "not.actually-a-jwt"

println("")
println("=== decode_batch ===")
let results, err = bridgeCall(bridge, "decode_batch", [[T_NONE, T_EXPIRED, T_NO_EXP, T_BAD]])
if err != null {
    println("decode_batch failed: " + err.message)
    bridgeClose(bridge)
    return
}

let ok = true

// (1) alg:none
let r0 = results[0]
println("[1] alg:none")
println("    alg:         " + r0["alg"])
println("    alg_warning: " + r0["alg_warning"])
println("    sub:         " + r0["sub"])
if r0["alg"] != "none" || r0["alg_warning"] == null {
    println("    FAIL: expected alg=none + warning")
    ok = false
}

// (2) HS256 expired with scopes
let r1 = results[1]
println("[2] HS256 expired")
println("    alg:         " + r1["alg"])
println("    alg_warning: " + r1["alg_warning"])
println("    expired:     " + str(r1["expired"]))
println("    scopes:      " + str(r1["scopes"]))
println("    exp_iso:     " + r1["exp_iso"])
if r1["alg"] != "HS256" || r1["expired"] != true {
    println("    FAIL: expected HS256 + expired=true")
    ok = false
}
if len(r1["scopes"]) != 3 {
    println("    FAIL: expected 3 scopes, got " + str(len(r1["scopes"])))
    ok = false
}

// (3) RS256 no exp
let r2 = results[2]
println("[3] RS256 missing exp")
println("    alg:         " + r2["alg"])
println("    missing_exp: " + str(r2["missing_exp"]))
println("    aud:         " + r2["aud"])
if r2["alg"] != "RS256" || r2["missing_exp"] != true {
    println("    FAIL: expected RS256 + missing_exp=true")
    ok = false
}
if r2["alg_warning"] != null {
    println("    FAIL: RS256 should not have alg_warning")
    ok = false
}

// (4) Malformed
let r3 = results[3]
println("[4] malformed input")
println("    error:       " + r3["error"])
if r3["error"] == null {
    println("    FAIL: expected error field populated")
    ok = false
}

println("")
if ok { println("OK") } else { println("FAILED") }

bridgeClose(bridge)
