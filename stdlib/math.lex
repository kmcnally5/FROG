// stdlib/math.lex — kLex standard math library
//
// All core math functions (floor, ceil, round, sqrt, abs, pow, exp,
// log, log2, log10, sin, cos, tan, asin, acos, atan, atan2,
// pi, e, mod, fmod, min, max) are built-in and always available
// without importing this module.
//
// This module provides higher-level utilities built on those primitives.
//
// Usage:
//   import "stdlib/math.lex" as math
//   println(math.lerp(0.0, 10.0, 0.5))   // 5.0
//   println(math.degrees(pi()))           // 180.0


// ----------------------------------------------------------------------------
// Angle conversion
// ----------------------------------------------------------------------------

fn degrees(rad) {
    return rad * 180.0 / pi()
}

fn radians(deg) {
    return deg * pi() / 180.0
}


// ----------------------------------------------------------------------------
// Geometry
// ----------------------------------------------------------------------------

// hypot returns the length of the hypotenuse of a right triangle with
// legs a and b. Avoids overflow compared to manual sqrt(a*a + b*b).
// hypot(3, 4) → 5.0
fn hypot(a, b) {
    return sqrt(float(a)*float(a) + float(b)*float(b))
}


// ----------------------------------------------------------------------------
// Interpolation
// ----------------------------------------------------------------------------

// lerp returns the linear interpolation between a and b at position t.
// t = 0.0 returns a, t = 1.0 returns b. t is not clamped.
// lerp(0.0, 10.0, 0.25) → 2.5
fn lerp(a, b, t) {
    return float(a) + float(t) * (float(b) - float(a))
}


// ----------------------------------------------------------------------------
// Clamping and sign
// ----------------------------------------------------------------------------

// clamp returns n constrained to [lo, hi].
// clamp(10, 0, 5) → 5    clamp(-1.0, 0.0, 1.0) → 0.0
fn clamp(n, lo, hi) {
    if n < lo { return lo }
    if n > hi { return hi }
    return n
}

// sign returns 1 if n > 0, -1 if n < 0, 0 if n == 0.
fn sign(n) {
    if n > 0 { return 1 }
    if n < 0 { return -1 }
    return 0
}


// ----------------------------------------------------------------------------
// Logarithm
// ----------------------------------------------------------------------------

// logBase returns the logarithm of x in the given base.
// logBase(8, 2) → 3.0    logBase(1000, 10) → 3.0
// RuntimeError if x <= 0 or base <= 0. Error if base == 1.
fn logBase(x, base) {
    let d = log(base)
    if d == 0.0 { return error("logBase: base cannot be 1", "INVALID_ARG") }
    return log(x) / d
}


// ----------------------------------------------------------------------------
// Integer utilities
// ----------------------------------------------------------------------------

fn even(n) { return mod(n, 2) == 0 }
fn odd(n)  { return mod(n, 2) != 0 }

// factorial returns n! for non-negative integers.
// factorial(0) → 1    factorial(5) → 120
fn factorial(n) {
    if n < 0 { return error("factorial: n must be non-negative", "INVALID_ARG") }
    let result = 1
    let i = 2
    while i <= n {
        result = result * i
        i = i + 1
    }
    return result
}

// gcd returns the greatest common divisor of two positive integers.
fn gcd(a, b) {
    while b != 0 {
        let t = b
        b = mod(a, b)
        a = t
    }
    return a
}

// lcm returns the least common multiple of two positive integers.
fn lcm(a, b) {
    return (a * b) / gcd(a, b)
}

// isPrime returns true if n is a prime number.
// isPrime(2) → true    isPrime(9) → false    isPrime(97) → true
fn isPrime(n) {
    if n < 2 { return false }
    if n == 2 { return true }
    if mod(n, 2) == 0 { return false }
    let i = 3
    while i * i <= n {
        if mod(n, i) == 0 { return false }
        i = i + 2
    }
    return true
}


// ----------------------------------------------------------------------------
// Array / statistics
// ----------------------------------------------------------------------------

// sum returns the total of all elements in an array of numbers.
fn sum(arr) {
    return reduce(arr, fn(acc, x) { return acc + x }, 0)
}

// product returns the product of all elements in an array of numbers.
fn product(arr) {
    return reduce(arr, fn(acc, x) { return acc * x }, 1)
}

// mean returns the arithmetic mean of an array of numbers. Always Float.
// mean([1, 2, 3, 4]) → 2.5
fn mean(arr) {
    return float(sum(arr)) / float(len(arr))
}

// variance returns the population variance of an array of numbers.
fn variance(arr) {
    let m = mean(arr)
    let n = len(arr)
    let total = 0.0
    let i = 0
    while i < n {
        let d = float(arr[i]) - m
        total = total + d * d
        i = i + 1
    }
    return total / float(n)
}

// stddev returns the population standard deviation of an array of numbers.
fn stddev(arr) {
    return sqrt(variance(arr))
}
