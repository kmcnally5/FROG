// tensorAxisReductionsTest.lex — coverage for FrogPy axis-aware
// reductions: sum_axis, mean_axis, min_axis, max_axis,
// argmin_axis, argmax_axis. v1 scope is 2-D only.

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

// ── reference matrix (2×3, all dtypes) ───────────────────────────
//
//   M = [ 1  2  3 ]
//       [ 4  5  6 ]
//
//   sum axis 0 (down cols) = [5, 7, 9]
//   sum axis 1 (across rows) = [6, 15]
//   mean axis 0 = [2.5, 3.5, 4.5]
//   mean axis 1 = [2.0, 5.0]
//   min axis 0  = [1, 2, 3]    max axis 0 = [4, 5, 6]
//   min axis 1  = [1, 4]       max axis 1 = [3, 6]

let mF64 = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f64"), [2, 3])
let mF32 = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f32"), [2, 3])
let mI64 = t.reshape(t.from_array([1, 2, 3, 4, 5, 6], "i64"), [2, 3])

// ── sum_axis ──

assertArraysEqual(tensorToArray(t.sum_axis(mF64, 0)), [5.0, 7.0, 9.0], "sum_axis f64 axis 0")
assertArraysEqual(tensorToArray(t.sum_axis(mF64, 1)), [6.0, 15.0], "sum_axis f64 axis 1")
a.assertEqual(t.dtype(t.sum_axis(mF64, 0)), "f64")

assertArraysEqual(tensorToArray(t.sum_axis(mI64, 0)), [5, 7, 9], "sum_axis i64 axis 0")
assertArraysEqual(tensorToArray(t.sum_axis(mI64, 1)), [6, 15], "sum_axis i64 axis 1")
a.assertEqual(t.dtype(t.sum_axis(mI64, 0)), "i64")

// negative axis works too
assertArraysEqual(tensorToArray(t.sum_axis(mF64, -1)), [6.0, 15.0], "sum_axis axis -1 == axis 1")
assertArraysEqual(tensorToArray(t.sum_axis(mF64, -2)), [5.0, 7.0, 9.0], "sum_axis axis -2 == axis 0")

// f32 (precision tolerance not needed at this scale — values are exact ints)
assertArraysEqual(tensorToArray(t.sum_axis(mF32, 0)), [5.0, 7.0, 9.0], "sum_axis f32 axis 0")

// ── mean_axis (always returns f64) ──

assertArraysEqual(tensorToArray(t.mean_axis(mF64, 0)), [2.5, 3.5, 4.5], "mean_axis f64 axis 0")
assertArraysEqual(tensorToArray(t.mean_axis(mF64, 1)), [2.0, 5.0], "mean_axis f64 axis 1")
a.assertEqual(t.dtype(t.mean_axis(mF64, 0)), "f64")

// i64 input → f64 output (NumPy parity)
assertArraysEqual(tensorToArray(t.mean_axis(mI64, 0)), [2.5, 3.5, 4.5], "mean_axis i64 → f64 axis 0")
a.assertEqual(t.dtype(t.mean_axis(mI64, 0)), "f64")
println("mean_axis i64 produces f64: OK")

// ── min_axis / max_axis ──

assertArraysEqual(tensorToArray(t.min_axis(mF64, 0)), [1.0, 2.0, 3.0], "min_axis f64 axis 0")
assertArraysEqual(tensorToArray(t.min_axis(mF64, 1)), [1.0, 4.0], "min_axis f64 axis 1")
assertArraysEqual(tensorToArray(t.max_axis(mF64, 0)), [4.0, 5.0, 6.0], "max_axis f64 axis 0")
assertArraysEqual(tensorToArray(t.max_axis(mF64, 1)), [3.0, 6.0], "max_axis f64 axis 1")

assertArraysEqual(tensorToArray(t.min_axis(mI64, 0)), [1, 2, 3], "min_axis i64 axis 0")
assertArraysEqual(tensorToArray(t.max_axis(mI64, 1)), [3, 6], "max_axis i64 axis 1")

// ── argmin_axis / argmax_axis ──
//
// For M above:
//   argmin axis 0 = [0, 0, 0]  (smallest in each col is row 0)
//   argmin axis 1 = [0, 0]     (smallest in each row is col 0)
//   argmax axis 0 = [1, 1, 1]  (biggest in each col is row 1)
//   argmax axis 1 = [2, 2]     (biggest in each row is col 2)

assertArraysEqual(tensorToArray(t.argmin_axis(mF64, 0)), [0, 0, 0], "argmin_axis f64 axis 0")
assertArraysEqual(tensorToArray(t.argmin_axis(mF64, 1)), [0, 0], "argmin_axis f64 axis 1")
assertArraysEqual(tensorToArray(t.argmax_axis(mF64, 0)), [1, 1, 1], "argmax_axis f64 axis 0")
assertArraysEqual(tensorToArray(t.argmax_axis(mF64, 1)), [2, 2], "argmax_axis f64 axis 1")

