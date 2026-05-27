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

// ── axis-aware reductions (N-D) ──
//
// Reduces along ONE axis, returning a tensor of rank-1 (drop variant)
// or the same rank with the reduced axis kept at size 1 (keepdims).
// axis may be negative; -1 is the last axis.
//
// Output dtype:
//   sum_axis, min_axis, max_axis      → same dtype as input
//   mean_axis                         → always f64 (NumPy parity)
//   argmin_axis, argmax_axis          → i64 (index into the reduced axis)
//
// NumPy parallel: np.sum(t, axis=N), np.mean(t, axis=N), etc.
// Empty-axis policy: sum_axis returns zeros; the other five error.
//
// keepdims variants pair naturally with broadcasting:
//
//   row_means = t.mean_axis_keepdims(x, -1)   // shape [B, S, 1]
//   centred   = t.sub(x, row_means)           // broadcasts cleanly
//
//   col_means = t.mean_axis(matrix, 0)        // shape [n]
//   row_norms_sq = t.sum_axis(t.mul(matrix, matrix), 1)   // per-row ‖·‖²

// sum_axis(t, axis) — sum along axis, drop the reduced dim. Same dtype as input.
fn sum_axis(t, axis)             { return _tensor_sum_axis(t, axis) }
// sum_axis_keepdims(t, axis) — sum along axis, keep the reduced dim at size 1. Same dtype as input.
fn sum_axis_keepdims(t, axis)    { return _tensor_sum_axis_keepdims(t, axis) }
// mean_axis(t, axis) — mean along axis, drop the reduced dim. Always f64.
fn mean_axis(t, axis)            { return _tensor_mean_axis(t, axis) }
// mean_axis_keepdims(t, axis) — mean along axis, keep the reduced dim at size 1. Always f64.
fn mean_axis_keepdims(t, axis)   { return _tensor_mean_axis_keepdims(t, axis) }
// min_axis(t, axis) — minimum along axis, drop the reduced dim. Same dtype as input.
fn min_axis(t, axis)             { return _tensor_min_axis(t, axis) }
// min_axis_keepdims(t, axis) — minimum along axis, keep the reduced dim at size 1.
fn min_axis_keepdims(t, axis)    { return _tensor_min_axis_keepdims(t, axis) }
// max_axis(t, axis) — maximum along axis, drop the reduced dim. Same dtype as input.
fn max_axis(t, axis)             { return _tensor_max_axis(t, axis) }
// max_axis_keepdims(t, axis) — maximum along axis, keep the reduced dim at size 1.
fn max_axis_keepdims(t, axis)    { return _tensor_max_axis_keepdims(t, axis) }
// argmin_axis(t, axis) — i64 indices of first minimum along axis (drop variant).
fn argmin_axis(t, axis)          { return _tensor_argmin_axis(t, axis) }
// argmin_axis_keepdims(t, axis) — argmin along axis, keep the reduced dim at size 1.
fn argmin_axis_keepdims(t, axis) { return _tensor_argmin_axis_keepdims(t, axis) }
// argmax_axis(t, axis) — i64 indices of first maximum along axis (drop variant).
fn argmax_axis(t, axis)          { return _tensor_argmax_axis(t, axis) }
// argmax_axis_keepdims(t, axis) — argmax along axis, keep the reduced dim at size 1.
fn argmax_axis_keepdims(t, axis) { return _tensor_argmax_axis_keepdims(t, axis) }

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

// ── Group 1: to_array / clone / cast ────────────────────────────────

// to_array(t) — extract all elements into a flat kLex array.
//   f32/f64 → array of Float;  i64 → array of Integer.
//   t.to_array(t.from_array([1, 2, 3], "f64"))  →  [1.0, 2.0, 3.0]
fn to_array(t) {
    return _tensor_to_array(t)
}

// clone(t) — return a fully independent copy of t.
// Mutations to the clone do not affect t (unlike reshape / flatten views).
// Cloning a slice view (from t.slice) materialises into a fresh
// contiguous tensor — same behaviour as t.contiguous on the view.
fn clone(t) {
    return _tensor_clone(t)
}

// cast(t, dtype) — return a new tensor with every element converted to dtype.
//   t.cast(myF64Tensor, "f32")    // narrow to single precision
//   t.cast(myI64Tensor, "f64")    // widen integers to double
// When src and target dtypes are the same this is equivalent to clone.
fn cast(t, dtype) {
    return _tensor_cast(t, dtype)
}

// slice(t, specs) — return a VIEW into t sharing backing data.
//
// specs is an array with one entry per axis of t. Each entry is
// either null (take all of this axis) or a 3-element array
// [start, stop, step] where any of start/stop/step may itself be
// null to take the default (0 / dim / 1). Negative start/stop count
// from the end of the axis. step must be positive in v1.
//
//   let big = t.full([1000, 1000], 1.0, "f64")
//   let middle  = t.slice(big, [[100, 500, 1], [200, 800, 1]])  // [400, 600]
//   let stride  = t.slice(big, [null, [0, null, 2]])            // every 2nd col
//   let lastTen = t.slice(big, [[-10, null, 1], null])          // last 10 rows
//
// Slices are views — mutations to the slice affect t and vice versa.
// Inspection ops (shape, dtype, numel, get, to_array, clone) work on
// views directly. Kernel-based ops (add, matmul, reductions, etc.)
// require contiguous input — call t.contiguous(view) to materialise
// before passing a slice to them.
fn slice(t, specs) {
    return _tensor_slice(t, specs)
}

