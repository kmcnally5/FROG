//go:build darwin || linux

package eval

// tensor_compute_cgo.go — cgo wrappers around the C kernels in
// eval/native/tensor_kernels.c. Build tag restricts to darwin and
// linux for v1; Windows users get the stub in tensor_compute_stub.go.
//
// Why a separate wrapper file
//
// The kLex codebase has historically been zero-cgo outside of the
// macOS-only MTL framework. Putting all the tensor cgo bindings in
// ONE file means there's a single place a maintainer reads to see
// the C-side surface. Each Go wrapper is a tight one-liner that
// converts the slice pointers and forwards the call — the kernel
// signatures stay obvious.
//
// Memory safety
//
// Every wrapper takes Go slices, passes pointers to the first
// element, and lets cgo manage the lifetime. Because the kernel
// signatures use plain pointers (no callback into Go), there's no
// Go-side pinning needed beyond the slice header staying alive for
// the duration of the call — which Go's escape analysis handles
// (the slice is referenced through the wrapper's parameters).
//
// CGO flags
//
//   -O3            — full optimisation
//   -ffast-math    — FP reassociation; commutative kernels only
//   -std=c99       — explicit ANSI C, no GNU extensions
//
// We deliberately don't set -march=native because that would
// produce binaries that fail on machines older than the build
// host. SSE2 / NEON baseline (set by default in clang/gcc for
// modern targets) is enough for the autovec wins we care about.

/*
#cgo CFLAGS: -O3 -ffast-math -std=c99
#cgo linux LDFLAGS: -lm
#include "tensor_kernels.h"
*/
import "C"

import (
	"runtime"
	"sync"
	"unsafe"
)

// tensorAddF32 wraps klex_tensor_add_f32. Pre-conditions enforced
// by the caller (Go-side builtin): out/a/b are all non-nil slices
// of the same length. The kernel handles len(out)==0 trivially
// (loop bound is 0).
func tensorAddF32(out, a, b []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_add_f32(
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&a[0])),
		(*C.float)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

