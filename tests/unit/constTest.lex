// constTest.lex — tests for immutable const bindings

// Basic declaration and read
const PI = 3.14159
println(PI)

// Reassignment from same scope is a RuntimeError
result, err = safe(fn() { PI = 3 })
println(err.message)

// Reassignment from inner function scope is also blocked
fn tryMutate() { PI = 999 }
result, err = safe(tryMutate)
println(err.message)

// let in the same scope as a const is blocked
result, err = safe(fn() {
    const X = 1
    let X = 2
})
println(err.message)

// let in an INNER scope may shadow without modifying the outer const
const BASE = 10
fn shadow() {
    let BASE = 999
    return BASE
}
println(shadow())   // 999
println(BASE)       // 10 — outer const unaffected

// const with a function value
const double = fn(x) { return x * 2 }
println(double(5))
result, err = safe(fn() { double = fn(x) { return x * 3 } })
println(err.message)

// multi-assign cannot overwrite a const
const ANSWER = 42
result, err = safe(fn() {
    ANSWER, x = safe(fn() { return 42 })
})
println(err.message)

// existing let and bare assignment are unaffected
let y = 100
y = 200
println(y)

// const in a function scope
fn makeConfig() {
    const HOST = "localhost"
    const PORT = 8080
    return HOST + ":" + str(PORT)
}
println(makeConfig())

// const with expression value
const MAX = 10 * 10
println(MAX)
