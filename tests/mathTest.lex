// floor
assert(floor(3.7)  == 3,  "floor(3.7)")
assert(floor(3.0)  == 3,  "floor(3.0)")
assert(floor(-2.3) == -3, "floor(-2.3)")
assert(floor(4)    == 4,  "floor(int)")

// ceil
assert(ceil(3.2)  == 4,  "ceil(3.2)")
assert(ceil(3.0)  == 3,  "ceil(3.0)")
assert(ceil(-2.7) == -2, "ceil(-2.7)")
assert(ceil(4)    == 4,  "ceil(int)")

// round
assert(round(3.5) == 4,  "round(3.5)")
assert(round(3.4) == 3,  "round(3.4)")
assert(round(-2.5) == -3, "round(-2.5)")
assert(round(4)   == 4,  "round(int)")

// sqrt
assert(sqrt(4)   == 2.0, "sqrt(4)")
assert(sqrt(9.0) == 3.0, "sqrt(9.0)")
assert(sqrt(0)   == 0.0, "sqrt(0)")

// math.lex — abs, min, max, clamp with floats
import "math.lex" as math

assert(math.abs(-3.7)  == 3.7,  "abs float")
assert(math.abs(-5)    == 5,    "abs int")
assert(math.max(1.5, 2.5) == 2.5, "max float")
assert(math.min(1.5, 2.5) == 1.5, "min float")
assert(math.clamp(5.5, 0.0, 3.0) == 3.0, "clamp float hi")
assert(math.clamp(-1.0, 0.0, 1.0) == 0.0, "clamp float lo")
assert(math.pi > 3.14 && math.pi < 3.15, "pi")
assert(math.e  > 2.71 && math.e  < 2.72, "e")

println("math tests passed")
