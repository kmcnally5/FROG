// stdlib/math.lex — kLex standard math library
//
// Provides mathematical operations for both integers and floats.
//
// Core float operations (floor, ceil, round, sqrt) are built into the
// interpreter and do not require an import — they are always available.
// This module provides additional utilities built on top of those primitives.
//
// Usage:
//   import "math.lex" as math
//   println(math.abs(-5.5))    // 5.5
//   println(math.max(3, 7))    // 7
//   println(math.pi)           // 3.141592653589793

// pi — the ratio of a circle's circumference to its diameter.
const pi = 3.141592653589793

// e — Euler's number, the base of the natural logarithm.
const e = 2.718281828459045


// ----------------------------------------------------------------------------
// Works for both integers and floats
// ----------------------------------------------------------------------------

// abs returns the absolute value of n.
// abs(-5) → 5    abs(-3.7) → 3.7    abs(4) → 4
fn abs(n) {
    if n < 0 { return -n }
    return n
}

// max returns the larger of a and b.
// Works for integers and floats. Mixed types are compared as floats.
// max(3, 7) → 7    max(1.5, 1.2) → 1.5
fn max(a, b) {
    if a > b { return a }
    return b
}

// min returns the smaller of a and b.
// Works for integers and floats. Mixed types are compared as floats.
// min(3, 7) → 3    min(1.5, 1.2) → 1.2
fn min(a, b) {
    if a < b { return a }
    return b
}

// clamp returns n constrained to the range [lo, hi].
// Works for integers and floats.
// clamp(10, 0, 5) → 5    clamp(-1.0, 0.0, 1.0) → 0.0
fn clamp(n, lo, hi) {
    if n < lo { return lo }
    if n > hi { return hi }
    return n
}

// sign returns 1 if n is positive, -1 if negative, 0 if zero.
// Works for integers and floats.
fn sign(n) {
    if n > 0 { return 1 }
    if n < 0 { return -1 }
    return 0
}


// ----------------------------------------------------------------------------
// Integer-only operations
// ----------------------------------------------------------------------------

// pow returns base raised to the power of exp (non-negative integer exponent).
// pow(2, 0) → 1    pow(2, 10) → 1024
// For float exponentiation use sqrt repeatedly or Newton's method.
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

// sum returns the total of all elements in an array of numbers.
fn sum(arr) {
    return reduce(arr, fn(acc, x) { acc + x }, 0)
}

// product returns the product of all elements in an array of numbers.
fn product(arr) {
    return reduce(arr, fn(acc, x) { acc * x }, 1)
}

// even returns true if n is divisible by 2 (integers only).
fn even(n) {
    return n % 2 == 0
}

// odd returns true if n is not divisible by 2 (integers only).
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


// A simple Discrete Cosine Transform implementation
PI = 3.14159265358979

// math.lex

fn dct_basis(u, v, x, y) {
    let cu = 1.0
    if u == 0 { 
        cu = 0.7071 
    }

    let cv = 1.0
    if v == 0 { 
        cv = 0.7071 
    }
    
    return cu * cv * cos((2 * x + 1) * u * PI / 16) * cos((2 * y + 1) * v * PI / 16)
}

fn apply_dct(block8x8) {
    let output = range(64) // 8x8 flattened
    for u in range(8) {
        for v in range(8) {
            let sum = 0.0
            for x in range(8) {
                for y in range(8) {
                    sum = sum + block8x8[x*8 + y] * dct_basis(u, v, x, y)
                }
            }
            output[u*8 + v] = sum / 4
        }
    }
    return output
}