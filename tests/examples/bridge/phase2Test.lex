// phase2Test.lex — verifies Phase 2 bridge features.
//
// Tests: notifications, concurrent calls, stdlib resilient bridge,
// circuit breaker, withBridge helper.

import "stdlib/bridge.lex" as br

const BRIDGE_PATH = "tests/examples/bridge/robustness_bridge.py"

passCount = 0
failCount = 0

fn expect(label, ok) {
    if ok == true {
        println("  PASS  " + label)
        passCount = passCount + 1
    } else {
        println("  FAIL  " + label)
        failCount = failCount + 1
    }
}

// ── Test 1: Notifications stream during a long call ──────────────────────────
println("")
println("── 1. Notifications stream during slow call ─────────────────────────")

bridge, err = nativeBridge("python3", [BRIDGE_PATH])
expect("bridge starts", err == null)
if err == null {
    notifCh = bridgeNotifications(bridge)

    received = makeArray(0)
    notifTask = async(fn() {
        let ch = notifCh
        let buf = makeArray(0)
        while true {
            msg, ok = recv(ch)
            if !ok { return buf }
            newBuf = makeArray(len(buf) + 1)
            i = 0
            while i < len(buf) { newBuf[i] = buf[i]  i = i + 1 }
            newBuf[len(buf)] = msg["done"]
            buf = newBuf
        }
    })

    result, cerr = bridgeCall(bridge, "slow_with_notifications", [10])
    bridgeClose(bridge)

    received = await(notifTask)

    expect("slow call returns done", cerr == null && result["result"] == "done")
    expect("notifications received (>= 5)", len(received) >= 5)
    expect("first notification has correct value", len(received) > 0 && received[0] == 1)
    allInOrder = true
    i = 0
    while i < len(received) - 1 {
        if received[i] >= received[i + 1] { allInOrder = false }
        i = i + 1
    }
    expect("notifications arrive in order", allInOrder)
}

// ── Test 2: Concurrent calls via async tasks ──────────────────────────────────
println("")
println("── 2. Concurrent calls on one bridge ────────────────────────────────")

bridge, err = nativeBridge("python3", [BRIDGE_PATH])
expect("bridge starts", err == null)
if err == null {
    // Launch 4 concurrent calls with different delays — shorter delays should
    // complete first despite being submitted later, proving independent routing.
    let _b = bridge
    t1 = async(fn() { return bridgeCall(_b, "echo_concurrent", ["A", 80]) })
    t2 = async(fn() { return bridgeCall(_b, "echo_concurrent", ["B", 20]) })
    t3 = async(fn() { return bridgeCall(_b, "echo_concurrent", ["C", 60]) })
    t4 = async(fn() { return bridgeCall(_b, "echo_concurrent", ["D", 40]) })

    r1, e1 = await(t1)
    r2, e2 = await(t2)
    r3, e3 = await(t3)
    r4, e4 = await(t4)

    expect("call A returns correct value", e1 == null && r1["value"] == "A")
    expect("call B returns correct value", e2 == null && r2["value"] == "B")
    expect("call C returns correct value", e3 == null && r3["value"] == "C")
    expect("call D returns correct value", e4 == null && r4["value"] == "D")
    expect("all 4 concurrent calls succeeded", e1 == null && e2 == null && e3 == null && e4 == null)

    bridgeClose(bridge)
}

// ── Test 3: ResilientBridge basic call + health_check ─────────────────────────
println("")
println("── 3. ResilientBridge startup + health_check ────────────────────────")

rb, err = br.newResilient("python3", [BRIDGE_PATH], {"require_deps": false})
expect("newResilient succeeds", err == null)
if err == null {
    result, cerr = rb.call("echo", ["hello from resilient"])
    expect("call through resilient bridge works", cerr == null && result == "hello from resilient")
    rb.close()
}

// ── Test 4: Auto-restart on BRIDGE_CLOSED ─────────────────────────────────────
println("")
println("── 4. Auto-restart on BRIDGE_CLOSED ─────────────────────────────────")

rb, err = br.newResilient("python3", [BRIDGE_PATH], {"max_retries": 3})
expect("resilient bridge starts", err == null)
if err == null {
    // This kills the subprocess — resilient bridge should restart and retry.
    result, cerr = rb.call("kill_self", [])

    // After kill_self the call fails (subprocess died before responding),
    // but _restart should have created a fresh bridge. The next call works.
    result2, cerr2 = rb.call("echo", ["after restart"])
    expect("bridge auto-restarted after kill_self", cerr2 == null && result2 == "after restart")
    expect("failure counter is reset after success", rb._failures == 0)
    rb.close()
}

// ── Test 5: Circuit breaker opens after max_retries failures ──────────────────
println("")
println("── 5. Circuit breaker opens after max_retries ───────────────────────")

rb, err = br.newResilient("python3", [BRIDGE_PATH], {"max_retries": 2})
expect("resilient bridge starts", err == null)
if err == null {
    // Kill the subprocess twice — maxRetries is 2, so circuit opens on 2nd.
    rb.call("kill_self", [])   // failure 1 → restart
    rb.call("kill_self", [])   // failure 2 → circuit opens

    _, cerr = rb.call("echo", ["should not reach bridge"])
    expect("circuit opened after 2 failures", cerr != null && cerr.code == "CIRCUIT_OPEN")
    expect("isCircuitOpen() helper agrees", br.isCircuitOpen(cerr))

    // Reset clears the circuit.
    rb.reset()
    result, cerr2 = rb.call("echo", ["after reset"])
    expect("bridge works after circuit reset", cerr2 == null && result == "after reset")
    rb.close()
}

// ── Test 6: withBridge helper ─────────────────────────────────────────────────
println("")
println("── 6. withBridge one-shot helper ────────────────────────────────────")

result, err = br.withBridge("python3", [BRIDGE_PATH], null, fn(b) {
    return bridgeCall(b, "echo", ["one-shot"])
})
expect("withBridge returns result", err == null && result == "one-shot")

// Bridge is automatically closed after withBridge returns.
// (We can't easily verify the subprocess is gone, but at least no error.)

// ── Test 7: isRetriable and isCircuitOpen helpers ─────────────────────────────
println("")
println("── 7. Error classification helpers ──────────────────────────────────")

bridge, err = nativeBridge("python3", [BRIDGE_PATH])
if err == null {
    _, kerr = bridgeCall(bridge, "kill_self", [])
    expect("isRetriable(BRIDGE_CLOSED) = true", br.isRetriable(kerr))
    expect("isCircuitOpen(BRIDGE_CLOSED) = false", br.isCircuitOpen(kerr) == false)
    expect("isRetriable(null) = false", br.isRetriable(null) == false)
    bridgeClose(bridge)
}

// ── Summary ───────────────────────────────────────────────────────────────────
println("")
println("─────────────────────────────────────────────────────────────────────")
println("Results: " + str(passCount) + " passed, " + str(failCount) + " failed")
if failCount > 0 {
    println("FAILED")
} else {
    println("ALL PASSED")
}
