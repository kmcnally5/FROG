// ============================================================
// FUSION BASELINE — what does a map/filter/reduce chain cost
// today, and how far is it from a single hand-fused loop?
// ============================================================
//
// Why this benchmark exists
//
//   kLex's structural claim is "arrays + channels are the unit of
//   computation." If that's going to cash out as a perf story, we
//   need to know two numbers:
//
//     CHAIN_TIME  — wall-clock of `reduce(filter(map(arr, f), p), g, init)`
//                   as it runs today. Three allocations, three
//                   passes, callCallable per element per stage.
//
//     FUSED_TIME  — wall-clock of the same logic written as ONE
//                   explicit `for` loop. This is the ceiling
//                   that runtime-level stream fusion could
//                   theoretically reach (it doesn't reach
//                   ceiling — see notes below).
//
//   Ratio of those two tells us whether fusion is worth doing.
//   <2x → don't bother, the cost is elsewhere (interpreter
//          dispatch, env lookup, callCallable overhead).
//   3-5x → real headroom; fusion is the right intervention.
//   10x+ → we're losing badly to allocation/GC pressure.
//
// What the pipeline does
//
//   For x in [0..N):
//     y = x * 2           (map: double)
//     keep y if y % 4 == 0 (filter: pick a subset)
//     z = y * y           (map: square the survivors)
//     total += z          (reduce: sum)
//
//   N = 1,000,000. All Integer arithmetic — no float quirks,
//   no allocations inside the body (one Object per element
//   per stage at most).
//
// Important caveats
//
//   - The "baseline loop" version is the BEST CASE for any
//     fusion-style optimisation. Real fusion still pays
//     callCallable overhead per element for user fns; the
//     baseline pays interpreter dispatch only for the inline
//     arithmetic. So the gap is an upper bound on what fusion
//     could deliver, not what it will deliver.
//
//   - We deliberately use INTEGER arithmetic. Float would add
//     Object allocation noise that masks the real signal.
//
//   - Each scenario runs N=1M elements; numbers are wall-clock
//     in milliseconds, smaller is better.

import "stdlib/stream_fusion.lex" as sf

let N = 1000000

// ------------------------------------------------------------
// Build the input array once, outside the timed regions.
// ------------------------------------------------------------
fn build_input() {
    let arr = makeArray(N, 0)
    for i in range(N) {
        arr[i] = i
    }
    return arr
}

// ------------------------------------------------------------
// Scenario 1 — explicit single-pass loop ("fusion ceiling").
//
// What a perfect fuser would produce: no intermediate arrays,
// no callCallable, no callable dispatch. Just the inner
// arithmetic in one tight loop.
// ------------------------------------------------------------
fn scenario_baseline(arr) {
    let total = 0
    for x in arr {
        let y = x * 2
        if y % 4 == 0 {
            let z = y * y
            total = total + z
        }
    }
    return total
}

// ------------------------------------------------------------
// Scenario 2 — chained map/filter/reduce.
//
// This is what real kLex code looks like when written in the
// "vector transformation" style. Three intermediate arrays,
// three full passes over the data, callCallable per element
// per stage.
// ------------------------------------------------------------
fn double(x) { return x * 2 }
fn keep(x)   { return x % 4 == 0 }
fn square(x) { return x * x }
fn add(a, b) { return a + b }

fn scenario_chain(arr) {
    let a = map(arr, double)
    let b = filter(a, keep)
    let c = map(b, square)
    return reduce(c, add, 0)
}

// ------------------------------------------------------------
// Scenario 3 — stdlib/stream_fusion userspace fuse().
//
// Existing approach: one pass, but the "fusion" is interpreted
// (each step is a hash lookup inside a kLex for-in loop). So
// in theory: no intermediate arrays. In practice: the dispatch
// happens in kLex bytecode, not Go.
//
// reduce-step isn't a thing in fuse() — it returns the
// filtered/mapped array and we sum it separately.
// ------------------------------------------------------------
fn scenario_streamfusion(arr) {
    let c = sf.fuse(arr,
        sf.mapStep(double),
        sf.filterStep(keep),
        sf.mapStep(square))
    return reduce(c, add, 0)
}

// ------------------------------------------------------------
// Timing harness — run fn(arr), print wall-clock ms + result.
// Result is printed so we can confirm all three scenarios
// agree (else one of them has a bug and the timings are
// meaningless).
// ------------------------------------------------------------
fn time_ms(label, body, arr) {
    let t0 = _timeNanos()
    let result = body(arr)
    let t1 = _timeNanos()
    let ms = float(t1 - t0) / 1000000.0
    println(label + ":  " + str(ms) + " ms   (result = " + str(result) + ")")
    return ms
}

// ------------------------------------------------------------
// Run
// ------------------------------------------------------------
println("=== Fusion baseline benchmark — N=" + str(N) + " ===")
println("")

let arr = build_input()

println("Building array of " + str(N) + " integers... done")
println("")

// Warm up each scenario once (one-off VM costs out of the way)
scenario_baseline(arr)
scenario_chain(arr)
scenario_streamfusion(arr)

println("--- measured runs ---")
let t_base   = time_ms("baseline single-loop  ", scenario_baseline, arr)
let t_chain  = time_ms("map/filter/map/reduce ", scenario_chain,    arr)
let t_stream = time_ms("stream_fusion.fuse()  ", scenario_streamfusion, arr)

println("")
println("--- ratios (baseline = 1.0x) ---")
println("chain   / baseline:  " + str(t_chain  / t_base) + "x")
println("stream  / baseline:  " + str(t_stream / t_base) + "x")
println("")
println("If chain >> baseline, runtime stream fusion has headroom.")
println("If chain ~= baseline, the cost is elsewhere (interpreter dispatch).")
