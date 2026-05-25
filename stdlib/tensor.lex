// stdlib/tensor.lex — FrogPy v1: NumPy-flavoured tensor library
// @module    tensor
// @version   0.1.0
// @since     klex 0.4.0
// @author    karl
// @summary   dense n-dimensional numeric arrays with native C kernels
//
// FrogPy is kLex's answer to NumPy — dense n-d arrays with element-wise
// ops, reductions, and matmul backed by native C kernels on macOS and
// Linux. The Go-side dispatchers are zero-allocation; the C kernels are
// compiled with -O3 -ffast-math so the compiler's autovectoriser handles
// most SIMD wins for us.
//
// Why a kLex tensor library exists at all: the bytecode VM gets us
// ~100× off native for arithmetic, which is fine for orchestration but
// untenable for the inner loop of any AI workload. NumPy uses C for the
// same reason — Python is the glue, C is the compute. FrogPy keeps kLex
// as the glue and pushes the compute to where it belongs.
//
// Usage
//
//   import "stdlib/tensor.lex" as t
//   a = t.zeros([3, 4], "f64")
//   b = t.from_array([1, 2, 3, 4], "f64")
//   c = t.add(a, b)   // shape mismatch errors cleanly
//   println(t.shape(c))
//   println(t.dtype(c))
//
// v1 scope (this file)
//
//   Construction:  zeros, from_array
//   Inspection:    shape, dtype, numel, get
//   Element-wise:  add, sub, mul, div, pow,
//                  neg, abs, exp, log, sqrt, sin, cos
//   Reductions:    sum, mean, min, max, argmin, argmax
//
// Linear algebra (this file):
//
//   matmul                  — 2-D matrix multiply; f32 routes through
//                             MPSMatrixMultiplication on macOS, naive
//                             autovec CPU kernel elsewhere and for
//                             f64 / i64 on every platform.
//
// Coming next (placeholder; see project_frogpy in memory):
//
//   transpose, dot
//   reshape, squeeze, expand_dims, flatten
//   broadcasting for same-shape and (later) shape-compatible inputs

// zeros(shape, dtype) — fresh tensor filled with zeros.
//   shape: array of non-negative ints, e.g. [3, 4]
//   dtype: "f32", "f64", or "i64"
fn zeros(shape, dtype) {
    return _tensor_zeros(shape, dtype)
}

// full(shape, value, dtype) — fresh tensor with every element set
// to `value`. NumPy equivalent: np.full.
//
//   t.full([2, 3], 1.0, "f64")   →  2×3 matrix of 1.0
//   t.full([512, 512], 0.1, "f32")
//
// Value conversion: f32 / f64 accept int or float; i64 requires int
// (floats rejected without explicit conversion — consistent with
// kLex's strict-typing policy).
fn full(shape, value, dtype) {
    return _tensor_full(shape, value, dtype)
}

// random(shape, dtype, seed) — fresh tensor filled with uniform
// random values in [0, 1) for float dtypes; full int64 range for
// i64. NumPy parallel: np.random.rand (for floats).
//
//   seed > 0 → deterministic; same seed produces identical output
//   seed = 0 → time-seeded (non-reproducible)
//
// Use a fixed seed for tests and benchmarks; use 0 for fresh
// entropy in production code.
fn random(shape, dtype, seed) {
    return _tensor_random(shape, dtype, seed)
}

// from_array(data, dtype) — build a 1-D tensor from a kLex array.
//   data:  array of numbers (int or float)
//   dtype: "f32", "f64", or "i64"
//
// Element conversions are strict: float values rejected for i64
// without explicit conversion; ints widen to float for f32/f64.
fn from_array(data, dtype) {
    return _tensor_from_array(data, dtype)
}

// ── element-wise binary ops ──────────────────────────────────────
//
// All binary ops (add/sub/mul/div/pow) support NumPy-style
// broadcasting. Either operand may be:
//
//   - a tensor with the same shape as the other operand
//   - a tensor with a broadcast-compatible shape (e.g. (m, n) op
//     (n,) or (m, 1) op (1, n))
//   - a number (Integer or Float), broadcast to the tensor's shape
//
// Broadcasting rules (NumPy semantics):
//   1. Align shapes from the trailing (rightmost) dimension.
//   2. Missing leading dimensions are treated as 1.
//   3. Two dims are compatible if equal OR one is 1; result takes
//      the max of each dim.
//
// Dtype rules: both operands must end up with the same dtype after
// scalar promotion. No implicit dtype promotion across tensors —
// f32 + f64 errors; convert explicitly.
//
// Implementation: the smaller / scalar operand is materialised into
// a fresh tensor at the broadcast shape, then the existing kernel
// runs. This trades an allocation for kernel simplicity; for huge
// tensors with heavily skewed sizes the allocation is real cost but
// the kernel call dominates in typical workloads.
//
// In-place variants (add_inplace etc.) do NOT broadcast — they
// require strictly matching shapes so the LHS isn't silently
// resized.

