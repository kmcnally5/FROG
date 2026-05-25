// helloTest.lex — verifies the __hello__ protocol negotiation handshake.
//
// Three scenarios:
//   1. Python helper bridge → protocol 1, capabilities include "schema",
//      helper identity fields populated.
//   2. Node helper bridge   → same negotiated set, language reports "node".
//   3. Legacy bridge (old_bridge.py, no __hello__ handler) → protocol 0,
//      capabilities empty, helper fields empty. Proves the negotiation
//      degrades gracefully and pre-handshake bridges keep working.
//
// bridgeInfo() is the introspection builtin under test. It returns:
//   { protocol, capabilities, helper, language, language_version }

println("=== python helper bridge ===")
let bridge, err = nativeBridge("python3", ["tests/examples/bridge/python_bridge.py"])
if err != null {
    println("FAIL: python bridge failed to start: " + err.message)
    return
}
let info = bridgeInfo(bridge)
println("protocol:         " + str(info["protocol"]))
println("capabilities:     " + str(info["capabilities"]))
println("helper:           " + info["helper"])
println("language:         " + info["language"])
println("language_version: " + info["language_version"])

if info["protocol"] != 1 {
    println("FAIL: expected protocol 1, got " + str(info["protocol"]))
}
if info["language"] != "python" {
    println("FAIL: expected language=python, got " + info["language"])
}
let hasSchema = false
for cap in info["capabilities"] {
    if cap == "schema" { hasSchema = true }
}
if !hasSchema {
    println("FAIL: expected 'schema' capability")
} else {
    println("OK python negotiation")
}

// Confirm an ordinary bridgeCall still works after the new handshake — the
// __hello__ exchange must not disturb the in-flight id pool or break the
// schema fetch that follows it.
let sum, err = bridgeCall(bridge, "add", [2, 3])
if err != null || sum != 5 {
    println("FAIL: add(2,3) after hello broken: " + str(err))
} else {
    println("OK add(2,3) = 5 after hello")
}
bridgeClose(bridge)

println("")
println("=== node helper bridge ===")
bridge, err = nativeBridge("node", ["tests/examples/bridge/node_bridge.js"])
if err != null {
    println("FAIL: node bridge failed to start: " + err.message)
    return
}
info = bridgeInfo(bridge)
println("protocol:         " + str(info["protocol"]))
println("capabilities:     " + str(info["capabilities"]))
println("helper:           " + info["helper"])
println("language:         " + info["language"])
println("language_version: " + info["language_version"])

if info["protocol"] != 1 {
    println("FAIL: expected protocol 1, got " + str(info["protocol"]))
}
if info["language"] != "node" {
    println("FAIL: expected language=node, got " + info["language"])
}
hasSchema = false
for cap in info["capabilities"] {
    if cap == "schema" { hasSchema = true }
}
if !hasSchema {
    println("FAIL: expected 'schema' capability")
} else {
    println("OK node negotiation")
}
bridgeClose(bridge)

println("")
println("=== legacy bridge (no __hello__) ===")
bridge, err = nativeBridge("python3", ["tests/examples/bridge/old_bridge.py"])
if err != null {
    println("FAIL: legacy bridge failed to start: " + err.message)
    return
}
info = bridgeInfo(bridge)
println("protocol:         " + str(info["protocol"]))
println("capabilities:     " + str(info["capabilities"]))
println("helper:           '" + info["helper"] + "'")
println("language:         '" + info["language"] + "'")

if info["protocol"] != 0 {
    println("FAIL: legacy bridge should be protocol 0, got " + str(info["protocol"]))
}
if len(info["capabilities"]) != 0 {
    println("FAIL: legacy bridge should have no capabilities, got " + str(info["capabilities"]))
} else {
    println("OK legacy bridge reports protocol 0, no capabilities")
}

// Sanity: legacy bridges must still respond to ordinary calls.
sum, err = bridgeCall(bridge, "add", [7, 8])
if err != null || sum != 15 {
    println("FAIL: legacy add(7,8) broken: " + str(err))
} else {
    println("OK legacy add(7,8) = 15")
}
bridgeClose(bridge)

println("")
println("hello negotiation tests complete")
