// frogpyMatmulBench.lex — measure FrogPy matmul throughput.
//
// Reports wall-clock time per matmul and effective GFLOPS for each
// (dtype, size) pair. GFLOPS = 2·m·k·n / time_seconds, the standard
// dense-matmul accounting (each output cell does k multiply-adds = 2k
// FLOPs; m·n output cells).
//
// What to look for:
//
// f32 (macOS):
//   - small sizes (≤256): MPS setup overhead dominates. Expect the
//     per-call time to be roughly constant — the GPU dispatch tax,
//     not the FLOPs.
//   - medium-large (512-2048): MPS amortises and approaches the
//     reported ~130 GFLOPS at 1024³ from stdlib/mtl.lex's reference
//     numbers (which used persistent buffers, so FrogPy with its
//     per-call upload/readback should sit slightly below that).
//
// f32 (Linux): single-threaded autovec CPU kernel. Throughput per
// thread is roughly memory-bandwidth-limited at small sizes and
// compute-bound at large sizes.
//
// f64 / i64: always CPU kernel. f64 should be ~2× slower than f32 at
// the same shape (double the bytes per element). i64 wraps on overflow
// rather than saturating, so peak GFLOPS may differ slightly.

import "stdlib/datetime.lex" as dt
import "stdlib/tensor.lex" as t

// runOne dispatches a single matmul of the requested (dtype, n) and
// returns ns-per-call. Adaptive iteration count: enough repetitions
// to amass ~500 ms of wall time, capped at 50 reps so very-large
// matmuls don't take forever.
fn runOne(dtype, n) {
    let a = t.zeros([n, n], dtype)
    let b = t.zeros([n, n], dtype)

    // Warmup (also primes any MPS shape-cache lookup on the f32 path).
    let _ = t.matmul(a, b)
    let _ = t.matmul(a, b)

    // Probe a single call to pick iteration count.
    let probeStart = dt.nowNanos()
    let _ = t.matmul(a, b)
    let probeEnd = dt.nowNanos()
    let probeNs = probeEnd - probeStart
    if probeNs < 1 {
        probeNs = 1
    }
    let target = 500000000
    let iters = target / probeNs
    if iters < 3 {
        iters = 3
    }
    if iters > 50 {
        iters = 50
    }

    let startNs = dt.nowNanos()
    let i = 0
    while i < iters {
        let _ = t.matmul(a, b)
        i = i + 1
    }
    let endNs = dt.nowNanos()

    let totalNs = endNs - startNs
    let nsPerCall = totalNs / iters

    // 2·n·n·n FLOPs per matmul; convert to GFLOPS = FLOPs / (ns·1e-9·1e9)
    //                                            = FLOPs / ns
    let flops = 2 * n * n * n
    let gflops = float(flops) / float(nsPerCall)

    let msPerCall = float(nsPerCall) / 1000000.0
    println("  " + dtype + "  n=" + str(n) +
            "  iters=" + str(iters) +
            "  ms/call=" + str(msPerCall) +
            "  GFLOPS=" + str(gflops))
    return nsPerCall
}

println("=== FrogPy matmul throughput ===")
println("")

// f32 — exercises the MPS path on macOS, CPU autovec on Linux.
println("f32 (macOS: MPS GPU; Linux: CPU autovec)")
let f32Sizes = [128, 256, 512, 1024, 2048]
let i = 0
while i < len(f32Sizes) {
    let _ = runOne("f32", f32Sizes[i])
    i = i + 1
}

println("")
println("f64 (CPU naive kernel, all platforms)")
let f64Sizes = [128, 256, 512, 1024]
i = 0
while i < len(f64Sizes) {
    let _ = runOne("f64", f64Sizes[i])
    i = i + 1
}

println("")
println("i64 (CPU naive kernel, all platforms)")
let i64Sizes = [128, 256, 512]
i = 0
while i < len(i64Sizes) {
    let _ = runOne("i64", i64Sizes[i])
    i = i + 1
}

println("")
println("=== done ===")
