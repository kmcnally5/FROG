/* tensor_kernels.h — kLex FrogPy v1 native compute kernels.
 *
 * Compiled into kLex on macOS and Linux via cgo (eval/tensor_compute_cgo.go).
 * Windows builds skip this file entirely and fall back to the stub in
 * eval/tensor_compute_stub.go, which surfaces a clean "tensor ops require
 * macOS or Linux" runtime error.
 *
 * Design discipline
 *
 *   - Pure ANSI C, no platform-specific syscalls or headers.
 *   - Each function takes pointer + length and writes into a caller-
 *     allocated output buffer. No malloc/free here — memory ownership
 *     is the Go caller's. This keeps cgo wrapping trivial and avoids
 *     cross-language allocator drama.
 *   - Compiled with -O3 -ffast-math; rely on the compiler's autovec
 *     for the common case. Hand-tuned SIMD only where measurement
 *     proves it necessary (deferred; v1 has no manual intrinsics).
 *   - Float kernels assume IEEE 754. Integer kernels are wraparound
 *     on overflow (consistent with kLex's general integer policy).
 */

#ifndef KLEX_TENSOR_KERNELS_H
#define KLEX_TENSOR_KERNELS_H

#include <stddef.h>
#include <stdint.h>

/* Element-wise add: out[i] = a[i] + b[i] for i in [0, n).
 * Aliasing is permitted (out may equal a or b). */
void klex_tensor_add_f32(float* out, const float* a, const float* b, size_t n);
void klex_tensor_add_f64(double* out, const double* a, const double* b, size_t n);
void klex_tensor_add_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n);

/* Element-wise sub: out[i] = a[i] - b[i]. Same aliasing rules. */
void klex_tensor_sub_f32(float* out, const float* a, const float* b, size_t n);
void klex_tensor_sub_f64(double* out, const double* a, const double* b, size_t n);
void klex_tensor_sub_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n);

/* Element-wise mul: out[i] = a[i] * b[i]. Integer overflow wraps. */
void klex_tensor_mul_f32(float* out, const float* a, const float* b, size_t n);
void klex_tensor_mul_f64(double* out, const double* a, const double* b, size_t n);
void klex_tensor_mul_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n);

/* Element-wise div: out[i] = a[i] / b[i].
 *
 * Float: IEEE 754 — n/0 = ±Inf, 0/0 = NaN (no error condition).
 *
 * Integer: the C kernel performs unguarded division. CALLER MUST
 * guarantee no zeros in b — the Go wrapper does a linear scan and
 * surfaces a clean kLex "division by zero" error before this is
 * called. Keeping the check Go-side lets us report the exact index
 * and avoids per-iteration branches in the hot kernel loop. */
void klex_tensor_div_f32(float* out, const float* a, const float* b, size_t n);
void klex_tensor_div_f64(double* out, const double* a, const double* b, size_t n);
void klex_tensor_div_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n);

/* Element-wise pow: out[i] = a[i] ** b[i].
 *
 * Float kernels delegate to libm's powf / pow — IEEE-conformant.
 * Negative bases with non-integer exponents produce NaN (no error).
 *
 * Integer kernel uses exponentiation-by-squaring. CALLER must
 * guarantee b[i] >= 0 — the Go wrapper scans and surfaces a clean
 * error for negative exponents (which would be < 1 for |a|>1 and
 * not representable in i64). Overflow wraps per the general i64
 * policy. */
void klex_tensor_pow_f32(float* out, const float* a, const float* b, size_t n);
void klex_tensor_pow_f64(double* out, const double* a, const double* b, size_t n);
void klex_tensor_pow_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n);

/* ===== unary kernels =====
 *
 * Each takes one input + one output buffer of equal length.
 * Aliasing is permitted (out may equal a). */

/* neg: out[i] = -a[i]. i64 negation of INT64_MIN wraps (per general
 * i64 policy — kLex does not special-case overflow). */
void klex_tensor_neg_f32(float* out, const float* a, size_t n);
void klex_tensor_neg_f64(double* out, const double* a, size_t n);
void klex_tensor_neg_i64(int64_t* out, const int64_t* a, size_t n);

