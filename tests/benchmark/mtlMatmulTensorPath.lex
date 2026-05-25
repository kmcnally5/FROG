// mtlMatmulTensorPath.lex — verify the Tier 2 win:
// mtl.matmulMPS(tensor, tensor, ...) should now skip the kLex Array
// element-by-element marshalling that mtl.matmulMPS(array, array, ...)
// pays per call.
//
// Three paths at the same size + same data:
//   (A) t.matmul(tensorA, tensorB)
//       — pure FrogPy, fully typed, fast upload AND fast readback
//   (B) mtl.matmulMPS(tensorA, tensorB, m, k, n)
//       — new path: fast upload via _mtlBufferFromTensor; readback
//         still returns kLex Array (preserves API)
//   (C) mtl.matmulMPS(arrayA, arrayB, m, k, n)
//       — legacy path: slow upload + slow readback
//
// Expectation: (A) ≈ (B) on the upload side; (B) is slower than (A)
// by roughly the readback marshalling cost.  (C) is significantly
// slower than both because of the upload tax.

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

// --- (B) mtl.matmulMPS with tensor inputs (new fast upload) ---
fn runMtlT() {
    let r, _ = mtl.matmulMPS(onesT, onesT, N, N, N)
    return r
}
let nsB = medianRun(runMtlT, 3, 10)
println("(B) mtl.matmulMPS(tensor, tensor, m,k,n)     median = " +
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
println("  (B) vs (C) — tensor input fast path        = " + str(float(nsC) / float(nsB)) + "x")
println("  (A) vs (B) — FrogPy vs mtl tensor path     = " + str(float(nsB) / float(nsA)) + "x (delta = readback marshalling)")

// Quick mixed-input sanity: should error cleanly.
let _v, err = safe(mtl.matmulMPS, onesT, onesArr, N, N, N)
a.assertTrue(isError(err))
println("")
println("mixed Array+Tensor input rejected cleanly: OK")

println("")
a.summary()
