// frogpyDiagnostic.lex — FrogPy probe #1: bandwidth-vs-size sweep
//
// Purpose: measure effective memory bandwidth (GB/s) for _tensor_add at
// six tensor sizes spanning four orders of magnitude. The shape of the
// curve tells us where the bottleneck lives:
//
//   - If GB/s rises monotonically with N and plateaus only at huge N,
//     the cgo call + Go allocation overhead is dominating small ops.
//     Fix: fusion, in-place ops, batched dispatch.
//
//   - If GB/s is flat across all sizes (and well below ~100 GB/s on M4),
//     the kernel itself is the limit. Most likely cause: autovectoriser
//     bailed (no `restrict` on pointers) so we're running scalar SIMD-
//     less code. Fix: add restrict or hand-NEON intrinsics.
//
//   - If GB/s starts high and falls off a cliff past ~L2 size (~16 MB
//     per buffer on M4), we're DRAM-bandwidth-bound — that's the actual
//     hardware ceiling, not a software bug.
//
// Methodology:
//   - f64 add only (representative element-wise op)
//   - For each N, run enough iterations so total time >= 200 ms
//   - Effective bytes per call (kernel work only) = 3 * N * 8
//     (read a, read b, write out)
//   - Report ns/call, ns/element, GB/s

import "stdlib/datetime.lex" as dt
import "stdlib/tensor.lex" as t

fn benchSize(n) {
    // Build inputs once
    let arrA = makeArray(n, 1.5)
    let arrB = makeArray(n, 2.5)
    let a = t.from_array(arrA, "f64")
    let b = t.from_array(arrB, "f64")

    // Warmup: 3 calls to amortise first-touch + cache effects
    _ = t.add(a, b)
    _ = t.add(a, b)
    _ = t.add(a, b)

    // Calibrate iteration count so total runtime ~= 200 ms.
    // Time a single call to estimate.
    let t0 = dt.nowNanos()
    _ = t.add(a, b)
    let t1 = dt.nowNanos()
    let oneCallNs = t1 - t0
    if oneCallNs < 1 {
        oneCallNs = 1
    }
    let target = 200000000  // 200 ms in ns
    let iters = target / oneCallNs
    if iters < 5 {
        iters = 5
    }
    if iters > 100000 {
        iters = 100000
    }

    // Timed run
    let start = dt.nowNanos()
    let i = 0
    while i < iters {
        _ = t.add(a, b)
        i = i + 1
    }
    let end = dt.nowNanos()

    let totalNs = end - start
    let nsPerCall = totalNs / iters
    let nsPerElem = totalNs / (iters * n)

    // GB/s: 3 buffers (a, b, out) * 8 bytes/elem * n elems per call
    // Total bytes moved across iters = iters * n * 3 * 8
    // GB/s = bytes / seconds / 1e9
    //      = (iters * n * 24) / (totalNs / 1e9) / 1e9
    //      = (iters * n * 24) / totalNs
    // (the two 1e9s cancel)
    let bytesPerCall = n * 24
    let totalBytes = iters * bytesPerCall
    // Use float math for GB/s
    let gbPerSec = float(totalBytes) / float(totalNs)

    println("N=" + str(n) + "  iters=" + str(iters) + "  ns/call=" + str(nsPerCall) + "  ns/elem=" + str(nsPerElem) + "  GB/s=" + str(gbPerSec))
}

println("=== FrogPy probe #1: bandwidth vs tensor size (f64 add) ===")
println("Theoretical M4 sustained BW: ~100 GB/s (LPDDR5X-7500, ~120 GB/s peak)")
println("")

benchSize(1000)         // 1K  -- 24 KB total, fits in L1
benchSize(10000)        // 10K -- 240 KB total, fits in L2
benchSize(100000)       // 100K -- 2.4 MB total, fits in SLC
benchSize(1000000)      // 1M -- 24 MB total, spills to DRAM
benchSize(10000000)     // 10M -- 240 MB, fully DRAM bound
benchSize(100000000)    // 100M -- 2.4 GB, DRAM bound + paging risk

println("")
println("=== Interpretation ===")
println("Rising curve + low small-N => cgo/alloc overhead dominates small ops")
println("Flat curve well below 100 GB/s => kernel-bound (likely scalar fallback)")
println("Cliff past 10M => normal DRAM ceiling")
