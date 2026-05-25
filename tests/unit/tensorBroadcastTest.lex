// tensorBroadcastTest.lex — coverage for FrogPy NumPy-style
// broadcasting on the allocating binary ops.
//
// Scope: scalar broadcasting + tensor-tensor shape-compat broadcasting
// for add / sub / mul / div / pow. Hand-computed references; uses the
// new assertClose helper for f32 where MPS-vs-CPU rounding differs.

import "stdlib/tensor.lex" as t
import "stdlib/assert.lex" as a

// ── helpers ──

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

fn assertArraysClose(got, want, tol, msg) {
    a.assertEqual(len(got), len(want))
    let i = 0
    while i < len(got) {
        a.assertClose(got[i], want[i], tol)
        i = i + 1
    }
    println(msg + ": OK")
}

// ── scalar broadcasting (tensor op scalar) ───────────────────────

let v = t.from_array([1.0, 2.0, 3.0, 4.0], "f64")

// add: tensor + scalar
let r1 = t.add(v, 10.0)
assertArraysEqual(tensorToArray(r1), [11.0, 12.0, 13.0, 14.0], "f64 tensor + scalar (float)")

// add: tensor + int scalar (promoted to f64)
let r2 = t.add(v, 5)
assertArraysEqual(tensorToArray(r2), [6.0, 7.0, 8.0, 9.0], "f64 tensor + scalar (int promotes)")

// scalar + tensor (left-side scalar)
let r3 = t.add(100.0, v)
assertArraysEqual(tensorToArray(r3), [101.0, 102.0, 103.0, 104.0], "scalar + f64 tensor")

// sub, mul, div: tensor op scalar
assertArraysEqual(tensorToArray(t.sub(v, 1.0)), [0.0, 1.0, 2.0, 3.0], "tensor - scalar")
assertArraysEqual(tensorToArray(t.mul(v, 2.0)), [2.0, 4.0, 6.0, 8.0], "tensor * scalar")
assertArraysEqual(tensorToArray(t.div(v, 2.0)), [0.5, 1.0, 1.5, 2.0], "tensor / scalar")

// scalar op tensor (left-side, non-commutative ops matter)
assertArraysEqual(tensorToArray(t.sub(10.0, v)), [9.0, 8.0, 7.0, 6.0], "scalar - tensor")
assertArraysEqual(tensorToArray(t.div(24.0, v)), [24.0, 12.0, 8.0, 6.0], "scalar / tensor")

// pow: tensor ** scalar
let p = t.from_array([1.0, 2.0, 3.0, 4.0], "f64")
assertArraysEqual(tensorToArray(t.pow(p, 2.0)), [1.0, 4.0, 9.0, 16.0], "tensor ** 2 (square)")

// i64 + int scalar
let vi = t.from_array([10, 20, 30, 40], "i64")
assertArraysEqual(tensorToArray(t.add(vi, 5)), [15, 25, 35, 45], "i64 tensor + int scalar")

// i64 + float scalar → error
let _v, err = safe(t.add, vi, 1.5)
a.assertTrue(isError(err))
println("i64 tensor + float scalar rejected: OK")

// both args scalar → error
_v, err = safe(t.add, 1, 2)
a.assertTrue(isError(err))
println("scalar + scalar (no tensor anchor) rejected: OK")

// ── tensor-tensor broadcasting: (m, n) op (n,) ───────────────────
//
// "add a per-column bias" idiom. Matrix 2x3 + row vector (3,):
//
//   A = [ 1 2 3 ]    bias = [ 10 20 30 ]
//       [ 4 5 6 ]
//
//   A + bias = [ 11 22 33 ]
//              [ 14 25 36 ]

let A23 = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f64"), [2, 3])
let bias3 = t.from_array([10.0, 20.0, 30.0], "f64")
let withBias = t.add(A23, bias3)
assertArraysEqual(t.shape(withBias), [2, 3], "(2,3)+(3,) → (2,3) shape")
assertArraysEqual(tensorToArray(withBias), [11.0, 22.0, 33.0, 14.0, 25.0, 36.0], "(m,n) + (n,) bias-add")

// Reversed: (n,) + (m, n) — same result, scalar-on-left semantics
let withBiasRev = t.add(bias3, A23)
assertArraysEqual(tensorToArray(withBiasRev), [11.0, 22.0, 33.0, 14.0, 25.0, 36.0], "(n,) + (m,n) bias-add")

