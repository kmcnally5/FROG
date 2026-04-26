// stdlib/math.lex — kLex standard math library
//
// Provides common mathematical operations that are not built into the language.
// All functions work on integers only — kLex does not have floats.
//
// Usage:
//   import "math.lex" as math
//   println(math.abs(-5))    // 5
//   println(math.max(3, 7))  // 7

// abs returns the absolute value of n.
fn abs(n) {
    if n < 0 { return -n }
    return n
}

// max returns the larger of two integers.
fn max(a, b) {
    if a > b { return a }
    return b
}

// min returns the smaller of two integers.
fn min(a, b) {
    if a < b { return a }
    return b
}

// clamp returns n constrained to the range [lo, hi].
// If n < lo returns lo. If n > hi returns hi. Otherwise returns n.
fn clamp(n, lo, hi) {
    if n < lo { return lo }
    if n > hi { return hi }
    return n
}

// pow returns base raised to the power of exp (non-negative integers only).
// pow(2, 0) returns 1. pow(2, 3) returns 8.
fn pow(base, exp) {
    if exp == 0 { return 1 }
    result = 1
    i = 0
    while i < exp {
        result = result * base
        i = i + 1
    }
    return result
}

// sum returns the total of all elements in an array of integers.
fn sum(arr) {
    return reduce(arr, fn(acc, x) { acc + x }, 0)
}

// product returns the product of all elements in an array of integers.
fn product(arr) {
    return reduce(arr, fn(acc, x) { acc * x }, 1)
}

// sign returns 1 if n is positive, -1 if negative, 0 if zero.
fn sign(n) {
    if n > 0 { return 1 }
    if n < 0 { return -1 }
    return 0
}

// even returns true if n is divisible by 2.
fn even(n) {
    return n % 2 == 0
}

// odd returns true if n is not divisible by 2.
fn odd(n) {
    return n % 2 != 0
}

// gcd returns the greatest common divisor of two positive integers.
// Uses Euclid's algorithm.
fn gcd(a, b) {
    while b != 0 {
        t = b
        b = a % b
        a = t
    }
    return a
}

// lcm returns the least common multiple of two positive integers.
fn lcm(a, b) {
    return (a * b) / gcd(a, b)
}
