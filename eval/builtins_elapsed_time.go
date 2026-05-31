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
	// elapsedTime() → float
	//   Wall-clock seconds since the kLex interpreter started. Sub-second
	//   precision. Use for benchmark/timing measurements:
	//     let start = elapsedTime()
	//     // ... work ...
	//     let secs = elapsedTime() - start
	Builtins["elapsedTime"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("elapsedTime expects no arguments", ast.Pos{})
		}
		return &Float{Value: time.Since(programStart).Seconds()}
	}}
}