// ── tensor-tensor broadcasting: (m, 1) op (1, n) → (m, n) ────────
//
// "outer product-like" addition: column vector + row vector =>
// matrix where C[i,j] = col[i] + row[j].
//
//   col = [ [1] [2] [3] ]    row = [ [10 20] ]
//   col + row = [ [11 21] [12 22] [13 23] ]

let col31 = t.reshape(t.from_array([1.0, 2.0, 3.0], "f64"), [3, 1])
let row12 = t.reshape(t.from_array([10.0, 20.0], "f64"), [1, 2])
let outer = t.add(col31, row12)
assertArraysEqual(t.shape(outer), [3, 2], "(3,1) + (1,2) → (3,2) shape")
assertArraysEqual(tensorToArray(outer), [11.0, 21.0, 12.0, 22.0, 13.0, 23.0], "(m,1)+(1,n) outer-add")

// ── leading-dim padding: (2,3,4) + (3,4) → (2,3,4) ───────────────
//
// "Same-pattern bias applied to a batch of matrices." Build a 2x3x4
// tensor of ones, add a 3x4 pattern; each "batch" slice gets the same
// pattern.

let batch = t.full([2, 3, 4], 1.0, "f64")
let pattern = t.reshape(t.from_array([1.0, 2.0, 3.0, 4.0,  5.0, 6.0, 7.0, 8.0,  9.0, 10.0, 11.0, 12.0], "f64"), [3, 4])
let batched = t.add(batch, pattern)
assertArraysEqual(t.shape(batched), [2, 3, 4], "(2,3,4) + (3,4) → (2,3,4) shape")
// Two batch slices, each = [2, 3, 4, 5,  6, 7, 8, 9,  10, 11, 12, 13]
let expected = [2.0, 3.0, 4.0, 5.0,  6.0, 7.0, 8.0, 9.0,  10.0, 11.0, 12.0, 13.0,
                2.0, 3.0, 4.0, 5.0,  6.0, 7.0, 8.0, 9.0,  10.0, 11.0, 12.0, 13.0]
assertArraysEqual(tensorToArray(batched), expected, "(2,3,4) + (3,4) replicates across leading dim")

// ── mul broadcasting: per-column scaling ─────────────────────────

let scale3 = t.from_array([10.0, 100.0, 1000.0], "f64")
let scaled = t.mul(A23, scale3)
assertArraysEqual(tensorToArray(scaled), [10.0, 200.0, 3000.0, 40.0, 500.0, 6000.0], "per-column scale")

// ── error cases ──────────────────────────────────────────────────

// Incompatible shapes: (3,4) and (3,5)
let X34 = t.zeros([3, 4], "f64")
let Y35 = t.zeros([3, 5], "f64")
_v, err = safe(t.add, X34, Y35)
a.assertTrue(isError(err))
println("(3,4) + (3,5) rejected (incompatible): OK")

// Dtype mismatch after promotion
let fT = t.from_array([1.0, 2.0], "f32")
let dT = t.from_array([1.0, 2.0], "f64")
_v, err = safe(t.add, fT, dT)
a.assertTrue(isError(err))
println("f32 + f64 dtype mismatch rejected: OK")

// ── in-place ops still strict (no broadcasting) ──────────────────
//
// In-place ops should NOT silently accept broadcastable inputs — the
// LHS shape would change under the caller's feet.

let lhsInplace = t.from_array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0], "f64")
let lhsMat = t.reshape(lhsInplace, [2, 3])
_v, err = safe(t.add_inplace, lhsMat, bias3)
a.assertTrue(isError(err))
println("in-place add rejects shape-compat (would resize LHS): OK")

// In-place add with same-shape still works (unchanged behavior)
let same1 = t.from_array([1.0, 2.0, 3.0], "f64")
let same2 = t.from_array([10.0, 20.0, 30.0], "f64")
let _ = t.add_inplace(same1, same2)
assertArraysEqual(tensorToArray(same1), [11.0, 22.0, 33.0], "in-place add same-shape (regression)")

// ── f32 scalar broadcasting (uses assertClose) ───────────────────

let f32T = t.from_array([1.0, 2.0, 3.0], "f32")
let f32Scaled = t.mul(f32T, 0.1)
let got = tensorToArray(f32Scaled)
assertArraysClose(got, [0.1, 0.2, 0.3], 0.0001, "f32 tensor * float scalar")

println("")
a.summary()
