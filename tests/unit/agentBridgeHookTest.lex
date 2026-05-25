// agentBridgeHookTest.lex — agent.onBridgeCall round-trip.
//
// Phase 3 hook for bridge-call telemetry. The hook fires once per
// _bridgeCall completion (synchronous round-trip), carrying fn name,
// argc, duration, and ok/error.
//
// Without a real bridge subprocess, we just verify the plumbing:
//   - register / clear works
//   - the underlying primitive (_setBridgeCallHook) accepts a callable
//     or null and rejects other types
//
// A full end-to-end check (real bridge → hook fires with timing)
// happens implicitly during tadPole demos where the FrogTooth
// telemetry registers onBridgeCall and we observe Python calls.

import "assert.lex" as t
import "agent.lex"  as agent

// ── 1. Register / clear basics ─────────────────────────────────────────
let callCount = 0

agent.onBridgeCall(fn(evt) {
    callCount = callCount + 1
})

// We haven't actually called any bridge — hook should not have fired yet.
t.assertEqual(callCount, 0)

// Clearing is safe.
agent.clearBridgeCall()
t.assertEqual(callCount, 0)

// ── 2. _setBridgeCallHook rejects non-callables ────────────────────────
let _r, err1 = safe(fn() {
    _setBridgeCallHook(42)
})
t.assertTrue(err1 != null)

let _r2, err2 = safe(fn() {
    _setBridgeCallHook("not a fn")
})
t.assertTrue(err2 != null)

// null is the documented clear sentinel.
let _r3, err3 = safe(fn() {
    _setBridgeCallHook(null)
})
t.assertTrue(err3 == null)

// Functions are accepted.
let _r4, err4 = safe(fn() {
    _setBridgeCallHook(fn(evt) { return null })
})
t.assertTrue(err4 == null)

agent.clearBridgeCall()

t.summary()
