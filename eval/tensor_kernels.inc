/* tensor_kernels.c — implementations of the kLex FrogPy v1 kernels.
 *
 * Build flags (set in eval/tensor_compute_cgo.go's CFLAGS):
 *
 *   -O3            — full optimisation including loop unrolling + autovec.
 *   -ffast-math    — allow reassociation of FP ops (matters for sum/reduce
 *                    autovec; float kernels here are commutative).
 *   -fno-strict-aliasing — restrict-style aliasing isn't promised; we don't
 *                    use the C `restrict` keyword to keep callers free to
 *                    alias in/out without UB.
 *
 * The kernels read like textbook reference impls on purpose. The compiler
 * vectorises them well; hand-tuning is reserved for places measurement
 * proves are bottlenecks.
 */

#include "tensor_kernels.h"
#include <math.h>

/* ===== element-wise add ===== */

void klex_tensor_add_f32(float* out, const float* a, const float* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = a[i] + b[i];
    }
}

void klex_tensor_add_f64(double* out, const double* a, const double* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = a[i] + b[i];
    }
}

void klex_tensor_add_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = a[i] + b[i];
    }
}

/* ===== element-wise sub ===== */

void klex_tensor_sub_f32(float* out, const float* a, const float* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = a[i] - b[i];
    }
}

void klex_tensor_sub_f64(double* out, const double* a, const double* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = a[i] - b[i];
    }
}

void klex_tensor_sub_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = a[i] - b[i];
    }
}

/* ===== element-wise mul ===== */

void klex_tensor_mul_f32(float* out, const float* a, const float* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = a[i] * b[i];
    }
}

void klex_tensor_mul_f64(double* out, const double* a, const double* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = a[i] * b[i];
    }
}

void klex_tensor_mul_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = a[i] * b[i];
    }
}

/* ===== element-wise div =====
 *
 * Float kernels rely on IEEE 754 — n/0 is well-defined (±Inf / NaN).
 * The i64 kernel does NOT guard against zero divisors; the Go wrapper
 * scans `b` and raises a kLex error before reaching here. */

void klex_tensor_div_f32(float* out, const float* a, const float* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = a[i] / b[i];
    }
}

void klex_tensor_div_f64(double* out, const double* a, const double* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = a[i] / b[i];
    }
}

void klex_tensor_div_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = a[i] / b[i];
    }
}

/* ===== element-wise pow =====
 *
 * Float: delegate to libm. -ffast-math may rewrite some calls (e.g.
 * pow(x, 2) → x*x); that's fine — kLex doesn't promise bit-exact
 * float results.
 *
 * Integer: classic exponentiation-by-squaring. b[i] is assumed
 * non-negative (Go wrapper pre-checks). Overflow wraps. */

void klex_tensor_pow_f32(float* out, const float* a, const float* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = powf(a[i], b[i]);
    }
}

void klex_tensor_pow_f64(double* out, const double* a, const double* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        out[i] = pow(a[i], b[i]);
    }
}

void klex_tensor_pow_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n) {
    for (size_t i = 0; i < n; i++) {
        int64_t base = a[i];
        int64_t exp = b[i];
        int64_t result = 1;
        /* exp >= 0 guaranteed by the Go-side pre-check; the loop
         * works correctly for exp == 0 (returns 1). */
        while (exp > 0) {
            if (exp & 1) {
                result *= base;
            }
            base *= base;
            exp >>= 1;
        }
        out[i] = result;
    }
}

/* ===== unary: neg ===== */

void klex_tensor_neg_f32(float* out, const float* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = -a[i];
}
void klex_tensor_neg_f64(double* out, const double* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = -a[i];
}
void klex_tensor_neg_i64(int64_t* out, const int64_t* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = -a[i];
}

/* ===== unary: abs ===== */

