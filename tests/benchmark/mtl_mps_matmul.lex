import "stdlib/mtl.lex" as mtl

if !mtl.isAvailable() {
    println("Metal unavailable")
    return
}

fn benchSize(sz) {
    println("=== ", sz, " × ", sz, " × ", sz, " ===")
    let A, _ = mtl.randomUnitVectors(sz, sz, 100)
    let B, _ = mtl.randomUnitVectors(sz, sz, 200)

    // Warm caches (first call compiles kernels / sets up MPS internals).
    _, _ = mtl.matmul(A, B, sz, sz, sz)
    _, _ = mtl.matmulMPS(A, B, sz, sz, sz)

    let t0 = _timeNanos()
    let naive, _ = mtl.matmul(A, B, sz, sz, sz)
    let t1 = _timeNanos()
    let naiveMs = float(t1 - t0) / 1000000.0

    let t2 = _timeNanos()
    let mps, _ = mtl.matmulMPS(A, B, sz, sz, sz)
    let t3 = _timeNanos()
    let mpsMs = float(t3 - t2) / 1000000.0

    // Spot-check correctness: pick one element from each, check diff.
    let diff = naive[5 * sz + 17] - mps[5 * sz + 17]
    if diff < 0.0 { diff = -diff }

    // FLOPS: 2 * m * k * n ops total (multiply + add per element).
    let ops = 2.0 * float(sz) * float(sz) * float(sz)
    let naiveGflops = ops / (naiveMs / 1000.0) / 1000000000.0
    let mpsGflops   = ops / (mpsMs   / 1000.0) / 1000000000.0

    println("  naive matmul: ", naiveMs, " ms  →  ", naiveGflops, " GFLOPS")
    println("  MPS matmul:   ", mpsMs,   " ms  →  ", mpsGflops,   " GFLOPS")
    println("  MPS speedup:  ", naiveMs / mpsMs, "×")
    println("  spot-check |naive - MPS|: ", diff)
    println("")
}

benchSize(256)
benchSize(512)
benchSize(1024)
