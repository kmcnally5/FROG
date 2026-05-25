//go:build !darwin && !linux

package eval

// tensor_compute_stub.go — pure-Go fallback for platforms (Windows,
// today) where the cgo-backed kernels in tensor_compute_cgo.go aren't
// compiled. Every wrapper here panics, which the tensor builtins
// guard against via tensorComputeAvailable() before calling.
//
// Why panic rather than no-op or pure-Go-loop
//
// kLex's identity on Windows has been "the same language with the
// same semantics, just maybe slower". Silently doing nothing would
// produce wrong tensor results; a pure-Go loop here would slow
// Windows tensor ops 5-10× vs Mac/Linux without making it obvious.
// Panicking is the loud failure mode — tensor builtins detect the
// platform via tensorComputeAvailable() and return a kLex *Error
// with a clean "tensor ops require macOS or Linux in v1" message.

func tensorAddF32(out, a, b []float32) {
	panic("tensorAddF32: native kernels not built on this platform")
}

func tensorAddF64(out, a, b []float64) {
	panic("tensorAddF64: native kernels not built on this platform")
}

func tensorAddI64(out, a, b []int64) {
	panic("tensorAddI64: native kernels not built on this platform")
}

func tensorSubF32(out, a, b []float32) {
	panic("tensorSubF32: native kernels not built on this platform")
}
func tensorSubF64(out, a, b []float64) {
	panic("tensorSubF64: native kernels not built on this platform")
}
func tensorSubI64(out, a, b []int64) {
	panic("tensorSubI64: native kernels not built on this platform")
}

func tensorMulF32(out, a, b []float32) {
	panic("tensorMulF32: native kernels not built on this platform")
}
func tensorMulF64(out, a, b []float64) {
	panic("tensorMulF64: native kernels not built on this platform")
}
func tensorMulI64(out, a, b []int64) {
	panic("tensorMulI64: native kernels not built on this platform")
}

func tensorDivF32(out, a, b []float32) {
	panic("tensorDivF32: native kernels not built on this platform")
}
func tensorDivF64(out, a, b []float64) {
	panic("tensorDivF64: native kernels not built on this platform")
}
func tensorDivI64(out, a, b []int64) {
	panic("tensorDivI64: native kernels not built on this platform")
}

func tensorPowF32(out, a, b []float32) {
	panic("tensorPowF32: native kernels not built on this platform")
}
func tensorPowF64(out, a, b []float64) {
	panic("tensorPowF64: native kernels not built on this platform")
}
func tensorPowI64(out, a, b []int64) {
	panic("tensorPowI64: native kernels not built on this platform")
}

func tensorNegF32(out, a []float32) {
	panic("tensorNegF32: native kernels not built on this platform")
}
func tensorNegF64(out, a []float64) {
	panic("tensorNegF64: native kernels not built on this platform")
}
func tensorNegI64(out, a []int64) {
	panic("tensorNegI64: native kernels not built on this platform")
}

func tensorAbsF32(out, a []float32) {
	panic("tensorAbsF32: native kernels not built on this platform")
}
func tensorAbsF64(out, a []float64) {
	panic("tensorAbsF64: native kernels not built on this platform")
}
func tensorAbsI64(out, a []int64) {
	panic("tensorAbsI64: native kernels not built on this platform")
}

func tensorExpF32(out, a []float32) {
	panic("tensorExpF32: native kernels not built on this platform")
}
func tensorExpF64(out, a []float64) {
	panic("tensorExpF64: native kernels not built on this platform")
}

func tensorLogF32(out, a []float32) {
	panic("tensorLogF32: native kernels not built on this platform")
}
func tensorLogF64(out, a []float64) {
	panic("tensorLogF64: native kernels not built on this platform")
}

func tensorSqrtF32(out, a []float32) {
	panic("tensorSqrtF32: native kernels not built on this platform")
}
func tensorSqrtF64(out, a []float64) {
	panic("tensorSqrtF64: native kernels not built on this platform")
}

func tensorSinF32(out, a []float32) {
	panic("tensorSinF32: native kernels not built on this platform")
}
func tensorSinF64(out, a []float64) {
	panic("tensorSinF64: native kernels not built on this platform")
}

func tensorCosF32(out, a []float32) {
	panic("tensorCosF32: native kernels not built on this platform")
}
func tensorCosF64(out, a []float64) {
	panic("tensorCosF64: native kernels not built on this platform")
}

// ── reduction stubs ──

func tensorSumF32(a []float32) float32 {
	panic("tensorSumF32: native kernels not built on this platform")
}
func tensorSumF64(a []float64) float64 {
	panic("tensorSumF64: native kernels not built on this platform")
}
func tensorSumI64(a []int64) int64 {
	panic("tensorSumI64: native kernels not built on this platform")
}