/* abs: out[i] = |a[i]|. i64 abs of INT64_MIN wraps; the result
 * stays INT64_MIN. Matches NumPy's np.abs(np.int64).min). */
void klex_tensor_abs_f32(float* out, const float* a, size_t n);
void klex_tensor_abs_f64(double* out, const double* a, size_t n);
void klex_tensor_abs_i64(int64_t* out, const int64_t* a, size_t n);

/* Transcendentals — float-only. Domain errors silently produce
 * NaN or ±Inf per IEEE 754; we don't raise. Use safe() if you
 * want to detect NaN downstream. */
void klex_tensor_exp_f32(float* out, const float* a, size_t n);
void klex_tensor_exp_f64(double* out, const double* a, size_t n);
void klex_tensor_log_f32(float* out, const float* a, size_t n);
void klex_tensor_log_f64(double* out, const double* a, size_t n);
void klex_tensor_sqrt_f32(float* out, const float* a, size_t n);
void klex_tensor_sqrt_f64(double* out, const double* a, size_t n);
void klex_tensor_sin_f32(float* out, const float* a, size_t n);
void klex_tensor_sin_f64(double* out, const double* a, size_t n);
void klex_tensor_cos_f32(float* out, const float* a, size_t n);
void klex_tensor_cos_f64(double* out, const double* a, size_t n);

/* ===== reductions =====
 *
 * Single-tensor → scalar. Mean is computed in Go (sum / n) so no
 * dedicated C kernel — the precision policy stays consistent with
 * sum's accumulator type.
 *
 * Accumulator types:
 *   - sum_f32 accumulates in float (matches NumPy's default; for
 *     better precision use sum on a f64 view)
 *   - sum_f64 accumulates in double
 *   - sum_i64 accumulates in int64 (overflow wraps)
 *
 * NaN handling on min/max is propagating but order-dependent
 * (C's `<`/`>` return false for NaN comparisons). For NaN-aware
 * reductions a separate kernel set would be needed; deferred.
 *
 * All min/max/argmin/argmax kernels assume n >= 1 — the Go-side
 * caller checks for empty tensors and returns a kLex error before
 * invoking. argmin/argmax return the first occurrence on ties. */

float   klex_tensor_sum_f32(const float* a, size_t n);
double  klex_tensor_sum_f64(const double* a, size_t n);
int64_t klex_tensor_sum_i64(const int64_t* a, size_t n);

float   klex_tensor_min_f32(const float* a, size_t n);
double  klex_tensor_min_f64(const double* a, size_t n);
int64_t klex_tensor_min_i64(const int64_t* a, size_t n);

float   klex_tensor_max_f32(const float* a, size_t n);
double  klex_tensor_max_f64(const double* a, size_t n);
int64_t klex_tensor_max_i64(const int64_t* a, size_t n);

size_t  klex_tensor_argmin_f32(const float* a, size_t n);
size_t  klex_tensor_argmin_f64(const double* a, size_t n);
size_t  klex_tensor_argmin_i64(const int64_t* a, size_t n);

size_t  klex_tensor_argmax_f32(const float* a, size_t n);
size_t  klex_tensor_argmax_f64(const double* a, size_t n);
size_t  klex_tensor_argmax_i64(const int64_t* a, size_t n);

/* ===== matmul =====
 *
 * C[m × n] := A[m × k] · B[k × n], all row-major. The output buffer
 * is zeroed inside the kernel — callers do NOT need to pre-zero c.
 * Aliasing between c and a/b is not permitted; callers must allocate
 * c distinct from both inputs.
 *
 * Loop order is i-k-j with the inner j loop walking contiguous memory
 * in both c and b. This is autovectoriser-friendly under -O3 -ffast-math
 * on both Clang and GCC for the float kernels; the i64 kernel auto-vecs
 * on AArch64 and AVX2.
 *
 * On Apple Silicon the f32 path is normally serviced by MPS (see
 * eval/tensor_matmul_mps_darwin.go); this CPU kernel is the fallback
 * when MPS is unavailable (Linux) or when dtype is f64/i64. */
void klex_tensor_matmul_f32(float* c, const float* a, const float* b,
                            size_t m, size_t k, size_t n);
void klex_tensor_matmul_f64(double* c, const double* a, const double* b,
                            size_t m, size_t k, size_t n);
