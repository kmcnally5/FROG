// streamTest.lex — verifies the bridgeStream() path end-to-end.
//
// Exercises:
//   - Streaming handler returning a clean sequence of items
//   - Streaming handler raising mid-stream (error delivered as final item)
//   - Type-mismatch check (calling a non-stream handler via bridgeStream)
//   - Type-mismatch check (calling a stream handler via bridgeCall)

let bridge, err = nativeBridge("python3", ["tests/examples/bridge/python_bridge.py"])
if err != null {
    println("bridge failed to start: " + err.message)
    return
}

println("=== happy path: count_from(10, 5) ===")
let ch, err = bridgeStream(bridge, "count_from", [10, 5])
if err != null {
    println("bridgeStream failed: " + err.message)
    bridgeClose(bridge)
    return
}
let buf = makeArray(16, null)
let idx = 0
for item in ch {
    buf[idx] = item
    idx = idx + 1
}
let items = slice(buf, 0, idx)
println("collected: " + str(items))   // expected [10, 11, 12, 13, 14]

println("")
println("=== mid-stream error: broken_stream() ===")
ch, err = bridgeStream(bridge, "broken_stream", [])
if err != null {
    println("bridgeStream failed: " + err.message)
    bridgeClose(bridge)
    return
}
let itemCount = 0
let sawError  = false
for item in ch {
    if type(item) == "ERROR" {
        sawError = true
        println("✓ mid-stream error received: " + item.message)
        println("  errorType: " + item.errorType)
    } else {
        itemCount = itemCount + 1
        println("  item: " + str(item))
    }
}
if !sawError {
    println("ERROR: expected to see a mid-stream error")
}
println("got " + str(itemCount) + " items before error (expected 2)")

println("")
println("=== mismatch: stream handler called via bridgeCall ===")
let r, err = bridgeCall(bridge, "count_from", [0, 3])
if err != null && indexOf(err.message, "bridgeStream") >= 0 {
    println("✓ rejected: " + err.message)
} else {
    println("ERROR: expected rejection, got " + str(err) + " / " + str(r))
}

println("")
println("=== mismatch: non-stream handler called via bridgeStream ===")
ch, err = bridgeStream(bridge, "add", [1, 2])
if err == null {
    // The stream call itself succeeded (bridge accepted the request).
    // The error arrives via the channel as the first item.
    for item in ch {
        if type(item) == "ERROR" {
            println("✓ rejected via channel: " + item.message)
            break
        }
    }
} else {
    println("✓ rejected at call site: " + err.message)
}

bridgeClose(bridge)
println("")
println("bridge closed cleanly")
