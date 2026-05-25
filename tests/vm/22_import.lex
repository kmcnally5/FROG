// Tier 22 — import statement.
//
// The VM delegates module loading to the tree-walker's import
// machinery (full search path + cache + cycle detection). Functions
// imported from a module are *eval.Function values; OpCall delegates
// to eval.CallCallable for those so cross-interpreter calls work.

import "stdlib/math.lex" as math

// Module functions — math.degrees etc. dispatched via tree-walker.
println(math.factorial(5))
println(math.gcd(12, 18))
println(math.lcm(4, 6))
println(math.sum([1, 2, 3, 4]))
println(math.even(4))
println(math.odd(7))

import "stdlib/strings.lex" as s
println(s.repeat("ab", 3))
println(s.padLeft("42", 5, "0"))
println(s.count("banana", "a"))
println(s.trimLeft("   left"))
println(s.trimRight("right   "))
