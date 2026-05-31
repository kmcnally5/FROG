// nodeTest.lex — verifies the Node helper (klex_bridge.js) is at parity with
// the Python helper across the bridge contract:
//
//   1. nativeBridge + schema handshake — bridge starts, __schema__ resolves
//   2. bridgeCall with arg + return validation
//   3. Streaming: sync generator (count_from) and async generator
//      (broken_stream) — same channel semantics, same mid-stream error path
//   4. Cancellation: break out of a slow stream, verify the bridge stops
//   5. Structured errors: errorType + traceback survive the wire
//   6. Return-type validation: handler that lies about its return type
//
// If this passes for Node it passes the same suite that schemaTest +
// streamTest + cancelTest cover for Python. One file because each section
// is short.

let bridge, err = bridgeOpen({"kind": "subprocess", "cmd": "node", "args": ["tests/bridge/node_bridge.js"]})
if err != null {
    println("bridge failed to start: " + err.message)
    return
}
println("=== bridge started ===")

// ── basic call ───────────────────────────────────────────────────────────────
let sum, err = bridgeCall(bridge, "add", [2, 3])
if err != null { println("add failed: " + err.message)  bridgeClose(bridge)  return }
println("add(2, 3) = " + str(sum))

let greet, err = bridgeCall(bridge, "greet", ["Karl"])
if err != null { println("greet failed: " + err.message)  bridgeClose(bridge)  return }
println("greet: " + greet)

// ── arg-validation rejection ─────────────────────────────────────────────────
println("")
println("=== arg validation ===")
_, err = bridgeCall(bridge, "add", ["nope", 3])
if err == null || err.code != "BRIDGE_SCHEMA_ARG" {
    println("FAIL: expected BRIDGE_SCHEMA_ARG, got " + str(err))
} else {
    println("✓ rejected bad arg: " + err.message)
}

// ── return-type validation (Python lies_about_return parity) ────────────────
println("")
println("=== return-type validation ===")
_, err = bridgeCall(bridge, "lies_about_return", [])
if err == null {
    println("FAIL: expected an error from lies_about_return")
} else {
    println("✓ " + err.message)
}

// ── structured error fields ──────────────────────────────────────────────────
println("")
println("=== structured errors ===")
_, err = bridgeCall(bridge, "open_missing", ["/no/such/path"])
if err == null {
    println("FAIL: expected open_missing to error")
} else {
    println("✓ err.code:      " + err.code)
    println("✓ err.errorType: " + err.errorType)
    if len(err.traceback) > 0 {
        println("✓ traceback present (" + str(len(err.traceback)) + " chars)")
    } else {
        println("FAIL: traceback empty")
    }
}

// ── sync generator stream ────────────────────────────────────────────────────
println("")
println("=== sync generator stream ===")
let ch, err = bridgeStream(bridge, "count_from", [10, 5])
if err != null { println("stream failed: " + err.message)  bridgeClose(bridge)  return }
let buf = makeArray(16, null)
let idx = 0
for item in ch {
    buf[idx] = item
    idx = idx + 1
}
let items = slice(buf, 0, idx)
println("collected: " + str(items))    // expect [10, 11, 12, 13, 14]

// ── async generator stream with mid-stream error ─────────────────────────────
println("")
println("=== async generator with mid-stream error ===")
ch, err = bridgeStream(bridge, "broken_stream", [])
if err != null { println("stream failed: " + err.message)  bridgeClose(bridge)  return }
let itemCount = 0
let sawError  = false
for item in ch {
    if type(item) == "ERROR" {
        sawError = true
        println("✓ mid-stream error: " + item.message)
        println("  errorType: " + item.errorType)
    } else {
        itemCount = itemCount + 1
    }
}
if !sawError { println("FAIL: expected an error item") }
println(str(itemCount) + " items before error (expected 2)")

// ── streaming cancel ─────────────────────────────────────────────────────────
println("")
println("=== streaming cancel ===")
ch, err = bridgeStream(bridge, "cancel_demo", [50, 100])
if err != null { println("stream failed: " + err.message)  bridgeClose(bridge)  return }
let received = 0
for item in ch {
    received = received + 1
    if received >= 3 { break }
}
sleep(200)
let produced, err = bridgeCall(bridge, "get_cancel_count", [])
if err != null { println("get_cancel_count failed: " + err.message)  bridgeClose(bridge)  return }
println("received: " + str(received) + ", bridge yielded: " + str(produced))
if produced > 10 {
    println("FAIL: cancel did not propagate — bridge ran on past the break")
} else {
    println("✓ cancel stopped the bridge within " + str(produced) + " items")
}

bridgeClose(bridge)
println("")
println("bridge closed cleanly")
