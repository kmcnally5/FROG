package eval

import (
	"fmt"
	"klex/ast"
	"math/rand/v2"
)

func init() {
	// rand — a random float in [0.0, 1.0).
	//
	// The global source is auto-seeded; no setup needed.
	//
	// @sig     rand() -> float
	// @returns a random float, 0.0 inclusive to 1.0 exclusive
	// @errors  RuntimeError if called with any arguments
	// @example no-run rand()
	// @since   0.1.0
	// @see     randInt, shuffle
	Builtins["rand"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("rand expects 0 arguments", ast.Pos{})
		}
		return &Float{Value: rand.Float64()}
	}}

	// randInt — a random integer in the inclusive range [min, max].
	//
	// Both endpoints are inclusive — randInt(1, 6) simulates a die roll.
	//
	// @sig     randInt(min: int, max: int) -> int
	// @param   min  the lowest possible value (inclusive)
	// @param   max  the highest possible value (inclusive)
	// @returns a random integer between min and max, inclusive
	// @errors  TypeError if either argument isn't an integer; RuntimeError if min > max
	// @example no-run randInt(1, 6)
	// @since   0.1.0
	// @see     rand, shuffle
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

	// shuffle — a new array with the elements in random order.
	//
	// The input is not mutated (consistent with push/pop/concat). Fisher-Yates.
	//
	// @sig     shuffle(arr: array) -> array
	// @param   arr  the array to shuffle (left unchanged)
	// @returns a new array with arr's elements randomly reordered
	// @errors  TypeError if arr is not an array
	// @example no-run shuffle([1, 2, 3, 4, 5])
	// @since   0.1.0
	// @see     rand, randInt, sort
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
