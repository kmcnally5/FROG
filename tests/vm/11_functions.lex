// Tier 11 — user-defined functions: define, call, return, recurse.
//
// Exercises FunctionLiteral compilation (sub-chunk + CompiledFunction
// constant), CallExpr's user-function dispatch path, and OpCall's
// recursive execute() invocation.

// Basic def + call
fn double(x) {
    return x * 2
}
println(double(7))

// Multiple params
fn add(a, b) {
    return a + b
}
println(add(3, 4))
println(add(10, 20))

// Explicit return. (kLex's tree-walker returns the last statement's
// value implicitly; the VM currently always returns null when the
// body falls off the bottom. Documented divergence — closure pass
// will revisit. Use `return` explicitly for now.)
fn returns42() {
    return 42
}
println(returns42())

// Function with conditional
fn abs(n) {
    if n < 0 {
        return 0 - n
    } else {
        return n
    }
}
println(abs(-7))
println(abs(7))
println(abs(0))

// Recursion — factorial
fn fact(n) {
    if n <= 1 {
        return 1
    }
    return n * fact(n - 1)
}
println(fact(5))
println(fact(10))

// Recursion — fibonacci
fn fib(n) {
    if n < 2 {
        return n
    }
    return fib(n - 1) + fib(n - 2)
}
println(fib(0))
println(fib(1))
println(fib(10))

// Function calling another function — DEFERRED until closures land.
// `sumSquares` referencing the outer-scope `square` needs upvalue
// support that the current self-slot trick doesn't cover. See
// project_vm_bytecode.md "Known divergences (deferred)".
//
// fn square(x) { return x * x }
// fn sumSquares(a, b) {
//     return square(a) + square(b)
// }
// println(sumSquares(3, 4))