void klex_tensor_abs_f32(float* out, const float* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = fabsf(a[i]);
}
void klex_tensor_abs_f64(double* out, const double* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = fabs(a[i]);
}
void klex_tensor_abs_i64(int64_t* out, const int64_t* a, size_t n) {
    /* Avoid llabs() — its handling of INT64_MIN is UB in some
     * libc impls. Explicit branchless absolute via sign-mask works
     * for all values; INT64_MIN wraps to itself, matching NumPy. */
    for (size_t i = 0; i < n; i++) {
        int64_t v = a[i];
        int64_t mask = v >> 63; /* arithmetic shift: all-1s if v<0 else 0 */
        out[i] = (v ^ mask) - mask;
    }
}

/* ===== unary: exp ===== */

void klex_tensor_exp_f32(float* out, const float* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = expf(a[i]);
}
void klex_tensor_exp_f64(double* out, const double* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = exp(a[i]);
}

/* ===== unary: log ===== */

void klex_tensor_log_f32(float* out, const float* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = logf(a[i]);
}
void klex_tensor_log_f64(double* out, const double* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = log(a[i]);
}

/* ===== unary: sqrt ===== */

void klex_tensor_sqrt_f32(float* out, const float* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = sqrtf(a[i]);
}
void klex_tensor_sqrt_f64(double* out, const double* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = sqrt(a[i]);
}

/* ===== unary: sin / cos ===== */

void klex_tensor_sin_f32(float* out, const float* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = sinf(a[i]);
}
void klex_tensor_sin_f64(double* out, const double* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = sin(a[i]);
}

void klex_tensor_cos_f32(float* out, const float* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = cosf(a[i]);
}
void klex_tensor_cos_f64(double* out, const double* a, size_t n) {
    for (size_t i = 0; i < n; i++) out[i] = cos(a[i]);
}

/* ===== reductions =====
 *
 * Each kernel reads `a[0..n)` and returns a single scalar. Mean is
 * intentionally absent — the Go side computes it as sum / n, which
 * keeps the precision policy aligned with sum and avoids duplicating
 * the loop in C just to divide at the end.
 *
 * min/max/argmin/argmax assume n >= 1. The Go-side caller checks
 * for empty tensors and raises a kLex "empty input" error before
 * dispatching here, so we don't need a guard in the hot loop. */

float klex_tensor_sum_f32(const float* a, size_t n) {
    float s = 0.0f;
    for (size_t i = 0; i < n; i++) s += a[i];
    return s;
}
double klex_tensor_sum_f64(const double* a, size_t n) {
    double s = 0.0;
    for (size_t i = 0; i < n; i++) s += a[i];
    return s;
}
int64_t klex_tensor_sum_i64(const int64_t* a, size_t n) {
    int64_t s = 0;
    for (size_t i = 0; i < n; i++) s += a[i];
    return s;
}

float klex_tensor_min_f32(const float* a, size_t n) {
    float m = a[0];
    for (size_t i = 1; i < n; i++) {
        if (a[i] < m) m = a[i];
    }
    return m;
}
double klex_tensor_min_f64(const double* a, size_t n) {
    double m = a[0];
    for (size_t i = 1; i < n; i++) {
        if (a[i] < m) m = a[i];
    }
    return m;
}
int64_t klex_tensor_min_i64(const int64_t* a, size_t n) {
    int64_t m = a[0];
    for (size_t i = 1; i < n; i++) {
        if (a[i] < m) m = a[i];
    }
    return m;
}

float klex_tensor_max_f32(const float* a, size_t n) {
    float m = a[0];
    for (size_t i = 1; i < n; i++) {
        if (a[i] > m) m = a[i];
    }
    return m;
}
double klex_tensor_max_f64(const double* a, size_t n) {
    double m = a[0];
    for (size_t i = 1; i < n; i++) {
        if (a[i] > m) m = a[i];
    }
    return m;
}
int64_t klex_tensor_max_i64(const int64_t* a, size_t n) {
    int64_t m = a[0];
    for (size_t i = 1; i < n; i++) {
        if (a[i] > m) m = a[i];
    }
    return m;
}

