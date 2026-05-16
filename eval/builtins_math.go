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

	// log returns the natural logarithm of x. Always returns Float.
	// x must be positive.
	// log(1) → 0.0    log(2.718281828) ≈ 1.0
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

	// log2 returns the base-2 logarithm of x. Always returns Float.
	// x must be positive.
	// log2(8) → 3.0    log2(1) → 0.0
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

	// log10 returns the base-10 logarithm of x. Always returns Float.
	// x must be positive.
	// log10(100) → 2.0    log10(1) → 0.0
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

	// pow returns base raised to the power exp. Always returns Float.
	// pow(2, 10) → 1024.0    pow(2.0, 0.5) → 1.4142135623730951
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

	// abs returns the absolute value of x. Returns the same type as input.
	// abs(-3) → 3    abs(-1.5) → 1.5    abs(4) → 4
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

	// asin returns the arc sine of x in radians. Always returns Float.
	// x must be in [-1, 1].
	// asin(1) → 1.5707963267948966 (π/2)
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

	// acos returns the arc cosine of x in radians. Always returns Float.
	// x must be in [-1, 1].
	// acos(1) → 0.0    acos(0) → 1.5707963267948966 (π/2)
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

	// atan returns the arc tangent of x in radians. Always returns Float.
	// atan(1) → 0.7853981633974483 (π/4)
	Builtins["atan"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("atan expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("atan: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		return &Float{Value: math.Atan(toFloat64(args[0]))}
	}}

	// atan2 returns the arc tangent of y/x in radians, using the signs of both
	// arguments to determine the correct quadrant. Always returns Float.
	// atan2(1.0, 1.0) → 0.7853981633974483 (π/4)
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

	// exp returns e raised to the power x. Always returns Float.
	// exp(1) → 2.718281828459045    exp(0) → 1.0
	Builtins["exp"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("exp expects 1 argument", ast.Pos{})
		}
		if !canArithmetic(args[0].Type()) {
			return typeError(fmt.Sprintf("exp: argument must be integer or float, got %s", args[0].Type()), ast.Pos{})
		}
		return &Float{Value: math.Exp(toFloat64(args[0]))}
	}}

	// pi returns the mathematical constant π. Always returns Float.
	// pi() → 3.141592653589793
	Builtins["pi"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("pi expects no arguments", ast.Pos{})
		}
		return &Float{Value: math.Pi}
	}}

	// e returns Euler's number. Always returns Float.
	// e() → 2.718281828459045
	Builtins["e"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("e expects no arguments", ast.Pos{})
		}
		return &Float{Value: math.E}
	}}

	// mod returns the integer remainder of a divided by b.
	// Both arguments must be integers.
	// mod(10, 3) → 1    mod(-10, 3) → -1
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

	// fmod returns the floating-point remainder of a divided by b.
	// Both arguments must be floats.
	// fmod(10.5, 3.2) → 0.9000000000000004
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

	// remap(val, inLow, inHigh, outLow, outHigh) → float
	// Re-map val from [inLow, inHigh] to [outLow, outHigh]. Not clamped.
	// Named remap (not map) to avoid collision with the higher-order map(arr, fn).
	Builtins["remap"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 5 {
			return typeError("remap expects 5 arguments: val, inLow, inHigh, outLow, outHigh", ast.Pos{})
		}
		for _, a := range args {
			if !canArithmetic(a.Type()) {
				return typeError("remap: all arguments must be numeric", ast.Pos{})
			}
		}
		val     := toFloat64(args[0])
		inLow   := toFloat64(args[1])
		inHigh  := toFloat64(args[2])
		outLow  := toFloat64(args[3])
		outHigh := toFloat64(args[4])
		if inHigh == inLow {
			return &Float{Value: outLow}
		}
		return &Float{Value: outLow + (val-inLow)/(inHigh-inLow)*(outHigh-outLow)}
	}}

	// constrain(val, lo, hi) → number
	// Clamp val to [lo, hi]. Returns integer if val is integer, float otherwise.
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
		lo  := toFloat64(args[1])
		hi  := toFloat64(args[2])
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

	// lerp(a, b, t) → float
	// Linear interpolation: a + (b-a)*t. Returns a at t=0, b at t=1. Not clamped.
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

	// hsl(h, s, l [, a]) → [r, g, b, a]
	// Convert HSL colour to a float array compatible with fill(), gradient(), and theme slots.
	// h, s, l, a all in [0.0, 1.0]. Alpha defaults to 1.0.
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
				if t < 0 { t += 1 }
				if t > 1 { t -= 1 }
				if t < 1.0/6.0 { return p + (q-p)*6*t }
				if t < 0.5 { return q }
				if t < 2.0/3.0 { return p + (q-p)*(2.0/3.0-t)*6 }
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
