// testFrogTooth.lex — kLex agentic-hook live monitor.
//
// FrogTooth is the first kLex tool built on top of the agentic
// runtime hooks shipped in v0.3.36+. It registers a handler for
// every available `agent.*` hook and pretty-prints every event to
// the terminal as it fires. While this program runs, every async
// task spawn / completion and every internal error gets reported
// live with zero modification to the producing code.
//
// Try it:
//     go run . tests/examples/testFrogTooth.lex          (tree-walker)
//     go run . --vm tests/examples/testFrogTooth.lex     (VM)
//
// The demo at the bottom of this file launches a handful of async
// tasks (some succeed, some error, some named, some anonymous) and
// triggers a main-thread error — all of which light up the monitor
// in real time.
//
// Copy the registration block at the top into your own program to
// get the same live telemetry. Then swap the println()s for
// whatever you actually want — log to disk, post to Slack, ask
// Claude to explain the error, stream to OpenTelemetry. The hooks
// are observer-only — nothing here can interfere with the host
// program's behaviour, and the cost when the host has no events
// is a single atomic pointer load.

import "agent.lex" as agent

// ── Hook handlers ────────────────────────────────────────────────────────
// Copy this block into your own kLex program for live monitoring.

agent.onAsyncSpawn(fn(evt) {
    println("🚀 #" + str(evt["task_id"]) + " " + evt["fn"])
})

agent.onAsyncDone(fn(evt) {
    let icon = "✓"
    if !evt["ok"] { icon = "✗" }
    let line = icon + " #" + str(evt["task_id"]) +
               " (" + str(evt["duration_ms"]) + "ms)"
    if !evt["ok"] {
        line = line + "  ← " + evt["error"]["kind"] +
               ": " + evt["error"]["message"]
    }
    println(line)
})

agent.onErrorBubble(fn(evt) {
    println("💥 [" + evt["kind"] + "] " + evt["message"] +
            " (line " + str(evt["line"]) + ")")
})

agent.onUiEvent(fn(evt) {
    let label = evt["label"]
    if label == "" { label = evt["widget"] }
    println("🖱  " + evt["kind"] + " " + evt["widget"] + ":" + label +
            "  value=" + str(evt["value"]))
})

agent.onBridgeCall(fn(evt) {
    let icon = "✓"
    if !evt["ok"] { icon = "✗" }
    let line = "🌉 " + icon + " " + evt["fn"] + "(" + str(evt["argc"]) +
               " args) " + str(evt["duration_ms"]) + "ms"
    if !evt["ok"] {
        line = line + "  ← " + evt["error"]["kind"] +
               ": " + evt["error"]["message"]
    }
    println(line)
})

// ── Demo scenarios ───────────────────────────────────────────────────────

println("")
println("─── FrogTooth — kLex agentic hook monitor ───────────────────────")
println("")

// 1. Anonymous successful task.
println("→ launching anonymous task...")
let t1 = async(fn() {
    sleep(30)
    return 42
})
sleep(80)

// 2. Named function task — note the spawn event's "fn" field carries
//    the name (under tree-walker; VM reports "<closure>").
fn fetchData() {
    sleep(50)
    return {"status": "ok"}
}
println("")
println("→ launching named task 'fetchData'...")
let t2 = async(fetchData)
sleep(120)

// 3. Errored task (RuntimeError from division by zero).
println("")
println("→ launching task that errors...")
let t3 = async(fn() {
    sleep(20)
    let x = 1 / 0
    return x
})
sleep(80)

// 4. Errored task (TypeError from strict-typing).
println("")
println("→ launching task with TypeError...")
let t4 = async(fn() {
    sleep(15)
    let result = "answer: " + 42
    return result
})
sleep(80)

// 5. Three parallel tasks of staggered durations. Watch the spawns
//    burst out first, then the dones come back interleaved as each
//    goroutine finishes.
println("")
println("→ launching 3 parallel tasks (staggered durations)...")
for n in [0, 1, 2] {
    let copy = n
    let t = async(fn() {
        sleep(40 + copy * 30)
        return copy * copy
    })
}
sleep(220)

// 6. Direct error on the main thread (not inside async). Demonstrates
//    that on_error_bubble fires for every internal error, not just
//    ones inside async tasks.
println("")
println("→ triggering main-thread error (out-of-bounds index)...")
let _result, err = safe(fn() {
    let arr = [1, 2, 3]
    return arr[99]
})
sleep(50)

println("")
println("─── monitor complete ────────────────────────────────────────────")