/* argmin/argmax: first occurrence wins on ties (strict `<` / `>`). */

size_t klex_tensor_argmin_f32(const float* a, size_t n) {
    size_t idx = 0;
    float m = a[0];
    for (size_t i = 1; i < n; i++) {
        if (a[i] < m) { m = a[i]; idx = i; }
    }
    return idx;
}
size_t klex_tensor_argmin_f64(const double* a, size_t n) {
    size_t idx = 0;
    double m = a[0];
    for (size_t i = 1; i < n; i++) {
        if (a[i] < m) { m = a[i]; idx = i; }
    }
    return idx;
}
size_t klex_tensor_argmin_i64(const int64_t* a, size_t n) {
    size_t idx = 0;
    int64_t m = a[0];
    for (size_t i = 1; i < n; i++) {
        if (a[i] < m) { m = a[i]; idx = i; }
    }
    return idx;
}

size_t klex_tensor_argmax_f32(const float* a, size_t n) {
    size_t idx = 0;
    float m = a[0];
    for (size_t i = 1; i < n; i++) {
        if (a[i] > m) { m = a[i]; idx = i; }
    }
    return idx;
}
size_t klex_tensor_argmax_f64(const double* a, size_t n) {
    size_t idx = 0;
    double m = a[0];
    for (size_t i = 1; i < n; i++) {
        if (a[i] > m) { m = a[i]; idx = i; }
    }
    return idx;
}
size_t klex_tensor_argmax_i64(const int64_t* a, size_t n) {
    size_t idx = 0;
    int64_t m = a[0];
    for (size_t i = 1; i < n; i++) {
        if (a[i] > m) { m = a[i]; idx = i; }
    }
    return idx;
}

/* ===== matmul =====
 *
 * Loop order i-k-j with the inner j loop walking contiguous output and
 * contiguous B row. Under -O3 -ffast-math both Clang and GCC vectorise
 * the inner j loop on AArch64 (NEON) and x86_64 (AVX2/AVX-512). On the
 * fallback path (CPU only), this is the kernel that runs.
 *
 * Output is zeroed inside the kernel — callers do NOT pre-zero c.
 */

void klex_tensor_matmul_f32(float* c, const float* a, const float* b,
                            size_t m, size_t k, size_t n) {
    for (size_t i = 0; i < m * n; i++) {
        c[i] = 0.0f;
    }
    for (size_t i = 0; i < m; i++) {
        float* crow = c + i * n;
        const float* arow = a + i * k;
        for (size_t p = 0; p < k; p++) {
            float aip = arow[p];
            const float* brow = b + p * n;
            for (size_t j = 0; j < n; j++) {
                crow[j] += aip * brow[j];
            }
        }
    }
}

void klex_tensor_matmul_f64(double* c, const double* a, const double* b,
                            size_t m, size_t k, size_t n) {
    for (size_t i = 0; i < m * n; i++) {
        c[i] = 0.0;
    }
    for (size_t i = 0; i < m; i++) {
        double* crow = c + i * n;
        const double* arow = a + i * k;
        for (size_t p = 0; p < k; p++) {
            double aip = arow[p];
            const double* brow = b + p * n;
            for (size_t j = 0; j < n; j++) {
                crow[j] += aip * brow[j];
            }
        }
    }
}

void klex_tensor_matmul_i64(int64_t* c, const int64_t* a, const int64_t* b,
                            size_t m, size_t k, size_t n) {
    for (size_t i = 0; i < m * n; i++) {
        c[i] = 0;
    }
    for (size_t i = 0; i < m; i++) {
        int64_t* crow = c + i * n;
        const int64_t* arow = a + i * k;
        for (size_t p = 0; p < k; p++) {
            int64_t aip = arow[p];
            const int64_t* brow = b + p * n;
            for (size_t j = 0; j < n; j++) {
                crow[j] += aip * brow[j];
            }
        }
    }
}

