// robustnessTest.lex — verifies Phase 1 bridge robustness features.
//
// Each section starts its own short-lived bridge so a failure in one test
// does not leak state into the next. This mirrors how real applications
// should use bridges defensively.

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

// ── Test 1: backward compatibility — 2-arg nativeBridge still works ──────────
println("")
println("── 1. Backward compatibility ────────────────────────────────────────")

bridge, err = nativeBridge("python3", [BRIDGE_PATH])
expect("2-arg nativeBridge() succeeds", err == null)
if err == null {
    result, cerr = bridgeCall(bridge, "echo", ["hello"])
    expect("basic echo() round-trip", cerr == null && result == "hello")
    bridgeClose(bridge)
}

// ── Test 2: options hash — timeout, max_response_mb, stderr_log ──────────────
println("")
println("── 2. Options hash accepted ─────────────────────────────────────────")

bridge, err = nativeBridge("python3", [BRIDGE_PATH], {
    "timeout_seconds": 5,
    "max_response_mb": 4,
})
expect("3-arg nativeBridge() with options succeeds", err == null)
if err == null {
    result, cerr = bridgeCall(bridge, "echo", [42])
    expect("call with bridge-level timeout works", cerr == null && result == 42)
    bridgeClose(bridge)
}

// ── Test 3: BRIDGE_TIMEOUT fires and bridge becomes tainted ──────────────────
println("")
println("── 3. Timeout taints the bridge ─────────────────────────────────────")

bridge, err = nativeBridge("python3", [BRIDGE_PATH], {"timeout_seconds": 1})
if err != null {
    expect("bridge start", false)
} else {
    result, cerr = bridgeCall(bridge, "hang_forever", [])
    timeoutFired = cerr != null && cerr.code == "BRIDGE_TIMEOUT"
    expect("hang_forever() triggers BRIDGE_TIMEOUT", timeoutFired)

    // Second call should fast-fail with BRIDGE_TAINTED rather than hang.
    result2, cerr2 = bridgeCall(bridge, "echo", ["after timeout"])
    taintedFired = cerr2 != null && cerr2.code == "BRIDGE_TAINTED"
    expect("subsequent call fails fast with BRIDGE_TAINTED", taintedFired)
    bridgeClose(bridge)
}

// ── Test 4: Per-call timeout override beats bridge default ───────────────────
println("")
println("── 4. Per-call timeout override ─────────────────────────────────────")

bridge, err = nativeBridge("python3", [BRIDGE_PATH], {"timeout_seconds": 30})
if err != null {
    expect("bridge start", false)
} else {
    // Override the 30s bridge default with a 1s per-call timeout.
    result, cerr = bridgeCall(bridge, "hang_forever", [], 1)
    overrodeOk = cerr != null && cerr.code == "BRIDGE_TIMEOUT"
    expect("4th-arg timeout overrides bridge default", overrodeOk)
    bridgeClose(bridge)
}

// ── Test 5: stderr is captured and surfaced via bridgeStderr() ───────────────
println("")
println("── 5. Stderr capture & bridgeStderr() ───────────────────────────────")

bridge, err = nativeBridge("python3", [BRIDGE_PATH])
if err != null {
    expect("bridge start", false)
} else {
    // This call writes a traceback to stderr then raises — the bridge
    // returns BRIDGE_ERROR with the exception message.
    result, cerr = bridgeCall(bridge, "crash_with_traceback", [])
    expect("crash_with_traceback() returns BRIDGE_ERROR", cerr != null && cerr.code == "BRIDGE_ERROR")

    // Now drain the captured stderr and verify the traceback is in there.
    stderrLines = bridgeStderr(bridge)
    foundTrace = false
    i = 0
    while i < len(stderrLines) {
        if indexOf(stderrLines[i], "RuntimeError") >= 0 { foundTrace = true }
        i = i + 1
    }
    expect("bridgeStderr() exposes the Python traceback", foundTrace)
    bridgeClose(bridge)
}

// ── Test 6: BRIDGE_CLOSED when subprocess dies ───────────────────────────────
println("")
println("── 6. BRIDGE_CLOSED on subprocess death ─────────────────────────────")

bridge, err = nativeBridge("python3", [BRIDGE_PATH])
if err != null {
    expect("bridge start", false)
} else {
    // kill_self does os._exit(1) without responding, so the read fails.
    result, cerr = bridgeCall(bridge, "kill_self", [])
    expect("kill_self() returns BRIDGE_CLOSED", cerr != null && cerr.code == "BRIDGE_CLOSED")
    bridgeClose(bridge)
}

// ── Test 7: Options-hash validation rejects bad input ────────────────────────
println("")
println("── 7. Options validation ────────────────────────────────────────────")

_, err = nativeBridge("python3", [BRIDGE_PATH], {"max_response_mb": 999})
expect("max_response_mb > 256 is rejected", err != null)

_, err = nativeBridge("python3", [BRIDGE_PATH], {"timeout_seconds": -5})
expect("negative timeout_seconds is rejected", err != null)

// ── Summary ──────────────────────────────────────────────────────────────────
println("")
println("─────────────────────────────────────────────────────────────────────")
println("Results: " + str(passCount) + " passed, " + str(failCount) + " failed")
if failCount > 0 {
    println("FAILED")
} else {
    println("ALL PASSED")
}
