// cancelTest.lex — verifies that breaking out of a for-in over a bridge
// stream now sends {"cancel": N} to the bridge and stops it producing.
//
// Before: the bridge kept yielding items into a closed channel until natural
// end — wasted work. After: the bridge sees the cancel within one iteration
// and exits cleanly. The test proves it by:
//   1. Starting a slow stream (50 items × 100ms — 5 seconds full duration).
//   2. Reading 3 items, then breaking.
//   3. Asking the bridge how many items it actually yielded. Without cancel:
//      50. With cancel: a small handful (the 3 consumed plus any in flight).

import "stdlib/datetime.lex" as dt

let bridge, err = bridgeOpen({"kind": "subprocess", "cmd": "python3", "args": ["tests/bridge/python_bridge.py"]})
if err != null {
    println("bridge failed to start: " + err.message)
    return
}

println("=== streaming cancel ===")
let ch, err = bridgeStream(bridge, "cancel_demo", [50, 100])
if err != null {
    println("bridgeStream failed: " + err.message)
    bridgeClose(bridge)
    return
}

let start    = dt.nowNanos()
let received = 0
for item in ch {
    received = received + 1
    if received >= 3 { break }
}
let elapsedMs = (dt.nowNanos() - start) / 1000000

println("received before break: " + str(received))
println("elapsed: " + str(elapsedMs) + " ms")

// Give the bridge a beat to land its last in-flight yield and ack the
// cancel, so the follow-up read sees a stable cancel_count rather than
// racing whatever item was mid-flight when we broke.
sleep(200)

let produced, err = bridgeCall(bridge, "get_cancel_count", [])
if err != null {
    println("get_cancel_count failed: " + err.message)
    bridgeClose(bridge)
    return
}

println("items the bridge actually yielded: " + str(produced))
println("")

let ok = true
if produced > 10 {
    println("FAIL: bridge produced " + str(produced) + " items — cancel did not propagate")
    ok = false
}
if elapsedMs > 2000 {
    println("FAIL: stream took " + str(elapsedMs) + " ms — cancel did not stop the producer")
    ok = false
}
if ok {
    println("OK — bridge stopped within " + str(produced) + " items of break")
}

bridgeClose(bridge)