/* ===== transpose (2-D) =====
 *
 * j outer / i inner: walks input contiguously, writes scattered to
 * out[i*m + j]. Read-side prefetch + write-combining buffers handle
 * this well for the size range FrogPy users hit.
 */

void klex_tensor_transpose2d_f32(float* out, const float* in, size_t m, size_t n) {
    for (size_t j = 0; j < m; j++) {
        const float* inRow = in + j * n;
        for (size_t i = 0; i < n; i++) {
            out[i * m + j] = inRow[i];
        }
    }
}

void klex_tensor_transpose2d_f64(double* out, const double* in, size_t m, size_t n) {
    for (size_t j = 0; j < m; j++) {
        const double* inRow = in + j * n;
        for (size_t i = 0; i < n; i++) {
            out[i * m + j] = inRow[i];
        }
    }
}

void klex_tensor_transpose2d_i64(int64_t* out, const int64_t* in, size_t m, size_t n) {
    for (size_t j = 0; j < m; j++) {
        const int64_t* inRow = in + j * n;
        for (size_t i = 0; i < n; i++) {
            out[i * m + j] = inRow[i];
        }
    }
}

/* ===== dot (1-D inner product) =====
 *
 * Fused multiply + sum in a single pass. Autovec under -O3
 * -ffast-math turns these into NEON / AVX vector reductions.
 */

float klex_tensor_dot_f32(const float* a, const float* b, size_t n) {
    float acc = 0.0f;
    for (size_t i = 0; i < n; i++) {
        acc += a[i] * b[i];
    }
    return acc;
}

double klex_tensor_dot_f64(const double* a, const double* b, size_t n) {
    double acc = 0.0;
    for (size_t i = 0; i < n; i++) {
        acc += a[i] * b[i];
    }
    return acc;
}

int64_t klex_tensor_dot_i64(const int64_t* a, const int64_t* b, size_t n) {
    int64_t acc = 0;
    for (size_t i = 0; i < n; i++) {
        acc += a[i] * b[i];
    }
    return acc;
}

/* ===== axis-aware reductions (N-D) =====
 *
 * Caller flattens the surrounding shape into (prefix, reduceLen, suffix).
 * Input element at flat index (p, r, s) lives at in[(p*reduceLen + r)*suffix + s];
 * output cell out[p*suffix + s] holds the reduction over r in [0, reduceLen).
 *
 * 2-D special cases:
 *   axis 0 on [m, n]  →  prefix=1, reduceLen=m, suffix=n
 *   axis 1 on [m, n]  →  prefix=m, reduceLen=n, suffix=1
 *
 * Two implementation branches per kernel:
 *   suffix == 1 — contiguous segment reduce; tight inner loop over a
 *                 segment of reduceLen contiguous elements per output.
 *   suffix  > 1 — strided reduce; initialise the per-prefix output strip
 *                 from r=0 then fold rows r=1..reduceLen-1 in place. Each
 *                 inner s-loop walks contiguous memory (autovec-friendly).
 *
 * min/max/argmin/argmax assume reduceLen >= 1 (Go-side guarantees this).
 * argmin/argmax break ties to first occurrence.
 */

/* ── sum ── */
void klex_tensor_sum_axis_f32(float* out, const float* in,
                              size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const float* seg = in + p * reduceLen;
            float s = 0.0f;
            for (size_t r = 0; r < reduceLen; r++) s += seg[r];
            out[p] = s;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            float* op = out + p * suffix;
            const float* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = 0.0f;
            for (size_t r = 0; r < reduceLen; r++) {
                const float* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) op[s] += row[s];
            }
        }
    }
}

void klex_tensor_sum_axis_f64(double* out, const double* in,
                              size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const double* seg = in + p * reduceLen;
            double s = 0.0;
            for (size_t r = 0; r < reduceLen; r++) s += seg[r];
            out[p] = s;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            double* op = out + p * suffix;
            const double* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = 0.0;
            for (size_t r = 0; r < reduceLen; r++) {
                const double* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) op[s] += row[s];
            }
        }
    }
}

