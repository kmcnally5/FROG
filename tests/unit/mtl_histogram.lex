import "stdlib/mtl.lex" as mtl

// Test 1: known-answer distribution.
// 10 values in [0, 10), 5 bins of width 2 → expect [2, 2, 2, 2, 2]
let arr1 = [0.5, 1.5, 2.5, 3.5, 4.5, 5.5, 6.5, 7.5, 8.5, 9.5]
let counts1, err = mtl.histogram(arr1, 5, 0.0, 10.0)
if err != null {
    println("histogram failed:", err)
    return
}
println("Test 1 — uniform 10 values into 5 bins:")
println("  counts:", counts1, "  (expect [2, 2, 2, 2, 2])")

// Test 2: skewed distribution
// Values clustered in lower half
let arr2 = [0.1, 0.2, 0.3, 1.0, 1.5, 1.9, 8.5, 9.5]
let counts2, err = mtl.histogram(arr2, 4, 0.0, 10.0)
if err != null {
    println("test 2 failed:", err)
    return
}
println("Test 2 — skewed 8 values into 4 bins of width 2.5:")
println("  counts:", counts2, "  (expect [3, 3, 0, 2])")

// Test 3: clamping behaviour — out-of-range values land in first/last bin.
let arr3 = [-5.0, 0.5, 1.5, 5.0, 11.0, 99.0]
let counts3, err = mtl.histogram(arr3, 2, 0.0, 2.0)
if err != null {
    println("test 3 failed:", err)
    return
}
println("Test 3 — out-of-range values clamp to edges:")
println("  counts:", counts3, "  (expect [2, 4] — two values < 0 land in bin 0; 11.0, 99.0, 5.0 clamp to bin 1; 1.5 → bin 1; 0.5 → bin 0)")

// Test 4: scale — 100k random values into 20 bins, check total count.
// Use GPU-generated unit vectors (range roughly -0.5 to 0.5 after scale).
let flat, _ = mtl.randomUnitVectors(100, 1000, 42)
// flat is 100*1000 = 100000 floats, normalised to unit length.
// Each "vector" has only 1 component, so values are exactly ±1.0.
// Use a different generator path that produces a real distribution.
// Generate via the same kernel at smaller batch sizes:
let batch, _ = mtl.randomUnitVectors(1000, 100, 7)
// 100k floats, roughly distributed in [-0.2, 0.2] after normalisation.

let mn, _ = mtl.reduceMin(batch)
let mx, _ = mtl.reduceMax(batch)
println("Test 4 — 100k floats, observed range:")
println("  min =", mn, "  max =", mx)

let bigCounts, err = mtl.histogram(batch, 20, mn, mx)
if err != null {
    println("test 4 failed:", err)
    return
}
let total = 0
let i = 0
while i < len(bigCounts) {
    total = total + bigCounts[i]
    i = i + 1
}
println("  20 bins, total count:", total, "  (expect", len(batch), ")")
println("  counts:", bigCounts)
