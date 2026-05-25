// Tier 3 — every arithmetic operator through the VM.
//
// Exercises InfixExpr (+ - * / %), integer/float promotion, string
// concatenation, and the eval.EvalBinaryOp shared dispatch.

// Integer arithmetic
println(2 + 3)
println(10 - 4)
println(6 * 7)
println(20 / 4)
println(17 % 5)
println(1 + 2 * 3 - 4)

// Float arithmetic
println(1.5 + 2.5)
println(10.0 / 4.0)

// Mixed int / float promotes
println(1 + 2.5)
println(7 / 2.0)

// String concatenation via +
println("hello" + " " + "world")

// Negative literals via unary minus on the operand
println(0 - 5)