void klex_tensor_sum_axis_i64(int64_t* out, const int64_t* in,
                              size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const int64_t* seg = in + p * reduceLen;
            int64_t s = 0;
            for (size_t r = 0; r < reduceLen; r++) s += seg[r];
            out[p] = s;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            int64_t* op = out + p * suffix;
            const int64_t* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = 0;
            for (size_t r = 0; r < reduceLen; r++) {
                const int64_t* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) op[s] += row[s];
            }
        }
    }
}

/* ── min ── */
void klex_tensor_min_axis_f32(float* out, const float* in,
                              size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const float* seg = in + p * reduceLen;
            float v = seg[0];
            for (size_t r = 1; r < reduceLen; r++) if (seg[r] < v) v = seg[r];
            out[p] = v;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            float* op = out + p * suffix;
            const float* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = base[s];
            for (size_t r = 1; r < reduceLen; r++) {
                const float* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) if (row[s] < op[s]) op[s] = row[s];
            }
        }
    }
}

void klex_tensor_min_axis_f64(double* out, const double* in,
                              size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const double* seg = in + p * reduceLen;
            double v = seg[0];
            for (size_t r = 1; r < reduceLen; r++) if (seg[r] < v) v = seg[r];
            out[p] = v;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            double* op = out + p * suffix;
            const double* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = base[s];
            for (size_t r = 1; r < reduceLen; r++) {
                const double* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) if (row[s] < op[s]) op[s] = row[s];
            }
        }
    }
}

void klex_tensor_min_axis_i64(int64_t* out, const int64_t* in,
                              size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const int64_t* seg = in + p * reduceLen;
            int64_t v = seg[0];
            for (size_t r = 1; r < reduceLen; r++) if (seg[r] < v) v = seg[r];
            out[p] = v;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            int64_t* op = out + p * suffix;
            const int64_t* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = base[s];
            for (size_t r = 1; r < reduceLen; r++) {
                const int64_t* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) if (row[s] < op[s]) op[s] = row[s];
            }
        }
    }
}

/* ── max ── */
void klex_tensor_max_axis_f32(float* out, const float* in,
                              size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const float* seg = in + p * reduceLen;
            float v = seg[0];
            for (size_t r = 1; r < reduceLen; r++) if (seg[r] > v) v = seg[r];
            out[p] = v;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            float* op = out + p * suffix;
            const float* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = base[s];
            for (size_t r = 1; r < reduceLen; r++) {
                const float* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) if (row[s] > op[s]) op[s] = row[s];
            }
        }
    }
}

void klex_tensor_max_axis_f64(double* out, const double* in,
                              size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const double* seg = in + p * reduceLen;
            double v = seg[0];
            for (size_t r = 1; r < reduceLen; r++) if (seg[r] > v) v = seg[r];
            out[p] = v;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            double* op = out + p * suffix;
            const double* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = base[s];
            for (size_t r = 1; r < reduceLen; r++) {
                const double* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) if (row[s] > op[s]) op[s] = row[s];
            }
        }
    }
}

void klex_tensor_max_axis_i64(int64_t* out, const int64_t* in,
                              size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const int64_t* seg = in + p * reduceLen;
            int64_t v = seg[0];
            for (size_t r = 1; r < reduceLen; r++) if (seg[r] > v) v = seg[r];
            out[p] = v;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            int64_t* op = out + p * suffix;
            const int64_t* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = base[s];
            for (size_t r = 1; r < reduceLen; r++) {
                const int64_t* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) if (row[s] > op[s]) op[s] = row[s];
            }
        }
    }
}

/* ── argmin ──
 * suffix==1: per-segment register-tracked scan (best path).
 * suffix >1: out[p*suffix+s] tracks the best r-index seen so far; on each
 *            new row we re-fetch base[best_r * suffix + s] to compare. One
 *            extra load per cell vs a separate best-values buffer — we
 *            avoid malloc to keep the kernel reentrant. */
