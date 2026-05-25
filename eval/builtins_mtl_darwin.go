// builtins_mtl_darwin.go — Metal-backed graphics + compute primitives.
//
// macOS-only by construction: the `_darwin` filename suffix triggers
// Go's automatic build-tag, so this file is invisible to the Linux
// and Windows builds. The matching stub file (builtins_mtl_stub.go)
// registers no-op error-returning versions of every builtin on those
// platforms so kLex scripts that reference `_mtl*` get a clean
// "macOS only" error rather than a "missing builtin" parse failure.
//
// Why Metal as a separate surface (rather than another OpenGL backend):
//
//   OpenGL handles the UI library well and works everywhere. Metal is
//   here for the workloads OpenGL doesn't reach on macOS — compute
//   shaders, hardware ray tracing (M3+), and zero-copy buffer sharing
//   with the CPU via unified memory. The two coexist; kLex picks
//   whichever fits the task.
//
// Builtin naming:
//
//   Every Metal-side primitive uses the `_mtl` prefix. These are
//   intentionally low-level — `stdlib/mtl.lex` and `stdlib/raytrace.lex`
//   wrap them in ergonomic kLex idioms.
//
// cgo layer:
//
//   The Objective-C side lives in mtl_bridge_darwin.{h,m}. cgo
//   auto-compiles the .m file because it sits in the same package
//   directory. The LDFLAGS pull in the three frameworks every Metal
//   project needs: Foundation (NSObject base), Metal (the GPU API),
//   and QuartzCore (CAMetalLayer / display integration — used in
//   later phases for window presentation).

package eval

/*
#cgo darwin CFLAGS:  -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Foundation -framework Metal -framework QuartzCore -framework MetalPerformanceShaders -framework MetalPerformanceShadersGraph
#include <stdlib.h>                // for C.free on strings allocated by C.CString
#include "mtl_bridge_darwin.h"
*/
import "C"

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"runtime/cgo"
	"unsafe"

	"klex/ast"
)

// goMtlCompletion is invoked from Apple's Metal completion thread
// via the addCompletedHandler block installed by the bridge. The
// uintptr is a cgo.Handle wrapping a buffered Go channel that the
// kLex-facing goroutine is waiting on; we forward the GPU's error
// string (or nil on success) to that channel.
//
// Runs on a non-Go OS thread that cgo wires up as a temporary M.
// The send must be non-blocking — channels are buffer size 1 and
// only ever receive exactly one message, so the buffer is always
// open when we arrive.
//
//export goMtlCompletion
func goMtlCompletion(goHandle C.uintptr_t, errStr *C.char) {
	h := cgo.Handle(uintptr(goHandle))
	ch, ok := h.Value().(chan string)
	if !ok {
		// Handle was deleted, or wrong type — drop on the floor.
		return
	}
	var msg string
	if errStr != nil {
		msg = C.GoString(errStr)
	}
	select {
	case ch <- msg:
	default:
		// Buffer full — shouldn't happen with our single-shot design.
	}
}

// runAsyncBridge bridges Metal's addCompletedHandler callback onto
// a kLex Channel. The submit closure must:
//
//   - Encode + commit a Metal command buffer that calls
//     goMtlCompletion(go_handle, ...) on completion (use the
//     bridge functions that take an `int64_t go_handle` parameter
//     in async mode).
//   - Return (0, nil) on successful submit; (-1, errStr) on
//     pre-submit validation failure.
//
// On pre-submit failure, the returned kLex Channel emits
// (null, errStr) immediately. On successful submit, it waits for
// the GPU completion callback, then emits (null, null) on success
// or (null, gpu_err) on GPU failure.
//
// The cgo.Handle is created here and deleted when the callback has
// delivered its result (or after a brief grace period if the GPU
// errored before our handler installed).
func runAsyncBridge(submit func(goHandle C.int64_t) (rc int, preSubmitErr string)) *Channel {
	ch := &Channel{
		ch:   make(chan Object, 1),
		done: make(chan struct{}),
	}
	go func() {
		defer func() {
			close(ch.ch)
			closeChannelDone(ch)
		}()

		// Caller-side cancellation check (kLex `close(ch)` before we
		// even started).
		select {
		case <-ch.done:
			return
		default:
		}

		// Buffered to 1 — completion handler sends exactly once.
		completionCh := make(chan string, 1)
		handle := cgo.NewHandle(completionCh)
		defer handle.Delete()

		rc, preSubmitErr := submit(C.int64_t(uintptr(handle)))
		if rc != 0 {
			// Pre-submit failure: no callback coming. Emit the error
			// directly.
			ch.ch <- &Tuple{Elements: []Object{NULL, &String{Value: preSubmitErr}}}
			return
		}

		// Wait for the GPU's addCompletedHandler to fire. While we
		// wait, this goroutine is parked on a Go channel — no OS
		// thread is held in cgo land.
		var gpuErr string
		select {
		case gpuErr = <-completionCh:
		case <-ch.done:
			// kLex cancelled mid-flight. The GPU work is already
			// running; its completion callback will deliver into
			// the (still-open) completionCh, which we abandon.
			// The cgo.Handle keeps completionCh alive long enough
			// for the callback to safely no-op the send.
			return
		}

		if gpuErr != "" {
			ch.ch <- &Tuple{Elements: []Object{NULL, &String{Value: gpuErr}}}
		} else {
			ch.ch <- &Tuple{Elements: []Object{NULL, NULL}}
		}
	}()
	return ch
}

