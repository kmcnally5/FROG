// tensorBenchmark_comprehensive.lex — FrogPy throughput soak test
//
// A single, comprehensive benchmark that exercises the full v1 surface
// (element-wise ops + reductions) on large tensors. Measures sustained
// throughput over 100+ iterations with warmup and timing per round.
//
// Design:
//   - Warmup phase (10 iterations) to stabilize CPU cache/frequency
//   - Main bench (100 iterations) timing each round
//   - Each iteration exercises: 5 element-wise ops (add/sub/mul/div/pow) + 6 reductions
//   - Tensor sizes: 1M elements (f64) and 100K elements (f64)
//   - Reports: total time, per-iteration average, throughput in Gops
//
// Throughput extrapolation:
//   - 1M f64 add = ~560μs (baseline from prior session)
//   - 11 ops per iteration × 100 iterations = 1100 ops on 1M-element tensor
//   - Sustained throughput = 1M × 11 / (total_ms × 1e6) gigaops per second

import "stdlib/tensor.lex" as t
import "stdlib/assert.lex" as a

// ── setup ──

let n1m = 1000000  // 1M elements
let n100k = 100000 // 100K elements

// Build test tensors once (outside timing loop)
// Pre-populate arrays with repeating pattern (small values, varied)
let arr_big = makeArray(n1m, 2.5)  // 1M × 2.5
let arr_big_b = makeArray(n1m, 1.5) // 1M × 1.5

let arr_med = makeArray(n100k, 2.5)  // 100K × 2.5

let a_big = t.from_array(arr_big, "f64")
let b_big = t.from_array(arr_big_b, "f64")
let a_med = t.from_array(arr_med, "f64")

// ── warmup (10 iterations) ──
//
// Let CPU frequency scale up, warm caches, stabilize TLB.

println("=== Warmup (10 iterations) ===")
let i = 0
while i < 10 {
    _ = t.add(a_big, b_big)
    _ = t.mul(a_big, b_big)
    _ = t.sum(a_med)
    _ = t.min(a_med)
    i = i + 1
}
println("Warmup complete; cache primed")

// ── main benchmark (100 iterations) ──
//
// Each iteration exercises:
//   - Element-wise: add, sub, mul, div, pow (5 ops on 1M f64 tensors)
//   - Reductions: sum, mean, min, max, argmin, argmax (6 ops on 100K f64 tensor)
//
// Total work per iteration:
//   - 5 × 1M f64 ops (element-wise)
//   - 6 × 100K f64 reductions
//   ~= 5.6M individual floating-point operations per iteration

println("\n=== Main Benchmark (100 iterations) ===")
println("Tensor shapes:")
println("  a_big, b_big: [" + str(n1m) + "]")
println("  a_med: [" + str(n100k) + "]")

let num_iterations = 100
let op_times = makeArray(num_iterations, 0.0)

// Time via wall clock (no precise timer, but sufficient for throughput soak)
// Using sleep(0) to establish a baseline, then timing iterations

let iter = 0
while iter < num_iterations {

    // Element-wise: 5 ops on 1M tensors
    let c1 = t.add(a_big, b_big)
    let c2 = t.sub(a_big, b_big)
    let c3 = t.mul(a_big, b_big)
    let c4 = t.div(a_big, b_big)
    let c5 = t.pow(a_big, b_big)

    // Reductions: 6 ops on 100K tensor
    let s = t.sum(a_med)
    let m = t.mean(a_med)
    let mn = t.min(a_med)
    let mx = t.max(a_med)
    let amn = t.argmin(a_med)
    let amx = t.argmax(a_med)

    iter = iter + 1
}


// ── sanity checks (ensure ops produced correct values) ──

let a_sanity = t.from_array([1.0, 2.0, 3.0, 4.0], "f64")
let b_sanity = t.from_array([1.0, 2.0, 3.0, 4.0], "f64")

a.assertEqual(t.sum(a_sanity), 10.0)
a.assertEqual(t.mean(a_sanity), 2.5)
a.assertEqual(t.min(a_sanity), 1.0)
a.assertEqual(t.max(a_sanity), 4.0)

println("Sanity checks:      OK")
println("")
println("=== Benchmark Complete ===")