// contiguous(t) — if t is already contiguous, return t unchanged
// (no copy — matches NumPy ascontiguousarray). If t is a strided
// view (from t.slice), materialise into a fresh contiguous tensor.
//
// Required before passing a slice view to any kernel-based op:
//
//   let view  = t.slice(big, [[100, 500, 1], null])
//   let safe  = t.contiguous(view)
//   let result = t.add(safe, other)         // kernel runs cleanly
//
fn contiguous(t) {
    return _tensor_contiguous(t)
}

// ── Group 2: clip / linspace / arange / eye ─────────────────────────

// clip(t, lo, hi) — clamp every element of t to [lo, hi].
//   lo and hi must be compatible numbers for t's dtype.
//   lo <= hi is required.
//   t.clip(scores, 0.0, 1.0)    // saturate to unit range
fn clip(t, lo, hi) {
    return _tensor_clip(t, lo, hi)
}

// linspace(start, stop, n, dtype) — 1-D tensor of n evenly spaced
// values from start to stop inclusive. n >= 1. stop is always exact.
//   t.linspace(0.0, 1.0, 5, "f64")   →  [0.0, 0.25, 0.5, 0.75, 1.0]
fn linspace(start, stop, n, dtype) {
    return _tensor_linspace(start, stop, n, dtype)
}

// arange(start, stop, step, dtype) — 1-D tensor of values from start
// up to (but not including) stop, spaced by step. step must be non-zero.
//   t.arange(0, 6, 2, "i64")   →  [0, 2, 4]
//   t.arange(0.0, 1.0, 0.25, "f64")  →  [0.0, 0.25, 0.5, 0.75]
fn arange(start, stop, step, dtype) {
    return _tensor_arange(start, stop, step, dtype)
}

// eye(n, dtype) — n×n identity matrix (1 on diagonal, 0 elsewhere).
//   t.eye(3, "f64")  →  [[1,0,0],[0,1,0],[0,0,1]]
fn eye(n, dtype) {
    return _tensor_eye(n, dtype)
}

// ── Group 3: comparison ops + where ─────────────────────────────────
//
// All comparison ops return an i64 tensor of 0s and 1s (a mask).
// Both operands may be tensors or scalars (broadcasting applies).
// Dtypes must match — use cast() to align before comparing.
//
// NaN behaviour follows IEEE 754: eq/lt/le/gt/ge return 0 for NaN;
// ne returns 1 (NaN != NaN is true). Matches NumPy semantics.

// eq(a, b) — element-wise a == b. Returns i64 mask.
fn eq(a, b) { return _tensor_eq(a, b) }
// ne(a, b) — element-wise a != b. Returns i64 mask.
fn ne(a, b) { return _tensor_ne(a, b) }
// lt(a, b) — element-wise a < b. Returns i64 mask.
fn lt(a, b) { return _tensor_lt(a, b) }
// le(a, b) — element-wise a <= b. Returns i64 mask.
fn le(a, b) { return _tensor_le(a, b) }
// gt(a, b) — element-wise a > b. Returns i64 mask.
fn gt(a, b) { return _tensor_gt(a, b) }
// ge(a, b) — element-wise a >= b. Returns i64 mask.
fn ge(a, b) { return _tensor_ge(a, b) }

// where(mask, x, y) — element-wise conditional: out[i] = x[i] if
// mask[i] != 0 else y[i]. mask must be an i64 tensor from a comparison
// op. x and y must have the same dtype; either may be a scalar.
//
//   relu = fn(t) { return where(gt(t, 0.0), t, 0.0) }
//   safe = where(ne(denom, 0.0), div(num, denom), 0.0)
fn where(mask, x, y) {
    return _tensor_where(mask, x, y)
}

// ── Group 4: concatenate / stack ─────────────────────────────────────

// concatenate(tensors, axis) — join an array of tensors along an
// existing axis. All tensors must share the same dtype and shape
// except at `axis`. NumPy equivalent: np.concatenate.
//
//   rows = t.concatenate([batchA, batchB], 0)   // stack row batches
//   cols = t.concatenate([featA, featB], 1)     // join column features
fn concatenate(tensors, axis) {
    return _tensor_concatenate(tensors, axis)
}

// stack(tensors, axis) — join an array of identically-shaped tensors
// along a NEW axis inserted at position `axis`. All tensors must have
// the same shape and dtype. Output rank = input rank + 1.
// NumPy equivalent: np.stack.
//
//   batch = t.stack([sample1, sample2, sample3], 0)
//   // [n] tensors → [3, n] matrix
fn stack(tensors, axis) {
    return _tensor_stack(tensors, axis)
}
