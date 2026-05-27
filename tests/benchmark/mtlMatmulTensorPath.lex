// mtlMatmulTensorPath.lex — verify the three clean matmul lanes
// (post 2026-05-27 FROG-cleanup of the mtl matmul surface):
//
//   (A) t.matmul(tensorA, tensorB)
//       — pure FrogPy, fully typed, direct sync MPS dispatch
//   (B) mtl.matmulMPSTensor(tensorA, tensorB)
//       — lower-level tensor entry point: zero-marshalling upload
//         (_mtlBufferFromTensor) AND zero-allocation readback
//         (_mtlReadBufferIntoTensor). Same kernel as (A); slight
//         overhead from the stdlib await-channel hop.
//   (C) mtl.matmulMPS(arrayA, arrayB, m, k, n)
//       — strict Array-only legacy path: per-element upload
//         conversion + per-element readback marshalling.
//
// Expectation: (A) ≈ (B) — both do full zero-marshalling on upload
// AND readback through the same MPS kernel; (B) pays a small
// channel-hop overhead. (C) is significantly slower than both
// because of the Array marshalling tax on both sides.

import "stdlib/datetime.lex" as dt
import "stdlib/tensor.lex" as t
import "stdlib/mtl.lex" as mtl
import "stdlib/assert.lex" as a

fn timeNs(fn_to_run) {
    let s = dt.nowNanos()
    let _ = fn_to_run()
    let e = dt.nowNanos()
    return e - s
}

fn medianRun(fn_to_run, warmup, iters) {
    let w = 0
    while w < warmup {
        let _ = fn_to_run()
        w = w + 1
    }
    let times = makeArray(iters, 0)
    let i = 0
    while i < iters {
        times[i] = timeNs(fn_to_run)
        i = i + 1
    }
    let s = 1
    while s < iters {
        let v = times[s]
        let j = s - 1
        while j >= 0 && times[j] > v {
            times[j + 1] = times[j]
            j = j - 1
        }
        times[j + 1] = v
        s = s + 1
    }
    return times[iters / 2]
}

fn gflopsAt(n, ns) {
    let flops = 2 * n * n * n
    return float(flops) / float(ns)
}

let N = 1024
println("=== mtl.matmulMPS tensor fast-path verification ===")
println("N = " + str(N) + " f32  (4 MB per input)")
println("")

// Build inputs both as tensors and as kLex Arrays from the same data.
// Use t.full now that it exists — no more O(n^2) ceremony.
println("setup...")
let setupStart = dt.nowNanos()
let onesArr = makeArray(N * N, 1.0)
let onesT = t.full([N, N], 1.0, "f32")
let setupEnd = dt.nowNanos()
println("  setup: " + str(float(setupEnd - setupStart) / 1000000.0) + " ms (note: t.full skips kLex Array allocation)")
println("")

// --- (A) t.matmul (pure FrogPy) ---
fn runFrogPy() { return t.matmul(onesT, onesT) }
let nsA = medianRun(runFrogPy, 3, 10)
println("(A) t.matmul(tensor, tensor)                 median = " +
    str(float(nsA) / 1000000.0) + " ms  (" + str(gflopsAt(N, nsA)) + " GFLOPS)")

// --- (B) mtl.matmulMPSTensor (zero-marshalling both sides) ---
fn runMtlT() {
    let r, _ = mtl.matmulMPSTensor(onesT, onesT)
    return r
}
let nsB = medianRun(runMtlT, 3, 10)
println("(B) mtl.matmulMPSTensor(tensor, tensor)      median = " +
    str(float(nsB) / 1000000.0) + " ms  (" + str(gflopsAt(N, nsB)) + " GFLOPS)")

// --- (C) mtl.matmulMPS with Array inputs (legacy slow upload) ---
fn runMtlArr() {
    let r, _ = mtl.matmulMPS(onesArr, onesArr, N, N, N)
    return r
}
let nsC = medianRun(runMtlArr, 3, 10)
println("(C) mtl.matmulMPS(array, array, m,k,n)       median = " +
    str(float(nsC) / 1000000.0) + " ms  (" + str(gflopsAt(N, nsC)) + " GFLOPS)")

println("")
println("Speedups:")
println("  (B) vs (C) — tensor entry point vs Array   = " + str(float(nsC) / float(nsB)) + "x")
println("  (A) vs (B) — t.matmul vs mtl tensor entry  = " + str(float(nsB) / float(nsA)) + "x (delta = stdlib await-channel hop)")

// Strict-lane sanity checks: each function now rejects the wrong input type.
let _v1, err1 = safe(mtl.matmulMPS, onesT, onesT, N, N, N)
a.assertTrue(isError(err1))
let _v2, err2 = safe(mtl.matmulMPSTensor, onesArr, onesArr)
a.assertTrue(isError(err2))
println("")
println("matmulMPS rejects tensor input:        OK")
println("matmulMPSTensor rejects Array input:   OK")

println("")
a.summary()
