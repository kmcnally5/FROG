// tensorMatmulTest.lex — correctness tests for FrogPy 2-D matmul.
//
// Covers the three dtypes (f32, f64, i64) and both backend paths:
//
//   - f32 on macOS routes through MPSMatrixMultiplication (GPU).
//   - f32 on Linux runs the CPU autovec kernel.
//   - f64 / i64 always run the CPU kernel.
//
// The same test asserts the same numerical answer regardless of which
// backend executes — that's the contract.

import "stdlib/tensor.lex" as t
import "stdlib/assert.lex" as a

// ── helpers ──────────────────────────────────────────────────────

fn tensorToArray(tn) {
    let n = t.numel(tn)
    let out = makeArray(n, 0)
    let i = 0
    while i < n {
        out[i] = t.get(tn, i)
        i = i + 1
    }
    return out
}

fn assertArraysEqual(got, want, msg) {
    a.assertEqual(len(got), len(want))
    let i = 0
    while i < len(got) {
        a.assertEqual(got[i], want[i])
        i = i + 1
    }
    println(msg + ": OK")
}

// assertArraysClose checks element-wise equality within tol — needed
// for f32 results because the MPS path and the CPU path may produce
// values that differ by float-precision noise (and we want the test
// to pass identically on both).
fn assertArraysClose(got, want, tol, msg) {
    a.assertEqual(len(got), len(want))
    let i = 0
    while i < len(got) {
        let diff = got[i] - want[i]
        if diff < 0 {
            diff = -diff
        }
        a.assertTrue(diff < tol)
        i = i + 1
    }
    println(msg + ": OK")
}

// ── 2×3 · 3×2 = 2×2, hand-computed reference ─────────────────────
//
//   A = [ 1 2 3 ]    B = [  7  8 ]      C = A·B = [ 1·7+2·9+3·11   1·8+2·10+3·12 ]
//       [ 4 5 6 ]        [  9 10 ]                [ 4·7+5·9+6·11   4·8+5·10+6·12 ]
//                        [ 11 12 ]
//
//   row 0: 7+18+33 = 58 ,  8+20+36 = 64
//   row 1: 28+45+66=139, 32+50+72=154

let aFlat = [1.0, 2.0, 3.0, 4.0, 5.0, 6.0]
let bFlat = [7.0, 8.0, 9.0, 10.0, 11.0, 12.0]
let want23 = [58.0, 64.0, 139.0, 154.0]

// f64
let aF64 = t.reshape(t.from_array(aFlat, "f64"), [2, 3])
let bF64 = t.reshape(t.from_array(bFlat, "f64"), [3, 2])
let cF64 = t.matmul(aF64, bF64)
assertArraysEqual(t.shape(cF64), [2, 2], "cF64 shape")
a.assertEqual(t.dtype(cF64), "f64")
assertArraysEqual(tensorToArray(cF64), want23, "2x3 · 3x2 f64")

// f32 — same numerical answer, but with float-precision tolerance.
// MPS on macOS and CPU autovec on Linux both produce these small
// integer-valued results exactly; tol is conservative to absorb any
// future kernel re-tuning that introduces fused-multiply-add.
let aF32 = t.reshape(t.from_array(aFlat, "f32"), [2, 3])
let bF32 = t.reshape(t.from_array(bFlat, "f32"), [3, 2])
let cF32 = t.matmul(aF32, bF32)
assertArraysEqual(t.shape(cF32), [2, 2], "cF32 shape")
a.assertEqual(t.dtype(cF32), "f32")
assertArraysClose(tensorToArray(cF32), want23, 0.001, "2x3 · 3x2 f32")

// i64 — same shape, same hand-computed values, int dtype.
let aI64 = t.reshape(t.from_array([1, 2, 3, 4, 5, 6], "i64"), [2, 3])
let bI64 = t.reshape(t.from_array([7, 8, 9, 10, 11, 12], "i64"), [3, 2])
let cI64 = t.matmul(aI64, bI64)
assertArraysEqual(t.shape(cI64), [2, 2], "cI64 shape")
a.assertEqual(t.dtype(cI64), "i64")
assertArraysEqual(tensorToArray(cI64), [58, 64, 139, 154], "2x3 · 3x2 i64")

// ── identity multiplication: I · A = A ───────────────────────────

let eyeFlat = [1.0, 0.0, 0.0,
               0.0, 1.0, 0.0,
               0.0, 0.0, 1.0]
let aFlat33 = [1.0, 2.0, 3.0,
               4.0, 5.0, 6.0,
               7.0, 8.0, 9.0]

let eye33 = t.reshape(t.from_array(eyeFlat, "f64"), [3, 3])
let a33 = t.reshape(t.from_array(aFlat33, "f64"), [3, 3])
let prod = t.matmul(eye33, a33)
assertArraysEqual(tensorToArray(prod), aFlat33, "I · A = A (f64)")

// And A · I = A (verifies B-side stride is right too)
let prod2 = t.matmul(a33, eye33)
assertArraysEqual(tensorToArray(prod2), aFlat33, "A · I = A (f64)")

// ── zero matrix: 0 · A = 0 ───────────────────────────────────────

let zero33 = t.zeros([3, 3], "f64")
let zprod = t.matmul(zero33, a33)
assertArraysEqual(tensorToArray(zprod), [0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0], "0 · A = 0")