void klex_tensor_argmin_axis_f32(int64_t* out, const float* in,
                                 size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const float* seg = in + p * reduceLen;
            int64_t best = 0;
            float bestVal = seg[0];
            for (size_t r = 1; r < reduceLen; r++) {
                if (seg[r] < bestVal) { bestVal = seg[r]; best = (int64_t)r; }
            }
            out[p] = best;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            int64_t* op = out + p * suffix;
            const float* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = 0;
            for (size_t r = 1; r < reduceLen; r++) {
                const float* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) {
                    if (row[s] < base[(size_t)op[s] * suffix + s]) op[s] = (int64_t)r;
                }
            }
        }
    }
}

void klex_tensor_argmin_axis_f64(int64_t* out, const double* in,
                                 size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const double* seg = in + p * reduceLen;
            int64_t best = 0;
            double bestVal = seg[0];
            for (size_t r = 1; r < reduceLen; r++) {
                if (seg[r] < bestVal) { bestVal = seg[r]; best = (int64_t)r; }
            }
            out[p] = best;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            int64_t* op = out + p * suffix;
            const double* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = 0;
            for (size_t r = 1; r < reduceLen; r++) {
                const double* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) {
                    if (row[s] < base[(size_t)op[s] * suffix + s]) op[s] = (int64_t)r;
                }
            }
        }
    }
}

void klex_tensor_argmin_axis_i64(int64_t* out, const int64_t* in,
                                 size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const int64_t* seg = in + p * reduceLen;
            int64_t best = 0;
            int64_t bestVal = seg[0];
            for (size_t r = 1; r < reduceLen; r++) {
                if (seg[r] < bestVal) { bestVal = seg[r]; best = (int64_t)r; }
            }
            out[p] = best;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            int64_t* op = out + p * suffix;
            const int64_t* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = 0;
            for (size_t r = 1; r < reduceLen; r++) {
                const int64_t* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) {
                    if (row[s] < base[(size_t)op[s] * suffix + s]) op[s] = (int64_t)r;
                }
            }
        }
    }
}

/* ── argmax ── */
void klex_tensor_argmax_axis_f32(int64_t* out, const float* in,
                                 size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const float* seg = in + p * reduceLen;
            int64_t best = 0;
            float bestVal = seg[0];
            for (size_t r = 1; r < reduceLen; r++) {
                if (seg[r] > bestVal) { bestVal = seg[r]; best = (int64_t)r; }
            }
            out[p] = best;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            int64_t* op = out + p * suffix;
            const float* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = 0;
            for (size_t r = 1; r < reduceLen; r++) {
                const float* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) {
                    if (row[s] > base[(size_t)op[s] * suffix + s]) op[s] = (int64_t)r;
                }
            }
        }
    }
}

void klex_tensor_argmax_axis_f64(int64_t* out, const double* in,
                                 size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const double* seg = in + p * reduceLen;
            int64_t best = 0;
            double bestVal = seg[0];
            for (size_t r = 1; r < reduceLen; r++) {
                if (seg[r] > bestVal) { bestVal = seg[r]; best = (int64_t)r; }
            }
            out[p] = best;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            int64_t* op = out + p * suffix;
            const double* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = 0;
            for (size_t r = 1; r < reduceLen; r++) {
                const double* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) {
                    if (row[s] > base[(size_t)op[s] * suffix + s]) op[s] = (int64_t)r;
                }
            }
        }
    }
}

