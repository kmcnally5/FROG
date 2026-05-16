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

// sin / cos / tan
assert(sin(0)  == 0.0, "sin(0)")
assert(cos(0)  == 1.0, "cos(0)")
assert(tan(0)  == 0.0, "tan(0)")

// abs — returns same type as input
assert(abs(-42)   == 42,   "abs int neg")
assert(abs(5)     == 5,    "abs int pos")
assert(abs(-3.14) == 3.14, "abs float neg")
assert(abs(2.5)   == 2.5,  "abs float pos")

// log / log2 / log10 — exact for clean powers
assert(log(1)     == 0.0, "log(1)")
assert(log2(8)    == 3.0, "log2(8)")
assert(log2(1)    == 0.0, "log2(1)")
assert(log10(100) == 2.0, "log10(100)")
assert(log10(1)   == 0.0, "log10(1)")

// pow
assert(pow(2, 10)  == 1024.0, "pow(2,10)")
assert(pow(2, 0)   == 1.0,    "pow(2,0)")
assert(pow(9, 0.5) == 3.0,    "pow(9,0.5)")

// exp
assert(exp(0) == 1.0, "exp(0)")
assert(abs(exp(1) - 2.718281828459045) < 0.000001, "exp(1)")

// pi() / e()
assert(abs(pi() - 3.141592653589793) < 0.000001, "pi()")
assert(abs(e()  - 2.718281828459045) < 0.000001, "e()")

// asin / acos — exact for boundary values
assert(asin(0.0) == 0.0, "asin(0)")
assert(acos(1.0) == 0.0, "acos(1)")
assert(abs(asin(1.0) - 1.5707963267948966) < 0.000001, "asin(1)")
assert(abs(acos(0.0) - 1.5707963267948966) < 0.000001, "acos(0)")

// atan / atan2
assert(atan(0)           == 0.0, "atan(0)")
assert(abs(atan(1.0)          - 0.7853981633974483) < 0.000001, "atan(1)")
assert(abs(atan2(1.0, 1.0)    - 0.7853981633974483) < 0.000001, "atan2(1,1)")
assert(abs(atan2(0.0, -1.0)   - pi())               < 0.000001, "atan2(0,-1) == pi")

// mod — integer remainder
assert(mod(10, 3)  == 1,  "mod(10,3)")
assert(mod(9, 3)   == 0,  "mod(9,3)")
assert(mod(-10, 3) == -1, "mod(-10,3)")

// fmod — float remainder
assert(abs(fmod(10.5, 3.2) - 0.9) < 0.0001, "fmod(10.5,3.2)")
assert(fmod(9.0, 3.0)      == 0.0,           "fmod(9.0,3.0)")

// stdlib/math.lex
import "stdlib/math.lex" as math

// clamp / sign
assert(math.clamp(5.5, 0.0, 3.0)  == 3.0,  "clamp hi")
assert(math.clamp(-1.0, 0.0, 1.0) == 0.0,  "clamp lo")
assert(math.clamp(0.5, 0.0, 1.0)  == 0.5,  "clamp mid")
assert(math.sign(5)   == 1,  "sign pos")
assert(math.sign(-3)  == -1, "sign neg")
assert(math.sign(0)   == 0,  "sign zero")

// angle conversion
assert(abs(math.degrees(pi())     - 180.0) < 0.000001, "degrees(pi)")
assert(abs(math.radians(180.0)    - pi())  < 0.000001, "radians(180)")
assert(abs(math.degrees(pi()/2.0) - 90.0) < 0.000001, "degrees(pi/2)")

// geometry
assert(math.hypot(3, 4)   == 5.0,  "hypot 3-4-5")
assert(math.hypot(0, 5.0) == 5.0,  "hypot 0-5")

// interpolation
assert(math.lerp(0.0, 10.0, 0.0)  == 0.0,  "lerp t=0")
assert(math.lerp(0.0, 10.0, 1.0)  == 10.0, "lerp t=1")
assert(math.lerp(0.0, 10.0, 0.5)  == 5.0,  "lerp t=0.5")

// logBase
assert(abs(math.logBase(8, 2)    - 3.0) < 0.000001, "logBase(8,2)")
assert(abs(math.logBase(1000, 10) - 3.0) < 0.000001, "logBase(1000,10)")

// integer utilities
assert(math.even(4)  == true,  "even(4)")
assert(math.even(3)  == false, "even(3)")
assert(math.odd(7)   == true,  "odd(7)")
assert(math.odd(8)   == false, "odd(8)")
assert(math.factorial(0) == 1,   "factorial(0)")
assert(math.factorial(5) == 120, "factorial(5)")
assert(math.gcd(12, 8)   == 4,   "gcd(12,8)")
assert(math.lcm(4, 6)    == 12,  "lcm(4,6)")
assert(math.isPrime(2)   == true,  "isPrime(2)")
assert(math.isPrime(97)  == true,  "isPrime(97)")
assert(math.isPrime(1)   == false, "isPrime(1)")
assert(math.isPrime(9)   == false, "isPrime(9)")

// array / statistics
assert(math.sum([1, 2, 3, 4])     == 10,  "sum")
assert(math.product([1, 2, 3, 4]) == 24,  "product")
assert(math.mean([1, 2, 3, 4])    == 2.5, "mean")
assert(abs(math.variance([2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0]) - 4.0) < 0.000001, "variance")
assert(abs(math.stddev([2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0])  - 2.0) < 0.000001, "stddev")

println("math tests passed")
