// assertTest.lex — tests for the assert() builtin

// Pass — no message
assert(true)
println("bare assert passed")

// Pass — with message
assert(1 + 1 == 2, "maths is broken")
println("assert with message passed")

// Fail — default message
result, err = safe(fn() { assert(false) })
println(err.message)

// Fail — custom message
result, err = safe(fn() { assert(1 == 2, "expected 1 to equal 2") })
println(err.message)

// TypeError — non-bool condition
result, err = safe(fn() { assert(42) })
println(err.message)

// TypeError — non-string message
result, err = safe(fn() { assert(false, 99) })
println(err.message)

// Practical use: precondition in a function
fn divide(a, b) {
    assert(b != 0, "divide: denominator must not be zero")
    return a / b
}
println(divide(10, 2))
result, err = safe(divide, 10, 0)
println(err.message)

// assert with comparison expressions
x = 42
assert(x > 0, "x must be positive")
assert(x == 42, "x must be 42")
assert(type(x) == "INTEGER", "x must be an integer")
println("all assertions passed")
