// builtins_mtl_stub.go — non-macOS stubs for every _mtl* builtin.
//
// Metal is Apple-only. On Linux and Windows the real implementation
// (eval/builtins_mtl_darwin.go) is excluded by Go's automatic
// `_darwin` build tag. This file fills the gap so kLex scripts that
// reference `_mtl*` receive a clear "macOS only" runtime error
// instead of an "unknown identifier" parse error — which is what
// you'd get if the builtin simply wasn't registered.
//
// The stubs return errors, never panic. This matches kLex's
// (value, error) tuple convention for fallible builtins, so calling
// code paths look identical on all three platforms:
//
//     info, err = _mtlInfo()
//     if err != null {
//         println("Metal unavailable:", err)
//         return
//     }
//
// Linux and Windows users see `err = "_mtlInfo: Metal is macOS-only"`
// and can branch on it cleanly.

//go:build !darwin

package eval

import "runtime"

// macStubTuple returns the (null, "Metal is macOS-only") tuple used
// as the error payload by every `_mtl*` stub.
func macStubTuple(name string) Object {
	return &Tuple{Elements: []Object{
		NULL,
		&String{Value: name + ": Metal is macOS-only — running on " + runtime.GOOS},
	}}
}

// macStubChannel returns a pre-loaded, pre-closed kLex Channel that
// emits a single (null, "Metal is macOS-only") tuple. Used for every
// `_mtl*` builtin that returns a Channel on darwin so the calling
// kLex code can use `recv(ch)` identically on all platforms — it
// just immediately gets the error.
func macStubChannel(name string) *Channel {
	ch := &Channel{
		ch:   make(chan Object, 1),
		done: make(chan struct{}),
	}
	ch.ch <- macStubTuple(name)
	close(ch.ch)
	closeChannelDone(ch)
	return ch
}

func init() {
	// Synchronous builtins: return tuple directly.
	Builtins["_mtlInfo"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlInfo")
	}}
	Builtins["_mtlSurface"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlSurface")
	}}
	Builtins["_mtlSurfaceRelease"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return NULL
	}}
	Builtins["_mtlSurfaceFromBytes"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlSurfaceFromBytes")
	}}
	Builtins["_mtlSurfaceReadRgba"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlSurfaceReadRgba")
	}}
	Builtins["_mtlKernel"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlKernel")
	}}
	Builtins["_mtlKernelRelease"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return NULL
	}}
	Builtins["_mtlBuffer"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlBuffer")
	}}
	Builtins["_mtlBufferFromTensor"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlBufferFromTensor")
	}}
	Builtins["_mtlBufferU32"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlBufferU32")
	}}
	Builtins["_mtlBufferAllocF32"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlBufferAllocF32")
	}}
	Builtins["_mtlBufferAllocU32"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlBufferAllocU32")
	}}
	Builtins["_mtlReadBuffer"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlReadBuffer")
	}}
	Builtins["_mtlReadBufferU32"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlReadBufferU32")
	}}
	Builtins["_mtlBufferRelease"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return NULL
	}}

	// Async builtins: return a pre-resolved Channel so kLex code that
	// does `result, err = recv(_mtlClear(...))` works identically on
	// every platform.
	Builtins["_mtlClear"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubChannel("_mtlClear")
	}}
	Builtins["_mtlSurfaceSavePng"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubChannel("_mtlSurfaceSavePng")
	}}
	Builtins["_mtlDispatch"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubChannel("_mtlDispatch")
	}}
	Builtins["_mtlAccel"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubChannel("_mtlAccel")
	}}
	Builtins["_mtlAccelIndexed"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubChannel("_mtlAccelIndexed")
	}}
	Builtins["_mtlMatmulMPS"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubChannel("_mtlMatmulMPS")
	}}
	Builtins["_mtlFFT"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubChannel("_mtlFFT")
	}}
	Builtins["_mtlBatchBegin"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlBatchBegin")
	}}
	Builtins["_mtlBatchDispatch"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubTuple("_mtlBatchDispatch")
	}}
	Builtins["_mtlBatchCommit"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return macStubChannel("_mtlBatchCommit")
	}}
	Builtins["_mtlBatchRelease"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return NULL
	}}
	Builtins["_mtlAccelRelease"] = &Builtin{Fn: func(args []Object) Object {
		_ = args
		return NULL
	}}
}