func tensorMinF32(a []float32) float32 {
	panic("tensorMinF32: native kernels not built on this platform")
}
func tensorMinF64(a []float64) float64 {
	panic("tensorMinF64: native kernels not built on this platform")
}
func tensorMinI64(a []int64) int64 {
	panic("tensorMinI64: native kernels not built on this platform")
}

func tensorMaxF32(a []float32) float32 {
	panic("tensorMaxF32: native kernels not built on this platform")
}
func tensorMaxF64(a []float64) float64 {
	panic("tensorMaxF64: native kernels not built on this platform")
}
func tensorMaxI64(a []int64) int64 {
	panic("tensorMaxI64: native kernels not built on this platform")
}

func tensorArgminF32(a []float32) int {
	panic("tensorArgminF32: native kernels not built on this platform")
}
func tensorArgminF64(a []float64) int {
	panic("tensorArgminF64: native kernels not built on this platform")
}
func tensorArgminI64(a []int64) int {
	panic("tensorArgminI64: native kernels not built on this platform")
}

func tensorArgmaxF32(a []float32) int {
	panic("tensorArgmaxF32: native kernels not built on this platform")
}
func tensorArgmaxF64(a []float64) int {
	panic("tensorArgmaxF64: native kernels not built on this platform")
}
func tensorArgmaxI64(a []int64) int {
	panic("tensorArgmaxI64: native kernels not built on this platform")
}

func tensorMatmulF32(out, a, b []float32, m, k, n int) {
	panic("tensorMatmulF32: native kernels not built on this platform")
}
func tensorMatmulF64(out, a, b []float64, m, k, n int) {
	panic("tensorMatmulF64: native kernels not built on this platform")
}
func tensorMatmulI64(out, a, b []int64, m, k, n int) {
	panic("tensorMatmulI64: native kernels not built on this platform")
}

func tensorTranspose2DF32(out, in []float32, m, n int) {
	panic("tensorTranspose2DF32: native kernels not built on this platform")
}
func tensorTranspose2DF64(out, in []float64, m, n int) {
	panic("tensorTranspose2DF64: native kernels not built on this platform")
}
func tensorTranspose2DI64(out, in []int64, m, n int) {
	panic("tensorTranspose2DI64: native kernels not built on this platform")
}

func tensorDotF32(a, b []float32) float32 {
	panic("tensorDotF32: native kernels not built on this platform")
}
func tensorDotF64(a, b []float64) float64 {
	panic("tensorDotF64: native kernels not built on this platform")
}
func tensorDotI64(a, b []int64) int64 {
	panic("tensorDotI64: native kernels not built on this platform")
}

func tensorSumAxis2DF32(out, in []float32, m, n, axis int) {
	panic("tensorSumAxis2DF32: native kernels not built on this platform")
}
func tensorSumAxis2DF64(out, in []float64, m, n, axis int) {
	panic("tensorSumAxis2DF64: native kernels not built on this platform")
}
func tensorSumAxis2DI64(out, in []int64, m, n, axis int) {
	panic("tensorSumAxis2DI64: native kernels not built on this platform")
}
func tensorMinAxis2DF32(out, in []float32, m, n, axis int) {
	panic("tensorMinAxis2DF32: native kernels not built on this platform")
}
func tensorMinAxis2DF64(out, in []float64, m, n, axis int) {
	panic("tensorMinAxis2DF64: native kernels not built on this platform")
}
func tensorMinAxis2DI64(out, in []int64, m, n, axis int) {
	panic("tensorMinAxis2DI64: native kernels not built on this platform")
}
func tensorMaxAxis2DF32(out, in []float32, m, n, axis int) {
	panic("tensorMaxAxis2DF32: native kernels not built on this platform")
}
func tensorMaxAxis2DF64(out, in []float64, m, n, axis int) {
	panic("tensorMaxAxis2DF64: native kernels not built on this platform")
}
func tensorMaxAxis2DI64(out, in []int64, m, n, axis int) {
	panic("tensorMaxAxis2DI64: native kernels not built on this platform")
}
func tensorArgminAxis2DF32(out []int64, in []float32, m, n, axis int) {
	panic("tensorArgminAxis2DF32: native kernels not built on this platform")
}
func tensorArgminAxis2DF64(out []int64, in []float64, m, n, axis int) {
	panic("tensorArgminAxis2DF64: native kernels not built on this platform")
}
func tensorArgminAxis2DI64(out, in []int64, m, n, axis int) {
	panic("tensorArgminAxis2DI64: native kernels not built on this platform")
}
func tensorArgmaxAxis2DF32(out []int64, in []float32, m, n, axis int) {
	panic("tensorArgmaxAxis2DF32: native kernels not built on this platform")
}
func tensorArgmaxAxis2DF64(out []int64, in []float64, m, n, axis int) {
	panic("tensorArgmaxAxis2DF64: native kernels not built on this platform")
}
func tensorArgmaxAxis2DI64(out, in []int64, m, n, axis int) {
	panic("tensorArgmaxAxis2DI64: native kernels not built on this platform")
}

func tensorComputeAvailable() bool { return false }