void klex_tensor_matmul_i64(int64_t* c, const int64_t* a, const int64_t* b,
                            size_t m, size_t k, size_t n);

/* ===== transpose (2-D) =====
 *
 * out is a fresh [n × m] row-major tensor; in is [m × n] row-major.
 * out[i*m + j] = in[j*n + i] for i in [0, n), j in [0, m).
 *
 * Loop walks input contiguously (j outer, i inner) — sequential reads
 * with strided writes. For typical matrix sizes this is faster than
 * the reverse order because read-side prefetch is more aggressive
 * than write-combining on modern CPUs. A tiled / blocked transpose
 * would gain another 2-3× on huge matrices; deferred to v2 if needed.
 */
void klex_tensor_transpose2d_f32(float* out, const float* in, size_t m, size_t n);
void klex_tensor_transpose2d_f64(double* out, const double* in, size_t m, size_t n);
void klex_tensor_transpose2d_i64(int64_t* out, const int64_t* in, size_t m, size_t n);

/* ===== dot product (1-D, fused mul+sum) =====
 *
 * Accumulator types match the input dtype (NumPy convention, same as
 * the existing sum_* kernels):
 *   - f32 → float accumulator
 *   - f64 → double accumulator
 *   - i64 → int64_t accumulator (wraps on overflow per kLex policy)
 *
 * Empty-input handling: caller's responsibility — the Go-side builtin
 * checks for n == 0 before dispatch and returns the dtype's zero
 * scalar without entering the kernel.
 */
float   klex_tensor_dot_f32(const float* a, const float* b, size_t n);
double  klex_tensor_dot_f64(const double* a, const double* b, size_t n);
int64_t klex_tensor_dot_i64(const int64_t* a, const int64_t* b, size_t n);

/* ===== axis-aware reductions (N-D) =====
 *
 * Input is row-major contiguous of arbitrary rank. The caller flattens
 * the shape around the reduction axis into three integers:
 *
 *   prefix    — product of dimensions BEFORE the reduced axis
 *               (1 if the axis is the leading dimension)
 *   reduceLen — size of the reduced axis itself
 *   suffix    — product of dimensions AFTER the reduced axis
 *               (1 if the axis is the trailing dimension)
 *
 * In flat indexing the input element at (p, r, s) where
 * 0 <= p < prefix, 0 <= r < reduceLen, 0 <= s < suffix lives at
 * in[(p * reduceLen + r) * suffix + s]. The kernel collapses the r
 * dimension; the output has prefix*suffix elements, indexed by
 * out[p * suffix + s]. The 2-D special cases fall out:
 *   2-D [m, n] axis 0  →  prefix=1, reduceLen=m, suffix=n  (per-column)
 *   2-D [m, n] axis 1  →  prefix=m, reduceLen=n, suffix=1  (per-row)
 *
 * Two access patterns inside each kernel:
 *   suffix == 1  → contiguous segment reduce; register-tracked accumulator
 *   suffix  > 1  → outer accumulate; initialise the suffix-wide output
 *                  strip from r=0 then fold rows r=1..reduceLen-1 in
 *                  place. Inner loop walks contiguous memory (good for
 *                  autovec) at the cost of one extra pass per output
 *                  cell vs the contiguous case.
 *
 * For min/max/argmin/argmax the kernel assumes reduceLen >= 1 (the
 * Go-side check enforces this). Ties in argmin/argmax resolve to the
 * first occurrence (matches the scalar versions). */

/* sum: out is same dtype as in */
void klex_tensor_sum_axis_f32(float*   out, const float*   in,
                              size_t prefix, size_t reduceLen, size_t suffix);
void klex_tensor_sum_axis_f64(double*  out, const double*  in,
                              size_t prefix, size_t reduceLen, size_t suffix);
void klex_tensor_sum_axis_i64(int64_t* out, const int64_t* in,
                              size_t prefix, size_t reduceLen, size_t suffix);

/* min / max: out is same dtype as in */
void klex_tensor_min_axis_f32(float*   out, const float*   in,
                              size_t prefix, size_t reduceLen, size_t suffix);
