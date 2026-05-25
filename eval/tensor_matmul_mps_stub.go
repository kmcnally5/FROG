//go:build !darwin

package eval

// tensor_matmul_mps_stub.go — non-darwin sentinel for the MPS matmul
// path. Returns handled=false so the dispatcher in builtins_tensor.go
// knows to route the call to the CPU kernel (tensorMatmulF32) instead.
//
// This is NOT a panic-stub like tensor_compute_stub.go's CPU fallbacks
// — MPS unavailability is the EXPECTED case on Linux, not a failure
// mode. The f32 path on Linux is fully functional via the C kernel;
// it just doesn't get the GPU win.

func tensorMatmulMPSf32(out, a, b []float32, m, k, n int) (handled bool, errMsg string) {
	return false, ""
}