// runAsyncSingle runs `work` in a goroutine and returns a kLex Channel
// that receives exactly one message — the work's result — and then
// closes. This is the single-shot async submission shape used by
// every Metal command that maps to "submit GPU work, await
// completion": _mtlClear, _mtlDispatch, _mtlBlit, _mtlAccelBuild, …
//
// Cancellation contract (matches the existing bridge stream pattern
// in builtins_bridge.go):
//
//   - Producer side (us): block-with-select on `done` so a cancelling
//     consumer doesn't get a value they can't receive.
//   - Consumer side (kLex): for-in's break handler closes ch.done.
//     A naked `recv()` doesn't cancel — call `close()` on the kLex
//     channel to abort.
//   - The actual GPU work CANNOT be aborted mid-flight (Metal has no
//     interrupt-shader API). Cancellation here means "don't bother
//     delivering the result and don't submit chained work". The
//     command buffer that's already on the GPU still runs to
//     completion in the background; we just drop its result.
//
// The work function is invoked from a fresh goroutine. It must be
// goroutine-safe (the cgo Metal bridge is — see the gLock guard in
// mtl_bridge_darwin.m). The Object it returns is whatever the kLex
// caller will receive from `recv(ch)` — by convention a Tuple of
// (result, error).
func runAsyncSingle(work func() Object) *Channel {
	ch := &Channel{
		ch:   make(chan Object, 1),
		done: make(chan struct{}),
	}
	go func() {
		// Cancel before we even start — saves a GPU submission if the
		// consumer already gave up (race-y but the right side of the
		// trade-off).
		select {
		case <-ch.done:
			close(ch.ch)
			closeChannelDone(ch)
			return
		default:
		}

		result := work()

		select {
		case ch.ch <- result:
		case <-ch.done:
		}
		close(ch.ch)
		closeChannelDone(ch)
	}()
	return ch
}

