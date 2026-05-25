// Tier 16 — closures.
//
// Exercises Lua-style upvalue capture: an inner function references
// a name from its enclosing scope; the compiler registers an upvalue,
// OpMakeClosure captures the parent cell, OpGetUpvalue reads through
// the shared cell. Mutations propagate.

// Top-level cross-function calls — `useOk` referencing `ok` resolves
// through the top-level → empty-outer chain. Confirms `ok` lookup
// from inside another top-level function works.
fn ok() {
    return 42
}
fn useOk() {
    return ok() + 1
}
println(useOk())

// Square + sumSquares — the test that blocked in tier 11.
fn square(x) { return x * x }
fn sumSquares(a, b) {
    return square(a) + square(b)
}
println(sumSquares(3, 4))

// Multi-step pipeline using closures across top-level functions
fn double(x) { return x * 2 }
fn inc(n) { return n + 1 }
fn pipeline(v) {
    return inc(double(v))
}
println(pipeline(5))

// Captured constants — reading a top-level value from inside a fn
const PI = 314
fn circumference(radius) {
    return radius * PI * 2
}
println(circumference(10))

// Mutual references in both directions
fn isEven(n) {
    if n == 0 { return true }
    return isOdd(n - 1)
}
fn isOdd(n) {
    if n == 0 { return false }
    return isEven(n - 1)
}
println(isEven(4))
println(isOdd(7))
println(isEven(7))
