//go:build darwin || linux

package eval

// tensor_diag_test.go — FrogPy probe #3: isolate where time is spent.
//
// Three benchmarks at the same tensor size (1M f64 elements) so the
// kernel work is identical. Differences expose the cost of each layer.
//
//   BenchmarkKernelOnly       — direct call to tensorAddF64 with a
//                                pre-allocated output buffer reused
//                                across iterations. Measures the
//                                cgo call + the C kernel only.
//
//   BenchmarkKernelPlusAlloc  — call tensorAddF64 with a fresh
//                                make([]float64, n) each iteration.
//                                The DIFFERENCE between this and
//                                KernelOnly is the cost of Go's
//                                zero-initialised allocator.
//
//   BenchmarkFullPath         — call newTensorFromShape + kernel
//                                (mirrors elementWiseBinary's hot
//                                path minus the validation). The
//                                DIFFERENCE vs KernelPlusAlloc is
//                                the Tensor header allocation +
//                                slice header indirection.
//
// Run with:
//   go test -bench=. -benchmem -benchtime=3s -run=^$ ./eval/...
//
// Interpretation cheat-sheet (M4):
//   - If KernelOnly is fast and KernelPlusAlloc is 2-3× slower, the
//     output zero-init is the main villain. Fix: arena / sync.Pool /
//     in-place ops.
//   - If FullPath ≈ KernelPlusAlloc, the Tensor struct overhead is
//     negligible (expected).
//   - If KernelOnly itself is well below 100 GB/s on 1M elements,
//     we ARE kernel-bound after all and need restrict / hand-NEON.

import "testing"

const benchN = 1_000_000

func makeBenchInputs() (a, b []float64) {
	a = make([]float64, benchN)
	b = make([]float64, benchN)
	for i := range a {
		a[i] = 1.5
		b[i] = 2.5
	}
	return
}

func BenchmarkKernelOnly(bm *testing.B) {
	a, b := makeBenchInputs()
	out := make([]float64, benchN)
	bm.SetBytes(int64(benchN * 8 * 3)) // 3 buffers touched
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		tensorAddF64(out, a, b)
	}
}

func BenchmarkKernelPlusAlloc(bm *testing.B) {
	a, b := makeBenchInputs()
	bm.SetBytes(int64(benchN * 8 * 3))
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		out := make([]float64, benchN) // zero-init every iteration
		tensorAddF64(out, a, b)
		_ = out
	}
}

func BenchmarkFullPath(bm *testing.B) {
	a, b := makeBenchInputs()
	shape := []int{benchN}
	bm.SetBytes(int64(benchN * 8 * 3))
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		out := newTensorFromShape(DTypeFloat64, shape)
		tensorAddF64(out.F64, a, b)
		_ = out
	}
}

// Same trio at a SMALL size where cgo overhead dominates.
// If the gap closes here, cgo per-call cost is the floor.

const benchSmallN = 1_000

func BenchmarkKernelOnlySmall(bm *testing.B) {
	a := make([]float64, benchSmallN)
	b := make([]float64, benchSmallN)
	out := make([]float64, benchSmallN)
	bm.SetBytes(int64(benchSmallN * 8 * 3))
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		tensorAddF64(out, a, b)
	}
}

func BenchmarkFullPathSmall(bm *testing.B) {
	a := make([]float64, benchSmallN)
	b := make([]float64, benchSmallN)
	shape := []int{benchSmallN}
	bm.SetBytes(int64(benchSmallN * 8 * 3))
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		out := newTensorFromShape(DTypeFloat64, shape)
		tensorAddF64(out.F64, a, b)
		_ = out
	}
}
