// tensorConstructorsTest.lex — coverage for FrogPy non-zeros tensor
// constructors (full + random) and the new assertClose helper.

import "stdlib/tensor.lex" as t
import "stdlib/assert.lex" as a

// ── full(shape, value, dtype) ────────────────────────────────────

// f64
let tf64 = t.full([3, 4], 2.5, "f64")
a.assertEqual(t.dtype(tf64), "f64")
a.assertEqual(t.numel(tf64), 12)
a.assertEqual(t.get(tf64, 0), 2.5)
a.assertEqual(t.get(tf64, 5), 2.5)
a.assertEqual(t.get(tf64, 11), 2.5)

// f32 — int value promotes to float
let tf32 = t.full([2, 2], 7, "f32")
a.assertEqual(t.dtype(tf32), "f32")
a.assertClose(t.get(tf32, 0), 7.0, 0.0001)
a.assertClose(t.get(tf32, 3), 7.0, 0.0001)

// i64 — int value direct
let ti64 = t.full([5], 42, "i64")
a.assertEqual(t.dtype(ti64), "i64")
a.assertEqual(t.get(ti64, 0), 42)
a.assertEqual(t.get(ti64, 4), 42)

// i64 + float value → error
let _v, err = safe(t.full, [3], 1.5, "i64")
a.assertTrue(isError(err))
println("full rejects float value into i64 tensor: OK")

// unknown dtype → error
_v, err = safe(t.full, [3], 1, "f128")
a.assertTrue(isError(err))
println("full rejects unknown dtype: OK")

// ── random(shape, dtype, seed) ───────────────────────────────────

// Deterministic: same seed produces identical output
let r1 = t.random([100], "f64", 42)
let r2 = t.random([100], "f64", 42)
a.assertEqual(t.get(r1, 0), t.get(r2, 0))
a.assertEqual(t.get(r1, 50), t.get(r2, 50))
a.assertEqual(t.get(r1, 99), t.get(r2, 99))
println("random with same seed is deterministic: OK")

// Different seed → different output (very likely; non-degenerate
// sequence collisions at the first cell are vanishingly improbable
// for PCG-family with distinct seeds)
let r3 = t.random([100], "f64", 99)
a.assertNotEqual(t.get(r1, 0), t.get(r3, 0))
println("random with different seed is different: OK")

// All values in [0, 1) for floats
let rF32 = t.random([1000], "f32", 1)
let i = 0
let allInRange = true
while i < 1000 {
    let v = t.get(rF32, i)
    if v < 0 || v >= 1.0 {
        allInRange = false
    }
    i = i + 1
}
a.assertTrue(allInRange)
println("random f32 values all in [0, 1): OK")

let rF64 = t.random([1000], "f64", 1)
i = 0
allInRange = true
while i < 1000 {
    let v = t.get(rF64, i)
    if v < 0 || v >= 1.0 {
        allInRange = false
    }
    i = i + 1
}
a.assertTrue(allInRange)
println("random f64 values all in [0, 1): OK")

// i64 produces non-negative int63 values
let rI64 = t.random([100], "i64", 1)
i = 0
let allNonNeg = true
while i < 100 {
    if t.get(rI64, i) < 0 {
        allNonNeg = false
    }
    i = i + 1
}
a.assertTrue(allNonNeg)
println("random i64 values are non-negative (int63): OK")

// ── assertClose ──────────────────────────────────────────────────

a.assertClose(1.0, 1.0, 0.001)              // exact
a.assertClose(1.0, 1.0001, 0.001)           // within
a.assertClose(1.0001, 1.0, 0.001)           // within, reversed
a.assertClose(0.0, 0.00001, 0.001)          // near zero
println("assertClose passing cases: OK")

println("")
a.summary()