void klex_tensor_argmax_axis_i64(int64_t* out, const int64_t* in,
                                 size_t prefix, size_t reduceLen, size_t suffix) {
    if (suffix == 1) {
        for (size_t p = 0; p < prefix; p++) {
            const int64_t* seg = in + p * reduceLen;
            int64_t best = 0;
            int64_t bestVal = seg[0];
            for (size_t r = 1; r < reduceLen; r++) {
                if (seg[r] > bestVal) { bestVal = seg[r]; best = (int64_t)r; }
            }
            out[p] = best;
        }
    } else {
        for (size_t p = 0; p < prefix; p++) {
            int64_t* op = out + p * suffix;
            const int64_t* base = in + p * reduceLen * suffix;
            for (size_t s = 0; s < suffix; s++) op[s] = 0;
            for (size_t r = 1; r < reduceLen; r++) {
                const int64_t* row = base + r * suffix;
                for (size_t s = 0; s < suffix; s++) {
                    if (row[s] > base[(size_t)op[s] * suffix + s]) op[s] = (int64_t)r;
                }
            }
        }
    }
}

/* ===== clip ===== */

void klex_tensor_clip_f32(float* out, const float* a, float lo, float hi, size_t n) {
    for (size_t i = 0; i < n; i++) {
        float v = a[i];
        if (v < lo) v = lo;
        else if (v > hi) v = hi;
        out[i] = v;
    }
}

void klex_tensor_clip_f64(double* out, const double* a, double lo, double hi, size_t n) {
    for (size_t i = 0; i < n; i++) {
        double v = a[i];
        if (v < lo) v = lo;
        else if (v > hi) v = hi;
        out[i] = v;
    }
}

void klex_tensor_clip_i64(int64_t* out, const int64_t* a, int64_t lo, int64_t hi, size_t n) {
    for (size_t i = 0; i < n; i++) {
        int64_t v = a[i];
        if (v < lo) v = lo;
        else if (v > hi) v = hi;
        out[i] = v;
    }
}

/* ===== element-wise comparisons =====
 *
 * Output is int64_t 0/1. NaN comparisons follow IEEE 754: < > <= >= ==
 * all return 0 for NaN inputs; != returns 1 (NaN != NaN is true). */

void klex_tensor_eq_f32(int64_t* out, const float* a, const float* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] == b[i]) ? 1 : 0; }
}
void klex_tensor_eq_f64(int64_t* out, const double* a, const double* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] == b[i]) ? 1 : 0; }
}
void klex_tensor_eq_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] == b[i]) ? 1 : 0; }
}

void klex_tensor_ne_f32(int64_t* out, const float* a, const float* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] != b[i]) ? 1 : 0; }
}
void klex_tensor_ne_f64(int64_t* out, const double* a, const double* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] != b[i]) ? 1 : 0; }
}
void klex_tensor_ne_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] != b[i]) ? 1 : 0; }
}

void klex_tensor_lt_f32(int64_t* out, const float* a, const float* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] < b[i]) ? 1 : 0; }
}
void klex_tensor_lt_f64(int64_t* out, const double* a, const double* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] < b[i]) ? 1 : 0; }
}
void klex_tensor_lt_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] < b[i]) ? 1 : 0; }
}

void klex_tensor_le_f32(int64_t* out, const float* a, const float* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] <= b[i]) ? 1 : 0; }
}
void klex_tensor_le_f64(int64_t* out, const double* a, const double* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] <= b[i]) ? 1 : 0; }
}
void klex_tensor_le_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] <= b[i]) ? 1 : 0; }
}

void klex_tensor_gt_f32(int64_t* out, const float* a, const float* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] > b[i]) ? 1 : 0; }
}
void klex_tensor_gt_f64(int64_t* out, const double* a, const double* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] > b[i]) ? 1 : 0; }
}
void klex_tensor_gt_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] > b[i]) ? 1 : 0; }
}

void klex_tensor_ge_f32(int64_t* out, const float* a, const float* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] >= b[i]) ? 1 : 0; }
}
void klex_tensor_ge_f64(int64_t* out, const double* a, const double* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] >= b[i]) ? 1 : 0; }
}
void klex_tensor_ge_i64(int64_t* out, const int64_t* a, const int64_t* b, size_t n) {
    for (size_t i = 0; i < n; i++) { out[i] = (a[i] >= b[i]) ? 1 : 0; }
}
