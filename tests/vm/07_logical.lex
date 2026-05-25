// Tier 7 — && / || short-circuit semantics.
//
// kLex's && returns the SECOND operand when the first is true,
// the first when the first is false. Same as Python's `and` /
// JavaScript's `&&`. Likewise || returns the first true value or
// the last value.

// && — basic truth tables
println(true && true)
println(true && false)
println(false && true)
println(false && false)

// || — basic truth tables
println(true || true)
println(true || false)
println(false || true)
println(false || false)

// Combined with comparison
let x = 5
if x > 0 && x < 10 {
    println("0 < x < 10")
}
if x < 0 || x > 100 {
    println("should NOT print")
} else {
    println("x in [0, 100]")
}

// Short-circuit avoids evaluating the right operand when not needed.
// (We can't easily PROVE this without side-effects, but the result
// type should match: both Booleans.)
println(false && false)
println(true || false)
