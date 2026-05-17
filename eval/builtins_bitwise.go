package eval

import (
	"fmt"
	"klex/ast"
)

func init() {
	// bitAnd(a, b) → integer
	// Bitwise AND of two integers. Both operands must be integer.
	// bitAnd(0b1100, 0b1010) → 8  (0b1000)
	// bitAnd(0xFF, 0x0F)     → 15 (0x0F)
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

	// bitOr(a, b) → integer
	// Bitwise OR of two integers. Both operands must be integer.
	// bitOr(0b1100, 0b0011) → 15 (0b1111)
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

	// bitXor(a, b) → integer
	// Bitwise XOR of two integers. Both operands must be integer.
	// bitXor(0b1100, 0b1010) → 6  (0b0110)
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

	// bitNot(x) → integer
	// Bitwise NOT (ones' complement) of an integer.
	// bitNot(0)  → -1
	// bitNot(-1) → 0
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

	// bitShiftLeft(x, n) → integer
	// Shift x left by n bits. Equivalent to x * 2^n for non-negative x.
	// n must be a non-negative integer.
	// bitShiftLeft(1, 4)  → 16
	// bitShiftLeft(3, 8)  → 768
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

	// bitShiftRight(x, n) → integer
	// Arithmetic right shift of x by n bits. Sign bit is preserved.
	// Equivalent to x / 2^n for non-negative x.
	// n must be a non-negative integer.
	// bitShiftRight(16, 4) → 1
	// bitShiftRight(256, 3) → 32
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