void klex_tensor_min_axis_f64(double*  out, const double*  in,
                              size_t prefix, size_t reduceLen, size_t suffix);
void klex_tensor_min_axis_i64(int64_t* out, const int64_t* in,
                              size_t prefix, size_t reduceLen, size_t suffix);
void klex_tensor_max_axis_f32(float*   out, const float*   in,
                              size_t prefix, size_t reduceLen, size_t suffix);
void klex_tensor_max_axis_f64(double*  out, const double*  in,
                              size_t prefix, size_t reduceLen, size_t suffix);
void klex_tensor_max_axis_i64(int64_t* out, const int64_t* in,
                              size_t prefix, size_t reduceLen, size_t suffix);

/* argmin / argmax: out is int64 (regardless of input dtype) */
void klex_tensor_argmin_axis_f32(int64_t* out, const float*   in,
                                 size_t prefix, size_t reduceLen, size_t suffix);
void klex_tensor_argmin_axis_f64(int64_t* out, const double*  in,
                                 size_t prefix, size_t reduceLen, size_t suffix);
void klex_tensor_argmin_axis_i64(int64_t* out, const int64_t* in,
                                 size_t prefix, size_t reduceLen, size_t suffix);
void klex_tensor_argmax_axis_f32(int64_t* out, const float*   in,
                                 size_t prefix, size_t reduceLen, size_t suffix);
void klex_tensor_argmax_axis_f64(int64_t* out, const double*  in,
                                 size_t prefix, size_t reduceLen, size_t suffix);
void klex_tensor_argmax_axis_i64(int64_t* out, const int64_t* in,
                                 size_t prefix, size_t reduceLen, size_t suffix);

/* ===== clip =====
 *
 * out[i] = clamp(a[i], lo, hi) for i in [0, n).
 * Aliasing is permitted (out may equal a).
 * lo <= hi is a caller invariant (not checked here). */
void klex_tensor_clip_f32(float*   out, const float*   a, float   lo, float   hi, size_t n);
void klex_tensor_clip_f64(double*  out, const double*  a, double  lo, double  hi, size_t n);
void klex_tensor_clip_i64(int64_t* out, const int64_t* a, int64_t lo, int64_t hi, size_t n);

/* ===== element-wise comparisons =====
 *
 * Each comparison writes 1 into out[i] when the condition holds, 0
 * otherwise. Output is always int64_t regardless of input dtype.
 *
 * Float NaN semantics follow IEEE 754: any comparison involving NaN
 * returns false (0), so eq/lt/le/gt/ge all produce 0 for NaN inputs;
 * ne produces 1 (NaN != NaN is true). This matches NumPy.
 *
 * Aliasing: out is int64_t, a/b are the source dtype — they cannot
 * alias by construction (different element sizes), so no aliasing
 * constraints apply. */

void klex_tensor_eq_f32(int64_t* out, const float*   a, const float*   b, size_t n);
void klex_tensor_eq_f64(int64_t* out, const double*  a, const double*  b, size_t n);
void klex_tensor_eq_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n);

void klex_tensor_ne_f32(int64_t* out, const float*   a, const float*   b, size_t n);
void klex_tensor_ne_f64(int64_t* out, const double*  a, const double*  b, size_t n);
void klex_tensor_ne_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n);

void klex_tensor_lt_f32(int64_t* out, const float*   a, const float*   b, size_t n);
void klex_tensor_lt_f64(int64_t* out, const double*  a, const double*  b, size_t n);
void klex_tensor_lt_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n);

void klex_tensor_le_f32(int64_t* out, const float*   a, const float*   b, size_t n);
void klex_tensor_le_f64(int64_t* out, const double*  a, const double*  b, size_t n);
void klex_tensor_le_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n);

void klex_tensor_gt_f32(int64_t* out, const float*   a, const float*   b, size_t n);
void klex_tensor_gt_f64(int64_t* out, const double*  a, const double*  b, size_t n);
void klex_tensor_gt_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n);

void klex_tensor_ge_f32(int64_t* out, const float*   a, const float*   b, size_t n);
void klex_tensor_ge_f64(int64_t* out, const double*  a, const double*  b, size_t n);
void klex_tensor_ge_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n);

#endif /* KLEX_TENSOR_KERNELS_H */
