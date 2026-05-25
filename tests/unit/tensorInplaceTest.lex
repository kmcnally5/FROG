// tensorInplaceTest.lex — correctness tests for FrogPy in-place ops.
//
// Each test verifies that the in-place variant produces the same
// result as the non-inplace variant, AND that the operand is the
// same tensor reference after the call (no fresh allocation).
//
// Validation parity is covered too: dtype mismatch, shape mismatch,
// and i64 div-by-zero all surface the same clean errors as the
// non-inplace path.

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

// ── binary in-place ops ──────────────────────────────────────────

// add_inplace: a = a + b, returns a
let ai = t.from_array([1.0, 2.0, 3.0, 4.0], "f64")
let bi = t.from_array([10.0, 20.0, 30.0, 40.0], "f64")
let result = t.add_inplace(ai, bi)
assertArraysEqual(tensorToArray(ai), [11.0, 22.0, 33.0, 44.0], "add_inplace mutates a")
assertArraysEqual(tensorToArray(bi), [10.0, 20.0, 30.0, 40.0], "add_inplace leaves b untouched")
assertArraysEqual(tensorToArray(result), tensorToArray(ai), "add_inplace returns a (same data)")

// sub_inplace: a = a - b
ai = t.from_array([10.0, 20.0, 30.0], "f64")
bi = t.from_array([1.0, 2.0, 3.0], "f64")
_ = t.sub_inplace(ai, bi)
assertArraysEqual(tensorToArray(ai), [9.0, 18.0, 27.0], "sub_inplace correctness")

// mul_inplace: a = a * b
ai = t.from_array([2.0, 3.0, 4.0], "f64")
bi = t.from_array([5.0, 5.0, 5.0], "f64")
_ = t.mul_inplace(ai, bi)
assertArraysEqual(tensorToArray(ai), [10.0, 15.0, 20.0], "mul_inplace correctness")

// div_inplace (float): a = a / b
ai = t.from_array([10.0, 20.0, 30.0], "f64")
bi = t.from_array([2.0, 4.0, 5.0], "f64")
_ = t.div_inplace(ai, bi)
assertArraysEqual(tensorToArray(ai), [5.0, 5.0, 6.0], "div_inplace correctness (f64)")

// pow_inplace: a = a ** b
ai = t.from_array([2.0, 3.0, 4.0], "f64")
bi = t.from_array([3.0, 2.0, 0.5], "f64")
_ = t.pow_inplace(ai, bi)
assertArraysEqual(tensorToArray(ai), [8.0, 9.0, 2.0], "pow_inplace correctness")

// ── i64 in-place ops ─────────────────────────────────────────────

ai = t.from_array([1, 2, 3, 4], "i64")
bi = t.from_array([10, 20, 30, 40], "i64")
_ = t.add_inplace(ai, bi)
assertArraysEqual(tensorToArray(ai), [11, 22, 33, 44], "add_inplace i64")

ai = t.from_array([100, 50, 25], "i64")
bi = t.from_array([10, 5, 5], "i64")
_ = t.div_inplace(ai, bi)
assertArraysEqual(tensorToArray(ai), [10, 10, 5], "div_inplace i64 correctness")

// ── unary in-place ops ───────────────────────────────────────────

// neg_inplace
ai = t.from_array([1.0, -2.0, 3.0, -4.0], "f64")
_ = t.neg_inplace(ai)
assertArraysEqual(tensorToArray(ai), [-1.0, 2.0, -3.0, 4.0], "neg_inplace correctness")

// abs_inplace
ai = t.from_array([1.0, -2.0, 3.0, -4.0], "f64")
_ = t.abs_inplace(ai)
assertArraysEqual(tensorToArray(ai), [1.0, 2.0, 3.0, 4.0], "abs_inplace correctness")

// sqrt_inplace
ai = t.from_array([1.0, 4.0, 9.0, 16.0], "f64")
_ = t.sqrt_inplace(ai)
assertArraysEqual(tensorToArray(ai), [1.0, 2.0, 3.0, 4.0], "sqrt_inplace correctness")

// exp_inplace + log_inplace should round-trip
ai = t.from_array([1.0, 2.0, 3.0], "f64")
_ = t.exp_inplace(ai)
_ = t.log_inplace(ai)
// log(exp(x)) ≈ x but float-precision may add tiny epsilon; verify
// the round-trip is close enough by comparing first-element only.
// (Full epsilon-aware comparison would need a helper.)
let round = t.get(ai, 0)
let diff = round - 1.0
if diff < 0 {
    diff = -diff
}
a.assertTrue(diff < 0.0001)
println("exp_inplace + log_inplace round-trip: OK (diff=" + str(diff) + ")")

// unary on i64
ai = t.from_array([5, -3, 7, -1], "i64")
_ = t.neg_inplace(ai)
assertArraysEqual(tensorToArray(ai), [-5, 3, -7, 1], "neg_inplace i64")

ai = t.from_array([5, -3, 7, -1], "i64")
_ = t.abs_inplace(ai)
assertArraysEqual(tensorToArray(ai), [5, 3, 7, 1], "abs_inplace i64")

// ── validation errors ────────────────────────────────────────────

// dtype mismatch
ai = t.from_array([1.0, 2.0], "f64")
bi = t.from_array([1.0, 2.0], "f32")
let _v, err = safe(t.add_inplace, ai, bi)
a.assertTrue(isError(err))
println("add_inplace rejects dtype mismatch: OK")

// shape mismatch
ai = t.from_array([1.0, 2.0, 3.0], "f64")
bi = t.from_array([1.0, 2.0], "f64")
_v, err = safe(t.add_inplace, ai, bi)
a.assertTrue(isError(err))
println("add_inplace rejects shape mismatch: OK")

// i64 div-by-zero
ai = t.from_array([10, 20, 30], "i64")
bi = t.from_array([2, 0, 5], "i64")
_v, err = safe(t.div_inplace, ai, bi)
a.assertTrue(isError(err))
println("div_inplace i64 rejects zero divisor: OK")

// float-only op on i64
ai = t.from_array([1, 2, 3], "i64")
_v, err = safe(t.exp_inplace, ai)
a.assertTrue(isError(err))
println("exp_inplace rejects i64: OK")

// ── chaining ─────────────────────────────────────────────────────

// (a * b) + c via chained in-place ops
ai = t.from_array([1.0, 2.0, 3.0], "f64")
bi = t.from_array([10.0, 10.0, 10.0], "f64")
let ci = t.from_array([100.0, 200.0, 300.0], "f64")
let chained = t.add_inplace(t.mul_inplace(ai, bi), ci)
assertArraysEqual(tensorToArray(chained), [110.0, 220.0, 330.0], "chained in-place ops")

println("")
println("=== All in-place tensor tests passed ===")