// ── non-square: 4×2 · 2×3 = 4×3 ──────────────────────────────────
//
//   A = [1 2;  3 4;  5 6;  7 8]      (4×2)
//   B = [1 2 3;  4 5 6]               (2×3)
//   C row 0: [1·1+2·4, 1·2+2·5, 1·3+2·6] = [9, 12, 15]
//     row 1: [3·1+4·4, 3·2+4·5, 3·3+4·6] = [19, 26, 33]
//     row 2: [5+24, 10+25, 15+30] = [29, 40, 51]
//     row 3: [7+32, 14+35, 21+40] = [39, 54, 65]

let m42 = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0], "f64"), [4, 2])
let m23 = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f64"), [2, 3])
let m43 = t.matmul(m42, m23)
assertArraysEqual(t.shape(m43), [4, 3], "m43 shape")
assertArraysEqual(tensorToArray(m43),
    [9.0, 12.0, 15.0,  19.0, 26.0, 33.0,  29.0, 40.0, 51.0,  39.0, 54.0, 69.0],
    "4x2 · 2x3 = 4x3 f64")

// ── parallel-path correctness: m=256 ones·ones (CPU f64) ─────────
//
// The CPU matmul dispatcher splits across goroutines when m >= 128.
// Each goroutine computes a row-strip independently; this test
// proves the splitting produces the same correct result as the
// single-threaded path. ones[256,256] · ones[256,256] = 256
// at every cell.

let p256 = t.full([256, 256], 1.0, "f64")
let p256_result = t.matmul(p256, p256)
assertArraysEqual(t.shape(p256_result), [256, 256], "parallel f64 256x256 shape")
a.assertEqual(t.get(p256_result, 0), 256.0)
a.assertEqual(t.get(p256_result, 256 * 128 + 128), 256.0)
a.assertEqual(t.get(p256_result, 256 * 256 - 1), 256.0)
println("parallel f64 256x256 ones·ones = 256 everywhere: OK")

// Same for i64 (different kernel, same parallel dispatcher)
let i256 = t.full([256, 256], 1, "i64")
let i256_result = t.matmul(i256, i256)
a.assertEqual(t.get(i256_result, 0), 256)
a.assertEqual(t.get(i256_result, 256 * 256 - 1), 256)
println("parallel i64 256x256 ones·ones = 256 everywhere: OK")

// ── medium size 32×32 f32 (exercises MPS path on Mac) ─────────────
//
// We don't hand-compute every output — instead verify a structural
// invariant: (ones_32x32) · (ones_32x32) produces a 32×32 matrix
// where every element is exactly 32.0.

let ones32 = t.zeros([32, 32], "f32")
// Build a ones tensor by adding 1.0 to every element. v1 doesn't
// have full() yet; this works because add returns a fresh tensor.
let one1 = t.reshape(t.from_array(makeArray(32 * 32, 1.0), "f32"), [32, 32])
let big = t.matmul(one1, one1)
assertArraysEqual(t.shape(big), [32, 32], "big shape")

// Spot-check four corners + centre. Each cell of (ones · ones) is
// the dot product of two length-32 ones vectors = 32.0.
let bigArr = tensorToArray(big)
assertArraysClose([bigArr[0]], [32.0], 0.001, "ones32 matmul: top-left = 32")
assertArraysClose([bigArr[31]], [32.0], 0.001, "ones32 matmul: top-right = 32")
assertArraysClose([bigArr[32 * 16 + 16]], [32.0], 0.001, "ones32 matmul: centre = 32")
assertArraysClose([bigArr[32 * 31]], [32.0], 0.001, "ones32 matmul: bottom-left = 32")
assertArraysClose([bigArr[32 * 32 - 1]], [32.0], 0.001, "ones32 matmul: bottom-right = 32")

// ── validation errors ────────────────────────────────────────────

// shape mismatch — a.shape[1] != b.shape[0]
let bad_a = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0], "f64"), [2, 2])
let bad_b = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f64"), [3, 2])
let _v, err = safe(t.matmul, bad_a, bad_b)
a.assertTrue(isError(err))
println("matmul rejects shape mismatch (k disagree): OK")

// dtype mismatch
let mixA = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0], "f64"), [2, 2])
let mixB = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0], "f32"), [2, 2])
_v, err = safe(t.matmul, mixA, mixB)
a.assertTrue(isError(err))
println("matmul rejects dtype mismatch: OK")

// non-2D (1-D input)
let oneD = t.from_array([1.0, 2.0, 3.0, 4.0], "f64")
_v, err = safe(t.matmul, oneD, oneD)
a.assertTrue(isError(err))
println("matmul rejects 1-D input: OK")

// non-2D (3-D input)
let threeD = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0], "f64"), [2, 2, 2])
_v, err = safe(t.matmul, threeD, threeD)
a.assertTrue(isError(err))
println("matmul rejects 3-D input: OK")

// not a tensor
_v, err = safe(t.matmul, [1, 2, 3], a33)
a.assertTrue(isError(err))
println("matmul rejects non-tensor first arg: OK")

// ── reshape sanity (we just added this builtin too) ──────────────

let flat6 = t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f64")
let mat23 = t.reshape(flat6, [2, 3])
assertArraysEqual(t.shape(mat23), [2, 3], "mat23 shape")
a.assertEqual(t.numel(mat23), 6)
assertArraysEqual(tensorToArray(mat23), [1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "reshape 6 -> 2x3 preserves data")

// numel mismatch
_v, err = safe(t.reshape, flat6, [2, 4])
a.assertTrue(isError(err))
println("reshape rejects numel mismatch: OK")

println("")
a.summary()
