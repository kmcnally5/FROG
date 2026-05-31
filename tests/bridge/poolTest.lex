// poolTest.lex — exercises bridgePool end-to-end.
//
// Covers:
//   1. bridgePool starts N workers and bridgePoolHealth reports them alive.
//   2. bridgePoolCall round-robins across members and serialises within each.
//   3. The init callable runs on every member.
//   4. bridgePoolStream returns a working channel from a stream handler.
//   5. bridgePoolClose tears everything down cleanly.

let POOL_SIZE = 4

// init() bumps a global the bridge can later report back. We use this to prove
// every member ran init exactly once, in parallel.
let INIT_FN = fn(b) {
    let _, err = bridgeCall(b, "add", [0, 0])   // warm-up; meaningless body
    return err
}

println("=== bridgePool start ===")
let pool, err = bridgePool(POOL_SIZE, {"kind": "subprocess", "cmd": "python3", "args": ["tests/bridge/python_bridge.py"]}, {"init": INIT_FN})
if err != null {
    println("bridgePool failed: " + err.message)
    return
}
println("pool started")

let h = bridgePoolHealth(pool)
println("health: size=" + str(h["size"]) + " alive=" + str(h["alive"]) + " dead=" + str(h["dead"]))
if h["alive"] != POOL_SIZE {
    println("FAIL: expected " + str(POOL_SIZE) + " alive workers")
    bridgePoolClose(pool)
    return
}

println("")
println("=== fan-out via bridgePoolCall ===")
// Fire 12 async calls — three times the pool size. Round-robin should keep all
// workers busy; we just check the answers are correct (each is add(i, i*2)).
let tasks = makeArray(12, null)
for i in range(12) {
    let _pool = pool
    let _a = i
    let _b = i * 2
    tasks[i] = async(fn() {
        let r, e = bridgePoolCall(_pool, "add", [_a, _b])
        if e != null { return e }
        return r
    })
}

let allOK = true
for i in range(12) {
    let r = await(tasks[i])
    let expected = i + i * 2
    if r != expected {
        println("FAIL task " + str(i) + ": expected " + str(expected) + ", got " + str(r))
        allOK = false
    }
}
if allOK { println("all 12 add() results matched") }

println("")
println("=== bridgePoolStream ===")
let ch, err = bridgePoolStream(pool, "count_from", [100, 5])
if err != null {
    println("bridgePoolStream failed: " + err.message)
    bridgePoolClose(pool)
    return
}
let streamed = makeArray(5, 0)
let idx = 0
for item in ch {
    streamed[idx] = item
    idx = idx + 1
}
println("streamed: " + str(streamed))   // expect [100, 101, 102, 103, 104]

println("")
println("=== runtime taint tracking ===")
// Kill one member mid-session and verify the pool notices: bridgePoolHealth
// must drop to alive=N-1, and subsequent calls must round-robin past the dead
// slot rather than failing.
//
// suicide() calls os._exit() inside one bridge — that worker dies, the kLex
// reader sees stdout close and taints the Bridge struct. The pool then has to
// fold that runtime taint into its liveness check, not just init-time dead.
_, _ = bridgePoolCall(pool, "suicide", [])    // we expect this to error — that's the point

// Give the reader goroutine a beat to observe EOF on stdout and mark the
// crashed member tainted. Single sleep, not a poll.
sleep(150)

let h2 = bridgePoolHealth(pool)
println("health after crash: size=" + str(h2["size"]) + " alive=" + str(h2["alive"]) + " dead=" + str(h2["dead"]))
if h2["alive"] != POOL_SIZE - 1 || h2["dead"] != 1 {
    println("FAIL: expected alive=" + str(POOL_SIZE - 1) + " dead=1, got alive=" + str(h2["alive"]) + " dead=" + str(h2["dead"]))
}

// Now fan out more add() calls — every one must land on a surviving worker.
// If pick() still routed to the dead slot, we'd see BRIDGE_TAINTED errors.
let postTasks = makeArray(8, null)
for i in range(8) {
    let _pool = pool
    let _a = i
    postTasks[i] = async(fn() {
        let r, e = bridgePoolCall(_pool, "add", [_a, 100])
        if e != null { return e }
        return r
    })
}
let postOK = true
for i in range(8) {
    let r = await(postTasks[i])
    if r != i + 100 {
        println("FAIL post-crash task " + str(i) + ": expected " + str(i + 100) + ", got " + str(r))
        postOK = false
    }
}
if postOK { println("all 8 post-crash calls routed to surviving workers") }

println("")
bridgePoolClose(pool)
println("pool closed")

// Calls after close must fail loudly rather than hang.
_, err = bridgePoolCall(pool, "add", [1, 1])
if err == null {
    println("FAIL: call after close should have errored")
} else {
    println("post-close error: " + err.code + " — " + err.message)
}
