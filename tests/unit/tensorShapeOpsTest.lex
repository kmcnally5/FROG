// tensorShapeOpsTest.lex — coverage for the small NumPy-shaped
// helpers added in the OFI #5 bundle: flatten, squeeze, expand_dims,
// dot.

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

// ── flatten ──────────────────────────────────────────────────────

let mat = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f64"), [2, 3])
let flat = t.flatten(mat)
assertArraysEqual(t.shape(flat), [6], "flatten 2x3 → shape [6]")
assertArraysEqual(tensorToArray(flat), [1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "flatten preserves row-major order")

// 3-D flatten
let cube = t.full([2, 2, 2], 7.0, "f64")
let cubeFlat = t.flatten(cube)
assertArraysEqual(t.shape(cubeFlat), [8], "flatten (2,2,2) → shape [8]")

// already 1-D
let vec = t.from_array([10.0, 20.0, 30.0], "f64")
let vecFlat = t.flatten(vec)
assertArraysEqual(t.shape(vecFlat), [3], "flatten 1-D stays 1-D")

// ── squeeze ──────────────────────────────────────────────────────

// remove leading size-1
let s1 = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0], "f64"), [1, 4])
let s1q = t.squeeze(s1)
assertArraysEqual(t.shape(s1q), [4], "squeeze (1,4) → (4,)")

// remove middle and trailing size-1
let s2 = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f64"), [2, 1, 3, 1])
let s2q = t.squeeze(s2)
assertArraysEqual(t.shape(s2q), [2, 3], "squeeze (2,1,3,1) → (2,3)")

// no size-1 dims → unchanged
let s3 = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f64"), [2, 3])
let s3q = t.squeeze(s3)
assertArraysEqual(t.shape(s3q), [2, 3], "squeeze with no size-1 dims unchanged")

// all 1s → scalar (0-D)
let s4 = t.reshape(t.from_array([42.0], "f64"), [1, 1, 1])
let s4q = t.squeeze(s4)
assertArraysEqual(t.shape(s4q), [], "squeeze all-1s → 0-D scalar")
a.assertEqual(t.numel(s4q), 1)
a.assertEqual(t.get(s4q, 0), 42.0)

// ── expand_dims ──────────────────────────────────────────────────

let e = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f64"), [2, 3])

// axis=0 → prepend
let e0 = t.expand_dims(e, 0)
assertArraysEqual(t.shape(e0), [1, 2, 3], "expand_dims (2,3) at axis 0 → (1,2,3)")

// axis=1 → insert middle
let e1 = t.expand_dims(e, 1)
assertArraysEqual(t.shape(e1), [2, 1, 3], "expand_dims (2,3) at axis 1 → (2,1,3)")

// axis=2 → append
let e2 = t.expand_dims(e, 2)
assertArraysEqual(t.shape(e2), [2, 3, 1], "expand_dims (2,3) at axis 2 → (2,3,1)")

// negative axis: -1 means "after last"
let eN1 = t.expand_dims(e, -1)
assertArraysEqual(t.shape(eN1), [2, 3, 1], "expand_dims at axis -1 → (2,3,1)")

// negative axis: -2 means "before last"
let eN2 = t.expand_dims(e, -2)
assertArraysEqual(t.shape(eN2), [2, 1, 3], "expand_dims at axis -2 → (2,1,3)")

// data preserved
assertArraysEqual(tensorToArray(e0), [1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "expand_dims preserves data")

// axis out of range
let _v, err = safe(t.expand_dims, e, 5)
a.assertTrue(isError(err))
println("expand_dims axis-out-of-range rejected: OK")

// expand_dims as broadcasting prep: (3,) → (3, 1) for col-shaped op
let row = t.from_array([10.0, 20.0, 30.0], "f64")
let col = t.expand_dims(row, -1)
assertArraysEqual(t.shape(col), [3, 1], "expand_dims to column shape")
// Now (3, 1) + (1, 4) broadcasts to (3, 4) — verify integration
let topRow = t.expand_dims(t.from_array([1.0, 2.0, 3.0, 4.0], "f64"), 0)
assertArraysEqual(t.shape(topRow), [1, 4], "expand_dims to row shape")
let outer = t.add(col, topRow)
assertArraysEqual(t.shape(outer), [3, 4], "(3,1) + (1,4) via expand_dims → (3,4)")
// [10+1, 10+2, 10+3, 10+4,  20+1, ...] = [11,12,13,14, 21,22,23,24, 31,32,33,34]
assertArraysEqual(tensorToArray(outer),
    [11.0, 12.0, 13.0, 14.0,  21.0, 22.0, 23.0, 24.0,  31.0, 32.0, 33.0, 34.0],
    "expand_dims + broadcast integration")

// ── dot ──────────────────────────────────────────────────────────

let v1 = t.from_array([1.0, 2.0, 3.0, 4.0], "f64")
let v2 = t.from_array([5.0, 6.0, 7.0, 8.0], "f64")
// 1*5 + 2*6 + 3*7 + 4*8 = 5 + 12 + 21 + 32 = 70
a.assertEqual(t.dot(v1, v2), 70.0)
println("dot f64 basic: OK")

// orthogonal vectors → 0
let xAxis = t.from_array([1.0, 0.0, 0.0], "f64")
let yAxis = t.from_array([0.0, 1.0, 0.0], "f64")
a.assertEqual(t.dot(xAxis, yAxis), 0.0)
println("dot of orthogonal vectors = 0: OK")

// self-dot = squared L2 norm
let v3 = t.from_array([3.0, 4.0], "f64")
a.assertEqual(t.dot(v3, v3), 25.0)
println("dot v·v = ||v||² (3-4-5 triangle): OK")

// f32 dot
let v1f = t.from_array([1.0, 2.0, 3.0, 4.0], "f32")
let v2f = t.from_array([5.0, 6.0, 7.0, 8.0], "f32")
a.assertClose(t.dot(v1f, v2f), 70.0, 0.001)
println("dot f32: OK")

// i64 dot
let v1i = t.from_array([1, 2, 3, 4], "i64")
let v2i = t.from_array([5, 6, 7, 8], "i64")
a.assertEqual(t.dot(v1i, v2i), 70)
println("dot i64: OK")

// dot errors
_v, err = safe(t.dot, v1, v1f)
a.assertTrue(isError(err))
println("dot rejects dtype mismatch: OK")

_v, err = safe(t.dot, v1, t.from_array([1.0, 2.0], "f64"))
a.assertTrue(isError(err))
println("dot rejects length mismatch: OK")

_v, err = safe(t.dot, mat, mat)
a.assertTrue(isError(err))
println("dot rejects 2-D input: OK")

println("")
a.summary()
