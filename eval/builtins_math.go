package eval

import (
	"fmt"
	"klex/ast"
	"math"
)

func init() {
	// floor returns the greatest integer less than or equal to x.
	// Accepts integer or float. Always returns Integer.
	// floor(3.7) → 3    floor(-2.3) → -3    floor(4) → 4
	Builtins["floor"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("floor expects 1 argument", ast.Pos{})
		}
		switch v := args[0].(type) {
		case *Integer:
			return v
		case *Float:
			return &Integer{Value: int(math.Floor(v.Value))}
		default:
			return typeError(fmt.Sprintf("floor: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
	}}

	// ceil returns the smallest integer greater than or equal to x.
	// Accepts integer or float. Always returns Integer.
	// ceil(3.2) → 4    ceil(-2.7) → -2    ceil(4) → 4
	Builtins["ceil"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("ceil expects 1 argument", ast.Pos{})
		}
		switch v := args[0].(type) {
		case *Integer:
			return v
		case *Float:
			return &Integer{Value: int(math.Ceil(v.Value))}
		default:
			return typeError(fmt.Sprintf("ceil: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
	}}

	// round returns the nearest integer to x, rounding half away from zero.
	// Accepts integer or float. Always returns Integer.
	// round(3.5) → 4    round(3.4) → 3    round(-2.5) → -3
	Builtins["round"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("round expects 1 argument", ast.Pos{})
		}
		switch v := args[0].(type) {
		case *Integer:
			return v
		case *Float:
			return &Integer{Value: int(math.Round(v.Value))}
		default:
			return typeError(fmt.Sprintf("round: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
	}}

	// sqrt returns the square root of x. Always returns Float.
	// Accepts integer or float. x must be non-negative.
	// sqrt(4) → 2.0    sqrt(2.0) → 1.4142135623730951
	Builtins["sqrt"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("sqrt expects 1 argument", ast.Pos{})
		}
		var v float64
		switch n := args[0].(type) {
		case *Integer:
			v = float64(n.Value)
		case *Float:
			v = n.Value
		default:
			return typeError(fmt.Sprintf("sqrt: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		if v < 0 {
			return runtimeError("sqrt: argument must be non-negative", ast.Pos{})
		}
		return &Float{Value: math.Sqrt(v)}
	}}

	// sin returns the sine of x (x in radians). Always returns Float.
	Builtins["sin"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("sin expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("sin: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		return &Float{Value: math.Sin(toFloat64(args[0]))}
	}}

	// cos returns the cosine of x (x in radians). Always returns Float.
	Builtins["cos"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("cos expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("cos: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		return &Float{Value: math.Cos(toFloat64(args[0]))}
	}}

	// tan returns the tangent of x (x in radians). Always returns Float.
	Builtins["tan"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("tan expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("tan: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		return &Float{Value: math.Tan(toFloat64(args[0]))}
	}}

	// min returns the smaller of two values. Accepts integers and floats.
	// Mixed types are compared as floats; the original value (and its type) is returned.
	// min(3, 7) → 3    min(1.5, 2.5) → 1.5    min(3, 2.5) → 2.5
	Builtins["min"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("min expects 2 arguments", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) || !canArithmetic(args[1].Type()) {
			return typeError(fmt.Sprintf("min: arguments must be integer or float, got %s and %s",
				args[0].Type(), args[1].Type()), ast.Pos{})
		}
		if toFloat64(args[0]) <= toFloat64(args[1]) {
			return args[0]
		}
		return args[1]
	}}

	// max returns the larger of two values. Accepts integers and floats.
	// Mixed types are compared as floats; the original value (and its type) is returned.
	// max(3, 7) → 7    max(1.5, 2.5) → 2.5    max(3, 2.5) → 3
	Builtins["max"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("max expects 2 arguments", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) || !canArithmetic(args[1].Type()) {
			return typeError(fmt.Sprintf("max: arguments must be integer or float, got %s and %s",
				args[0].Type(), args[1].Type()), ast.Pos{})
		}
		if toFloat64(args[0]) >= toFloat64(args[1]) {
			return args[0]
		}
		return args[1]
	}}
}