a.assertEqual(t.dtype(t.argmin_axis(mF64, 0)), "i64")
println("argmin_axis returns i64 dtype regardless of input: OK")

// argmin with mixed signs — verify it's actually finding the min
let mixedFlat = [3.0, -1.0,  5.0, 2.0,  -7.0, 4.0]
let mixed = t.reshape(t.from_array(mixedFlat, "f64"), [2, 3])
// row 0 = [3, -1, 5];   row 1 = [2, -7, 4]
// argmin axis 1 (per row) = [1, 1]  (col 1 is smallest in both rows)
// argmax axis 1 (per row) = [2, 2]  (col 2 is largest in both rows)
assertArraysEqual(tensorToArray(t.argmin_axis(mixed, 1)), [1, 1], "argmin_axis mixed signs row")
assertArraysEqual(tensorToArray(t.argmax_axis(mixed, 1)), [2, 2], "argmax_axis mixed signs row")
// argmin axis 0 (per col) — col 0: [3,2]→row 1; col 1: [-1,-7]→row 1; col 2: [5,4]→row 1
assertArraysEqual(tensorToArray(t.argmin_axis(mixed, 0)), [1, 1, 1], "argmin_axis mixed signs col")

// ── ties: first-occurrence wins ──
//
//   T = [ 5  5  5 ]
//       [ 5  5  5 ]
//   argmin/argmax axis 0 = [0, 0, 0]  (row 0 wins on ties)
//   argmin/argmax axis 1 = [0, 0]    (col 0 wins on ties)

let ties = t.full([2, 3], 5.0, "f64")
assertArraysEqual(tensorToArray(t.argmin_axis(ties, 0)), [0, 0, 0], "argmin_axis ties axis 0 → row 0")
assertArraysEqual(tensorToArray(t.argmax_axis(ties, 0)), [0, 0, 0], "argmax_axis ties axis 0 → row 0")
assertArraysEqual(tensorToArray(t.argmin_axis(ties, 1)), [0, 0], "argmin_axis ties axis 1 → col 0")

// ── integration: column means via mean_axis + broadcasting ───────
//
// Centre each column by subtracting the per-column mean. Tests
// composition with broadcasting (mean_axis returns shape [n] which
// broadcasts cleanly against an [m, n] matrix).

let data = t.reshape(t.from_array([10.0, 20.0, 30.0, 12.0, 22.0, 32.0, 14.0, 24.0, 34.0], "f64"), [3, 3])
// per-col means: [12, 22, 32]
let colMeans = t.mean_axis(data, 0)
assertArraysEqual(tensorToArray(colMeans), [12.0, 22.0, 32.0], "per-col means")

let centred = t.sub(data, colMeans)
// expected: each row has its column's mean subtracted
//   row 0: [-2, -2, -2]
//   row 1: [ 0,  0,  0]
//   row 2: [ 2,  2,  2]
assertArraysEqual(tensorToArray(centred),
    [-2.0, -2.0, -2.0,  0.0, 0.0, 0.0,  2.0, 2.0, 2.0],
    "centre by column means: mean_axis + broadcast composition")

// ── error cases ──────────────────────────────────────────────────

// non-2D input
let oneD = t.from_array([1.0, 2.0, 3.0], "f64")
let _v, err = safe(t.sum_axis, oneD, 0)
a.assertTrue(isError(err))
println("sum_axis rejects 1-D input: OK")

let threeD = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0], "f64"), [2, 2, 2])
_v, err = safe(t.sum_axis, threeD, 0)
a.assertTrue(isError(err))
println("sum_axis rejects 3-D input (v1 = 2-D only): OK")

// axis out of range
_v, err = safe(t.sum_axis, mF64, 2)
a.assertTrue(isError(err))
println("sum_axis rejects axis 2 (only 0, 1 valid for 2-D): OK")

_v, err = safe(t.sum_axis, mF64, -3)
a.assertTrue(isError(err))
println("sum_axis rejects axis -3: OK")

// axis not an integer
_v, err = safe(t.sum_axis, mF64, 1.5)
a.assertTrue(isError(err))
println("sum_axis rejects non-integer axis: OK")

// mean on empty axis errors
let empty = t.zeros([0, 3], "f64")
_v, err = safe(t.mean_axis, empty, 0)
a.assertTrue(isError(err))
println("mean_axis rejects empty reduction axis: OK")

// but sum on empty axis returns zeros (identity)
let emptySum = t.sum_axis(empty, 0)
assertArraysEqual(t.shape(emptySum), [3], "sum_axis empty axis returns shape [3]")
assertArraysEqual(tensorToArray(emptySum), [0.0, 0.0, 0.0], "sum_axis empty axis returns zeros")

println("")
a.summary()
