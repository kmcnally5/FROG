// schemaTest.lex — Phase 3 end-to-end verification.
//
// Confirms the whole pipeline:
//   1. nativeBridge sets PYTHONPATH so klex_bridge resolves
//   2. The __schema__ handshake populates b.schemas during nativeBridge
//   3. bridgeSchema() returns the cached map
//   4. bridgeCall validates args against the schema before marshal,
//      surfacing BRIDGE_SCHEMA_ARG errors at the call site

bridge, err = nativeBridge("python3", ["tests/examples/bridge/python_bridge.py"])
if err != null {
    println("bridge failed to start: " + err.message)
    return
}

println("=== bridgeSchema(bridge) ===")
schemas = bridgeSchema(bridge)
if schemas == null {
    println("ERROR: schemas not populated — handshake must have failed")
    bridgeClose(bridge)
    return
}
for k in keys(schemas) {
    s = schemas[k]
    println(k + ":")
    println("  args:    " + str(s["args"]))
    println("  returns: " + str(s["returns"]))
}

println("")
println("=== bridgeSchema(bridge, fnName) ===")
s = bridgeSchema(bridge, "stats")
println("stats: " + str(s))

s = bridgeSchema(bridge, "no_such_fn")
println("no_such_fn: " + str(s))

println("")
println("=== well-typed calls ===")

r, _ = bridgeCall(bridge, "add", [2, 3])
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

bridgeClose(bridge)
println("")
println("bridge closed cleanly")
