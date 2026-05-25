// tensorReductionsTest.lex — FrogPy v1 reductions
//
// Covers sum / mean / min / max / argmin / argmax across all three
// dtypes (f32, f64, i64). Verifies:
//   - correct scalar values for populated tensors
//   - mean always returns a float (including for i64 inputs)
//   - first-occurrence-wins on argmin/argmax ties
//   - sum on an empty tensor returns 0 (identity element)
//   - mean/min/max/argmin/argmax on empty error cleanly

import "stdlib/tensor.lex" as t
import "stdlib/assert.lex" as a

// ── sum, populated ──

let xF32 = t.from_array([1.0, 2.0, 3.0, 4.0], "f32")
let xF64 = t.from_array([1.0, 2.0, 3.0, 4.0], "f64")
let xI64 = t.from_array([1, 2, 3, 4],          "i64")

a.assertEqual(t.sum(xF32), 10.0)
a.assertEqual(t.sum(xF64), 10.0)
a.assertEqual(t.sum(xI64), 10)

// ── mean (always returns float) ──

a.assertEqual(t.mean(xF32), 2.5)
a.assertEqual(t.mean(xF64), 2.5)
a.assertEqual(t.mean(xI64), 2.5)

// ── min / max, populated ──

let unsortedF64 = t.from_array([4.0, 1.0, 3.0, 2.0], "f64")
let unsortedI64 = t.from_array([4, 1, 3, 2],         "i64")

a.assertEqual(t.min(unsortedF64), 1.0)
a.assertEqual(t.max(unsortedF64), 4.0)
a.assertEqual(t.min(unsortedI64), 1)
a.assertEqual(t.max(unsortedI64), 4)

// ── argmin / argmax ──

a.assertEqual(t.argmin(unsortedF64), 1)
a.assertEqual(t.argmax(unsortedF64), 0)
a.assertEqual(t.argmin(unsortedI64), 1)
a.assertEqual(t.argmax(unsortedI64), 0)

// First-occurrence-wins on ties: [3,1,1,3] -> argmin=1, argmax=0
let ties = t.from_array([3, 1, 1, 3], "i64")
a.assertEqual(t.argmin(ties), 1)
a.assertEqual(t.argmax(ties), 0)

// ── negatives + mixed signs ──

let signed = t.from_array([-5.0, -1.0, 0.0, 3.0, 2.0], "f64")
a.assertEqual(t.min(signed), -5.0)
a.assertEqual(t.max(signed),  3.0)
a.assertEqual(t.argmin(signed), 0)
a.assertEqual(t.argmax(signed), 3)

// ── single-element tensor ──

let one = t.from_array([42.0], "f64")
a.assertEqual(t.sum(one),    42.0)
a.assertEqual(t.mean(one),   42.0)
a.assertEqual(t.min(one),    42.0)
a.assertEqual(t.max(one),    42.0)
a.assertEqual(t.argmin(one), 0)
a.assertEqual(t.argmax(one), 0)

// ── empty tensor: sum returns 0 (identity) ──

let emptyF64 = t.from_array([], "f64")
let emptyI64 = t.from_array([], "i64")

a.assertEqual(t.sum(emptyF64), 0.0)
a.assertEqual(t.sum(emptyI64), 0)

// ── empty tensor: mean/min/max/argmin/argmax error cleanly ──
//
// safe() lifts the Error into a normal value. We check that each safe
// call produces an error rather than a real return value.

let r1, e1 = safe(t.mean,   emptyF64)
let r2, e2 = safe(t.min,    emptyF64)
let r3, e3 = safe(t.max,    emptyF64)
let r4, e4 = safe(t.argmin, emptyF64)
let r5, e5 = safe(t.argmax, emptyF64)

a.assertTrue(isError(e1))
a.assertTrue(isError(e2))
a.assertTrue(isError(e3))
a.assertTrue(isError(e4))
a.assertTrue(isError(e5))

a.summary()