// add(a, b) — element-wise sum. Broadcasting applies.
fn add(a, b) {
    return _tensor_add(a, b)
}

// sub(a, b) — element-wise difference (a - b). Broadcasting applies.
fn sub(a, b) {
    return _tensor_sub(a, b)
}

// mul(a, b) — element-wise product. Broadcasting applies. Integer
// overflow wraps.
fn mul(a, b) {
    return _tensor_mul(a, b)
}

// div(a, b) — element-wise quotient. Broadcasting applies.
// Float: IEEE 754 — n/0 = ±Inf, 0/0 = NaN (no error raised).
// Integer: division-by-zero is rejected with a runtime error
// reporting the offending index; the C kernel doesn't guard.
fn div(a, b) {
    return _tensor_div(a, b)
}

// pow(a, b) — element-wise exponentiation (a ** b).
// Float: libm pow/powf — IEEE-conformant. Negative bases with
// non-integer exponents → NaN (no error).
// Integer: exponentiation by squaring. Negative i64 exponents are
// rejected (result < 1 isn't representable in i64); use f32/f64.
// Overflow wraps per the general i64 policy.
fn pow(a, b) {
    return _tensor_pow(a, b)
}

// ── unary element-wise ops ──
//
// neg + abs work on all three dtypes (f32/f64/i64). The
// transcendentals (exp, log, sqrt, sin, cos) are float-only —
// passing an i64 tensor produces a clean "use f32 or f64" error.
//
// Domain edges (negative log, sqrt of negative) follow IEEE 754
// and silently produce NaN; use safe() if you want to detect.

// neg(a) — element-wise negation. Works on f32/f64/i64.
fn neg(a)  { return _tensor_neg(a) }
// abs(a) — element-wise absolute value. Works on f32/f64/i64.
fn abs(a)  { return _tensor_abs(a) }
// exp(a) — element-wise e^x. Float tensors (f32/f64) only.
fn exp(a)  { return _tensor_exp(a) }
// log(a) — element-wise natural logarithm. Float tensors only. Negative input yields NaN (IEEE 754).
fn log(a)  { return _tensor_log(a) }
// sqrt(a) — element-wise square root. Float tensors only. Negative input yields NaN (IEEE 754).
fn sqrt(a) { return _tensor_sqrt(a) }
// sin(a) — element-wise sine (radians). Float tensors only.
fn sin(a)  { return _tensor_sin(a) }
// cos(a) — element-wise cosine (radians). Float tensors only.
fn cos(a)  { return _tensor_cos(a) }

// shape(t) — array of int dimensions, e.g. shape(zeros([3, 4], "f64")) -> [3, 4]
fn shape(t) {
    return _tensor_shape(t)
}

// dtype(t) — "f32", "f64", or "i64"
fn dtype(t) {
    return _tensor_dtype(t)
}

// numel(t) — total element count (product of shape).
fn numel(t) {
    return _tensor_numel(t)
}

// get(t, i) — linear element access (treats tensor as flat 1-D).
// Bounds-checked.
fn get(t, i) {
    return _tensor_get(t, i)
}

// ── reductions ──
//
// Each reduction collapses a tensor to a single scalar.
//
//   sum, min, max     return a same-dtype scalar (float for f32/f64,
//                     int for i64).
//   mean              always returns a float (matches NumPy: np.mean
//                     of int64 arrays returns float).
//   argmin, argmax    return the integer index of the first
//                     occurrence on ties.
//
// Empty-tensor policy: sum returns 0 (its identity element). mean,
// min, max, argmin, argmax error cleanly — they have no well-defined
// value on an empty input.

// sum(t) — sum all elements to a scalar. Returns same dtype (0 for empty tensor).
fn sum(t)    { return _tensor_sum(t) }
// mean(t) — mean of all elements. Always returns float. Errors on empty tensor.
fn mean(t)   { return _tensor_mean(t) }
// min(t) — minimum element value. Returns same-dtype scalar. Errors on empty tensor.
fn min(t)    { return _tensor_min(t) }
// max(t) — maximum element value. Returns same-dtype scalar. Errors on empty tensor.
fn max(t)    { return _tensor_max(t) }
// argmin(t) — linear index of the first minimum element. Returns integer. Errors on empty tensor.
fn argmin(t) { return _tensor_argmin(t) }
// argmax(t) — linear index of the first maximum element. Returns integer. Errors on empty tensor.
fn argmax(t) { return _tensor_argmax(t) }

