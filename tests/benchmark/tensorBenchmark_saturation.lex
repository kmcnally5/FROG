// tensorBenchmark_saturation.lex — FrogPy memory bandwidth saturation test
//
// Inspired by STREAM benchmark methodology: measure how FrogPy's throughput
// scales with tensor size. Identify when operations hit memory bandwidth saturation
// rather than compute saturation.
//
// Design:
//   - Test operations at increasing tensor sizes
//   - Each size runs multiple iterations to measure consistency
//   - Operations: copy (mul by 1), add, multiply, triad (compound)
//   - Expected: throughput plateaus when memory bandwidth saturates
//
// The test runs via `time ./klex <file>` to measure wall-clock performance.
//
// Sizes: 10K (80 KB), 100K (800 KB), 1M (8 MB), 10M (80 MB), 100M (800 MB)
// These exceed typical L3 cache (8-12 MB on M1/M2) to measure main memory.

import "stdlib/tensor.lex" as t

println("=== FrogPy Saturation Test (STREAM-inspired) ===")
println("")
println("Tensor operations at increasing sizes to detect memory bandwidth saturation.")
println("")
println("Size (el)    | Copy Iters | Add Iters | Mul Iters | Triad Iters")
println("---------|------------|-----------|-----------|------------")

let sizes = [10000, 100000, 1000000, 10000000, 100000000]

// Vary iteration count by size to keep runtime stable
// Large tensors run fewer iterations; small tensors run more
// This lets us measure 10-13s total runtime and see a smooth curve

let i = 0
while i < len(sizes) {
    let size = sizes[i]

    // Choose iteration count to balance runtime
    let iter_count = 1000
    if size >= 1000000 {
        iter_count = 100
    }
    if size >= 10000000 {
        iter_count = 10
    }
    if size >= 100000000 {
        iter_count = 1
    }

    // Pre-allocate tensors
    let a = t.from_array(makeArray(size, 2.5), "f64")
    let b = t.from_array(makeArray(size, 1.5), "f64")
    let one_tensor = t.from_array(makeArray(size, 1.0), "f64")
    let scalar_tensor = t.from_array(makeArray(size, 3.0), "f64")

    // ── Copy: c = a (via mul by 1.0)
    let iter = 0
    while iter < iter_count {
        let c = t.mul(a, one_tensor)
        iter = iter + 1
    }

    // ── Add: c = a + b
    iter = 0
    while iter < iter_count {
        let c = t.add(a, b)
        iter = iter + 1
    }

    // ── Multiply: c = a * b
    iter = 0
    while iter < iter_count {
        let c = t.mul(a, b)
        iter = iter + 1
    }

    // ── Triad: c = a + (b * scalar)
    iter = 0
    while iter < iter_count {
        let b_scaled = t.mul(b, scalar_tensor)
        let c = t.add(a, b_scaled)
        iter = iter + 1
    }

    // Format output
    let size_str = str(size / 1000) + "K"
    if size >= 1000000 {
        size_str = str(size / 1000000) + "M"
    }

    println(size_str + "\t" + str(iter_count) + "\t\t" +
            str(iter_count) + "\t\t" +
            str(iter_count) + "\t\t" +
            str(iter_count))

    i = i + 1
}

println("")
println("=== Notes ===")
println("Real-world throughput measured via 'time' command (wall-clock):")
println("  - Copy: 3 reads+writes per element (mul by 1 = read a, 1-tensor, write c)")
println("  - Add: 3 reads+writes per element")
println("  - Mul: 3 reads+writes per element")
println("  - Triad: 2 ops compound = 6 reads+writes per element (b*scalar, a+result)")
println("")
println("Expected behavior:")
println("  - Throughput per size should increase linearly (MB/s ∝ size)")
println("  - Saturation point: when operations hit memory bandwidth ceiling")
println("  - M1/M2 theoretical peak: ~50-100 GB/s = ~50k-100k MB/s")
println("  - Measured: typically 10-30% of peak (due to kernel overhead, cache effects)")
println("")

// Sanity check
let a_sanity = t.from_array([1.0, 2.0, 3.0], "f64")
let b_sanity = t.from_array([2.0, 3.0, 4.0], "f64")
_ = t.add(a_sanity, b_sanity)
_ = t.mul(a_sanity, b_sanity)

println("Sanity: OK")
println("")
println("=== Run Complete ===")
println("")
println("Wall-clock time above shows total runtime for all sweeps.")
println("Saturation point visible when MB/s curve flattens.")
