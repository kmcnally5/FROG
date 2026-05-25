// Tier 4 — all six comparison operators + ==/!= for the strict
// types kLex supports.

// Numeric comparisons
println(3 < 5)
println(3 > 5)
println(3 <= 3)
println(3 >= 4)

// Equality across primitive types — kLex is strict: comparing
// Int to Float is a TypeError, not silent promotion. We test the
// same-type cases here; the cross-type-error path needs separate
// VM error-propagation work to round-trip identically.
println(42 == 42)
println(42 != 43)
println(1.5 == 1.5)
println("a" == "a")
println("a" == "b")
println(true == true)
println(null == null)
println(null == 0)

// String ordering (lexicographic)
println("apple" < "banana")
println("banana" < "apple")
