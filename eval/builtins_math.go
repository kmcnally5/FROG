package eval

import (
	"fmt"
	"klex/ast"
	"math"
)

func init() {
	// floor — the greatest integer less than or equal to x.
	//
	// @sig     floor(x: number) -> int
	// @param   x  an integer or float
	// @returns the largest integer ≤ x (an integer passes through unchanged)
	// @errors  TypeError if x is not numeric
	// @example floor(3.7)    → 3
	// @example floor(-2.3)   → -3
	// @since   0.1.0
	// @see     ceil, round, constrain
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

	// ceil — the smallest integer greater than or equal to x.
	//
	// @sig     ceil(x: number) -> int
	// @param   x  an integer or float
	// @returns the smallest integer ≥ x (an integer passes through unchanged)
	// @errors  TypeError if x is not numeric
	// @example ceil(3.2)    → 4
	// @example ceil(-2.7)   → -2
	// @since   0.1.0
	// @see     floor, round
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

	// round — the nearest integer to x, rounding halves away from zero.
	//
	// @sig     round(x: number) -> int
	// @param   x  an integer or float
	// @returns the nearest integer; ties (.5) round away from zero
	// @errors  TypeError if x is not numeric
	// @example round(3.5)    → 4
	// @example round(-2.5)   → -3
	// @since   0.1.0
	// @see     floor, ceil
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

	// sqrt — the square root of x.
	//
	// @sig     sqrt(x: number) -> float
	// @param   x  a non-negative integer or float
	// @returns the square root, always as a float
	// @errors  TypeError if x is not numeric; RuntimeError if x is negative
	// @example sqrt(4)     → 2
	// @example sqrt(2.0)   → 1.4142135623730951
	// @since   0.1.0
	// @see     pow
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

	// sin — the sine of x (x in radians).
	//
	// @sig     sin(x: number) -> float
	// @param   x  an angle in radians
	// @returns the sine of x, always as a float
	// @errors  TypeError if x is not numeric
	// @example sin(0)   → 0
	// @since   0.1.0
	// @see     cos, tan, asin, pi
	Builtins["sin"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("sin expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("sin: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		return &Float{Value: math.Sin(toFloat64(args[0]))}
	}}

	// cos — the cosine of x (x in radians).
	//
	// @sig     cos(x: number) -> float
	// @param   x  an angle in radians
	// @returns the cosine of x, always as a float
	// @errors  TypeError if x is not numeric
	// @example cos(0)   → 1
	// @since   0.1.0
	// @see     sin, tan, acos, pi
	Builtins["cos"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("cos expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("cos: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		return &Float{Value: math.Cos(toFloat64(args[0]))}
	}}

	// tan — the tangent of x (x in radians).
	//
	// @sig     tan(x: number) -> float
	// @param   x  an angle in radians
	// @returns the tangent of x, always as a float
	// @errors  TypeError if x is not numeric
	// @example tan(0)   → 0
	// @since   0.1.0
	// @see     sin, cos, atan
	Builtins["tan"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("tan expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("tan: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		return &Float{Value: math.Tan(toFloat64(args[0]))}
	}}

	// min — the smaller of two numbers.
	//
	// Mixed int/float pairs are compared as floats, but the original value (and
	// its type) is returned unchanged.
	//
	// @sig     min(a: number, b: number) -> number
	// @param   a  first value
	// @param   b  second value
	// @returns whichever of a or b is smaller, with its original type
	// @errors  TypeError if either argument is not numeric
	// @example min(3, 7)       → 3
	// @example min(1.5, 2.5)   → 1.5
	// @since   0.1.0
	// @see     max, constrain
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

	// max — the larger of two numbers.
	//
	// Mixed int/float pairs are compared as floats, but the original value (and
	// its type) is returned unchanged.
	//
	// @sig     max(a: number, b: number) -> number
	// @param   a  first value
	// @param   b  second value
	// @returns whichever of a or b is larger, with its original type
	// @errors  TypeError if either argument is not numeric
	// @example max(3, 7)       → 7
	// @example max(1.5, 2.5)   → 2.5
	// @since   0.1.0
	// @see     min, constrain
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

	// log — the natural logarithm (base e) of x.
	//
	// @sig     log(x: number) -> float
	// @param   x  a positive integer or float
	// @returns ln(x), always as a float
	// @errors  TypeError if x is not numeric; RuntimeError if x <= 0
	// @example log(1)   → 0
	// @since   0.1.0
	// @see     log2, log10, exp
	Builtins["log"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("log expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("log: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		v := toFloat64(args[0])
		if v <= 0 {
			return runtimeError("log: argument must be positive", ast.Pos{})
		}
		return &Float{Value: math.Log(v)}
	}}

	// log2 — the base-2 logarithm of x.
	//
	// @sig     log2(x: number) -> float
	// @param   x  a positive integer or float
	// @returns log base 2 of x, always as a float
	// @errors  TypeError if x is not numeric; RuntimeError if x <= 0
	// @example log2(8)   → 3
	// @since   0.1.0
	// @see     log, log10
	Builtins["log2"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("log2 expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("log2: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		v := toFloat64(args[0])
		if v <= 0 {
			return runtimeError("log2: argument must be positive", ast.Pos{})
		}
		return &Float{Value: math.Log2(v)}
	}}

	// log10 — the base-10 logarithm of x.
	//
	// @sig     log10(x: number) -> float
	// @param   x  a positive integer or float
	// @returns log base 10 of x, always as a float
	// @errors  TypeError if x is not numeric; RuntimeError if x <= 0
	// @example log10(100)   → 2
	// @since   0.1.0
	// @see     log, log2
	Builtins["log10"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("log10 expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("log10: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		v := toFloat64(args[0])
		if v <= 0 {
			return runtimeError("log10: argument must be positive", ast.Pos{})
		}
		return &Float{Value: math.Log10(v)}
	}}

	// pow — base raised to the power exp.
	//
	// @sig     pow(base: number, exp: number) -> float
	// @param   base  the base
	// @param   exp   the exponent
	// @returns base**exp, always as a float
	// @errors  TypeError if either argument is not numeric
	// @example pow(2, 10)     → 1024
	// @example pow(2.0, 0.5)  → 1.4142135623730951
	// @since   0.1.0
	// @see     sqrt, exp
	Builtins["pow"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("pow expects 2 arguments", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) || !canArithmetic(args[1].Type()) {
			return typeError(fmt.Sprintf("pow: arguments must be integer or float, got %s and %s",
				args[0].Type(), args[1].Type()), ast.Pos{})
		}
		return &Float{Value: math.Pow(toFloat64(args[0]), toFloat64(args[1]))}
	}}

	// abs — the absolute value of x, preserving its type.
	//
	// @sig     abs(x: number) -> number
	// @param   x  an integer or float
	// @returns |x| with the same type as x (int in → int out, float in → float out)
	// @errors  TypeError if x is not numeric
	// @example abs(-3)     → 3
	// @example abs(-1.5)   → 1.5
	// @since   0.1.0
	// @see     min, max
	Builtins["abs"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("abs expects 1 argument", ast.Pos{})
		}
		switch v := args[0].(type) {
		case *Integer:
			if v.Value < 0 {
				return &Integer{Value: -v.Value}
			}
			return v
		case *Float:
			return &Float{Value: math.Abs(v.Value)}
		default:
			return typeError(fmt.Sprintf("abs: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
	}}

	// asin — the arc sine of x, in radians.
	//
	// @sig     asin(x: number) -> float
	// @param   x  a value in [-1, 1]
	// @returns the angle in radians whose sine is x
	// @errors  TypeError if x is not numeric; RuntimeError if x is outside [-1, 1]
	// @example asin(1)   → 1.5707963267948966
	// @since   0.1.0
	// @see     sin, acos, atan
	Builtins["asin"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("asin expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("asin: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		v := toFloat64(args[0])
		if v < -1 || v > 1 {
			return runtimeError("asin: argument must be in [-1, 1]", ast.Pos{})
		}
		return &Float{Value: math.Asin(v)}
	}}

	// acos — the arc cosine of x, in radians.
	//
	// @sig     acos(x: number) -> float
	// @param   x  a value in [-1, 1]
	// @returns the angle in radians whose cosine is x
	// @errors  TypeError if x is not numeric; RuntimeError if x is outside [-1, 1]
	// @example acos(1)   → 0
	// @since   0.1.0
	// @see     cos, asin, atan
	Builtins["acos"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("acos expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("acos: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		v := toFloat64(args[0])
		if v < -1 || v > 1 {
			return runtimeError("acos: argument must be in [-1, 1]", ast.Pos{})
		}
		return &Float{Value: math.Acos(v)}
	}}

	// atan — the arc tangent of x, in radians.
	//
	// @sig     atan(x: number) -> float
	// @param   x  any number
	// @returns the angle in radians whose tangent is x, in (-π/2, π/2)
	// @errors  TypeError if x is not numeric
	// @example atan(1)   → 0.7853981633974483
	// @since   0.1.0
	// @see     tan, atan2, asin, acos
	Builtins["atan"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("atan expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("atan: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		return &Float{Value: math.Atan(toFloat64(args[0]))}
	}}

	// atan2 — the arc tangent of y/x, using both signs to pick the quadrant.
	//
	// Unlike atan(y/x), atan2 returns an angle in the full range (-π, π] because
	// it knows which quadrant (y, x) is in.
	//
	// @sig     atan2(y: number, x: number) -> float
	// @param   y  the ordinate (vertical component)
	// @param   x  the abscissa (horizontal component)
	// @returns the angle in radians from the positive x-axis to the point (x, y)
	// @errors  TypeError if either argument is not numeric
	// @example atan2(1.0, 1.0)   → 0.7853981633974483
	// @since   0.1.0
	// @see     atan
	Builtins["atan2"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("atan2 expects 2 arguments", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) || !canArithmetic(args[1].Type()) {
			return typeError(fmt.Sprintf("atan2: arguments must be integer or float, got %s and %s",
				args[0].Type(), args[1].Type()), ast.Pos{})
		}
		return &Float{Value: math.Atan2(toFloat64(args[0]), toFloat64(args[1]))}
	}}

	// exp — e raised to the power x.
	//
	// @sig     exp(x: number) -> float
	// @param   x  the exponent
	// @returns e**x, always as a float
	// @errors  TypeError if x is not numeric
	// @example exp(0)   → 1
	// @since   0.1.0
	// @see     log, pow, e
	Builtins["exp"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("exp expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("exp: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		return &Float{Value: math.Exp(toFloat64(args[0]))}
	}}

	// pi — the mathematical constant π.
	//
	// @sig     pi() -> float
	// @returns π (3.141592653589793)
	// @errors  RuntimeError if called with any arguments
	// @example pi()   → 3.141592653589793
	// @since   0.1.0
	// @see     e, sin, cos
	Builtins["pi"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("pi expects no arguments", ast.Pos{})
		}
		return &Float{Value: math.Pi}
	}}

	// e — Euler's number.
	//
	// @sig     e() -> float
	// @returns e (2.718281828459045)
	// @errors  RuntimeError if called with any arguments
	// @example e()   → 2.718281828459045
	// @since   0.1.0
	// @see     pi, exp, log
	Builtins["e"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("e expects no arguments", ast.Pos{})
		}
		return &Float{Value: math.E}
	}}

	// mod — the integer remainder of a divided by b.
	//
	// The result takes the sign of the dividend a (Go's % semantics), so
	// mod(-10, 3) is -1, not 2.
	//
	// @sig     mod(a: int, b: int) -> int
	// @param   a  the dividend
	// @param   b  the divisor (must be non-zero)
	// @returns the remainder a % b, with the sign of a
	// @errors  TypeError if either argument is not an integer; RuntimeError on division by zero
	// @example mod(10, 3)    → 1
	// @example mod(-10, 3)   → -1
	// @since   0.1.0
	// @see     fmod
	Builtins["mod"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("mod expects 2 arguments", ast.Pos{})
		}
		a, aOk := args[0].(*Integer)
		b, bOk := args[1].(*Integer)
		if !aOk || !bOk {
			return typeError(fmt.Sprintf("mod: arguments must be integers, got %s and %s",
				args[0].Type(), args[1].Type()), ast.Pos{})
		}
		if b.Value == 0 {
			return runtimeError("mod: division by zero", ast.Pos{})
		}
		return &Integer{Value: a.Value % b.Value}
	}}

	// fmod — the floating-point remainder of a divided by b.
	//
	// @sig     fmod(a: float, b: float) -> float
	// @param   a  the dividend (must be a float)
	// @param   b  the divisor (must be a non-zero float)
	// @returns the floating-point remainder of a/b
	// @errors  TypeError if either argument is not a float; RuntimeError on division by zero
	// @example fmod(10.0, 3.0)   → 1
	// @since   0.1.0
	// @see     mod
	Builtins["fmod"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("fmod expects 2 arguments", ast.Pos{})
		}
		a, aOk := args[0].(*Float)
		b, bOk := args[1].(*Float)
		if !aOk || !bOk {
			return typeError(fmt.Sprintf("fmod: arguments must be floats, got %s and %s",
				args[0].Type(), args[1].Type()), ast.Pos{})
		}
		if b.Value == 0 {
			return runtimeError("fmod: division by zero", ast.Pos{})
		}
		return &Float{Value: math.Mod(a.Value, b.Value)}
	}}

	// remap — re-scale val from one range to another (not clamped).
	//
	// Linearly maps val from [inLow, inHigh] onto [outLow, outHigh]. Named remap
	// (not map) to avoid colliding with the higher-order map(arr, fn).
	//
	// @sig     remap(val: number, inLow: number, inHigh: number, outLow: number, outHigh: number) -> float
	// @param   val      the value to re-scale
	// @param   inLow    low end of the input range
	// @param   inHigh   high end of the input range
	// @param   outLow   low end of the output range
	// @param   outHigh  high end of the output range
	// @returns val mapped into the output range; not clamped, so it may fall outside it
	// @errors  TypeError if any argument is not numeric
	// @example remap(5, 0, 10, 0, 100)   → 50
	// @since   0.1.0
	// @see     lerp, constrain
	Builtins["remap"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 5 {
			return typeError("remap expects 5 arguments: val, inLow, inHigh, outLow, outHigh", ast.Pos{})
		}
		for _, a := range args {
			if !canArithmetic(a.Type()) {
				return typeError("remap: all arguments must be numeric", ast.Pos{})
			}
		}
		val := toFloat64(args[0])
		inLow := toFloat64(args[1])
		inHigh := toFloat64(args[2])
		outLow := toFloat64(args[3])
		outHigh := toFloat64(args[4])
		if inHigh == inLow {
			return &Float{Value: outLow}
		}
		return &Float{Value: outLow + (val-inLow)/(inHigh-inLow)*(outHigh-outLow)}
	}}

	// constrain — clamp val to the range [lo, hi].
	//
	// @sig     constrain(val: number, lo: number, hi: number) -> number
	// @param   val  the value to clamp
	// @param   lo   lower bound
	// @param   hi   upper bound
	// @returns val limited to [lo, hi]; an integer val yields an integer, otherwise a float
	// @errors  TypeError if any argument is not numeric
	// @example constrain(15, 0, 10)   → 10
	// @since   0.1.0
	// @see     min, max, remap
	Builtins["constrain"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return typeError("constrain expects 3 arguments: val, lo, hi", ast.Pos{})
		}
		for _, a := range args {
			if !canArithmetic(a.Type()) {
				return typeError("constrain: all arguments must be numeric", ast.Pos{})
			}
		}
		val := toFloat64(args[0])
		lo := toFloat64(args[1])
		hi := toFloat64(args[2])
		if val < lo {
			val = lo
		}
		if val > hi {
			val = hi
		}
		if _, ok := args[0].(*Integer); ok {
			return &Integer{Value: int(val)}
		}
		return &Float{Value: val}
	}}

	// lerp — linear interpolation between a and b by fraction t.
	//
	// Computes a + (b-a)*t: returns a at t=0, b at t=1. Not clamped, so t outside
	// [0, 1] extrapolates beyond the endpoints.
	//
	// @sig     lerp(a: number, b: number, t: number) -> float
	// @param   a  start value (returned at t=0)
	// @param   b  end value (returned at t=1)
	// @param   t  interpolation fraction, normally in [0, 1]
	// @returns a + (b-a)*t, always as a float
	// @errors  TypeError if any argument is not numeric
	// @example lerp(0, 10, 0.5)   → 5
	// @since   0.1.0
	// @see     remap, constrain
	Builtins["lerp"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return typeError("lerp expects 3 arguments: a, b, t", ast.Pos{})
		}
		for _, a := range args {
			if !canArithmetic(a.Type()) {
				return typeError("lerp: all arguments must be numeric", ast.Pos{})
			}
		}
		a := toFloat64(args[0])
		b := toFloat64(args[1])
		t := toFloat64(args[2])
		return &Float{Value: a + (b-a)*t}
	}}

	// hsl — convert an HSL colour to an [r, g, b, a] float array.
	//
	// The result plugs straight into fill(), gradient(), and theme slots. All
	// components are in [0.0, 1.0]; alpha defaults to 1.0.
	//
	// @sig     hsl(h: number, s: number, l: number, [a: number]) -> array
	// @param   h  hue, 0.0..1.0
	// @param   s  saturation, 0.0..1.0
	// @param   l  lightness, 0.0..1.0
	// @param   a  alpha, 0.0..1.0; defaults to 1.0
	// @returns a 4-element float array [r, g, b, a]
	// @errors  TypeError if any argument is not numeric
	// @example hsl(0, 0, 0.5)   → [0.5, 0.5, 0.5, 1]
	// @since   0.1.0
	// @see     fill, gradient
	Builtins["hsl"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 3 || len(args) > 4 {
			return typeError("hsl expects 3-4 arguments: h, s, l [, a]", ast.Pos{})
		}
		for _, a := range args {
			if !canArithmetic(a.Type()) {
				return typeError("hsl: all arguments must be numeric (0.0-1.0)", ast.Pos{})
			}
		}
		h := toFloat64(args[0])
		s := toFloat64(args[1])
		l := toFloat64(args[2])
		a := 1.0
		if len(args) == 4 {
			a = toFloat64(args[3])
		}
		var r, g, b float64
		if s == 0 {
			r, g, b = l, l, l
		} else {
			hue2rgb := func(p, q, t float64) float64 {
				if t < 0 {
					t += 1
				}
				if t > 1 {
					t -= 1
				}
				if t < 1.0/6.0 {
					return p + (q-p)*6*t
				}
				if t < 0.5 {
					return q
				}
				if t < 2.0/3.0 {
					return p + (q-p)*(2.0/3.0-t)*6
				}
				return p
			}
			var q float64
			if l < 0.5 {
				q = l * (1 + s)
			} else {
				q = l + s - l*s
			}
			p := 2*l - q
			r = hue2rgb(p, q, h+1.0/3.0)
			g = hue2rgb(p, q, h)
			b = hue2rgb(p, q, h-1.0/3.0)
		}
		return &Array{Elements: []Object{
			&Float{Value: r},
			&Float{Value: g},
			&Float{Value: b},
			&Float{Value: a},
		}}
	}}
}
