// agentUiHookTest.lex — agent.onUiEvent round-trip via the
// _uiEvent producer primitive.
//
// Phase 3 hook for UI interaction events. The hook is normally fired
// from stdlib/ui.lex's interactive widgets when the user clicks /
// drags / toggles / selects / types. We can't simulate a real UI
// interaction headless, so we exercise the plumbing via the
// `_uiEvent` builtin directly — same code path the widgets use.

import "stdlib/assert.lex" as t
import "stdlib/agent.lex"  as agent

// ── 1. _uiEventActive() reflects registration state ─────────────────────
agent.clearUiEvent()
t.assertFalse(_uiEventActive())

agent.onUiEvent(fn(evt) { return null })
t.assertTrue(_uiEventActive())

agent.clearUiEvent()
t.assertFalse(_uiEventActive())

// ── 2. Hook receives well-formed event hash ─────────────────────────────
let state = {"count": 0, "last": null}

agent.onUiEvent(fn(evt) {
    state["count"] = state["count"] + 1
    state["last"]  = evt
})

// Simulate a button click event (the same call ui.lex's button widget
// makes when the user clicks).
_uiEvent("click", "button", "Save", null, 120, 240)

t.assertEqual(state["count"], 1)
let e1 = state["last"]
t.assertEqual(e1["kind"], "click")
t.assertEqual(e1["widget"], "button")
t.assertEqual(e1["label"], "Save")
t.assertEqual(e1["value"], null)
t.assertEqual(e1["x"], 120)
t.assertEqual(e1["y"], 240)

// Slider drag with a new value.
_uiEvent("drag", "slider", "gamma", 0.75, 300, 180)
t.assertEqual(state["count"], 2)
let e2 = state["last"]
t.assertEqual(e2["kind"], "drag")
t.assertEqual(e2["widget"], "slider")
t.assertEqual(e2["value"], 0.75)

// Dropdown select with an int index.
_uiEvent("select", "dropdown", "cinematic lighting", 3, 80, 95)
t.assertEqual(state["last"]["value"], 3)
t.assertEqual(state["last"]["label"], "cinematic lighting")

// Checkbox toggle with bool value.
_uiEvent("toggle", "checkbox", "Lock aspect ratio", true, 50, 50)
t.assertEqual(state["last"]["value"], true)

// ── 3. Re-entry guard: hook that itself errors doesn't loop ─────────────
agent.clearUiEvent()
state["count"] = 0

agent.onUiEvent(fn(evt) {
    state["count"] = state["count"] + 1
    let dead = 1 / 0   // hook crashes
    return dead
})

_uiEvent("click", "button", "Boom", null, 0, 0)
// Hook ran exactly once before its own error tripped the re-entry guard.
t.assertEqual(state["count"], 1)

// ── 4. clearUiEvent unregisters ─────────────────────────────────────────
agent.clearUiEvent()
state["count"] = 0
_uiEvent("click", "button", "Quiet", null, 0, 0)
t.assertEqual(state["count"], 0)

// ── 5. _setUiEventHook rejects non-callables ────────────────────────────
let _r, err1 = safe(fn() { _setUiEventHook(42) })
t.assertTrue(err1 != null)

let _r2, err2 = safe(fn() { _setUiEventHook(null) })  // null is fine
t.assertTrue(err2 == null)

t.summary()
