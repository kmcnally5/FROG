import "stdlib/mtl.lex" as mtl

if !mtl.isAvailable() {
    println("Metal unavailable")
    return
}

// Test 1: pure sinusoid → spectrum has a peak at bin k (and N-k for the
// complex conjugate). A k=5 cosine over N=64 samples should peak at bins
// 5 and 59 (= 64-5).

let N = 64
let K_TARGET = 5
let twoPi = 6.283185307179586

println("Test 1 — N=", N, ", pure cosine at bin ", K_TARGET)

// Build N-element real signal as interleaved (re, im) with im=0.
let input = makeArray(2 * N, 0.0)
let i = 0
while i < N {
    input[2 * i] = cos(twoPi * float(K_TARGET) * float(i) / float(N))
    // input[2*i + 1] stays 0
    i = i + 1
}

let output, err = mtl.fft(input, N)
if err != null {
    println("fft failed:", err)
    return
}

// Find bin with largest |X[k]| = sqrt(re^2 + im^2).
let maxMag = 0.0
let maxIdx = -1
let k = 0
while k < N {
    let re = output[2 * k]
    let im = output[2 * k + 1]
    let mag = sqrt(re * re + im * im)
    if mag > maxMag {
        maxMag = mag
        maxIdx = k
    }
    k = k + 1
}
println("  peak bin: ", maxIdx, "  magnitude: ", maxMag,
        "  (expect bin 5 or 59, magnitude N/2 = ", N/2, ")")

// Sanity-check the second-largest peak — should be the conjugate at N-k.
let secondMag = 0.0
let secondIdx = -1
k = 0
while k < N {
    if k != maxIdx {
        let re = output[2 * k]
        let im = output[2 * k + 1]
        let mag = sqrt(re * re + im * im)
        if mag > secondMag {
            secondMag = mag
            secondIdx = k
        }
    }
    k = k + 1
}
println("  2nd peak: bin ", secondIdx, "  magnitude: ", secondMag,
        "  (expect ", N - maxIdx, ")")

// Test 2: FFT then IFFT should give back the original (within float precision).
// scaling: ifft is unscaled, so divide by N afterwards.
println("")
println("Test 2 — round-trip FFT → IFFT (should recover input)")

let inverse, err = mtl.ifft(output, N)
if err != null {
    println("ifft failed:", err)
    return
}

// Compare divided-by-N inverse to original input. Check max diff.
let maxDiff = 0.0
i = 0
while i < 2 * N {
    let recovered = inverse[i] / float(N)
    let d = recovered - input[i]
    if d < 0.0 { d = -d }
    if d > maxDiff { maxDiff = d }
    i = i + 1
}
println("  max |recovered - input|: ", maxDiff, "  (expect < 1e-5)")

// Test 3: larger FFT for timing
println("")
println("Test 3 — N=8192 FFT timing")
let N3 = 8192
let input3 = makeArray(2 * N3, 0.0)
i = 0
while i < N3 {
    input3[2 * i] = cos(twoPi * 73.0 * float(i) / float(N3))
    i = i + 1
}

// warm-up
_, _ = mtl.fft(input3, N3)

let t0 = _timeNanos()
let out3, _ = mtl.fft(input3, N3)
let t1 = _timeNanos()
let fftMs = float(t1 - t0) / 1000000.0
println("  FFT(8192) took ", fftMs, " ms")
