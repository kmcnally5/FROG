//go:build darwin

package eval

// tensor_matmul_mps_darwin.go — Apple MPS dispatch for FrogPy f32 matmul.
//
// Path: Tensor.F32 slice → MTLBuffer (shared storage, zero-copy on
// Apple Silicon) → MPSMatrixMultiplication via mtl_matmul_mps in
// SYNCHRONOUS mode (go_handle=0) → MTLBuffer read-back → release.
//
// Why sync mode and not runAsyncBridge: the FrogPy v1 surface is
// synchronous (t.matmul, t.add, etc. all return Tensors directly).
// go_handle=0 path in commit_or_install_completion() does
// commit + waitUntilCompleted in the bridge, which is exactly the
// semantics this caller wants. Saves a goroutine + channel hop.
//
// Buffer lifecycle: A and B are uploaded fresh per call (no caching
// in v1). C is allocated via mtl_buffer_alloc (zero-init, no upload).
// All three are released before return regardless of success/error.

/*
#include "mtl_bridge_darwin.h"
*/
import "C"

import "unsafe"

// tensorMatmulMPSf32 dispatches an f32 matmul on the GPU via MPS.
//
//	out: pre-allocated []float32 of length m*n (written in place)
//	a:   []float32 of length m*k, row-major
//	b:   []float32 of length k*n, row-major
//
// Returns (true, "") on success.
// Returns (true, errMsg) if MPS was tried but failed — do NOT fall
// back to CPU; the caller surfaces the error.
// (false, "") would mean "MPS unavailable on this platform" — but
// on darwin that never happens, so this function always returns
// handled=true. The non-darwin stub returns (false, "") so the
// dispatcher in builtins_tensor.go can route to the CPU kernel.
func tensorMatmulMPSf32(out, a, b []float32, m, k, n int) (handled bool, errMsg string) {
	if m == 0 || k == 0 || n == 0 {
		return true, ""
	}

	errBuf := make([]C.char, 512)
	errPtr := (*C.char)(unsafe.Pointer(&errBuf[0]))
	errLen := C.size_t(len(errBuf))

	aHandle := C.mtl_buffer_create_f32(
		(*C.float)(unsafe.Pointer(&a[0])), C.int(m*k),
		errPtr, errLen)
	if aHandle == 0 {
		return true, "matmul/MPS: upload A: " + C.GoString(errPtr)
	}

	bHandle := C.mtl_buffer_create_f32(
		(*C.float)(unsafe.Pointer(&b[0])), C.int(k*n),
		errPtr, errLen)
	if bHandle == 0 {
		C.mtl_buffer_release(aHandle)
		return true, "matmul/MPS: upload B: " + C.GoString(errPtr)
	}

	cHandle := C.mtl_buffer_alloc(
		C.int(m*n*4), // bytes; 4 = sizeof(float32)
		errPtr, errLen)
	if cHandle == 0 {
		C.mtl_buffer_release(aHandle)
		C.mtl_buffer_release(bHandle)
		return true, "matmul/MPS: alloc C: " + C.GoString(errPtr)
	}

	// Synchronous dispatch: go_handle=0 routes the bridge through
	// commit + waitUntilCompleted instead of the async completion
	// handler path.
	rc := C.mtl_matmul_mps(
		aHandle, bHandle, cHandle,
		C.int(m), C.int(k), C.int(n),
		C.int64_t(0),
		errPtr, errLen)
	if rc != 0 {
		C.mtl_buffer_release(aHandle)
		C.mtl_buffer_release(bHandle)
		C.mtl_buffer_release(cHandle)
		return true, "matmul/MPS: dispatch: " + C.GoString(errPtr)
	}

	readRC := C.mtl_buffer_read_f32(
		cHandle,
		(*C.float)(unsafe.Pointer(&out[0])), C.int(m*n),
		errPtr, errLen)

	C.mtl_buffer_release(aHandle)
	C.mtl_buffer_release(bHandle)
	C.mtl_buffer_release(cHandle)

	if readRC != 0 {
		return true, "matmul/MPS: read C: " + C.GoString(errPtr)
	}
	return true, ""
}