func tensorAddF64(out, a, b []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_add_f64(
		(*C.double)(unsafe.Pointer(&out[0])),
		(*C.double)(unsafe.Pointer(&a[0])),
		(*C.double)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

func tensorAddI64(out, a, b []int64) {
	n := len(out)
	if n == 0 {
		return
	}
	// Go's int64 is C's int64_t (matching ABI on all supported
	// platforms). Pointer cast is sound because of size + layout
	// guarantees.
	C.klex_tensor_add_i64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.int64_t)(unsafe.Pointer(&a[0])),
		(*C.int64_t)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

// ── sub ──

func tensorSubF32(out, a, b []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_sub_f32(
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&a[0])),
		(*C.float)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

func tensorSubF64(out, a, b []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_sub_f64(
		(*C.double)(unsafe.Pointer(&out[0])),
		(*C.double)(unsafe.Pointer(&a[0])),
		(*C.double)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

func tensorSubI64(out, a, b []int64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_sub_i64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.int64_t)(unsafe.Pointer(&a[0])),
		(*C.int64_t)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

// ── mul ──

func tensorMulF32(out, a, b []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_mul_f32(
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&a[0])),
		(*C.float)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

func tensorMulF64(out, a, b []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_mul_f64(
		(*C.double)(unsafe.Pointer(&out[0])),
		(*C.double)(unsafe.Pointer(&a[0])),
		(*C.double)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

func tensorMulI64(out, a, b []int64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_mul_i64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.int64_t)(unsafe.Pointer(&a[0])),
		(*C.int64_t)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

// ── div ──
//
// Float kernels rely on IEEE 754 (n/0 = ±Inf, 0/0 = NaN). The i64
// wrapper here matches the C signature; the zero-check for i64
// happens in builtins_tensor.go's div builtin BEFORE calling this.

func tensorDivF32(out, a, b []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_div_f32(
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&a[0])),
		(*C.float)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

func tensorDivF64(out, a, b []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_div_f64(
		(*C.double)(unsafe.Pointer(&out[0])),
		(*C.double)(unsafe.Pointer(&a[0])),
		(*C.double)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

func tensorDivI64(out, a, b []int64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_div_i64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.int64_t)(unsafe.Pointer(&a[0])),
		(*C.int64_t)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

// ── pow ──
//
// Float kernels go through libm (powf/pow). Integer kernel uses
// exponentiation-by-squaring and requires b[i] >= 0 — pre-checked
// in the _tensor_pow builtin via its binaryKernel.preCheck.

func tensorPowF32(out, a, b []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_pow_f32(
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&a[0])),
		(*C.float)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

func tensorPowF64(out, a, b []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_pow_f64(
		(*C.double)(unsafe.Pointer(&out[0])),
		(*C.double)(unsafe.Pointer(&a[0])),
		(*C.double)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

func tensorPowI64(out, a, b []int64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_pow_i64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.int64_t)(unsafe.Pointer(&a[0])),
		(*C.int64_t)(unsafe.Pointer(&b[0])),
		C.size_t(n),
	)
}

// ── unary wrappers ──
//
// Each takes input + output slices. Float-only ops (exp, log, sqrt,
// sin, cos) have no i64 variant — the elementWiseUnary helper
// surfaces a clean "not supported for i64" error at the builtin
// layer when needed.

func tensorNegF32(out, a []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_neg_f32((*C.float)(unsafe.Pointer(&out[0])), (*C.float)(unsafe.Pointer(&a[0])), C.size_t(n))
}
func tensorNegF64(out, a []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_neg_f64((*C.double)(unsafe.Pointer(&out[0])), (*C.double)(unsafe.Pointer(&a[0])), C.size_t(n))
}
func tensorNegI64(out, a []int64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_neg_i64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.int64_t)(unsafe.Pointer(&a[0])), C.size_t(n))
}

func tensorAbsF32(out, a []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_abs_f32((*C.float)(unsafe.Pointer(&out[0])), (*C.float)(unsafe.Pointer(&a[0])), C.size_t(n))
}
func tensorAbsF64(out, a []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_abs_f64((*C.double)(unsafe.Pointer(&out[0])), (*C.double)(unsafe.Pointer(&a[0])), C.size_t(n))
}
func tensorAbsI64(out, a []int64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_abs_i64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.int64_t)(unsafe.Pointer(&a[0])), C.size_t(n))
}

func tensorExpF32(out, a []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_exp_f32((*C.float)(unsafe.Pointer(&out[0])), (*C.float)(unsafe.Pointer(&a[0])), C.size_t(n))
}
func tensorExpF64(out, a []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_exp_f64((*C.double)(unsafe.Pointer(&out[0])), (*C.double)(unsafe.Pointer(&a[0])), C.size_t(n))
}

func tensorLogF32(out, a []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_log_f32((*C.float)(unsafe.Pointer(&out[0])), (*C.float)(unsafe.Pointer(&a[0])), C.size_t(n))
}
func tensorLogF64(out, a []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_log_f64((*C.double)(unsafe.Pointer(&out[0])), (*C.double)(unsafe.Pointer(&a[0])), C.size_t(n))
}

func tensorSqrtF32(out, a []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_sqrt_f32((*C.float)(unsafe.Pointer(&out[0])), (*C.float)(unsafe.Pointer(&a[0])), C.size_t(n))
}
func tensorSqrtF64(out, a []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_sqrt_f64((*C.double)(unsafe.Pointer(&out[0])), (*C.double)(unsafe.Pointer(&a[0])), C.size_t(n))
}

func tensorSinF32(out, a []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_sin_f32((*C.float)(unsafe.Pointer(&out[0])), (*C.float)(unsafe.Pointer(&a[0])), C.size_t(n))
}
func tensorSinF64(out, a []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_sin_f64((*C.double)(unsafe.Pointer(&out[0])), (*C.double)(unsafe.Pointer(&a[0])), C.size_t(n))
}

func tensorCosF32(out, a []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_cos_f32((*C.float)(unsafe.Pointer(&out[0])), (*C.float)(unsafe.Pointer(&a[0])), C.size_t(n))
}
func tensorCosF64(out, a []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_cos_f64((*C.double)(unsafe.Pointer(&out[0])), (*C.double)(unsafe.Pointer(&a[0])), C.size_t(n))
}

// ── reduction wrappers ──
//
// Each takes a single input slice and returns a scalar. Sum on an
// empty slice returns the zero value (consistent with NumPy
// np.sum([])); min/max/argmin/argmax on an empty slice would be
// undefined behaviour in C and the builtin layer rejects them
// before getting here.

func tensorSumF32(a []float32) float32 {
	if len(a) == 0 {
		return 0
	}
	return float32(C.klex_tensor_sum_f32((*C.float)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}
func tensorSumF64(a []float64) float64 {
	if len(a) == 0 {
		return 0
	}
	return float64(C.klex_tensor_sum_f64((*C.double)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}
func tensorSumI64(a []int64) int64 {
	if len(a) == 0 {
		return 0
	}
	return int64(C.klex_tensor_sum_i64((*C.int64_t)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}

func tensorMinF32(a []float32) float32 {
	return float32(C.klex_tensor_min_f32((*C.float)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}
func tensorMinF64(a []float64) float64 {
	return float64(C.klex_tensor_min_f64((*C.double)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}
func tensorMinI64(a []int64) int64 {
	return int64(C.klex_tensor_min_i64((*C.int64_t)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}

func tensorMaxF32(a []float32) float32 {
	return float32(C.klex_tensor_max_f32((*C.float)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}
func tensorMaxF64(a []float64) float64 {
	return float64(C.klex_tensor_max_f64((*C.double)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}
func tensorMaxI64(a []int64) int64 {
	return int64(C.klex_tensor_max_i64((*C.int64_t)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}

func tensorArgminF32(a []float32) int {
	return int(C.klex_tensor_argmin_f32((*C.float)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}
func tensorArgminF64(a []float64) int {
	return int(C.klex_tensor_argmin_f64((*C.double)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}
func tensorArgminI64(a []int64) int {
	return int(C.klex_tensor_argmin_i64((*C.int64_t)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}

func tensorArgmaxF32(a []float32) int {
	return int(C.klex_tensor_argmax_f32((*C.float)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}
func tensorArgmaxF64(a []float64) int {
	return int(C.klex_tensor_argmax_f64((*C.double)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}
func tensorArgmaxI64(a []int64) int {
	return int(C.klex_tensor_argmax_i64((*C.int64_t)(unsafe.Pointer(&a[0])), C.size_t(len(a))))
}

// ── matmul wrappers ──
//
// CPU fallback for matmul. On Apple Silicon the F32 path is normally
// serviced by MPS (eval/tensor_matmul_mps_darwin.go); these kernels
// run when MPS is unavailable (Linux) or when dtype is F64 / I64.
//
// Architecture:
//
//   tensorMatmul{F32,F64,I64}       — public dispatchers; row-strip
//                                     parallelism above the threshold,
//                                     single-strip below.
//   tensorMatmul{F32,F64,I64}Strip  — single-threaded cgo wrapper.
//                                     Computes one row-strip of the
//                                     output (the strip's m rows are
//                                     parameter `m`). Called either
//                                     directly (small matrices) or
//                                     from worker goroutines (large).
//
// All three slices are caller-owned. The C kernels zero the output
// buffer themselves — callers do NOT pre-zero. Shape validation
// lives in the builtin layer; getting here means the slices are
// already the right size for the parameters.

// matmulParallelThreshold is the minimum `m` (output rows) at which
// row-strip parallelism is profitable. Below this the goroutine
// spawn + sync overhead exceeds the kernel work — measured at ~16 µs
// of overhead per goroutine, while a 64×k×n matmul does roughly
// 64·k·n·2 FLOPs which at ~17 GF/s for f64 takes only a few µs at
// k=n=64. Threshold of 128 keeps the parallel path comfortably above
// breakeven on all dtypes.
const matmulParallelThreshold = 128

// matmulParallelStrips splits a logical m-row matmul into worker
// strips and invokes `work` for each strip with its row range. Used
// by tensorMatmul{F32,F64,I64} below; the closure handles the
// dtype-specific cgo call on its row slice.
//
// Workers = min(NumCPU, m) so we never spawn more than one goroutine
// per row. On Apple Silicon NumCPU returns the total core count
// (P+E); the OS scheduler decides which type runs each goroutine and
// for FP-heavy work tends to park them on P-cores.
func matmulParallelStrips(m int, work func(rowStart, rowEnd int)) {
	workers := runtime.NumCPU()
	if workers > m {
		workers = m
	}
	rowsPerWorker := m / workers
	extra := m % workers
	var wg sync.WaitGroup
	rs := 0
	for w := 0; w < workers; w++ {
		rows := rowsPerWorker
		if w < extra {
			rows++
		}
		if rows == 0 {
			continue
		}
		re := rs + rows
		wg.Add(1)
		go func(s, e int) {
			defer wg.Done()
			work(s, e)
		}(rs, re)
		rs = re
	}
	wg.Wait()
}

func tensorMatmulF32(out, a, b []float32, m, k, n int) {
	if m == 0 || k == 0 || n == 0 {
		return
	}
	if m < matmulParallelThreshold {
		tensorMatmulF32Strip(out, a, b, m, k, n)
		return
	}
	matmulParallelStrips(m, func(rs, re int) {
		rows := re - rs
		tensorMatmulF32Strip(
			out[rs*n:re*n],
			a[rs*k:re*k],
			b,
			rows, k, n,
		)
	})
}

func tensorMatmulF64(out, a, b []float64, m, k, n int) {
	if m == 0 || k == 0 || n == 0 {
		return
	}
	if m < matmulParallelThreshold {
		tensorMatmulF64Strip(out, a, b, m, k, n)
		return
	}
	matmulParallelStrips(m, func(rs, re int) {
		rows := re - rs
		tensorMatmulF64Strip(
			out[rs*n:re*n],
			a[rs*k:re*k],
			b,
			rows, k, n,
		)
	})
}

func tensorMatmulI64(out, a, b []int64, m, k, n int) {
	if m == 0 || k == 0 || n == 0 {
		return
	}
	if m < matmulParallelThreshold {
		tensorMatmulI64Strip(out, a, b, m, k, n)
		return
	}
	matmulParallelStrips(m, func(rs, re int) {
		rows := re - rs
		tensorMatmulI64Strip(
			out[rs*n:re*n],
			a[rs*k:re*k],
			b,
			rows, k, n,
		)
	})
}

// ── single-strip cgo calls (the actual kernel invocations) ──

func tensorMatmulF32Strip(out, a, b []float32, m, k, n int) {
	if m == 0 || k == 0 || n == 0 {
		return
	}
	C.klex_tensor_matmul_f32(
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&a[0])),
		(*C.float)(unsafe.Pointer(&b[0])),
		C.size_t(m), C.size_t(k), C.size_t(n),
	)
}

func tensorMatmulF64Strip(out, a, b []float64, m, k, n int) {
	if m == 0 || k == 0 || n == 0 {
		return
	}
	C.klex_tensor_matmul_f64(
		(*C.double)(unsafe.Pointer(&out[0])),
		(*C.double)(unsafe.Pointer(&a[0])),
		(*C.double)(unsafe.Pointer(&b[0])),
		C.size_t(m), C.size_t(k), C.size_t(n),
	)
}

func tensorMatmulI64Strip(out, a, b []int64, m, k, n int) {
	if m == 0 || k == 0 || n == 0 {
		return
	}
	C.klex_tensor_matmul_i64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.int64_t)(unsafe.Pointer(&a[0])),
		(*C.int64_t)(unsafe.Pointer(&b[0])),
		C.size_t(m), C.size_t(k), C.size_t(n),
	)
}

// ── transpose (2-D) wrappers ──
//
// Single-threaded for now — the cache-friendly inner loop is fast
// enough that a 1024×1024 f64 transpose takes ~3 ms (autovec'd).
// Multi-threading row-strips would help for very large matrices
// but the current bottleneck is the strided write side, not loop
// iteration; parallelism doesn't help that. Deferred.

func tensorTranspose2DF32(out, in []float32, m, n int) {
	if m == 0 || n == 0 {
		return
	}
	C.klex_tensor_transpose2d_f32(
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&in[0])),
		C.size_t(m), C.size_t(n),
	)
}

func tensorTranspose2DF64(out, in []float64, m, n int) {
	if m == 0 || n == 0 {
		return
	}
	C.klex_tensor_transpose2d_f64(
		(*C.double)(unsafe.Pointer(&out[0])),
		(*C.double)(unsafe.Pointer(&in[0])),
		C.size_t(m), C.size_t(n),
	)
}

func tensorTranspose2DI64(out, in []int64, m, n int) {
	if m == 0 || n == 0 {
		return
	}
	C.klex_tensor_transpose2d_i64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.int64_t)(unsafe.Pointer(&in[0])),
		C.size_t(m), C.size_t(n),
	)
}

// ── dot product wrappers ──
//
// Fused mul+sum in one C pass — avoids the temporary tensor that a
// pure-stdlib `sum(mul(a,b))` would allocate. Accumulator types
// match the input dtype (NumPy-style; see tensor_kernels.h).

func tensorDotF32(a, b []float32) float32 {
	if len(a) == 0 {
		return 0
	}
	return float32(C.klex_tensor_dot_f32(
		(*C.float)(unsafe.Pointer(&a[0])),
		(*C.float)(unsafe.Pointer(&b[0])),
		C.size_t(len(a)),
	))
}

func tensorDotF64(a, b []float64) float64 {
	if len(a) == 0 {
		return 0
	}
	return float64(C.klex_tensor_dot_f64(
		(*C.double)(unsafe.Pointer(&a[0])),
		(*C.double)(unsafe.Pointer(&b[0])),
		C.size_t(len(a)),
	))
}

func tensorDotI64(a, b []int64) int64 {
	if len(a) == 0 {
		return 0
	}
	return int64(C.klex_tensor_dot_i64(
		(*C.int64_t)(unsafe.Pointer(&a[0])),
		(*C.int64_t)(unsafe.Pointer(&b[0])),
		C.size_t(len(a)),
	))
}

// ── axis-aware reductions (N-D, single-axis collapse) ──
//
// 15 wrappers covering (sum / min / max / argmin / argmax) × 3 dtypes.
// Mean is computed in Go as `sum_axis / reduce_size` so no dedicated
// kernel — same precision policy as scalar mean. argmin/argmax write
// into int64 output regardless of input dtype.
//
// Caller flattens the surrounding tensor shape into (prefix, reduceLen,
// suffix) — see tensor_kernels.h for the contract.

func tensorSumAxisF32(out, in []float32, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_sum_axis_f32(
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}
func tensorSumAxisF64(out, in []float64, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_sum_axis_f64(
		(*C.double)(unsafe.Pointer(&out[0])),
		(*C.double)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}
func tensorSumAxisI64(out, in []int64, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_sum_axis_i64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.int64_t)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}

func tensorMinAxisF32(out, in []float32, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_min_axis_f32(
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}
func tensorMinAxisF64(out, in []float64, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_min_axis_f64(
		(*C.double)(unsafe.Pointer(&out[0])),
		(*C.double)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}
func tensorMinAxisI64(out, in []int64, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_min_axis_i64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.int64_t)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}

func tensorMaxAxisF32(out, in []float32, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_max_axis_f32(
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}
func tensorMaxAxisF64(out, in []float64, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_max_axis_f64(
		(*C.double)(unsafe.Pointer(&out[0])),
		(*C.double)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}
func tensorMaxAxisI64(out, in []int64, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_max_axis_i64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.int64_t)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}

func tensorArgminAxisF32(out []int64, in []float32, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_argmin_axis_f32(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}
func tensorArgminAxisF64(out []int64, in []float64, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_argmin_axis_f64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.double)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}
func tensorArgminAxisI64(out, in []int64, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_argmin_axis_i64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.int64_t)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}

func tensorArgmaxAxisF32(out []int64, in []float32, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_argmax_axis_f32(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}
func tensorArgmaxAxisF64(out []int64, in []float64, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_argmax_axis_f64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.double)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}
func tensorArgmaxAxisI64(out, in []int64, prefix, reduceLen, suffix int) {
	if prefix == 0 || reduceLen == 0 || suffix == 0 {
		return
	}
	C.klex_tensor_argmax_axis_i64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.int64_t)(unsafe.Pointer(&in[0])),
		C.size_t(prefix), C.size_t(reduceLen), C.size_t(suffix),
	)
}

// tensorComputeAvailable reports whether tensor ops are supported
// on this build. Used by the tensor builtins for a clean error
// path. Always true under the cgo build tag; the stub file's
// matching false definition kicks in on Windows.
// ── clip wrappers ──
//
// Scalar lo/hi are passed by value — cgo handles the C scalar parameter
// convention for float/double/int64_t without any pointer indirection.

func tensorClipF32(out, a []float32, lo, hi float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_clip_f32(
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&a[0])),
		C.float(lo), C.float(hi),
		C.size_t(n),
	)
}

func tensorClipF64(out, a []float64, lo, hi float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_clip_f64(
		(*C.double)(unsafe.Pointer(&out[0])),
		(*C.double)(unsafe.Pointer(&a[0])),
		C.double(lo), C.double(hi),
		C.size_t(n),
	)
}

func tensorClipI64(out, a []int64, lo, hi int64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_clip_i64(
		(*C.int64_t)(unsafe.Pointer(&out[0])),
		(*C.int64_t)(unsafe.Pointer(&a[0])),
		C.int64_t(lo), C.int64_t(hi),
		C.size_t(n),
	)
}

// ── comparison wrappers ──
//
// Output is always []int64 (0 or 1), inputs are the source dtype.
// Because out and a/b have different element sizes they cannot alias.

func tensorEqF32(out []int64, a, b []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_eq_f32((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.float)(unsafe.Pointer(&a[0])), (*C.float)(unsafe.Pointer(&b[0])), C.size_t(n))
}
func tensorEqF64(out []int64, a, b []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_eq_f64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.double)(unsafe.Pointer(&a[0])), (*C.double)(unsafe.Pointer(&b[0])), C.size_t(n))
}
func tensorEqI64(out, a, b []int64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_eq_i64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.int64_t)(unsafe.Pointer(&a[0])), (*C.int64_t)(unsafe.Pointer(&b[0])), C.size_t(n))
}

func tensorNeF32(out []int64, a, b []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_ne_f32((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.float)(unsafe.Pointer(&a[0])), (*C.float)(unsafe.Pointer(&b[0])), C.size_t(n))
}
func tensorNeF64(out []int64, a, b []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_ne_f64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.double)(unsafe.Pointer(&a[0])), (*C.double)(unsafe.Pointer(&b[0])), C.size_t(n))
}
func tensorNeI64(out, a, b []int64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_ne_i64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.int64_t)(unsafe.Pointer(&a[0])), (*C.int64_t)(unsafe.Pointer(&b[0])), C.size_t(n))
}

func tensorLtF32(out []int64, a, b []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_lt_f32((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.float)(unsafe.Pointer(&a[0])), (*C.float)(unsafe.Pointer(&b[0])), C.size_t(n))
}
func tensorLtF64(out []int64, a, b []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_lt_f64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.double)(unsafe.Pointer(&a[0])), (*C.double)(unsafe.Pointer(&b[0])), C.size_t(n))
}
func tensorLtI64(out, a, b []int64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_lt_i64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.int64_t)(unsafe.Pointer(&a[0])), (*C.int64_t)(unsafe.Pointer(&b[0])), C.size_t(n))
}

func tensorLeF32(out []int64, a, b []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_le_f32((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.float)(unsafe.Pointer(&a[0])), (*C.float)(unsafe.Pointer(&b[0])), C.size_t(n))
}
func tensorLeF64(out []int64, a, b []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_le_f64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.double)(unsafe.Pointer(&a[0])), (*C.double)(unsafe.Pointer(&b[0])), C.size_t(n))
}
func tensorLeI64(out, a, b []int64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_le_i64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.int64_t)(unsafe.Pointer(&a[0])), (*C.int64_t)(unsafe.Pointer(&b[0])), C.size_t(n))
}

func tensorGtF32(out []int64, a, b []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_gt_f32((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.float)(unsafe.Pointer(&a[0])), (*C.float)(unsafe.Pointer(&b[0])), C.size_t(n))
}
func tensorGtF64(out []int64, a, b []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_gt_f64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.double)(unsafe.Pointer(&a[0])), (*C.double)(unsafe.Pointer(&b[0])), C.size_t(n))
}
func tensorGtI64(out, a, b []int64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_gt_i64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.int64_t)(unsafe.Pointer(&a[0])), (*C.int64_t)(unsafe.Pointer(&b[0])), C.size_t(n))
}

func tensorGeF32(out []int64, a, b []float32) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_ge_f32((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.float)(unsafe.Pointer(&a[0])), (*C.float)(unsafe.Pointer(&b[0])), C.size_t(n))
}
func tensorGeF64(out []int64, a, b []float64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_ge_f64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.double)(unsafe.Pointer(&a[0])), (*C.double)(unsafe.Pointer(&b[0])), C.size_t(n))
}
func tensorGeI64(out, a, b []int64) {
	n := len(out)
	if n == 0 {
		return
	}
	C.klex_tensor_ge_i64((*C.int64_t)(unsafe.Pointer(&out[0])), (*C.int64_t)(unsafe.Pointer(&a[0])), (*C.int64_t)(unsafe.Pointer(&b[0])), C.size_t(n))
}

func tensorComputeAvailable() bool { return true }
