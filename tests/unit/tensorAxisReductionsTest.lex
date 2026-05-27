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

// 1-D input now reduces to a scalar tensor (covered further down)
// — the old 2-D-only restriction was lifted in the N-D rewrite.
let _v = null
let err = null

// axis out of range for 2-D
_v, err = safe(t.sum_axis, mF64, 2)
a.assertTrue(isError(err))
println("sum_axis rejects axis 2 (out of range for 2-D shape): OK")

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

// ── N-D coverage ───────────────────────────────────────────────
//
// Reference 3-D tensor shape [2, 2, 3], values 1..12 row-major:
//
//   [[[ 1,  2,  3],
//     [ 4,  5,  6]],
//    [[ 7,  8,  9],
//     [10, 11, 12]]]
//
// Per-axis expected sums:
//   axis 0 → shape [2, 3], flat [8, 10, 12, 14, 16, 18]
//   axis 1 → shape [2, 3], flat [5, 7, 9, 17, 19, 21]
//   axis 2 → shape [2, 2], flat [6, 15, 24, 33]
println("")
println("── N-D axis reductions ──")

let m3 = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0, 12.0], "f64"), [2, 2, 3])

let s0 = t.sum_axis(m3, 0)
assertArraysEqual(t.shape(s0), [2, 3], "3-D sum_axis axis 0 shape [2, 3]")
assertArraysEqual(tensorToArray(s0), [8.0, 10.0, 12.0, 14.0, 16.0, 18.0], "3-D sum_axis axis 0 values")

let s1 = t.sum_axis(m3, 1)
assertArraysEqual(t.shape(s1), [2, 3], "3-D sum_axis axis 1 shape [2, 3]")
assertArraysEqual(tensorToArray(s1), [5.0, 7.0, 9.0, 17.0, 19.0, 21.0], "3-D sum_axis axis 1 values")

let s2 = t.sum_axis(m3, 2)
assertArraysEqual(t.shape(s2), [2, 2], "3-D sum_axis axis 2 shape [2, 2]")
assertArraysEqual(tensorToArray(s2), [6.0, 15.0, 24.0, 33.0], "3-D sum_axis axis 2 values")

// Negative axis: -1 == axis 2 (last); -3 == axis 0 (first)
let sNeg1 = t.sum_axis(m3, -1)
assertArraysEqual(tensorToArray(sNeg1), tensorToArray(s2), "3-D sum_axis axis -1 == axis 2")
let sNeg3 = t.sum_axis(m3, -3)
assertArraysEqual(tensorToArray(sNeg3), tensorToArray(s0), "3-D sum_axis axis -3 == axis 0")

// mean: same shape, divided by reduceLen. axis 2 reduces over 3 elements.
let me2 = t.mean_axis(m3, 2)
assertArraysEqual(t.shape(me2), [2, 2], "3-D mean_axis axis 2 shape [2, 2]")
assertArraysEqual(tensorToArray(me2), [2.0, 5.0, 8.0, 11.0], "3-D mean_axis axis 2 values (sum/3)")

// min/max along axis 2: per-row min is row[0]; per-row max is row[2]
let mn2 = t.min_axis(m3, 2)
assertArraysEqual(tensorToArray(mn2), [1.0, 4.0, 7.0, 10.0], "3-D min_axis axis 2 values")
let mx2 = t.max_axis(m3, 2)
assertArraysEqual(tensorToArray(mx2), [3.0, 6.0, 9.0, 12.0], "3-D max_axis axis 2 values")

// argmin/argmax along axis 2: positions within the reduced axis.
// Every row is sorted ascending, so argmin=0 and argmax=2 for all output cells.
let am2 = t.argmin_axis(m3, 2)
assertArraysEqual(tensorToArray(am2), [0, 0, 0, 0], "3-D argmin_axis axis 2 values")
let ax2 = t.argmax_axis(m3, 2)
assertArraysEqual(tensorToArray(ax2), [2, 2, 2, 2], "3-D argmax_axis axis 2 values")

// argmin/argmax along axis 0: index 0 is always the smaller block.
let am0 = t.argmin_axis(m3, 0)
assertArraysEqual(t.shape(am0), [2, 3], "3-D argmin_axis axis 0 shape")
assertArraysEqual(tensorToArray(am0), [0, 0, 0, 0, 0, 0], "3-D argmin_axis axis 0 (all 0)")
let ax0 = t.argmax_axis(m3, 0)
assertArraysEqual(tensorToArray(ax0), [1, 1, 1, 1, 1, 1], "3-D argmax_axis axis 0 (all 1)")

