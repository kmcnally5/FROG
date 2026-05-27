// tensorSlicingTest.lex — coverage for t.slice / t.contiguous, the
// view-based slicing surface added in OFI #2. View semantics:
// _tensor_slice returns a tensor that shares backing storage with
// the source; _tensor_contiguous materialises a strided view to a
// fresh contiguous tensor.

import "stdlib/tensor.lex" as t
import "stdlib/assert.lex" as a

fn tensorToArray(tn) {
    return t.to_array(tn)
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

// ── 1-D slicing ─────────────────────────────────────────────────

let v = t.from_array([0.0, 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0], "f64")

// basic [start, stop, step]
let v15 = t.slice(v, [[1, 5, 1]])
assertArraysEqual(t.shape(v15), [4], "1-D slice [1:5:1] shape")
assertArraysEqual(tensorToArray(v15), [1.0, 2.0, 3.0, 4.0], "1-D slice [1:5:1] values")

// step != 1
let vEven = t.slice(v, [[0, 10, 2]])
assertArraysEqual(t.shape(vEven), [5], "1-D slice [0:10:2] shape")
assertArraysEqual(tensorToArray(vEven), [0.0, 2.0, 4.0, 6.0, 8.0], "1-D slice [0:10:2] values")

// negative start/stop
let vTail = t.slice(v, [[-3, null, 1]])
assertArraysEqual(t.shape(vTail), [3], "1-D slice [-3:] shape")
assertArraysEqual(tensorToArray(vTail), [7.0, 8.0, 9.0], "1-D slice [-3:] values")

// nulls = full axis
let vFull = t.slice(v, [null])
assertArraysEqual(t.shape(vFull), [10], "1-D slice null = full axis")
assertArraysEqual(tensorToArray(vFull), tensorToArray(v), "1-D slice null values match source")

// stop > dim clamps; out-of-bounds is fine
let vTo20 = t.slice(v, [[5, 20, 1]])
assertArraysEqual(tensorToArray(vTo20), [5.0, 6.0, 7.0, 8.0, 9.0], "1-D slice [5:20] clamps to dim")

// empty result (start >= stop)
let vEmpty = t.slice(v, [[5, 3, 1]])
assertArraysEqual(t.shape(vEmpty), [0], "1-D slice [5:3] empty shape")
a.assertEqual(t.numel(vEmpty), 0)
println("1-D slice empty numel == 0: OK")

// ── 2-D slicing ─────────────────────────────────────────────────
//
// Reference matrix (4 × 4 f64, values 0..15):
//
//   [[ 0,  1,  2,  3],
//    [ 4,  5,  6,  7],
//    [ 8,  9, 10, 11],
//    [12, 13, 14, 15]]

let m = t.reshape(t.from_array([0.0, 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0, 12.0, 13.0, 14.0, 15.0], "f64"), [4, 4])

// Row range, full cols
let rows12 = t.slice(m, [[1, 3, 1], null])
assertArraysEqual(t.shape(rows12), [2, 4], "2-D slice rows 1-2 shape")
assertArraysEqual(tensorToArray(rows12), [4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0], "2-D slice rows 1-2 values")

// Full rows, col range (strided)
let cols12 = t.slice(m, [null, [1, 3, 1]])
assertArraysEqual(t.shape(cols12), [4, 2], "2-D slice cols 1-2 shape")
assertArraysEqual(tensorToArray(cols12), [1.0, 2.0, 5.0, 6.0, 9.0, 10.0, 13.0, 14.0], "2-D slice cols 1-2 values")

// Row range AND col range
let block = t.slice(m, [[1, 3, 1], [1, 3, 1]])
assertArraysEqual(t.shape(block), [2, 2], "2-D block slice shape")
assertArraysEqual(tensorToArray(block), [5.0, 6.0, 9.0, 10.0], "2-D block slice values")

// Step in both axes
let chess = t.slice(m, [[0, 4, 2], [0, 4, 2]])
assertArraysEqual(t.shape(chess), [2, 2], "2-D step-2 both axes shape")
assertArraysEqual(tensorToArray(chess), [0.0, 2.0, 8.0, 10.0], "2-D step-2 both axes values")

// Last 2 rows via negative index
let last2 = t.slice(m, [[-2, null, 1], null])
assertArraysEqual(t.shape(last2), [2, 4], "2-D last 2 rows shape")
assertArraysEqual(tensorToArray(last2), [8.0, 9.0, 10.0, 11.0, 12.0, 13.0, 14.0, 15.0], "2-D last 2 rows values")

// ── 3-D slicing ─────────────────────────────────────────────────

let m3 = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0, 12.0], "f64"), [2, 2, 3])

// Slice axis 0 — pick second outer block
let m3s = t.slice(m3, [[1, 2, 1], null, null])
assertArraysEqual(t.shape(m3s), [1, 2, 3], "3-D slice axis 0 shape")
assertArraysEqual(tensorToArray(m3s), [7.0, 8.0, 9.0, 10.0, 11.0, 12.0], "3-D slice axis 0 values")

