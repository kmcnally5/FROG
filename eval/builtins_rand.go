package eval

import (
	"fmt"
	"klex/ast"
	"math/rand/v2"
)

func init() {
	// rand returns a random float in [0.0, 1.0).
	// The global source is automatically seeded — no setup required.
	// Usage: rand()  →  0.7341...
	Builtins["rand"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("rand expects 0 arguments", ast.Pos{})
		}
		return &Float{Value: rand.Float64()}
	}}

	// randInt returns a random integer in the closed range [min, max].
	// Both endpoints are inclusive: randInt(1, 6) simulates a die roll.
	// min must be <= max.
	// Usage: randInt(1, 10)  →  7
	Builtins["randInt"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("randInt expects 2 arguments", ast.Pos{})
		}
		lo, ok1 := args[0].(*Integer)
		hi, ok2 := args[1].(*Integer)
		if !ok1 || !ok2 {
			return typeError(fmt.Sprintf("randInt: arguments must be integer, got %s and %s",
				args[0].Type(), args[1].Type()), ast.Pos{})
		}
		if lo.Value > hi.Value {
			return runtimeError(fmt.Sprintf("randInt: min (%d) must be <= max (%d)",
				lo.Value, hi.Value), ast.Pos{})
		}
		n := hi.Value - lo.Value + 1
		return &Integer{Value: lo.Value + rand.IntN(n)}
	}}

	// shuffle returns a new array with the elements in random order.
	// The original array is not mutated (consistent with push, pop, concat).
	// Usage: shuffle([1, 2, 3, 4, 5])  →  [3, 1, 5, 2, 4]
	Builtins["shuffle"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("shuffle expects 1 argument", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("shuffle: argument must be array, got %s",
				args[0].Type()), ast.Pos{})
		}
		// Copy then Fisher-Yates shuffle.
		out := make([]Object, len(arr.Elements))
		copy(out, arr.Elements)
		rand.Shuffle(len(out), func(i, j int) {
			out[i], out[j] = out[j], out[i]
		})
		return &Array{Elements: out}
	}}
}