// init registers every `_mtl*` builtin. Called automatically when
// the eval package loads on macOS.
func init() {
	// _mtlInfo() → hash
	//
	// Returns a hash describing the default Metal device. Used as the
	// "is Metal available?" smoke test and the canonical place to read
	// capability flags before code that depends on them runs.
	//
	// Shape of returned hash:
	//   {
	//     "name":                  "Apple M2 Pro",
	//     "registry_id":           4294968574,
	//     "has_unified_memory":    true,
	//     "supports_raytracing":   false,    // true on M3+
	//     "supports_apple7":       true,     // M1+/A14+ baseline
	//     "max_threads_per_group": 1024,
	//     "max_bind_slots":        8,        // MTL_MAX_BIND from the bridge
	//   }
	//
	// `max_bind_slots` is the hard cap on textures, buffers, and
	// acceleration structures per dispatch — exposed so kLex code can
	// validate kernel layouts at the call site instead of waiting for
	// the bridge to reject the dispatch.
	//
	// On older hardware where no Metal device is available, returns
	// (null, error) — never a partially-filled hash.
	Builtins["_mtlInfo"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_mtlInfo expects 0 arguments", ast.Pos{})
		}

		var info C.MtlDeviceInfo
		errBuf := make([]C.char, 256)
		errBufLen := C.size_t(len(errBuf))

		rc := C.mtl_default_device_info(&info,
			(*C.char)(unsafe.Pointer(&errBuf[0])), errBufLen)
		if rc != 0 {
			return &Tuple{Elements: []Object{
				NULL,
				&String{Value: C.GoString(&errBuf[0])},
			}}
		}

		// Convert the C struct into a kLex hash. The field set here
		// is deliberately small — a "first useful subset" of MTLDevice.
		// More fields can be added as later phases need them; doing
		// it now would be speculation.
		h := makeHash(
			"name", &String{Value: C.GoString(&info.name[0])},
			"registry_id", &Integer{Value: int(info.registry_id)},
			"has_unified_memory", boolObj(info.has_unified_memory != 0),
			"supports_raytracing", boolObj(info.supports_raytracing != 0),
			"supports_apple7", boolObj(info.supports_family_apple7 != 0),
			"max_threads_per_group", &Integer{Value: int(info.max_threads_per_group)},
			"max_bind_slots", &Integer{Value: int(C.MTL_MAX_BIND)},
		)

		return &Tuple{Elements: []Object{h, NULL}}
	}}

	// _mtlSurface(width, height) → (handle, err)
	//
	// Allocates an offscreen RGBA8 Metal texture of the given size,
	// retained on the bridge side under an integer handle. The handle
	// is what every other `_mtl*` surface function expects.
	//
	// `width` and `height` must be positive integers. The texture's
	// storage mode is "shared" — zero-copy CPU read-back on Apple
	// Silicon, a blit on Intel.
	Builtins["_mtlSurface"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_mtlSurface expects (width, height)", ast.Pos{})
		}
		w, wok := args[0].(*Integer)
		h, hok := args[1].(*Integer)
		if !wok || !hok {
			return typeError("_mtlSurface expects (width: int, height: int)", ast.Pos{})
		}

		errBuf := make([]C.char, 256)
		handle := C.mtl_surface_create(C.int(w.Value), C.int(h.Value),
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
		if handle == 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}
		return &Tuple{Elements: []Object{&Integer{Value: int(handle)}, NULL}}
	}}

	// _mtlSurfaceFromBytes(bytes, width, height) → (surface, err)
	//
	// Creates a fresh RGBA8 Metal surface of the requested dimensions
	// and uploads `bytes` into it. `bytes` must be exactly width*height*4
	// bytes (RGBA8 row-major). The returned handle is the same kind of
	// handle _mtlSurface returns and is freed with _mtlSurfaceRelease.
	//
	// Use case: pull an in-memory RGBA image (e.g. a generated image
	// from an SD endpoint) onto a Metal surface so a compute kernel
	// can post-process it. Pair with _mtlSurfaceReadRgba on the output
	// surface to read the processed pixels back.
	Builtins["_mtlSurfaceFromBytes"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("_mtlSurfaceFromBytes expects (bytes, width, height)", ast.Pos{})
		}
		bs, bok := args[0].(*Bytes)
		w, wok := args[1].(*Integer)
		h, hok := args[2].(*Integer)
		if !bok || !wok || !hok {
			return typeError("_mtlSurfaceFromBytes expects (bytes: bytes, width: int, height: int)", ast.Pos{})
		}
		expected := w.Value * h.Value * 4
		if len(bs.Value) != expected {
			return runtimeError(fmt.Sprintf(
				"_mtlSurfaceFromBytes: bytes length %d does not match width*height*4 = %d",
				len(bs.Value), expected), ast.Pos{})
		}

		errBuf := make([]C.char, 256)
		var pixPtr *C.uint8_t
		if len(bs.Value) > 0 {
			pixPtr = (*C.uint8_t)(unsafe.Pointer(&bs.Value[0]))
		}
		handle := C.mtl_surface_create_from_bytes(
			pixPtr, C.size_t(len(bs.Value)),
			C.int(w.Value), C.int(h.Value),
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
		if handle == 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}
		return &Tuple{Elements: []Object{&Integer{Value: int(handle)}, NULL}}
	}}

	// _mtlSurfaceReadRgba(surface) → (bytes, err)
	//
	// Reads the surface's pixels back into a kLex bytes value (RGBA8
	// row-major, exactly width*height*4 bytes). Counterpart to
	// _mtlSurfaceFromBytes — completes the round-trip for image
	// post-processing pipelines.
	//
	// On Apple Silicon (unified memory) this is a zero-copy view into
	// the texture's shared storage; the kLex bytes are an independent
	// copy so the caller can mutate them without affecting the texture.
	Builtins["_mtlSurfaceReadRgba"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlSurfaceReadRgba expects (surface)", ast.Pos{})
		}
		surf, sok := args[0].(*Integer)
		if !sok {
			return typeError("_mtlSurfaceReadRgba expects (surface: int)", ast.Pos{})
		}
		handle := surf.Value

		errBuf := make([]C.char, 256)
		var cw, ch C.int
		if rc := C.mtl_surface_size(C.int64_t(handle), &cw, &ch,
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf))); rc != 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}
		pix := make([]byte, int(cw)*int(ch)*4)
		if rc := C.mtl_surface_to_rgba(C.int64_t(handle),
			(*C.uint8_t)(unsafe.Pointer(&pix[0])), C.size_t(len(pix)),
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf))); rc != 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}
		return &Tuple{Elements: []Object{&Bytes{Value: pix}, NULL}}
	}}

	// _mtlClear(surface, [r, g, b, a]) → Channel of (null, err)
	//
	// Submits a GPU clear-to-color command for the surface and returns
	// a kLex Channel that emits ONE message — `(null, null)` on success
	// or `(null, error_string)` on failure — then closes.
	//
	// Async by design: the kLex thread keeps running while the GPU
	// works. Wait for completion with `recv(ch)`. The Go bridge owns
	// the goroutine that blocks on the MTLCommandBuffer; kLex never
	// blocks on Metal directly.
	//
	// Cancellation: closing the channel (`close(ch)`) causes the
	// result to be dropped on the floor. The command buffer already
	// on the GPU still runs to completion (Metal has no interrupt) —
	// cancellation just means "don't deliver the result".
	Builtins["_mtlClear"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_mtlClear expects (surface, [r, g, b, a])", ast.Pos{})
		}
		surf, sok := args[0].(*Integer)
		col, cok := args[1].(*Array)
		if !sok || !cok {
			return typeError("_mtlClear expects (surface: int, color: [r,g,b,a])", ast.Pos{})
		}
		if len(col.Elements) != 4 {
			return runtimeError("_mtlClear: color must be a 4-element array [r, g, b, a]", ast.Pos{})
		}
		var rgba [4]float32
		for i, el := range col.Elements {
			switch v := el.(type) {
			case *Float:
				rgba[i] = float32(v.Value)
			case *Integer:
				rgba[i] = float32(v.Value)
			default:
				return typeError("_mtlClear: color components must be numbers", ast.Pos{})
			}
		}
		handle := surf.Value

		// Async path: submit returns immediately after commit.
		// Metal's completion thread fires goMtlCompletion, which
		// signals the channel runAsyncBridge is waiting on. No
		// goroutine is parked in cgo land during the GPU work.
		return runAsyncBridge(func(goHandle C.int64_t) (int, string) {
			errBuf := make([]C.char, 256)
			rc := C.mtl_surface_clear(C.int64_t(handle),
				C.float(rgba[0]), C.float(rgba[1]), C.float(rgba[2]), C.float(rgba[3]),
				goHandle,
				(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
			if rc != 0 {
				return int(rc), C.GoString(&errBuf[0])
			}
			return 0, ""
		})
	}}

	// _mtlSurfaceSavePng(surface, path) → Channel of (null, err)
	//
	// Reads the surface pixels back to CPU memory (RGBA8), PNG-encodes
	// them, and writes the result at `path`. Returns a single-shot
	// channel that emits one (null, null) on success or (null, err)
	// on failure, then closes.
	//
	// The async path matters because (a) read-back from a GPU texture
	// that's still rendering can stall until the GPU catches up, and
	// (b) PNG encoding a 4K image is a few-millisecond CPU job that
	// shouldn't block the kLex thread. Both happen in the goroutine.
	Builtins["_mtlSurfaceSavePng"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_mtlSurfaceSavePng expects (surface, path)", ast.Pos{})
		}
		surf, sok := args[0].(*Integer)
		path, pok := args[1].(*String)
		if !sok || !pok {
			return typeError("_mtlSurfaceSavePng expects (surface: int, path: string)", ast.Pos{})
		}
		handle := surf.Value
		outPath := path.Value

		return runAsyncSingle(func() Object {
			errBuf := make([]C.char, 256)
			var cw, ch C.int
			rc := C.mtl_surface_size(C.int64_t(handle), &cw, &ch,
				(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
			if rc != 0 {
				return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
			}
			width, height := int(cw), int(ch)
			pixels := make([]byte, width*height*4)

			rc = C.mtl_surface_to_rgba(C.int64_t(handle),
				(*C.uint8_t)(unsafe.Pointer(&pixels[0])), C.size_t(len(pixels)),
				(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
			if rc != 0 {
				return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
			}

			img := &image.NRGBA{
				Pix:    pixels,
				Stride: width * 4,
				Rect:   image.Rect(0, 0, width, height),
			}
			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err != nil {
				return &Tuple{Elements: []Object{NULL, &String{Value: "png encode: " + err.Error()}}}
			}
			if err := os.WriteFile(outPath, buf.Bytes(), 0644); err != nil {
				return &Tuple{Elements: []Object{NULL, &String{Value: "write: " + err.Error()}}}
			}
			return &Tuple{Elements: []Object{NULL, NULL}}
		})
	}}

	// _mtlSurfaceRelease(surface) → null
	//
	// Drops the bridge's retain on the surface so the GPU memory can
	// be freed. Idempotent — calling with an unknown handle is a no-op.
	// kLex has no destructors; callers must release surfaces
	// explicitly. Forgotten releases leak GPU memory but don't crash.
	Builtins["_mtlSurfaceRelease"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlSurfaceRelease expects (surface)", ast.Pos{})
		}
		surf, sok := args[0].(*Integer)
		if !sok {
			return typeError("_mtlSurfaceRelease expects (surface: int)", ast.Pos{})
		}
		C.mtl_surface_release(C.int64_t(surf.Value))
		return NULL
	}}

	// _mtlKernel(msl_source, fn_name) → (handle, err)
	//
	// Compiles MSL source code and produces a compute pipeline state
	// for the named function. The returned handle is reusable across
	// many dispatches — compile once, dispatch many.
	//
	// MSL is Apple's Metal Shading Language: a C++14-ish dialect with
	// GPU-specific qualifiers (`kernel`, `thread_position_in_grid`,
	// `texture2d`, etc.). Compile errors come back with the same
	// detail you'd see in Xcode — line/column + the message.
	//
	// Sync because compilation is bounded (~10 ms for the kernels we
	// have planned) and the result is needed before any dispatch can
	// be set up. If we ever hit a kernel that takes >100 ms to compile
	// we can move this to async — but speculative async for fast work
	// would just add complexity for no win.
	Builtins["_mtlKernel"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_mtlKernel expects (msl_source, fn_name)", ast.Pos{})
		}
		src, srcOk := args[0].(*String)
		fn, fnOk := args[1].(*String)
		if !srcOk || !fnOk {
			return typeError("_mtlKernel expects (msl_source: string, fn_name: string)", ast.Pos{})
		}

		// CString allocates a NUL-terminated copy of the Go string.
		// We must free both explicitly — cgo doesn't reach into
		// these to do it for us.
		cSrc := C.CString(src.Value)
		defer C.free(unsafe.Pointer(cSrc))
		cFn := C.CString(fn.Value)
		defer C.free(unsafe.Pointer(cFn))

		errBuf := make([]C.char, 1024)
		handle := C.mtl_kernel_create(cSrc, cFn,
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
		if handle == 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}
		return &Tuple{Elements: []Object{&Integer{Value: int(handle)}, NULL}}
	}}

	// _mtlKernelRelease(kernel) → null
	//
	// Drops the bridge's retain on the compiled pipeline state.
	// Idempotent — unknown handles are no-ops.
	Builtins["_mtlKernelRelease"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlKernelRelease expects (kernel)", ast.Pos{})
		}
		k, ok := args[0].(*Integer)
		if !ok {
			return typeError("_mtlKernelRelease expects (kernel: int)", ast.Pos{})
		}
		C.mtl_kernel_release(C.int64_t(k.Value))
		return NULL
	}}

	// _mtlBuffer(floats) → (handle, err)
	//
	// Wraps an array of floats as a GPU-accessible MTLBuffer. The
	// caller passes a kLex Array; each element must be a number
	// (int or float — both are accepted and coerced to float32).
	// The buffer's storage mode is Shared, so on Apple Silicon
	// there is no host→device copy: the bytes are visible to both
	// sides at the same address.
	//
	// Sync because the underlying newBufferWithBytes call is a
	// few-microsecond allocation; async would add complexity for no win.
	Builtins["_mtlBuffer"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlBuffer expects (floats: array)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError("_mtlBuffer expects an array of numbers", ast.Pos{})
		}
		n := len(arr.Elements)
		if n == 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_mtlBuffer: array is empty"}}}
		}

		// Marshal kLex Array → []float32 with explicit per-element
		// type checks so a wrongly-typed element is rejected with a
		// pointed error rather than silently coerced.
		data := make([]float32, n)
		for i, el := range arr.Elements {
			switch v := el.(type) {
			case *Float:
				data[i] = float32(v.Value)
			case *Integer:
				data[i] = float32(v.Value)
			default:
				return typeError("_mtlBuffer: every element must be a number", ast.Pos{})
			}
		}

		errBuf := make([]C.char, 256)
		handle := C.mtl_buffer_create_f32(
			(*C.float)(unsafe.Pointer(&data[0])), C.int(n),
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
		if handle == 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}
		return &Tuple{Elements: []Object{&Integer{Value: int(handle)}, NULL}}
	}}

	// _mtlBufferFromTensor(t) → (handle, err)
	//
	// Zero-marshalling upload from a FrogPy f32 tensor straight to an
	// MTLBuffer. The tensor's F32 slice is passed by pointer to
	// mtl_buffer_create_f32 — the same C entry point as _mtlBuffer
	// uses, but skipping the per-element kLex-Float-to-float32 loop
	// (which dominates _mtlBuffer's cost for large arrays).
	//
	// Measured ~5× faster than _mtlBuffer for 1024² f32 uploads.
	//
	// Restrictions: tensor must be DType f32 and contiguous. Other
	// dtypes error cleanly — convert via your own kernel first or
	// use _mtlBuffer with an explicit Array conversion if you really
	// need f64 / i64 on the GPU.
	Builtins["_mtlBufferFromTensor"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlBufferFromTensor expects (t: tensor)", ast.Pos{})
		}
		tn, ok := args[0].(*Tensor)
		if !ok {
			return typeError(fmt.Sprintf("_mtlBufferFromTensor: argument must be a tensor, got %s", args[0].Type()), ast.Pos{})
		}
		if tn.DType != DTypeFloat32 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_mtlBufferFromTensor: only f32 tensors are supported on the MTLBuffer surface"}}}
		}
		if !tn.IsContiguous() {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_mtlBufferFromTensor: tensor must be contiguous"}}}
		}
		n := len(tn.F32)
		if n == 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_mtlBufferFromTensor: tensor is empty"}}}
		}
		errBuf := make([]C.char, 256)
		handle := C.mtl_buffer_create_f32(
			(*C.float)(unsafe.Pointer(&tn.F32[0])), C.int(n),
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
		if handle == 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}
		return &Tuple{Elements: []Object{&Integer{Value: int(handle)}, NULL}}
	}}

	// _mtlBufferAllocF32(count) → (handle, err)
	//
	// Allocates a zero-initialised GPU buffer big enough to hold
	// `count` float32 values. Unlike _mtlBuffer (which marshals a
	// kLex Array element-by-element) this skips the CPU-side data
	// entirely — the Metal allocator zero-fills at the OS level for
	// free. Use this for any output buffer that a kernel fully
	// overwrites (reductions, FFT, matmul, etc.).
	Builtins["_mtlBufferAllocF32"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlBufferAllocF32 expects (count)", ast.Pos{})
		}
		n, ok := args[0].(*Integer)
		if !ok {
			return typeError("_mtlBufferAllocF32 expects (count: int)", ast.Pos{})
		}
		if n.Value <= 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_mtlBufferAllocF32: count must be positive"}}}
		}
		bytes := n.Value * 4
		errBuf := make([]C.char, 256)
		handle := C.mtl_buffer_alloc(C.int(bytes),
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
		if handle == 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}
		return &Tuple{Elements: []Object{&Integer{Value: int(handle)}, NULL}}
	}}

	// _mtlBufferAllocU32(count) → (handle, err)
	//
	// uint32 counterpart of _mtlBufferAllocF32. Same underlying
	// allocator (both are 4 bytes/element); separate builtin for
	// API symmetry with _mtlBuffer / _mtlBufferU32.
	Builtins["_mtlBufferAllocU32"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlBufferAllocU32 expects (count)", ast.Pos{})
		}
		n, ok := args[0].(*Integer)
		if !ok {
			return typeError("_mtlBufferAllocU32 expects (count: int)", ast.Pos{})
		}
		if n.Value <= 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_mtlBufferAllocU32: count must be positive"}}}
		}
		bytes := n.Value * 4
		errBuf := make([]C.char, 256)
		handle := C.mtl_buffer_alloc(C.int(bytes),
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
		if handle == 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}
		return &Tuple{Elements: []Object{&Integer{Value: int(handle)}, NULL}}
	}}

	// _mtlBufferU32(ints) → (handle, err)
	//
	// Same as _mtlBuffer but for uint32-typed buffers — needed for
	// triangle index buffers in indexed acceleration structures.
	// Accepts a kLex array of Integers; values are coerced to
	// uint32 (must be non-negative and <= 4_294_967_295).
	//
	// The bridge stores u32 buffers in the SAME handle table as f32
	// buffers — MTLBuffer is type-opaque at the Metal level. Kernel
	// authors must declare the buffer with the right MSL type
	// (`constant uint*` vs `constant float*`) or get silent corruption.
	Builtins["_mtlBufferU32"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlBufferU32 expects (ints: array)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError("_mtlBufferU32 expects an array of integers", ast.Pos{})
		}
		n := len(arr.Elements)
		if n == 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_mtlBufferU32: array is empty"}}}
		}
		data := make([]uint32, n)
		for i, el := range arr.Elements {
			iv, ok := el.(*Integer)
			if !ok {
				return typeError("_mtlBufferU32: every element must be an integer", ast.Pos{})
			}
			if iv.Value < 0 {
				return &Tuple{Elements: []Object{NULL, &String{Value: "_mtlBufferU32: element is negative (uint32 can't represent it)"}}}
			}
			data[i] = uint32(iv.Value)
		}

		errBuf := make([]C.char, 256)
		handle := C.mtl_buffer_create_u32(
			(*C.uint32_t)(unsafe.Pointer(&data[0])), C.int(n),
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
		if handle == 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}
		return &Tuple{Elements: []Object{&Integer{Value: int(handle)}, NULL}}
	}}

	// _mtlReadBuffer(buffer) → (floats, err)
	//
	// Copies the buffer's contents into a fresh kLex Array of floats.
	// Sync because shared-storage buffers are CPU-visible directly —
	// no GPU round-trip needed. The caller is responsible for
	// ensuring any GPU work that writes to the buffer has finished
	// (e.g. by awaiting the dispatch's completion channel) before
	// calling this.
	Builtins["_mtlReadBuffer"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlReadBuffer expects (buffer)", ast.Pos{})
		}
		b, ok := args[0].(*Integer)
		if !ok {
			return typeError("_mtlReadBuffer expects (buffer: int)", ast.Pos{})
		}

		count := int(C.mtl_buffer_count_f32(C.int64_t(b.Value)))
		if count < 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_mtlReadBuffer: unknown buffer handle"}}}
		}

		data := make([]float32, count)
		errBuf := make([]C.char, 256)
		rc := C.mtl_buffer_read_f32(C.int64_t(b.Value),
			(*C.float)(unsafe.Pointer(&data[0])), C.int(count),
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
		if rc != 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}

		// Float32 → kLex Float (which is float64). Lossless promotion.
		elems := make([]Object, count)
		for i, f := range data {
			elems[i] = &Float{Value: float64(f)}
		}
		return &Tuple{Elements: []Object{&Array{Elements: elems}, NULL}}
	}}

	// _mtlReadBufferU32(buffer) → (ints, err)
	//
	// uint32 counterpart of _mtlReadBuffer. Returns a kLex Array of
	// Integer values. Used for reading back atomic-counter outputs
	// (e.g. histogram bin counts).
	Builtins["_mtlReadBufferU32"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlReadBufferU32 expects (buffer)", ast.Pos{})
		}
		b, ok := args[0].(*Integer)
		if !ok {
			return typeError("_mtlReadBufferU32 expects (buffer: int)", ast.Pos{})
		}

		// f32 and u32 buffers have the same element size (4 bytes),
		// so the count-by-bytes function works for both.
		count := int(C.mtl_buffer_count_f32(C.int64_t(b.Value)))
		if count < 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_mtlReadBufferU32: unknown buffer handle"}}}
		}

		data := make([]uint32, count)
		errBuf := make([]C.char, 256)
		rc := C.mtl_buffer_read_u32(C.int64_t(b.Value),
			(*C.uint32_t)(unsafe.Pointer(&data[0])), C.int(count),
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
		if rc != 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}

		elems := make([]Object, count)
		for i, v := range data {
			elems[i] = &Integer{Value: int(v)}
		}
		return &Tuple{Elements: []Object{&Array{Elements: elems}, NULL}}
	}}

	// _mtlBufferRelease(buffer) → null
	//
	// Drops the bridge's retain. Idempotent.
	Builtins["_mtlBufferRelease"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlBufferRelease expects (buffer)", ast.Pos{})
		}
		b, ok := args[0].(*Integer)
		if !ok {
			return typeError("_mtlBufferRelease expects (buffer: int)", ast.Pos{})
		}
		C.mtl_buffer_release(C.int64_t(b.Value))
		return NULL
	}}

	// _mtlBatchBegin() → (handle, err)
	//
	// Opens a fresh batch command buffer. Subsequent _mtlBatchDispatch
	// calls add compute encoders to the same buffer; _mtlBatchCommit
	// closes the batch and submits. Use for tight loops that issue
	// many dispatches.
	Builtins["_mtlBatchBegin"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_mtlBatchBegin expects 0 arguments", ast.Pos{})
		}
		errBuf := make([]C.char, 256)
		handle := C.mtl_batch_begin(
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
		if handle == 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}
		return &Tuple{Elements: []Object{&Integer{Value: int(handle)}, NULL}}
	}}

	// _mtlBatchDispatch(batch, kernel, bindings, grid) → (null, err)
	//
	// Adds one compute dispatch to an open batch. Synchronous on the
	// CPU (just encoding) — the GPU work doesn't run until commit.
	Builtins["_mtlBatchDispatch"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("_mtlBatchDispatch expects (batch, kernel, bindings, grid)", ast.Pos{})
		}
		bArg, bOk := args[0].(*Integer)
		kArg, kOk := args[1].(*Integer)
		bindArg, bindOk := args[2].(*Hash)
		gArg, gOk := args[3].(*Array)
		if !bOk || !kOk || !bindOk || !gOk {
			return typeError("_mtlBatchDispatch expects (batch: int, kernel: int, bindings: hash, grid: array)", ast.Pos{})
		}
		if len(gArg.Elements) != 3 {
			return runtimeError("_mtlBatchDispatch: grid must be a 3-element array", ast.Pos{})
		}

		var grid [3]int
		for i, el := range gArg.Elements {
			switch v := el.(type) {
			case *Integer:
				grid[i] = v.Value
			case *Float:
				grid[i] = int(v.Value)
			default:
				return typeError("_mtlBatchDispatch: grid components must be numbers", ast.Pos{})
			}
		}

		var textures, buffers, accels []int64
		if texArr, ok := lookupHashArray(bindArg, "textures"); ok {
			ts, err := toHandleSlice(texArr, "textures")
			if err != nil {
				return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
			}
			textures = ts
		}
		if bufArr, ok := lookupHashArray(bindArg, "buffers"); ok {
			bs, err := toHandleSlice(bufArr, "buffers")
			if err != nil {
				return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
			}
			buffers = bs
		}
		if accArr, ok := lookupHashArray(bindArg, "accels"); ok {
			as, err := toHandleSlice(accArr, "accels")
			if err != nil {
				return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
			}
			accels = as
		}

		var spec C.MtlDispatchSpec
		spec.kernel_handle = C.int64_t(kArg.Value)
		spec.texture_count = C.int(len(textures))
		for i, h := range textures {
			spec.texture_handles[i] = C.int64_t(h)
		}
		spec.buffer_count = C.int(len(buffers))
		for i, h := range buffers {
			spec.buffer_handles[i] = C.int64_t(h)
		}
		spec.accel_count = C.int(len(accels))
		for i, h := range accels {
			spec.accel_handles[i] = C.int64_t(h)
		}
		spec.grid_x = C.int(grid[0])
		spec.grid_y = C.int(grid[1])
		spec.grid_z = C.int(grid[2])

		errBuf := make([]C.char, 512)
		rc := C.mtl_batch_dispatch(C.int64_t(bArg.Value), &spec,
			(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
		if rc != 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _mtlBatchRelease(batch) → null
	//
	// Abandons a batch without committing — used for error-path cleanup
	// when a mid-batch dispatch fails. Idempotent: releasing an unknown
	// handle is a no-op. Always pair with _mtlBatchBegin if any dispatch
	// returns an error, otherwise the command buffer leaks until process
	// exit.
	Builtins["_mtlBatchRelease"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlBatchRelease expects (batch)", ast.Pos{})
		}
		b, ok := args[0].(*Integer)
		if !ok {
			return typeError("_mtlBatchRelease expects (batch: int)", ast.Pos{})
		}
		C.mtl_batch_release(C.int64_t(b.Value))
		return NULL
	}}

	// _mtlBatchCommit(batch) → Channel of (null, err)
	//
	// Commits the batch's command buffer and returns a Channel that
	// emits when the GPU finishes. Releases the batch handle.
	Builtins["_mtlBatchCommit"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlBatchCommit expects (batch)", ast.Pos{})
		}
		b, ok := args[0].(*Integer)
		if !ok {
			return typeError("_mtlBatchCommit expects (batch: int)", ast.Pos{})
		}
		batchHandle := int64(b.Value)

		return runAsyncBridge(func(goHandle C.int64_t) (int, string) {
			errBuf := make([]C.char, 512)
			rc := C.mtl_batch_commit(C.int64_t(batchHandle), goHandle,
				(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
			if rc != 0 {
				return int(rc), C.GoString(&errBuf[0])
			}
			return 0, ""
		})
	}}

	// _mtlAccel(vertex_buffer, vertex_count) → Channel of (handle, err)
	//
	// Builds a ray-tracing acceleration structure (MTLPrimitiveAS) from
	// a triangle-list vertex buffer. The buffer must hold `vertex_count`
	// float3 positions (12 bytes each), and vertex_count must be a
	// positive multiple of 3 — every three consecutive vertices forms
	// one triangle.
	//
	// Async because accel builds are GPU-side and non-trivially slow
	// for any real scene. The returned Channel emits ONE message:
	// (accel_handle: int, null) on success or (null, err) on failure.
	//
	// The accel structure retains its source vertex buffer internally,
	// so releasing the kLex buffer handle does NOT invalidate the accel.
	// Conversely, releasing the accel does NOT release the buffer —
	// kLex must release both when done.
	Builtins["_mtlAccel"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_mtlAccel expects (vertex_buffer, vertex_count)", ast.Pos{})
		}
		vbuf, vOk := args[0].(*Integer)
		vc, cOk := args[1].(*Integer)
		if !vOk || !cOk {
			return typeError("_mtlAccel expects (vertex_buffer: int, vertex_count: int)", ast.Pos{})
		}
		bufHandle := int64(vbuf.Value)
		count := vc.Value

		return runAsyncSingle(func() Object {
			errBuf := make([]C.char, 512)
			handle := C.mtl_accel_build_triangles(
				C.int64_t(bufHandle), C.int(count),
				(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
			if handle == 0 {
				return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
			}
			return &Tuple{Elements: []Object{&Integer{Value: int(handle)}, NULL}}
		})
	}}

	// _mtlMatmulMPS(a_handle, b_handle, c_handle, m, k, n) → Channel of (null, err)
	//
	// MPS-accelerated matrix multiply: C := A · B using Apple's
	// MPSMatrixMultiplication. All three buffer handles must already
	// exist (typically created via _mtlBuffer). C is overwritten in
	// place.  Async via runAsyncSingle — channel emits one (null,err)
	// when the GPU finishes.
	//
	// For non-trivial sizes (256³+) this hits 10-20× the naive
	// matmul kernel's throughput.
	Builtins["_mtlMatmulMPS"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 6 {
			return runtimeError("_mtlMatmulMPS expects (a, b, c, m, k, n)", ast.Pos{})
		}
		a, aOk := args[0].(*Integer)
		b, bOk := args[1].(*Integer)
		c, cOk := args[2].(*Integer)
		m, mOk := args[3].(*Integer)
		k, kOk := args[4].(*Integer)
		n, nOk := args[5].(*Integer)
		if !aOk || !bOk || !cOk || !mOk || !kOk || !nOk {
			return typeError("_mtlMatmulMPS expects (a, b, c, m, k, n) all as integers", ast.Pos{})
		}
		aH := int64(a.Value)
		bH := int64(b.Value)
		cH := int64(c.Value)
		mm := m.Value
		kk := k.Value
		nn := n.Value

		return runAsyncBridge(func(goHandle C.int64_t) (int, string) {
			errBuf := make([]C.char, 512)
			rc := C.mtl_matmul_mps(
				C.int64_t(aH), C.int64_t(bH), C.int64_t(cH),
				C.int(mm), C.int(kk), C.int(nn),
				goHandle,
				(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
			if rc != 0 {
				return int(rc), C.GoString(&errBuf[0])
			}
			return 0, ""
		})
	}}

	// _mtlFFT(in_handle, out_handle, n, inverse) → Channel of (null, err)
	//
	// 1D complex-to-complex Fast Fourier Transform via MPSGraph.
	// Both buffers hold 2n floats in interleaved (re, im) layout —
	// same byte format as NumPy / SciPy complex64 arrays.
	//
	//   inverse=0  →  forward FFT
	//   inverse=1  →  inverse FFT (caller divides by n for a true inverse)
	//
	// Async via runAsyncSingle.
	Builtins["_mtlFFT"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("_mtlFFT expects (in, out, n, inverse)", ast.Pos{})
		}
		inH, iOk := args[0].(*Integer)
		outH, oOk := args[1].(*Integer)
		nArg, nOk := args[2].(*Integer)
		invArg, vOk := args[3].(*Integer)
		if !iOk || !oOk || !nOk || !vOk {
			return typeError("_mtlFFT expects (in: int, out: int, n: int, inverse: int)", ast.Pos{})
		}
		inHandle := int64(inH.Value)
		outHandle := int64(outH.Value)
		n := nArg.Value
		inverse := invArg.Value

		return runAsyncBridge(func(goHandle C.int64_t) (int, string) {
			errBuf := make([]C.char, 512)
			rc := C.mtl_fft_complex_1d(
				C.int64_t(inHandle), C.int64_t(outHandle),
				C.int(n), C.int(inverse),
				goHandle,
				(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
			if rc != 0 {
				return int(rc), C.GoString(&errBuf[0])
			}
			return 0, ""
		})
	}}

	// _mtlAccelIndexed(vertex_buffer, vertex_count, index_buffer, index_count)
	//                                                  → Channel of (handle, err)
	//
	// Indexed-geometry variant of _mtlAccel. The vertex buffer is a
	// float32 buffer of N float3 positions. The index buffer is a
	// UINT32 buffer of triangle indices — every 3 consecutive uint32s
	// reference 3 vertex positions to form one triangle.
	//
	// Use this instead of _mtlAccel whenever multiple triangles share
	// EXACT vertex positions — unindexed lists silently degenerate
	// the BVH in that case (see feedback-mtl-shared-verts-bvh memory).
	Builtins["_mtlAccelIndexed"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("_mtlAccelIndexed expects (vbuf, vertexCount, ibuf, indexCount)", ast.Pos{})
		}
		vbuf, vOk := args[0].(*Integer)
		vc, vcOk := args[1].(*Integer)
		ibuf, iOk := args[2].(*Integer)
		ic, icOk := args[3].(*Integer)
		if !vOk || !vcOk || !iOk || !icOk {
			return typeError("_mtlAccelIndexed expects (vbuf: int, vertexCount: int, ibuf: int, indexCount: int)", ast.Pos{})
		}
		vBufH := int64(vbuf.Value)
		vCount := vc.Value
		iBufH := int64(ibuf.Value)
		iCount := ic.Value

		return runAsyncSingle(func() Object {
			errBuf := make([]C.char, 512)
			handle := C.mtl_accel_build_indexed_triangles(
				C.int64_t(vBufH), C.int(vCount),
				C.int64_t(iBufH), C.int(iCount),
				(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
			if handle == 0 {
				return &Tuple{Elements: []Object{NULL, &String{Value: C.GoString(&errBuf[0])}}}
			}
			return &Tuple{Elements: []Object{&Integer{Value: int(handle)}, NULL}}
		})
	}}

	// _mtlAccelRelease(accel) → null
	//
	// Drops the bridge's retain on the acceleration structure. The
	// underlying vertex buffer (passed in via _mtlAccel) is NOT
	// released by this call — caller manages its own buffer handle.
	Builtins["_mtlAccelRelease"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mtlAccelRelease expects (accel)", ast.Pos{})
		}
		a, ok := args[0].(*Integer)
		if !ok {
			return typeError("_mtlAccelRelease expects (accel: int)", ast.Pos{})
		}
		C.mtl_accel_release(C.int64_t(a.Value))
		return NULL
	}}

	// _mtlDispatch(kernel, bindings, grid) → Channel of (null, err)
	//
	// Submits one compute-shader dispatch. Returns a single-shot
	// Channel that emits (null, null) on success or (null, err) on
	// failure, then closes.
	//
	// Arguments:
	//
	//   kernel    : int        — handle from _mtlKernel
	//   bindings  : hash       — { "textures": [t0, t1, ...],
	//                              "buffers":  [b0, b1, ...],
	//                              "accels":   [a0, a1, ...] }
	//                            Textures bind to [[texture(i)]].
	//                            Buffers bind to [[buffer(i)]].
	//                            Acceleration structures bind to
	//                            [[buffer(buffer_count + i)]] — they
	//                            share the buffer slot space and
	//                            come immediately after the regular
	//                            buffers. Any key is optional.
	//   grid      : array      — [x, y, z] thread count for the
	//                            dispatch. Use 1 for unused
	//                            dimensions, e.g. [n, 1, 1] for a
	//                            1-D pass over `n` elements.
	//
	// Async via runAsyncSingle: the kLex thread keeps running while
	// the GPU works. Wait for completion with `recv(ch)`. After a
	// successful recv, any buffers/textures bound here are safe to
	// read on the CPU side (shared storage + completion = visible).
	Builtins["_mtlDispatch"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("_mtlDispatch expects (kernel, bindings, grid)", ast.Pos{})
		}
		kArg, kOk := args[0].(*Integer)
		bArg, bOk := args[1].(*Hash)
		gArg, gOk := args[2].(*Array)
		if !kOk || !bOk || !gOk {
			return typeError("_mtlDispatch expects (kernel: int, bindings: hash, grid: array)", ast.Pos{})
		}
		if len(gArg.Elements) != 3 {
			return runtimeError("_mtlDispatch: grid must be a 3-element array [x, y, z]", ast.Pos{})
		}

		// Extract grid components — both Integer and Float are accepted
		// (Float is rounded down to int).
		var grid [3]int
		for i, el := range gArg.Elements {
			switch v := el.(type) {
			case *Integer:
				grid[i] = v.Value
			case *Float:
				grid[i] = int(v.Value)
			default:
				return typeError("_mtlDispatch: grid components must be numbers", ast.Pos{})
			}
		}

		// Pull texture, buffer, accel arrays out of the bindings hash.
		// A missing key means "no bindings of this type" — not an error.
		var textures, buffers, accels []int64
		if texArr, ok := lookupHashArray(bArg, "textures"); ok {
			ts, err := toHandleSlice(texArr, "textures")
			if err != nil {
				return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
			}
			textures = ts
		}
		if bufArr, ok := lookupHashArray(bArg, "buffers"); ok {
			bs, err := toHandleSlice(bufArr, "buffers")
			if err != nil {
				return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
			}
			buffers = bs
		}
		if accArr, ok := lookupHashArray(bArg, "accels"); ok {
			as, err := toHandleSlice(accArr, "accels")
			if err != nil {
				return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
			}
			accels = as
		}
		if len(textures) > 8 || len(buffers) > 8 || len(accels) > 8 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_mtlDispatch: too many bindings (max 8 of each type)"}}}
		}

		kernel := int64(kArg.Value)

		return runAsyncBridge(func(goHandle C.int64_t) (int, string) {
			var spec C.MtlDispatchSpec
			spec.kernel_handle = C.int64_t(kernel)
			spec.texture_count = C.int(len(textures))
			for i, h := range textures {
				spec.texture_handles[i] = C.int64_t(h)
			}
			spec.buffer_count = C.int(len(buffers))
			for i, h := range buffers {
				spec.buffer_handles[i] = C.int64_t(h)
			}
			spec.accel_count = C.int(len(accels))
			for i, h := range accels {
				spec.accel_handles[i] = C.int64_t(h)
			}
			spec.grid_x = C.int(grid[0])
			spec.grid_y = C.int(grid[1])
			spec.grid_z = C.int(grid[2])

			errBuf := make([]C.char, 512)
			rc := C.mtl_dispatch(&spec, goHandle,
				(*C.char)(unsafe.Pointer(&errBuf[0])), C.size_t(len(errBuf)))
			if rc != 0 {
				return int(rc), C.GoString(&errBuf[0])
			}
			return 0, ""
		})
	}}
}

// lookupHashArray fetches a key whose value should be a kLex Array.
// Returns (array, true) on hit; (nil, false) when the key is absent
// or holds a non-Array value (treated as "not provided" for the
// optional-binding case).
func lookupHashArray(h *Hash, key string) (*Array, bool) {
	hk := HashKey{Type: STRING_OBJ, Value: key}
	pair, ok := h.Pairs[hk]
	if !ok {
		return nil, false
	}
	arr, isArr := pair.Value.(*Array)
	if !isArr {
		return nil, false
	}
	return arr, true
}

// toHandleSlice converts a kLex Array of Integer handles into a
// []int64. `label` is used in error messages so the caller knows
// which binding type failed type checking.
func toHandleSlice(arr *Array, label string) ([]int64, error) {
	out := make([]int64, len(arr.Elements))
	for i, el := range arr.Elements {
		v, ok := el.(*Integer)
		if !ok {
			return nil, fmt.Errorf("_mtlDispatch: %s[%d] must be an integer handle", label, i)
		}
		out[i] = int64(v.Value)
	}
	return out, nil
}
