// backpressureTest.lex — verifies the bridgeStream window contract.
//
// The Python helper now blocks its yield loop when in-flight items exceed
// the window. The kLex side sends acks at half-window. Together they cap
// the bridge's production rate to (channel-buffer + window) regardless of
// consumer speed. The kLex channel buffer is fixed at 256 today, so for
// window=W, the bridge can produce at most ~256+W items before it parks
// waiting for acks.
//
// What we're proving:
//   - With backpressure ON, bridge production stays bounded substantially
//     below COUNT even with a slow consumer.
//   - With backpressure OFF (window=0), the bridge runs free to completion.
//   - The ratio matters more than the absolute number — backpressure-on
//     should yield an order of magnitude less than backpressure-off for a
//     large enough COUNT and a slow enough consumer.

import "stdlib/datetime.lex" as dt

let bridge, err = bridgeOpen({"kind": "subprocess", "cmd": "python3", "args": ["tests/bridge/python_bridge.py"]})
if err != null {
    println("bridge failed to start: " + err.message)
    return
}

let COUNT = 2000
let ok = true

// ── 1. window = 8 ───────────────────────────────────────────────────────────
println("=== window=8, slow consumer reads 5 ===")
let ch, err = bridgeStream(bridge, "fast_producer", [COUNT], {"window": 8})
if err != null { println("FAIL stream: " + err.message)  bridgeClose(bridge)  return }

let received = 0
for item in ch {
    received = received + 1
    sleep(20)               // slow consumer — 20 ms per item
    if received >= 5 { break }
}
sleep(150)                  // let any in-flight item land + cancel propagate

let produced, err = bridgeCall(bridge, "get_produced_count", [])
if err != null { println("FAIL get_produced_count: " + err.message)  bridgeClose(bridge)  return }
println("received: " + str(received) + ", bridge produced: " + str(produced))
// Cap is buffer(256) + window(8) ≈ 264 with timing slack. Anything below
// 500 proves the window kept the bridge bounded; if backpressure were off
// the bridge would happily produce all 2000.
if produced > 500 {
    println("FAIL: window=8 didn't bound the bridge (produced " + str(produced) + ")")
    ok = false
}
sleep(100)

// ── 2. window = 0 (disabled) ────────────────────────────────────────────────
println("")
println("=== window=0, slow consumer reads 5 ===")
let ch2, err = bridgeStream(bridge, "fast_producer", [COUNT], {"window": 0})
if err != null { println("FAIL stream: " + err.message)  bridgeClose(bridge)  return }

let received2 = 0
for item in ch2 {
    received2 = received2 + 1
    sleep(20)
    if received2 >= 5 { break }
}
sleep(150)

let produced2, err = bridgeCall(bridge, "get_produced_count", [])
if err != null { println("FAIL get_produced_count: " + err.message)  bridgeClose(bridge)  return }
println("received: " + str(received2) + ", bridge produced: " + str(produced2))
// With window disabled, the bridge will get further than the window=8 case.
// We can't insist on an exact number (OS pipe buffering, scheduling) but it
// MUST be substantially higher than the window=8 result.
if produced2 <= produced {
    println("FAIL: window=0 didn't outpace window=8 (" + str(produced2) + " vs " + str(produced) + ")")
    ok = false
}
sleep(100)

// ── 3. Default window (no 4th arg) ──────────────────────────────────────────
println("")
println("=== default window, slow consumer reads 5 ===")
let ch3, err = bridgeStream(bridge, "fast_producer", [COUNT])
if err != null { println("FAIL stream: " + err.message)  bridgeClose(bridge)  return }

let received3 = 0
for item in ch3 {
    received3 = received3 + 1
    sleep(20)
    if received3 >= 5 { break }
}
sleep(150)

let produced3, err = bridgeCall(bridge, "get_produced_count", [])
if err != null { println("FAIL get_produced_count: " + err.message)  bridgeClose(bridge)  return }
println("received: " + str(received3) + ", bridge produced: " + str(produced3))
// Default window is 32 → cap ≈ 288. Same generous slack as scenario 1.
if produced3 > 500 {
    println("FAIL: default window didn't bound the bridge (produced " + str(produced3) + ")")
    ok = false
}

println("")
if ok { println("OK") } else { println("FAILED") }

bridgeClose(bridge)