// ── axis-aware reductions (2-D only in v1) ──
//
// Reduces along ONE axis, returning a 1-D tensor instead of a scalar.
// For 2-D input shape [m, n]:
//   axis 0 (or -2)  → reduce rows; output shape [n]   (per-column op)
//   axis 1 (or -1)  → reduce cols; output shape [m]   (per-row op)
//
// Output dtype:
//   sum_axis, min_axis, max_axis      → same dtype as input
//   mean_axis                         → always f64 (NumPy parity)
//   argmin_axis, argmax_axis          → always i64 (index tensor)
//
// NumPy parallel: np.sum(t, axis=N), np.mean(t, axis=N), etc.
// Empty-axis policy: sum_axis returns zeros; the other five error.
//
// N-D axis reductions and the keepdims=True option are deferred to v2.
//
//   col_means = t.mean_axis(matrix, 0)   // per-column mean
//   row_norms_sq = t.sum_axis(t.mul(matrix, matrix), 1)   // per-row ‖·‖²

// sum_axis(t, axis) — reduce along axis returning a 1-D sum tensor. Same dtype as input.
fn sum_axis(t, axis)    { return _tensor_sum_axis(t, axis) }
// mean_axis(t, axis) — reduce along axis returning a 1-D mean tensor. Always f64.
fn mean_axis(t, axis)   { return _tensor_mean_axis(t, axis) }
// min_axis(t, axis) — reduce along axis returning a 1-D minimum tensor. Same dtype as input.
fn min_axis(t, axis)    { return _tensor_min_axis(t, axis) }
// max_axis(t, axis) — reduce along axis returning a 1-D maximum tensor. Same dtype as input.
fn max_axis(t, axis)    { return _tensor_max_axis(t, axis) }
// argmin_axis(t, axis) — reduce along axis returning a 1-D i64 tensor of first minimum indices.
fn argmin_axis(t, axis) { return _tensor_argmin_axis(t, axis) }
// argmax_axis(t, axis) — reduce along axis returning a 1-D i64 tensor of first maximum indices.
fn argmax_axis(t, axis) { return _tensor_argmax_axis(t, axis) }

// ── in-place element-wise ops ──
//
// Each `_inplace` variant mutates its FIRST argument and returns
// the same tensor reference. Use these in tight loops where you
// don't need to preserve `a`'s old contents — they skip the fresh
// allocation + zero-init that the non-inplace variants do.
//
// Measured on M4 (2026-05-23 diagnostic): f64 add at 1M elements
// runs at 119 GB/s in-place vs 79 GB/s allocating — a ~50% speedup
// from cutting one full pass over the output buffer.
//
// Caveat: the result overwrites `a`. If you need the old value,
// copy first or use the non-inplace variant. b is never mutated.
//
// Examples:
//
//   // hot loop: accumulate into the same buffer every iteration
//   acc = t.zeros([1000000], "f64")
//   i = 0
//   while i < numBatches {
//       batch = loadBatch(i)
//       t.add_inplace(acc, batch)   // acc += batch, no alloc
//       i = i + 1
//   }
//
//   // chaining still works because each call returns its first arg
//   t.add_inplace(t.mul_inplace(a, b), c)   // a = (a * b) + c
//
// Shape, dtype, and contiguity rules are identical to the non-
// inplace variants. Float-only ops (exp/log/sqrt/sin/cos) still
// reject i64 inputs with the same clean error.

// add_inplace(a, b) — a += b element-wise in place. Returns a. Avoids output buffer allocation.
fn add_inplace(a, b)  { return _tensor_add_inplace(a, b) }
// sub_inplace(a, b) — a -= b element-wise in place. Returns a.
fn sub_inplace(a, b)  { return _tensor_sub_inplace(a, b) }
// mul_inplace(a, b) — a *= b element-wise in place. Returns a.
fn mul_inplace(a, b)  { return _tensor_mul_inplace(a, b) }
// div_inplace(a, b) — a /= b element-wise in place. Returns a.
fn div_inplace(a, b)  { return _tensor_div_inplace(a, b) }
// pow_inplace(a, b) — a = a^b element-wise in place. Returns a.
fn pow_inplace(a, b)  { return _tensor_pow_inplace(a, b) }

