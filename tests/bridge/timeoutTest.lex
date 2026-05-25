// timeoutTest.lex — verifies bridgeStream idle + total timeouts.
//
// Uses the existing cancel_demo handler in python_bridge.py (50 items × 100ms
// = 5s natural duration). We exercise three scenarios:
//
//   1. Idle timeout fires.  Consumer reads 2 items, then doesn't iterate fast
//      enough — wait wait, that's the cancel case. For idle timeout we need
//      a stream that PAUSES between yields, longer than our idle threshold.
//      cancel_demo with delay=300ms and idle=100ms triggers idle on the 2nd
//      item. The consumer doesn't break; the watchdog cuts the stream.
//
//   2. Total timeout fires.  Long-running stream cut at total seconds.
//
//   3. No-timeout regression. bridgeStream with no 4th arg still works for
//      a normal stream (count_from(0, 5) — 5 fast items, no delay).
//
// Each case asserts the consumer sees the expected end-state — either an
// error item with code "BRIDGE_TIMEOUT" or a clean run with no error item.

import "stdlib/datetime.lex" as dt

let bridge, err = nativeBridge("python3", ["tests/examples/bridge/python_bridge.py"])
if err != null {
    println("bridge failed to start: " + err.message)
    return
}

let ok = true

// ── 1. Idle timeout ──────────────────────────────────────────────────────────
// Bridge yields one item every 500ms. Idle threshold is 0.2s (200ms). The
// first item should arrive almost immediately, then 200ms later (before the
// next yield at 500ms) the idle watchdog fires.
println("=== idle timeout (delay=500ms, idle=0.2s) ===")
let ch, err = bridgeStream(bridge, "cancel_demo", [50, 500], {"idle": 0.2})
if err != null { println("FAIL bridgeStream: " + err.message)  bridgeClose(bridge)  return }

let start  = dt.nowNanos()
let items  = 0
let sawTimeout = false
let timeoutCode = ""
for item in ch {
    if type(item) == "ERROR" {
        sawTimeout  = (item.code == "BRIDGE_TIMEOUT")
        timeoutCode = item.code
        println("✓ received error item: " + item.code + " — " + item.message)
        break
    }
    items = items + 1
}
let elapsedMs = (dt.nowNanos() - start) / 1000000
println("items before timeout: " + str(items) + ", elapsed: " + str(elapsedMs) + " ms")
if !sawTimeout {
    println("FAIL: expected BRIDGE_TIMEOUT, got code=" + timeoutCode)
    ok = false
}
if elapsedMs > 1500 {
    println("FAIL: idle timeout took too long (" + str(elapsedMs) + " ms)")
    ok = false
}

// Settle: give the bridge a moment to process the cancel before next call.
sleep(200)

// ── 2. Total timeout ─────────────────────────────────────────────────────────
println("")
println("=== total timeout (delay=50ms, total=1s) ===")
let ch2, err = bridgeStream(bridge, "cancel_demo", [200, 50], {"total": 1})
if err != null { println("FAIL bridgeStream: " + err.message)  bridgeClose(bridge)  return }

let start2  = dt.nowNanos()
let items2  = 0
let sawTimeout2 = false
for item in ch2 {
    if type(item) == "ERROR" {
        sawTimeout2 = (item.code == "BRIDGE_TIMEOUT")
        println("✓ received error item: " + item.code + " — " + item.message)
        break
    }
    items2 = items2 + 1
}
let elapsedMs2 = (dt.nowNanos() - start2) / 1000000
println("items before total: " + str(items2) + ", elapsed: " + str(elapsedMs2) + " ms")
if !sawTimeout2 {
    println("FAIL: expected BRIDGE_TIMEOUT on total")
    ok = false
}
if elapsedMs2 < 800 || elapsedMs2 > 2500 {
    println("FAIL: total fired at " + str(elapsedMs2) + " ms (expected ~1000)")
    ok = false
}

sleep(200)

// ── 3. No-timeout regression ────────────────────────────────────────────────
println("")
println("=== no timeout (regression) ===")
let ch3, err = bridgeStream(bridge, "count_from", [0, 5])
if err != null { println("FAIL bridgeStream: " + err.message)  bridgeClose(bridge)  return }
let buf = makeArray(8, null)
let idx = 0
let sawError3 = false
for item in ch3 {
    if type(item) == "ERROR" {
        sawError3 = true
        println("FAIL: unexpected error " + item.code)
        ok = false
        break
    }
    buf[idx] = item
    idx = idx + 1
}
let got = slice(buf, 0, idx)
println("collected: " + str(got))
if len(got) != 5 {
    println("FAIL: expected 5 items, got " + str(len(got)))
    ok = false
}

println("")
if ok { println("OK") } else { println("FAILED") }

bridgeClose(bridge)