// ── keepdims variants ──────────────────────────────────────────
println("")
println("── keepdims variants ──")

let sk2 = t.sum_axis_keepdims(m3, 2)
assertArraysEqual(t.shape(sk2), [2, 2, 1], "sum_axis_keepdims axis 2 shape [2, 2, 1]")
assertArraysEqual(tensorToArray(sk2), [6.0, 15.0, 24.0, 33.0], "sum_axis_keepdims axis 2 values match drop")

let sk0 = t.sum_axis_keepdims(m3, 0)
assertArraysEqual(t.shape(sk0), [1, 2, 3], "sum_axis_keepdims axis 0 shape [1, 2, 3]")
assertArraysEqual(tensorToArray(sk0), [8.0, 10.0, 12.0, 14.0, 16.0, 18.0], "sum_axis_keepdims axis 0 values match drop")

let mek2 = t.mean_axis_keepdims(m3, 2)
assertArraysEqual(t.shape(mek2), [2, 2, 1], "mean_axis_keepdims axis 2 shape")
assertArraysEqual(tensorToArray(mek2), [2.0, 5.0, 8.0, 11.0], "mean_axis_keepdims axis 2 values")

let mnk1 = t.min_axis_keepdims(m3, 1)
assertArraysEqual(t.shape(mnk1), [2, 1, 3], "min_axis_keepdims axis 1 shape")
let mxk1 = t.max_axis_keepdims(m3, 1)
assertArraysEqual(t.shape(mxk1), [2, 1, 3], "max_axis_keepdims axis 1 shape")

let amk2 = t.argmin_axis_keepdims(m3, 2)
assertArraysEqual(t.shape(amk2), [2, 2, 1], "argmin_axis_keepdims axis 2 shape")
let axk2 = t.argmax_axis_keepdims(m3, 2)
assertArraysEqual(t.shape(axk2), [2, 2, 1], "argmax_axis_keepdims axis 2 shape")

// The whole point of keepdims: broadcast back without expand_dims.
// Centring along the last axis (layer-norm style).
let centred = t.sub(m3, mek2)
// Row [1, 2, 3] minus its mean 2 → [-1, 0, 1]
// Row [4, 5, 6] minus its mean 5 → [-1, 0, 1]
// All four rows produce the same pattern.
assertArraysEqual(tensorToArray(centred), [-1.0, 0.0, 1.0, -1.0, 0.0, 1.0, -1.0, 0.0, 1.0, -1.0, 0.0, 1.0], "x - mean(x, -1, keepdims) broadcasts cleanly")

// ── 1-D edge case ──────────────────────────────────────────────
let v = t.from_array([3.0, 1.0, 4.0, 1.0, 5.0], "f64")
let vsum = t.sum_axis(v, 0)
assertArraysEqual(t.shape(vsum), [], "1-D sum_axis axis 0 → scalar tensor (shape [])")
a.assertEqual(t.get(vsum, 0), 14.0)
println("1-D sum_axis axis 0 == 14: OK")

let vsumKd = t.sum_axis_keepdims(v, 0)
assertArraysEqual(t.shape(vsumKd), [1], "1-D sum_axis_keepdims axis 0 → shape [1]")
a.assertEqual(t.get(vsumKd, 0), 14.0)
println("1-D sum_axis_keepdims axis 0 == 14: OK")

// ── i64 N-D ────────────────────────────────────────────────────
let i3 = t.reshape(t.from_array([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12], "i64"), [2, 2, 3])
let isum2 = t.sum_axis(i3, 2)
assertArraysEqual(tensorToArray(isum2), [6, 15, 24, 33], "i64 3-D sum_axis axis 2")

// ── f32 N-D ────────────────────────────────────────────────────
let f3 = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0, 12.0], "f32"), [2, 2, 3])
let fsum2 = t.sum_axis(f3, 2)
assertArraysEqual(tensorToArray(fsum2), [6.0, 15.0, 24.0, 33.0], "f32 3-D sum_axis axis 2")

// ── N-D out-of-range axis errors cleanly ───────────────────────
_v, err = safe(t.sum_axis, m3, 3)
a.assertTrue(isError(err))
println("3-D sum_axis rejects axis 3 (rank 3, valid [-3, 2]): OK")

_v, err = safe(t.sum_axis, m3, -4)
a.assertTrue(isError(err))
println("3-D sum_axis rejects axis -4: OK")

println("")
a.summary()
