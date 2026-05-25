// frogpyMatmulVerify.lex — sanity-check FrogPy matmul numbers
// against an independent reference and the existing mtl.matmulMPS.
//
// Motivation: frogpyMatmulBench.lex reports ~1058 GFLOPS at 1024³
// (f32) — far above the 130 GFLOPS documented in stdlib/mtl.lex.
// Before believing the headline number this bench answers:
//
//   1. Does FrogPy produce the CORRECT result for non-zero inputs?
//      (Zeros could in principle be short-circuited; ones are a
//      trivial-but-verifiable reference: ones·ones[i,j] = n.)
//
//   2. Is FrogPy's time the same for zero inputs vs non-zero inputs?
//      If yes → MPS doesn't short-circuit on data values, so the
//      original bench's zero-input number is a real measurement of
//      the kernel work.
//
//   3. How does FrogPy compare to stdlib/mtl.lex's matmulMPS on the
//      SAME size + SAME data? Both call the same _mtlMatmulMPS
//      bridge. The only difference is buffer-upload path:
//        - FrogPy: tensor.F32 ([]float32) → mtl_buffer_create_f32
//                  (one memcpy at the cgo boundary, zero kLex
//                  marshalling)
//        - mtl.matmulMPS: kLex Array → _mtlBuffer (element-by-element
//                         kLex-Float-to-float32 conversion, then upload)
//      For a 1024² matrix that's 1M element marshallings per upload
//      × 2 input matrices + 1M Float allocations on readback. If the
//      delta accounts for the gap, the kernel itself is the documented
//      speed; FrogPy just avoids the marshalling tax.

import "stdlib/datetime.lex" as dt
import "stdlib/tensor.lex" as t
import "stdlib/mtl.lex" as mtl
import "stdlib/assert.lex" as a

// ── helpers ──────────────────────────────────────────────────────

fn timeNs(fn_to_run) {
    let s = dt.nowNanos()
    let _ = fn_to_run()
    let e = dt.nowNanos()
    return e - s
}

fn gflopsAt(n, ns) {
    let flops = 2 * n * n * n
    return float(flops) / float(ns)
}

fn isCloseF32(got, want, tol) {
    let d = got - want
    if d < 0 { d = -d }
    return d < tol
}

// Best-of-many run timer. Returns the MEDIAN ns/call across `iters`
// runs after `warmup` discarded runs. Median (not min) so a single
// thermal blip doesn't flatter the result; not the mean so a single
// stall doesn't tank it either.
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

    // Insertion sort (iters is small, 10-30).
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

// ── setup: n=1024 (4 MB per f32 matrix) ──────────────────────────

let N = 1024
println("=== FrogPy matmul verification ===")
println("N = " + str(N) + "  (f32, 4 MB per input matrix)")
println("")

// Build a kLex array of ones, then turn it into two forms:
//   - flat kLex Array (for mtl.matmulMPS, which takes Array)
//   - 2-D FrogPy tensor (for t.matmul)
println("Setting up input data — n=1024 means 1M elements; this takes a moment...")
let setupStart = dt.nowNanos()
let onesArr = makeArray(N * N, 1.0)
let onesT = t.reshape(t.from_array(onesArr, "f32"), [N, N])
let setupEnd = dt.nowNanos()
println("  setup: " + str(float(setupEnd - setupStart) / 1000000.0) + " ms")
println("")

// ── (1) correctness: ones·ones[i,j] should be exactly N ──────────

println("--- correctness check (ones · ones, expect every cell = " + str(N) + ") ---")

let correctTensor = t.matmul(onesT, onesT)
let c00 = t.get(correctTensor, 0)
let cMid = t.get(correctTensor, N * N / 2 + N / 2)
let cLast = t.get(correctTensor, N * N - 1)

println("  FrogPy ones·ones [0,0]   = " + str(c00))
println("  FrogPy ones·ones [mid]   = " + str(cMid))
println("  FrogPy ones·ones [last]  = " + str(cLast))
a.assertTrue(isCloseF32(c00, float(N), 0.01))
a.assertTrue(isCloseF32(cMid, float(N), 0.01))
a.assertTrue(isCloseF32(cLast, float(N), 0.01))

let mtlResult, mtlErr = mtl.matmulMPS(onesArr, onesArr, N, N, N)
if mtlErr != null {
    println("  mtl.matmulMPS error: " + str(mtlErr))
    return
}
let mc00 = mtlResult[0]
let mcMid = mtlResult[N * N / 2 + N / 2]
let mcLast = mtlResult[N * N - 1]
println("  mtl    ones·ones [0,0]   = " + str(mc00))
println("  mtl    ones·ones [mid]   = " + str(mcMid))
println("  mtl    ones·ones [last]  = " + str(mcLast))
a.assertTrue(isCloseF32(mc00, float(N), 0.01))
a.assertTrue(isCloseF32(mcMid, float(N), 0.01))
a.assertTrue(isCloseF32(mcLast, float(N), 0.01))

println("")

// ── (2) zeros vs ones timing ─────────────────────────────────────
//
// If MPS short-circuits on zero inputs, zeros will be much faster
// than ones. Same time = no short-circuit = the headline number is
// a real measurement of the kernel work.

println("--- zero vs non-zero data: does MPS short-circuit zeros? ---")
let zerosT = t.zeros([N, N], "f32")

fn runFrogPyZeros() {
    return t.matmul(zerosT, zerosT)
}
fn runFrogPyOnes() {
    return t.matmul(onesT, onesT)
}

let zerosNs = medianRun(runFrogPyZeros, 3, 10)
let onesNs  = medianRun(runFrogPyOnes,  3, 10)

println("  FrogPy zeros·zeros  median = " + str(float(zerosNs) / 1000000.0) + " ms  (" + str(gflopsAt(N, zerosNs)) + " GFLOPS)")
println("  FrogPy ones·ones    median = " + str(float(onesNs)  / 1000000.0) + " ms  (" + str(gflopsAt(N, onesNs))  + " GFLOPS)")

let ratio = float(onesNs) / float(zerosNs)
println("  ratio ones/zeros    = " + str(ratio) + "x")
println("  (≈1.0 means MPS does the FLOPs regardless of data — headline number is real)")
println("")

// ── (3) head-to-head: FrogPy vs mtl.matmulMPS ────────────────────
//
// Same MPS kernel under the hood; only the upload/readback path
// differs. Gap = marshalling cost.

println("--- head-to-head FrogPy vs mtl.matmulMPS (same kernel, different upload path) ---")

fn runMtl() {
    let r, _ = mtl.matmulMPS(onesArr, onesArr, N, N, N)
    return r
}

let frogPyNs = medianRun(runFrogPyOnes, 3, 10)
let mtlNs    = medianRun(runMtl,        3, 10)

println("  FrogPy (typed []float32 path)         median = " + str(float(frogPyNs) / 1000000.0) + " ms  (" + str(gflopsAt(N, frogPyNs)) + " GFLOPS)")
println("  mtl.matmulMPS (kLex Array marshalling) median = " + str(float(mtlNs)   / 1000000.0) + " ms  (" + str(gflopsAt(N, mtlNs))    + " GFLOPS)")

let speedup = float(mtlNs) / float(frogPyNs)
println("  FrogPy speedup vs mtl.matmulMPS = " + str(speedup) + "x")
println("  (gap is the kLex Array <-> []float32 marshalling tax; the MPS GPU")
println("   work is identical between the two paths)")

println("")
println("=== verification complete ===")
a.summary()
