// agentAsyncHookTest.lex — agent.onAsyncSpawn / agent.onAsyncDone round-trip
//
// Phase 2 of kLex's agentic runtime hooks. Locks in:
//   - hook registration via agent.onAsyncSpawn / onAsyncDone
//   - spawn fires BEFORE the goroutine starts; carries task_id, fn,
//     argc, spawned_at
//   - done fires AFTER the goroutine returns; carries the matching
//     task_id, duration_ms, ok flag, error info on failure
//   - successful task → ok=true, error=null
//   - errored task (internal error) → ok=false, error has kind+message
//   - user errors from error() do NOT mark the task as failed (matches
//     isError() = !IsUserError semantics)
//   - clearAsyncSpawn / clearAsyncDone unregister

import "stdlib/assert.lex" as t
import "stdlib/agent.lex"  as agent

// Hash counters avoid push()'s immutable-array semantics — closures
// can mutate hash values in place via h["k"] = v.
let state = {"spawnCount": 0, "doneCount": 0, "lastSpawn": null, "lastDone": null}

agent.onAsyncSpawn(fn(evt) {
    state["spawnCount"] = state["spawnCount"] + 1
    state["lastSpawn"]  = evt
})
agent.onAsyncDone(fn(evt) {
    state["doneCount"] = state["doneCount"] + 1
    state["lastDone"]  = evt
})

// ── 1. Successful task: spawn → done with ok=true ───────────────────────
let task1 = async(fn() {
    sleep(20)
    return 42
})

let i = 0
while i < 50 && state["doneCount"] < 1 {
    sleep(10)
    i = i + 1
}

t.assertEqual(state["spawnCount"], 1)
t.assertEqual(state["doneCount"], 1)

let sp = state["lastSpawn"]
let dn = state["lastDone"]

t.assertEqual(sp["task_id"], dn["task_id"])  // spawn and done paired
t.assertTrue(sp["task_id"] > 0)
t.assertEqual(sp["argc"], 0)
t.assertTrue(sp["spawned_at"] > 0)
// fn label is interpreter-dependent: tree-walker reports "<anon>"
// for unnamed *Function; VM reports "<closure>" for *CompiledFunction.
// Both convey "this was an anonymous closure" to the agent.
t.assertTrue(sp["fn"] == "<anon>" || sp["fn"] == "<closure>")

t.assertTrue(dn["ok"])
t.assertEqual(dn["error"], null)
t.assertTrue(dn["duration_ms"] >= 0)         // sleep was 20ms but timer resolution varies

// ── 2. Errored task: ok=false, error populated ──────────────────────────
state["spawnCount"] = 0
state["doneCount"]  = 0
state["lastDone"]   = null

let task2 = async(fn() {
    let x = 1 / 0   // internal RuntimeError — bubbles out of the task
    return x
})

let j = 0
while j < 50 && state["doneCount"] < 1 {
    sleep(10)
    j = j + 1
}

t.assertEqual(state["doneCount"], 1)
let dn2 = state["lastDone"]
t.assertFalse(dn2["ok"])
t.assertTrue(dn2["error"] != null)
t.assertEqual(dn2["error"]["kind"], "RuntimeError")
t.assertTrue(len(dn2["error"]["message"]) > 0)

// ── 3. Task returning a user error() value — still ok=true ──────────────
//      User errors are first-class values, not propagation signals.
state["spawnCount"] = 0
state["doneCount"]  = 0
state["lastDone"]   = null

let task3 = async(fn() {
    return error("MY_CODE", "user-shaped value")
})

let k = 0
while k < 50 && state["doneCount"] < 1 {
    sleep(10)
    k = k + 1
}

t.assertEqual(state["doneCount"], 1)
let dn3 = state["lastDone"]
t.assertTrue(dn3["ok"])                      // user errors aren't task failures
t.assertEqual(dn3["error"], null)

// ── 4. clearAsyncSpawn / clearAsyncDone unregister ──────────────────────
agent.clearAsyncSpawn()
agent.clearAsyncDone()
state["spawnCount"] = 0
state["doneCount"]  = 0

let task4 = async(fn() {
    sleep(10)
    return "no-one's listening"
})

let m = 0
while m < 30 {
    sleep(10)
    m = m + 1
}

t.assertEqual(state["spawnCount"], 0)
t.assertEqual(state["doneCount"], 0)

t.summary()