// neg_inplace(a) — negate a in place. Returns a.
fn neg_inplace(a)  { return _tensor_neg_inplace(a) }
// abs_inplace(a) — absolute value of a in place. Returns a.
fn abs_inplace(a)  { return _tensor_abs_inplace(a) }
// exp_inplace(a) — e^x applied to a in place. Float tensors only. Returns a.
fn exp_inplace(a)  { return _tensor_exp_inplace(a) }
// log_inplace(a) — natural log applied to a in place. Float tensors only. Returns a.
fn log_inplace(a)  { return _tensor_log_inplace(a) }
// sqrt_inplace(a) — square root applied to a in place. Float tensors only. Returns a.
fn sqrt_inplace(a) { return _tensor_sqrt_inplace(a) }
// sin_inplace(a) — sine applied to a in place (radians). Float tensors only. Returns a.
fn sin_inplace(a)  { return _tensor_sin_inplace(a) }
// cos_inplace(a) — cosine applied to a in place (radians). Float tensors only. Returns a.
fn cos_inplace(a)  { return _tensor_cos_inplace(a) }

// ── shape manipulation ──

// flatten(t) — return a 1-D view of t (shape [numel(t)]). Shares
// the backing data with t (NumPy's `ravel`; FrogPy's `flatten`
// returns a view, not a copy, for memory efficiency).
//
//   t.flatten(t.zeros([3, 4], "f64"))   // shape [12]
fn flatten(t) {
    return _tensor_reshape(t, [_tensor_numel(t)])
}

// squeeze(t) — return a view with all size-1 dimensions removed.
//   t.squeeze(t.zeros([1, 3, 1, 4], "f64"))   // shape [3, 4]
// If every dimension is 1, result is a 0-D (scalar) tensor.
// NumPy parallel: np.squeeze (without axis= argument).
fn squeeze(t) {
    return _tensor_squeeze(t)
}

// expand_dims(t, axis) — insert a new size-1 dimension at `axis`.
// axis may be negative (counts from end). Common use: prepare a 1-D
// vector for broadcasting against a matrix.
//
//   t.expand_dims(vec, 0)   // (n,) → (1, n)  for row-shaped broadcast
//   t.expand_dims(vec, -1)  // (n,) → (n, 1)  for col-shaped broadcast
//
// NumPy parallel: np.expand_dims.
fn expand_dims(t, axis) {
    return _tensor_expand_dims(t, axis)
}

// dot(a, b) — 1-D inner product. Both inputs must be 1-D with
// matching length and dtype. Returns a scalar. Fused mul+sum in
// the underlying C kernel — no temporary tensor allocated.
//
// For 2-D matrix multiplication use matmul; for matrix · vector
// use matmul with the vector reshaped to a column.
//
// NumPy parallel: np.dot for the 1-D × 1-D case.
fn dot(a, b) {
    return _tensor_dot(a, b)
}

// reshape(t, newShape) — return a tensor with the same data and a
// different shape. product(newShape) must equal numel(t).
//
// The returned tensor shares the backing data with t (NumPy-style
// view), so in-place ops on either alias mutate both. Use this to
// promote a 1-D from_array result to 2-D for matmul, etc.
//
//   row = t.from_array([1, 2, 3, 4, 5, 6], "f64")
//   mat = t.reshape(row, [2, 3])    // 2×3 matrix, same backing data
//
// v1 only handles contiguous → contiguous reshape.
fn reshape(t, newShape) {
    return _tensor_reshape(t, newShape)
}

// ── linear algebra ──

// transpose(t) — 2-D matrix transpose. Shape [m, n] → [n, m].
//
// Materialising — returns a fresh contiguous tensor (does NOT share
// storage with the input). Use directly as input to matmul for the
// common A·Bᵀ pattern:
//
//   ATbA = t.matmul(t.transpose(A), A)   // A^T · A (m×n → n×n)
//
// v1 is 2-D only; N-D transpose (NumPy's `np.transpose(a, axes)`
// permutation form) is deferred.
fn transpose(t) {
    return _tensor_transpose(t)
}

// matmul(a, b) — matrix multiply C := A · B.
//
// Shapes:  a is [m, k], b is [k, n], result is [m, n]. Both inputs
// must be 2-D and contiguous; dtypes must match (no promotion).
//
// Backend dispatch:
//   f32 on macOS  →  Apple MPSMatrixMultiplication (GPU). Wins
//                    decisively above ~512³; the GPU dispatch is
//                    slightly slower than CPU below that, but the
//                    delta is sub-millisecond.
//   f32 on Linux  →  CPU naive kernel (-O3 -ffast-math autovec).
//   f64 / i64     →  CPU naive kernel on every platform. MPS is
//                    f32-only at the current bridge surface.
//
// Output is bit-exact identical between the GPU and CPU f32 paths
// (verified for the underlying mtl.matmulMPS in stdlib/mtl.lex).
fn matmul(a, b) {
    return _tensor_matmul(a, b)
}