// Slice axis 2 — strided inner dim
let m3s2 = t.slice(m3, [null, null, [0, 3, 2]])
assertArraysEqual(t.shape(m3s2), [2, 2, 2], "3-D slice axis 2 step-2 shape")
assertArraysEqual(tensorToArray(m3s2), [1.0, 3.0, 4.0, 6.0, 7.0, 9.0, 10.0, 12.0], "3-D slice axis 2 step-2 values")

// ── view semantics: shares backing storage ──────────────────────

// In-place op through a view affects the original.
let base = t.full([4], 1.0, "f64")
let view = t.slice(base, [[0, 4, 1]])
// Note: in-place ops require contiguous; this view IS contiguous-equivalent
// but Strides!=nil so it's flagged non-contiguous. Use a materialise round-trip:
let viewMat = t.contiguous(view)
// (We can't actually mutate `base` through an in-place op on a view in v1
// because in-place ops error on non-contiguous tensors. But we CAN verify
// view + get sees the source data, and the view-of-contiguous round-trip
// produces equal contents.)
assertArraysEqual(tensorToArray(viewMat), [1.0, 1.0, 1.0, 1.0], "view materialise preserves data")

// Direct view via get — bypasses contiguity requirement.
let probe = t.slice(m, [[0, 2, 1], [0, 2, 1]])  // [[0, 1], [4, 5]]
a.assertEqual(t.get(probe, 0), 0.0)
a.assertEqual(t.get(probe, 1), 1.0)
a.assertEqual(t.get(probe, 2), 4.0)
a.assertEqual(t.get(probe, 3), 5.0)
println("get() on strided view returns correct logical values: OK")

// ── t.contiguous on a view materialises ─────────────────────────

let strided = t.slice(m, [null, [0, 4, 2]])  // every other col
let solid = t.contiguous(strided)
assertArraysEqual(t.shape(solid), [4, 2], "contiguous(view) preserves shape")
assertArraysEqual(tensorToArray(solid), [0.0, 2.0, 4.0, 6.0, 8.0, 10.0, 12.0, 14.0], "contiguous(view) values match logical layout")

// t.contiguous on already-contiguous is identity (no-op fast path)
let alreadySolid = t.full([3], 7.0, "f64")
let stillSolid = t.contiguous(alreadySolid)
assertArraysEqual(tensorToArray(stillSolid), [7.0, 7.0, 7.0], "contiguous(contiguous) returns equivalent data")

// ── kernel ops error on strided ────────────────────────────────

let kview = t.slice(m, [null, [0, 4, 2]])
let _v = null
let err = null
_v, err = safe(t.add, kview, kview)
a.assertTrue(isError(err))
println("t.add rejects strided input: OK")

_v, err = safe(t.sum, kview)
a.assertTrue(isError(err))
println("t.sum rejects strided input: OK")

_v, err = safe(t.matmul, kview, kview)
a.assertTrue(isError(err))
println("t.matmul rejects strided input: OK")

// After contiguous, kernels work
let kviewSolid = t.contiguous(kview)
let kSum = t.sum(kviewSolid)
// Sum of [0, 2, 4, 6, 8, 10, 12, 14] = 56
a.assertEqual(kSum, 56.0)
println("t.sum works on contiguous(view): OK")

// ── t.clone on a strided view materialises (same as contiguous) ─

let cloned = t.clone(strided)
assertArraysEqual(tensorToArray(cloned), tensorToArray(solid), "clone(view) values match contiguous(view)")

// ── i64 slicing works ──────────────────────────────────────────

let mi = t.reshape(t.from_array([1, 2, 3, 4, 5, 6, 7, 8, 9], "i64"), [3, 3])
let miSlice = t.slice(mi, [[0, 2, 1], [1, 3, 1]])  // top-right 2x2 = [[2,3], [5,6]]
assertArraysEqual(t.shape(miSlice), [2, 2], "i64 slice shape")
assertArraysEqual(tensorToArray(miSlice), [2, 3, 5, 6], "i64 slice values")

// ── error cases ────────────────────────────────────────────────

// wrong specs length
_v, err = safe(t.slice, m, [[0, 4, 1]])
a.assertTrue(isError(err))
println("slice rejects wrong specs length: OK")

// non-array spec inside specs array
_v, err = safe(t.slice, m, [null, 42])
a.assertTrue(isError(err))
println("slice rejects non-array/null spec entry: OK")

// step <= 0
_v, err = safe(t.slice, m, [null, [0, 4, 0]])
a.assertTrue(isError(err))
println("slice rejects step == 0: OK")

_v, err = safe(t.slice, m, [null, [0, 4, -1]])
a.assertTrue(isError(err))
println("slice rejects negative step: OK")

// 3-element check
_v, err = safe(t.slice, m, [null, [0, 4]])
a.assertTrue(isError(err))
println("slice rejects spec of length 2: OK")

// can't slice a view (must contiguous first)
_v, err = safe(t.slice, strided, [null, null])
a.assertTrue(isError(err))
println("slice rejects already-strided source: OK")

// can't slice a scalar tensor (rank 0)
let scalar = t.sum_axis(v, 0)  // reduces 1-D → scalar, shape []
_v, err = safe(t.slice, scalar, [])
a.assertTrue(isError(err))
println("slice rejects rank-0 source: OK")

println("")
a.summary()
