// tensorTransposeTest.lex — coverage for FrogPy 2-D transpose.
//
// Tests basic shape swap, all dtypes, edge cases (single row, single
// col, square), the involution property (transpose twice = identity),
// and the matmul interaction (A · Aᵀ is symmetric).

import "stdlib/tensor.lex" as t
import "stdlib/assert.lex" as a

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

// ── basic 2×3 → 3×2 transpose, hand-computed ─────────────────────
//
//   A = [ 1 2 3 ]      A^T = [ 1 4 ]
//       [ 4 5 6 ]            [ 2 5 ]
//                            [ 3 6 ]

let aF64 = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f64"), [2, 3])
let aT = t.transpose(aF64)
assertArraysEqual(t.shape(aT), [3, 2], "2x3 -> 3x2 shape")
assertArraysEqual(tensorToArray(aT), [1.0, 4.0, 2.0, 5.0, 3.0, 6.0], "2x3 f64 transpose values")

// Same for f32
let aF32 = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f32"), [2, 3])
let aT32 = t.transpose(aF32)
assertArraysEqual(t.shape(aT32), [3, 2], "2x3 f32 transpose shape")
assertArraysEqual(tensorToArray(aT32), [1.0, 4.0, 2.0, 5.0, 3.0, 6.0], "2x3 f32 transpose values")

// Same for i64
let aI64 = t.reshape(t.from_array([1, 2, 3, 4, 5, 6], "i64"), [2, 3])
let aTI = t.transpose(aI64)
assertArraysEqual(t.shape(aTI), [3, 2], "2x3 i64 transpose shape")
assertArraysEqual(tensorToArray(aTI), [1, 4, 2, 5, 3, 6], "2x3 i64 transpose values")

// ── involution: transpose(transpose(A)) == A ─────────────────────

let aTT = t.transpose(aT)
assertArraysEqual(t.shape(aTT), [2, 3], "transpose involution preserves shape")
assertArraysEqual(tensorToArray(aTT), [1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "transpose involution preserves values")

// ── single row (1, n) → (n, 1) ───────────────────────────────────

let row = t.reshape(t.from_array([10.0, 20.0, 30.0, 40.0], "f64"), [1, 4])
let rowT = t.transpose(row)
assertArraysEqual(t.shape(rowT), [4, 1], "(1,4) -> (4,1) shape")
assertArraysEqual(tensorToArray(rowT), [10.0, 20.0, 30.0, 40.0], "(1,4) transpose values (data unchanged)")

// ── single column (n, 1) → (1, n) ────────────────────────────────

let col = t.reshape(t.from_array([100.0, 200.0, 300.0], "f64"), [3, 1])
let colT = t.transpose(col)
assertArraysEqual(t.shape(colT), [1, 3], "(3,1) -> (1,3) shape")
assertArraysEqual(tensorToArray(colT), [100.0, 200.0, 300.0], "(3,1) transpose values")

// ── square (3, 3) ────────────────────────────────────────────────
//
//   M = [ 1 2 3 ]      M^T = [ 1 4 7 ]
//       [ 4 5 6 ]            [ 2 5 8 ]
//       [ 7 8 9 ]            [ 3 6 9 ]

let sq = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0], "f64"), [3, 3])
let sqT = t.transpose(sq)
assertArraysEqual(t.shape(sqT), [3, 3], "square transpose shape")
assertArraysEqual(tensorToArray(sqT), [1.0, 4.0, 7.0, 2.0, 5.0, 8.0, 3.0, 6.0, 9.0], "3x3 transpose values")

// ── matmul interaction: A · A^T is symmetric ─────────────────────
//
// For any matrix A, the product A · A^T is symmetric: result[i,j] ==
// result[j,i]. This verifies transpose is wired correctly into the
// matmul pipeline.

let ATA_input = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f64"), [2, 3])
let ATA = t.matmul(ATA_input, t.transpose(ATA_input))
assertArraysEqual(t.shape(ATA), [2, 2], "A · A^T shape (2x3 · 3x2 = 2x2)")
// ATA[0,0] = [1,2,3] · [1,2,3] = 1+4+9 = 14
// ATA[0,1] = [1,2,3] · [4,5,6] = 4+10+18 = 32
// ATA[1,0] = [4,5,6] · [1,2,3] = 4+10+18 = 32  (symmetric)
// ATA[1,1] = [4,5,6] · [4,5,6] = 16+25+36 = 77
assertArraysEqual(tensorToArray(ATA), [14.0, 32.0, 32.0, 77.0], "A · A^T values (and symmetric)")

// ── error: non-2D input ──────────────────────────────────────────

let oneD = t.from_array([1.0, 2.0, 3.0], "f64")
let _v, err = safe(t.transpose, oneD)
a.assertTrue(isError(err))
println("transpose rejects 1-D input: OK")

let threeD = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0], "f64"), [2, 2, 2])
_v, err = safe(t.transpose, threeD)
a.assertTrue(isError(err))
println("transpose rejects 3-D input: OK")

// ── error: non-tensor ────────────────────────────────────────────

_v, err = safe(t.transpose, [1, 2, 3])
a.assertTrue(isError(err))
println("transpose rejects non-tensor input: OK")

println("")
a.summary()
