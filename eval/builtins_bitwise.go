package eval

import (
	"fmt"
	"klex/ast"
)

func init() {
	// bitAnd — bitwise AND of two integers.
	//
	// @sig     bitAnd(a: int, b: int) -> int
	// @param   a  first integer
	// @param   b  second integer
	// @returns a & b
	// @errors  TypeError if either argument is not an integer
	// @example bitAnd(12, 10)   → 8
	// @since   0.1.0
	// @see     bitOr, bitXor, bitNot
	Builtins["bitAnd"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("bitAnd expects 2 arguments", ast.Pos{})
		}
		a, ok1 := args[0].(*Integer)
		b, ok2 := args[1].(*Integer)
		if !ok1 || !ok2 {
			return typeError(fmt.Sprintf("bitAnd: arguments must be integer, got %s and %s",
				args[0].Type(), args[1].Type()), ast.Pos{})
		}
		return &Integer{Value: a.Value & b.Value}
	}}

	// bitOr — bitwise OR of two integers.
	//
	// @sig     bitOr(a: int, b: int) -> int
	// @param   a  first integer
	// @param   b  second integer
	// @returns a | b
	// @errors  TypeError if either argument is not an integer
	// @example bitOr(12, 3)   → 15
	// @since   0.1.0
	// @see     bitAnd, bitXor, bitNot
	Builtins["bitOr"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("bitOr expects 2 arguments", ast.Pos{})
		}
		a, ok1 := args[0].(*Integer)
		b, ok2 := args[1].(*Integer)
		if !ok1 || !ok2 {
			return typeError(fmt.Sprintf("bitOr: arguments must be integer, got %s and %s",
				args[0].Type(), args[1].Type()), ast.Pos{})
		}
		return &Integer{Value: a.Value | b.Value}
	}}

	// bitXor — bitwise XOR of two integers.
	//
	// @sig     bitXor(a: int, b: int) -> int
	// @param   a  first integer
	// @param   b  second integer
	// @returns a ^ b
	// @errors  TypeError if either argument is not an integer
	// @example bitXor(12, 10)   → 6
	// @since   0.1.0
	// @see     bitAnd, bitOr, bitNot
	Builtins["bitXor"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("bitXor expects 2 arguments", ast.Pos{})
		}
		a, ok1 := args[0].(*Integer)
		b, ok2 := args[1].(*Integer)
		if !ok1 || !ok2 {
			return typeError(fmt.Sprintf("bitXor: arguments must be integer, got %s and %s",
				args[0].Type(), args[1].Type()), ast.Pos{})
		}
		return &Integer{Value: a.Value ^ b.Value}
	}}

	// bitNot — bitwise NOT (ones' complement) of an integer.
	//
	// @sig     bitNot(x: int) -> int
	// @param   x  the integer to invert
	// @returns ^x — every bit flipped, i.e. -(x + 1)
	// @errors  TypeError if x is not an integer
	// @example bitNot(0)    → -1
	// @example bitNot(-1)   → 0
	// @since   0.1.0
	// @see     bitAnd, bitOr, bitXor
	Builtins["bitNot"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bitNot expects 1 argument", ast.Pos{})
		}
		a, ok := args[0].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("bitNot: argument must be integer, got %s",
				args[0].Type()), ast.Pos{})
		}
		return &Integer{Value: ^a.Value}
	}}

	// bitShiftLeft — shift x left by n bits.
	//
	// Equivalent to x * 2^n for non-negative x.
	//
	// @sig     bitShiftLeft(x: int, n: int) -> int
	// @param   x  the integer to shift
	// @param   n  the number of bits to shift by (non-negative)
	// @returns x << n
	// @errors  TypeError if either argument is not an integer; RuntimeError if n is negative
	// @example bitShiftLeft(1, 4)   → 16
	// @example bitShiftLeft(3, 8)   → 768
	// @since   0.1.0
	// @see     bitShiftRight
	Builtins["bitShiftLeft"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("bitShiftLeft expects 2 arguments", ast.Pos{})
		}
		a, ok1 := args[0].(*Integer)
		n, ok2 := args[1].(*Integer)
		if !ok1 || !ok2 {
			return typeError(fmt.Sprintf("bitShiftLeft: arguments must be integer, got %s and %s",
				args[0].Type(), args[1].Type()), ast.Pos{})
		}
		if n.Value < 0 {
			return runtimeError("bitShiftLeft: shift amount must be non-negative", ast.Pos{})
		}
		return &Integer{Value: a.Value << uint(n.Value)}
	}}

	// bitShiftRight — arithmetic right shift of x by n bits (sign-preserving).
	//
	// Equivalent to x / 2^n for non-negative x; the sign bit is preserved for
	// negative x.
	//
	// @sig     bitShiftRight(x: int, n: int) -> int
	// @param   x  the integer to shift
	// @param   n  the number of bits to shift by (non-negative)
	// @returns x >> n
	// @errors  TypeError if either argument is not an integer; RuntimeError if n is negative
	// @example bitShiftRight(16, 4)    → 1
	// @example bitShiftRight(256, 3)   → 32
	// @since   0.1.0
	// @see     bitShiftLeft
	Builtins["bitShiftRight"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("bitShiftRight expects 2 arguments", ast.Pos{})
		}
		a, ok1 := args[0].(*Integer)
		n, ok2 := args[1].(*Integer)
		if !ok1 || !ok2 {
			return typeError(fmt.Sprintf("bitShiftRight: arguments must be integer, got %s and %s",
				args[0].Type(), args[1].Type()), ast.Pos{})
		}
		if n.Value < 0 {
			return runtimeError("bitShiftRight: shift amount must be non-negative", ast.Pos{})
		}
		return &Integer{Value: a.Value >> uint(n.Value)}
	}}
}
