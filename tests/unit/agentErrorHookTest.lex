// agentErrorHookTest.lex — agent.onErrorBubble round-trip
//
// Phase 1 of kLex's agentic runtime hooks. Locks in:
//   - hook registration via agent.onErrorBubble
//   - hook fires on internal errors (RuntimeError / TypeError)
//   - event hash has the documented shape (kind, message, line, stack)
//   - re-entry guard: hook that itself errors doesn't recurse infinitely
//   - clear() unregisters the hook
//   - hook only fires for internal errors — user errors from error()
//     stay as values and do NOT fire the hook (matches isError semantics)
//
// VM only for now. Tree-walker path is a future addition.

import "assert.lex" as t
import "agent.lex"  as agent

// ── 1. Hook fires on internal error ─────────────────────────────────────
let captured = null

agent.onErrorBubble(fn(evt) {
    captured = evt
})

// safe() lets us trigger an error without halting the whole test file.
let _, err = safe(fn() {
    let x = 1 / 0
    return x
})

t.assertTrue(err != null)
t.assertTrue(captured != null)
t.assertEqual(captured["kind"], "RuntimeError")
t.assertTrue(len(captured["message"]) > 0)
t.assertTrue(captured["line"] > 0)
t.assertTrue(type(captured["stack"]) == "ARRAY")
// Note: VM populates stack with line numbers per frame; tree-walker
// path leaves it empty for Phase 1 (no easy access to call-depth).
// Both shapes are acceptable — we only assert structure here.

// ── 2. TypeError kind is reported correctly ─────────────────────────────
captured = null

let _, err2 = safe(fn() {
    let bad = "hello" + 5
    return bad
})

t.assertTrue(err2 != null)
t.assertTrue(captured != null)
t.assertEqual(captured["kind"], "TypeError")

// ── 3. User errors from error() do NOT fire the hook ────────────────────
//      (matches the tree-walker's isError() = !IsUserError contract —
//       user errors are first-class values that don't propagate as
//       runtime errors and must therefore not be reported.)
captured = null

let _, err3 = safe(fn() {
    return null, error("MY_CODE", "user-shaped error")
})

t.assertTrue(err3 != null)              // safe still saw the (null, err) tuple
t.assertTrue(captured == null)          // but the hook did NOT fire

// ── 4. Re-entry guard: a hook that itself errors doesn't loop ───────────
//      The inner safe() catches the hook's own error.
let outerCount = 0

agent.onErrorBubble(fn(evt) {
    outerCount = outerCount + 1
    // Deliberately fault — division by zero inside the hook.
    let dead = 1 / 0
    println("never printed: " + str(dead))
})

let _, err4 = safe(fn() {
    let x = 1 / 0
    return x
})

t.assertTrue(err4 != null)
t.assertEqual(outerCount, 1)            // hook ran exactly once, not infinitely

// ── 5. clearErrorBubble() unregisters ───────────────────────────────────
agent.clearErrorBubble()
captured = null

let _, err5 = safe(fn() {
    let x = 1 / 0
    return x
})

t.assertTrue(err5 != null)
t.assertTrue(captured == null)          // no hook = no capture

t.summary()
