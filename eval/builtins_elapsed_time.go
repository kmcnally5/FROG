package eval

import (
	"klex/ast"
	"time"
)

// programStart is captured when the eval package initialises — the
// reference point for elapsedTime(). Lives outside builtins_graphics.go
// (which is //go:build !js) so this builtin is available in WASM too;
// many tests use it for timing measurements that have nothing to do with
// graphics.
var programStart = time.Now()

func init() {
	// elapsedTime — wall-clock seconds since the interpreter started.
	//
	// A monotonically increasing float with sub-second precision, measured from
	// program start. Subtract two readings to time a section of work. Available on
	// desktop and in the browser/WASM build (it's deliberately kept out of the
	// graphics file so timing works without a window).
	//
	// @sig     elapsedTime() -> float
	// @returns seconds elapsed since the interpreter started
	// @errors  RuntimeError if called with any arguments
	// @example no-run start = elapsedTime()
	// @since   0.1.0
	// @see     sleep
	Builtins["elapsedTime"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("elapsedTime expects no arguments", ast.Pos{})
		}
		return &Float{Value: time.Since(programStart).Seconds()}
	}}
}
