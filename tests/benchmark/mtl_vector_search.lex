// mtl_vector_search.lex — GPU vs CPU dot-product (cosine similarity)
// benchmark for kLex's Metal subsystem.
//
// Three measurements:
//   1. CPU baseline at N=500   — pure-kLex loop
//   2. GPU at the same N=500    — apples-to-apples speedup
//   3. GPU at N=10000           — shows the scale CPU can't reach
//
// Test data is generated ON THE GPU via mtl.randomUnitVectors —
// pure-kLex random+normalise is ~250× slower per vector than the
// interpreter takes to set up the buffer it then hands to Metal.

import "stdlib/mtl.lex" as mtl

if !mtl.isAvailable() {
    println("Metal unavailable on this platform — exiting")
    return
}
let info, _ = mtl.device()
println("Device: ", info["name"])
println("")

fn cpuDotProducts(q, batch, n, dim) {
    let out = makeArray(n, 0.0)
    let i = 0
    while i < n {
        let sum  = 0.0
        let base = i * dim
        let j = 0
        while j < dim {
            sum = sum + q[j] * batch[base + j]
            j = j + 1
        }
        out[i] = sum
        i = i + 1
    }
    return out
}

fn absDiff(a, b) {
    let d = a - b
    if d < 0.0 { d = -d }
    return d
}

let D = 128
let N = 500
let seed = 12345

println("Test 1 — N=", N, ", D=", D)

let t0 = _timeNanos()
let batch, err = mtl.randomUnitVectors(N, D, seed)
let t1 = _timeNanos()
if err != null {
    println("vector generation failed: ", err)
    return
}
let genMs = float(t1 - t0) / 1000000.0
println("  generated ", N, "×", D, " vectors on GPU in ", genMs, " ms")

let qFlat, _ = mtl.randomUnitVectors(1, D, seed + 1)
let query = qFlat

// Warm the kernel cache.
_, _ = mtl.batchDot(query, batch, D)

let t2 = _timeNanos()
let cpuOut = cpuDotProducts(query, batch, N, D)
let t3 = _timeNanos()
let cpuMs = float(t3 - t2) / 1000000.0

let t4 = _timeNanos()
let gpuOut, gerr = mtl.batchDot(query, batch, D)
let t5 = _timeNanos()
if gerr != null {
    println("GPU dispatch failed: ", gerr)
    return
}
let gpuMs = float(t5 - t4) / 1000000.0

let maxDiff = 0.0
let k = 0
while k < N {
    let d = absDiff(cpuOut[k], gpuOut[k])
    if d > maxDiff { maxDiff = d }
    k = k + 1
}

println("  CPU:     ", cpuMs, " ms")
println("  GPU:     ", gpuMs, " ms")
let speedup1 = cpuMs / gpuMs
println("  speedup: ", speedup1, "×")
println("  max |CPU − GPU| diff: ", maxDiff, "  (float32 precision floor ≈ 1e-6)")
println("")

let N2 = 10000
let estCpuMs = cpuMs * N2 / N
println("Test 2 — GPU at N=", N2, ", D=", D)
println("  (CPU would take an estimated ", estCpuMs / 1000.0, " s)")

let t6 = _timeNanos()
let batch2, _ = mtl.randomUnitVectors(N2, D, seed)
let t7 = _timeNanos()
let gen2 = float(t7 - t6) / 1000000.0
println("  generated ", N2, " vectors on GPU in ", gen2, " ms")

let t8 = _timeNanos()
let gpuOut2, gerr = mtl.batchDot(query, batch2, D)
let t9 = _timeNanos()
if gerr != null {
    println("GPU dispatch failed: ", gerr)
    return
}
let gpuMs2 = float(t9 - t8) / 1000000.0
println("  GPU dot-product across ", N2, " vectors: ", gpuMs2, " ms")

fn topK(scores, k) {
    let n = len(scores)
    let idxs = makeArray(k, 0)
    let vals = makeArray(k, -1000000000.0)
    let i = 0
    while i < n {
        let s = scores[i]
        let j = 0
        while j < k && s <= vals[j] {
            j = j + 1
        }
        if j < k {
            let m = k - 1
            while m > j {
                vals[m] = vals[m-1]
                idxs[m] = idxs[m-1]
                m = m - 1
            }
            vals[j] = s
            idxs[j] = i
        }
        i = i + 1
    }
    return idxs, vals
}

let t10 = _timeNanos()
let topIdxs, topVals = topK(gpuOut2, 3)
let t11 = _timeNanos()
let topMs = float(t11 - t10) / 1000000.0
println("  top-K (k=3) on CPU:     ", topMs, " ms")
println("  best match: idx=", topIdxs[0], "  score=", topVals[0])
println("  2nd:        idx=", topIdxs[1], "  score=", topVals[1])
println("  3rd:        idx=", topIdxs[2], "  score=", topVals[2])

println("")
println("================================================================")
println("  Test 1 speedup at N=", N, ": ", speedup1, "×")
println("  Test 2 GPU-only at N=", N2, ": ", gpuMs2, " ms")
println("================================================================")
