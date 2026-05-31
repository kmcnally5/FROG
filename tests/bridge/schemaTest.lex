// schemaTest.lex — Phase 3 end-to-end verification.
//
// Confirms the whole pipeline:
//   1. nativeBridge sets PYTHONPATH so klex_bridge resolves
//   2. The __schema__ handshake populates b.schemas during nativeBridge
//   3. bridgeSchema() returns the cached map
//   4. bridgeCall validates args against the schema before marshal,
//      surfacing BRIDGE_SCHEMA_ARG errors at the call site

let bridge, err = bridgeOpen({"kind": "subprocess", "cmd": "python3", "args": ["tests/bridge/python_bridge.py"]})
if err != null {
    println("bridge failed to start: " + err.message)
    return
}

println("=== bridgeSchema(bridge) ===")
let schemas = bridgeSchema(bridge)
if schemas == null {
    println("ERROR: schemas not populated — handshake must have failed")
    bridgeClose(bridge)
    return
}
for k in keys(schemas) {
    let s = schemas[k]
    println(k + ":")
    println("  args:    " + str(s["args"]))
    println("  returns: " + str(s["returns"]))
}

println("")
println("=== bridgeSchema(bridge, fnName) ===")
let s = bridgeSchema(bridge, "stats")
println("stats: " + str(s))

s = bridgeSchema(bridge, "no_such_fn")
println("no_such_fn: " + str(s))

println("")
println("=== well-typed calls ===")

let r, _ = bridgeCall(bridge, "add", [2, 3])
println("add(2, 3) = " + str(r))

r, _ = bridgeCall(bridge, "stats", [[1, 2, 3, 4, 5]])
println("stats([1..5]) = " + str(r))

r, _ = bridgeCall(bridge, "primes_up_to", [20])
println("primes_up_to(20) = " + str(r))

println("")
println("=== kLex-side schema rejection (BRIDGE_SCHEMA_ARG) ===")

// 'name' is declared "string" — passing an int should be rejected.
r, err = bridgeCall(bridge, "greet", [42])
if err != null && err.code == "BRIDGE_SCHEMA_ARG" {
    println("✓ " + err.message)
} else {
    println("ERROR: expected BRIDGE_SCHEMA_ARG, got " + str(err))
}

// 'a' is declared "int" — passing a string should be rejected.
r, err = bridgeCall(bridge, "add", ["two", 3])
if err != null && err.code == "BRIDGE_SCHEMA_ARG" {
    println("✓ " + err.message)
} else {
    println("ERROR: expected BRIDGE_SCHEMA_ARG, got " + str(err))
}

// Wrong arg count — multiply expects 2.
r, err = bridgeCall(bridge, "multiply", [5])
if err != null && err.code == "BRIDGE_SCHEMA_ARG" {
    println("✓ " + err.message)
} else {
    println("ERROR: expected BRIDGE_SCHEMA_ARG, got " + str(err))
}

println("")
println("=== return-type validation (Python-side, surfaces as BRIDGE_ERROR) ===")

// lies_about_return declares returns="int" but actually returns a string.
// The Python helper's _validate_return catches this before the response is
// sent. kLex therefore sees an ordinary error response (BRIDGE_ERROR).
// (Hand-rolled bridges that bypass klex_bridge would instead trip the
// kLex-side BRIDGE_SCHEMA_RETURN check after the bad value arrives.)
r, err = bridgeCall(bridge, "lies_about_return", [])
if err != null && indexOf(err.message, "return value") >= 0 {
    println("✓ " + err.message)
} else {
    println("ERROR: expected return-value mismatch, got " + str(err))
}

println("")
println("=== structured errors (err.errorType + err.traceback) ===")

// open_missing tries to read a path that doesn't exist. The Python helper
// catches the FileNotFoundError, wraps it, and sends error_type +
// traceback alongside the message. kLex's err object exposes both.
r, err = bridgeCall(bridge, "open_missing", ["/no/such/path/exists/here"])
if err == null {
    println("ERROR: expected the call to fail")
} else {
    println("✓ err.code:       " + err.code)
    println("✓ err.errorType:  " + err.errorType)
    println("✓ err.message:    " + err.message)
    // Traceback is multi-line; just show the first line as proof it's there.
    let tb = err.traceback
    let nl = indexOf(tb, "\n")
    if nl < 0 { nl = len(tb) }
    println("✓ err.traceback first line: " + substr(tb, 0, nl))
}

bridgeClose(bridge)
println("")
println("bridge closed cleanly")
